package api

import (
	"encoding/json"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/memory"
)

// Memory write-back: recall_memory can only find what was written, and until
// now nothing in production wrote — the memory loop stood open. After each
// completed model turn we retain one bounded experience, tenant-scoped with
// the tenant as its own readership, mirroring exactly the scoping the recall
// tool enforces (SemanticRecallScoped with principals [tenant]). A memory the
// recall path cannot admit is never written.
//
// Deliberately excluded: rewrite (a user edit, not a model turn) and refusal
// shells with no model output. Regenerate counts — each fresh answer the user
// saw is a real turn.

// maxExperienceRunes bounds a single retained experience. Memories are recall
// context, not transcripts; the prompt names the cause, the outcome names the
// result, and neither stores whole messages.
const maxExperienceRunes = 600

func boundRunes(s string, n int) string {
	t := strings.TrimSpace(s)
	if len([]rune(t)) > n {
		t = string([]rune(t)[:n]) + "…"
	}
	return t
}

// latestEventID returns the session's newest ledger event id, or 0 when the
// session has no events yet. Used to attribute tool calls to the turn that
// just ran instead of earlier turns.
func (s *Server) latestEventID(sessionID string) int64 {
	if s.ledger == nil {
		return 0
	}
	events, err := s.ledger.GetRecentEvents(sessionID, 1)
	if err != nil || len(events) == 0 {
		return 0
	}
	return events[0].ID
}

// toolNamesSince names the tools this turn actually dispatched, in order,
// deduped, bounded. Only tool_called events newer than beforeID count —
// attributing an earlier turn's tools to this one would write a false
// experience. Unknown on ledger error (empty, never guessed).
func (s *Server) toolNamesSince(sessionID string, beforeID int64) []string {
	if s.ledger == nil {
		return nil
	}
	events, err := s.ledger.GetRecentEvents(sessionID, 100)
	if err != nil {
		return nil
	}
	var names []string
	seen := map[string]bool{}
	// GetRecentEvents returns newest first; walk backwards for order.
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.ID <= beforeID || e.EventType != "tool_called" {
			continue
		}
		var p struct {
			Tool string `json:"tool"`
		}
		if err := json.Unmarshal([]byte(e.Payload), &p); err != nil || p.Tool == "" {
			continue
		}
		if !seen[p.Tool] {
			seen[p.Tool] = true
			names = append(names, p.Tool)
		}
		if len(names) >= 8 {
			break
		}
	}
	return names
}

// chatOutcome classifies a turn's result from what the call site actually
// observed — never inferred. proposalJSON presence, generation error, and
// empty reply are the only inputs.
func chatOutcome(reply string, hasProposal bool, genErr error) string {
	if genErr != nil && strings.TrimSpace(reply) == "" {
		return "engine-error:" + boundRunes(genErr.Error(), 80)
	}
	if hasProposal {
		return "answered-with-proposal"
	}
	if genErr != nil {
		return "answered-despite-engine-error"
	}
	return "answered"
}

// chatImportance rates retention: proposal-bearing and tool-using turns carry
// more future value than plain answers. Values stay inside (0,1]; the store
// treats them as relative rank, not absolute truth.
func chatImportance(hasProposal, hadError, usedTools bool) float64 {
	switch {
	case hasProposal && usedTools:
		return 0.8
	case hasProposal || usedTools:
		return 0.65
	case hadError:
		return 0.4
	default:
		return 0.5
	}
}

// retainTurnExperience stores one bounded experience for a completed model
// turn. Importance is caller-rated: tool-using and proposal-bearing turns
// outrank plain answers. Never fails the turn on memory error — retention is
// best-effort, the reply already went out.
func (s *Server) retainTurnExperience(sessionID, kind, prompt, outcome string, tools []string, importance float64) {
	if s.memoryStore == nil {
		return
	}
	prompt = boundRunes(prompt, 400)
	outcome = boundRunes(outcome, 120)
	if prompt == "" && outcome == "" {
		return
	}
	var b strings.Builder
	b.WriteString(kind)
	b.WriteString(" turn | user: ")
	b.WriteString(prompt)
	b.WriteString(" | outcome: ")
	b.WriteString(outcome)
	if len(tools) > 0 {
		b.WriteString(" | tools: ")
		b.WriteString(strings.Join(tools, ","))
	}
	content := boundRunes(b.String(), maxExperienceRunes)

	tenant := "default"
	if s.sessionMgr != nil {
		if sess, err := s.sessionMgr.GetSession(sessionID); err == nil && sess.TenantID != "" {
			tenant = sess.TenantID
		}
	}
	s.memoryStore.RetainScoped(tenant, tenant, memory.BankExperiences, content, importance, nil)
}
