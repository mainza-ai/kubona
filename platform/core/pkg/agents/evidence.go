package agents

// R4 — run-scoped evidence: full bytes live server-side (in the
// collective's run, never stuffed into prompts); turns see bounded views
// with parent links; oversized results resolve through a ready-to-call
// continuation instead of lossy truncation; the pinned turn budget
// refuses pre-construction with resource_constraint.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/mainza-ai/kubona/core/pkg/tools"
)

// EvidenceItem is one retained blob: a full turn, a full (untruncated)
// observation, or an imported bundle ancestor. Content is server-side;
// prompts carry views + refs, never the bytes.
type EvidenceItem struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`
	Kind     string `json:"kind"` // "turn" | "observation" | "ancestor"
	Label    string `json:"label"`
	Content  string `json:"-"` // full bytes: served paged, never embedded
	Runes    int    `json:"runes"`
}

// EvidenceStore is append-only per run: pages are stable across growth
// (nothing is ever edited or removed, so offset/limit reads never shift).
type EvidenceStore struct {
	mu    sync.Mutex
	runID string
	items []EvidenceItem
	byID  map[string]int
}

// NewEvidenceStore opens a run-scoped store. runID "" means ad-hoc use
// (tests); tribunal runs pass their obligation subject for linkage.
func NewEvidenceStore(runID string) *EvidenceStore {
	return &EvidenceStore{runID: runID, byID: map[string]int{}}
}

// Put retains content and returns its stable id (evd-<run>-<seq>).
func (s *EvidenceStore) Put(kind, label, content, parentID string) EvidenceItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := fmt.Sprintf("evd-%s-%d", s.runID, len(s.items))
	it := EvidenceItem{ID: id, ParentID: parentID, Kind: kind, Label: label,
		Content: content, Runes: len([]rune(content))}
	s.byID[id] = len(s.items)
	s.items = append(s.items, it)
	return it
}

// Get returns a retained item (metadata + content for server-side use).
func (s *EvidenceStore) Get(id string) (EvidenceItem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, ok := s.byID[id]
	if !ok {
		return EvidenceItem{}, false
	}
	return s.items[i], true
}

// Page reads a stable offset/limit window over an item's runes.
// Returns the page, the next offset (-1 at end), and the total.
// Negative offsets refuse; over-long limits clamp to the remainder.
func (s *EvidenceStore) Page(id string, offset, limit int) (string, int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, ok := s.byID[id]
	if !ok {
		return "", 0, 0, fmt.Errorf("unknown evidence %q: refused", id)
	}
	if offset < 0 {
		return "", 0, 0, fmt.Errorf("negative offset refused")
	}
	if limit <= 0 {
		return "", 0, 0, fmt.Errorf("non-positive limit refused")
	}
	runes := []rune(s.items[i].Content)
	total := len(runes)
	if offset >= total {
		return "", -1, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	next := end
	if next >= total {
		next = -1
	}
	return string(runes[offset:end]), next, total, nil
}

// Count reports retained items (tests + gauges).
func (s *EvidenceStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

// Summarize renders the bounded turn view: first maxRunes runes plus the
// ref that resolves the rest. Summaries are what prompts embed.
func Summarize(it EvidenceItem, maxRunes int) string {
	r := []rune(it.Content)
	head := string(r)
	more := ""
	if len(r) > maxRunes {
		head = string(r[:maxRunes])
		more = fmt.Sprintf("…[%d more runes via evidence_continue %s]", len(r)-maxRunes, it.ID)
	}
	parent := ""
	if it.ParentID != "" {
		parent = " parent=" + it.ParentID
	}
	return fmt.Sprintf("[%s %s%s ref=%s] %s%s", it.Kind, it.Label, parent, it.ID, head, more)
}

// ResourceConstraintError is the pinned-budget refusal (R4): the
// projected turn context exceeds TurnContextRunes. Code() reports the
// machine-readable code R5's typed errors will adopt.
type ResourceConstraintError struct {
	Have   int
	Budget int
	What   string
}

func (e *ResourceConstraintError) Error() string {
	return fmt.Sprintf("resource_constraint: %s projects %d runes over the %d-rune turn budget (shrink inputs or page evidence)", e.What, e.Have, e.Budget)
}

// Code returns the stable machine-readable code.
func (e *ResourceConstraintError) Code() string { return "resource_constraint" }

// ProjectTurnContext builds the bounded user message for a turn and
// refuses pre-construction when it exceeds budget. parts are the
// task line, bounded prior-turn summaries, and memory — all already
// projected; this function only measures + enforces (one gate, never
// two copies of the budget). Empty parts are skipped (resumed runs pass
// an ancestor note; fresh runs pass none).
func ProjectTurnContext(what string, parts []string, budgetRunes int) (string, error) {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, p)
		}
	}
	joined := strings.Join(kept, "\n\n")
	if n := len([]rune(joined)); n > budgetRunes {
		return "", &ResourceConstraintError{Have: n, Budget: budgetRunes, What: what}
	}
	return joined, nil
}

// ctxKeyEvidence carries the run store to tools dispatched mid-turn.
type ctxKeyEvidence struct{}

// ContextWithEvidenceStore attaches the run store for evidence_continue.
func ContextWithEvidenceStore(ctx context.Context, s *EvidenceStore) context.Context {
	return context.WithValue(ctx, ctxKeyEvidence{}, s)
}

// EvidenceStoreFromContext retrieves the run store (nil when absent —
// the continuation tool refuses outside a wired run).
func EvidenceStoreFromContext(ctx context.Context) *EvidenceStore {
	s, _ := ctx.Value(ctxKeyEvidence{}).(*EvidenceStore)
	return s
}

// EvidenceContinueDefinition is the ready-to-call continuation (R4):
// oversized observations resolve through this read-only tool, paged, so
// prompts never carry lossy truncation. The run store arrives via context
// (ContextWithEvidenceStore); outside a wired run the tool refuses
// instead of hallucinating evidence. Production (api/agent_tools.go) and
// tests share THIS definition — never a second copy.
func EvidenceContinueDefinition() tools.Definition {
	return tools.Definition{
		Name:        "evidence_continue",
		Description: "Fetch a page of retained tribunal evidence by id (offset/limit over runes). Evidence too large for a turn resolves here, never truncated.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"evidence_id": map[string]any{"type": "string"},
				"offset":      map[string]any{"type": "integer"},
				"limit":       map[string]any{"type": "integer"},
			},
			"required":             []any{"evidence_id"},
			"additionalProperties": false,
		},
		Capability: "evidence:read",
		ReadOnly:   true,
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			store := EvidenceStoreFromContext(ctx)
			if store == nil {
				return nil, fmt.Errorf("no evidence store in context: continuations resolve inside a wired tribunal run only")
			}
			var a struct {
				EvidenceID string `json:"evidence_id"`
				Offset     *int   `json:"offset"`
				Limit      *int   `json:"limit"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, fmt.Errorf("bad arguments: %w", err)
			}
			offset, limit := 0, 2000
			if a.Offset != nil {
				offset = *a.Offset
			}
			if a.Limit != nil {
				limit = *a.Limit
			}
			content, next, total, err := store.Page(a.EvidenceID, offset, limit)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"evidence_id": a.EvidenceID, "content": content,
				"next_offset": next, "total_runes": total,
			}, nil
		},
	}
}

// TribunalBundle is the portable turn bundle (P1-6 remainder): a run's
// turns with coverage, verdict, and synthesis — exportable as JSON,
// resumable via ExecuteTribunalResume. No endpoint yet (surface work);
// the flow is real and tested at the collective level.
type TribunalBundle struct {
	Version        int            `json:"version"`
	ObligationID   string         `json:"obligation_id"`
	Target         string         `json:"target"`
	Molecule       string         `json:"molecule"`
	Turns          []TribunalTurn `json:"turns"`
	Synthesis      string         `json:"synthesis"`
	Consensus      bool           `json:"consensus_passed"`
	ParentEvidence []string       `json:"parent_evidence,omitempty"`
}

// MarshalTribunalBundle exports a completed run as a portable bundle.
func MarshalTribunalBundle(res *TribunalResult) ([]byte, error) {
	if res == nil {
		return nil, fmt.Errorf("no result to bundle")
	}
	if len(res.Turns) == 0 {
		return nil, fmt.Errorf("turnless result bundles nothing (empty input never closes, never bundles)")
	}
	return json.Marshal(TribunalBundle{
		Version: 1, ObligationID: res.ObligationID, Target: res.Target,
		Molecule: res.Molecule, Turns: res.Turns,
		Synthesis: res.Synthesis, Consensus: res.ConsensusPassed,
	})
}

// ParseTribunalBundle validates an imported bundle (version-gated,
// non-empty — a corrupt bundle resumes nothing).
func ParseTribunalBundle(raw []byte) (*TribunalBundle, error) {
	var b TribunalBundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("bundle does not parse: %w", err)
	}
	if b.Version != 1 {
		return nil, fmt.Errorf("unsupported bundle version %d: this code reads v1 only", b.Version)
	}
	if len(b.Turns) == 0 {
		return nil, fmt.Errorf("bundle carries no turns: refusing to resume from emptiness")
	}
	return &b, nil
}
