package api

// B1 — server-owned proposal rail.
//
// A proposal is minted by the server from state it already owns: the stored
// campaign (campaign source) or a stored assistant message's proposal block
// (message source). The client receives {id, digest, body} and later launches
// by naming that proposal — it never supplies the circuit, backend, or budget
// at launch. Minting records the campaign revision and the calibration
// identity the plan was priced against; if either moved, launch refuses with
// proposal_stale instead of dispatching against a stale plan.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/fidelity"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// proposalBody is the canonical, server-built launch spec. Field order is
// stable and it is serialized once: the stored bytes ARE the digest input.
type proposalBody struct {
	Source          string  `json:"source"`
	Target          string  `json:"target"`
	QASM            string  `json:"qasm"`
	Backend         string  `json:"backend"`
	Shots           int     `json:"shots"`
	Seed            int     `json:"seed"`
	MaxCostUSD      float64 `json:"max_cost_usd"`
	ClassicalAnchor string  `json:"classical_anchor,omitempty"`
	SpendEstimate   string  `json:"spend_estimate,omitempty"`
	DesignedBy      string  `json:"designed_by,omitempty"`
}

// proposalMintError marks a plan that cannot be priced or routed honestly.
// The HTTP layer maps it to proposal_unmintable (400) with the reason.
type proposalMintError struct{ reason string }

func (e *proposalMintError) Error() string { return e.reason }

func unmintable(format string, args ...any) error {
	return &proposalMintError{reason: fmt.Sprintf(format, args...)}
}

func proposalDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// proposalView is the wire projection. Body is the exact stored JSON.
type proposalView struct {
	ID                     string          `json:"id"`
	SessionID              string          `json:"session_id"`
	MessageID              string          `json:"message_id,omitempty"`
	Digest                 string          `json:"digest"`
	Body                   json.RawMessage `json:"body"`
	BasisCampaignRev       int64           `json:"basis_campaign_rev"`
	BasisCalibrationDigest string          `json:"basis_calibration_digest,omitempty"`
	BasisTainted           bool            `json:"basis_tainted,omitempty"`
	Status                 string          `json:"status"`
	LaunchedJobID          string          `json:"launched_job_id,omitempty"`
	CreatedAt              time.Time       `json:"created_at"`
}

func proposalViewFrom(p *ledger.Proposal) *proposalView {
	if p == nil {
		return nil
	}
	return &proposalView{
		ID: p.ID, SessionID: p.SessionID, MessageID: p.MessageID,
		Digest: p.Digest, Body: json.RawMessage(p.BodyJSON),
		BasisCampaignRev:       p.BasisCampaignRev,
		BasisCalibrationDigest: p.BasisCalibrationDigest,
		Status:                 p.Status, LaunchedJobID: p.LaunchedJobID,
		CreatedAt: p.CreatedAt,
	}
}

// calibrationBasis identifies the calibration a plan is priced against. It is
// the device profile's provenance identity (name|source|version|calibrated_at)
// resolved from the server's live fidelity gate; simulators have no
// calibration dependence and carry an empty basis.
func (s *Server) calibrationBasis(backend string) (string, error) {
	if backend == "local_simulator" {
		return "", nil
	}
	device, err := fidelity.ResolveDeviceForBackend(backend)
	if err != nil {
		return "", err
	}
	profile, ok := s.fidelityGate.Profile(device)
	if !ok {
		return "", fmt.Errorf("no calibration profile for %q: cannot bind a priced plan to a device", backend)
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		profile.Name, profile.Source, profile.Version, profile.CalibratedAt,
	}, "|")))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// mintProposalBody validates and canonicalizes a plan, then persists it with
// its basis. It is the only writer of proposal rows.
func (s *Server) mintProposalBody(sessionID, messageID string, body proposalBody, basisTainted bool) (*ledger.Proposal, error) {
	if strings.TrimSpace(body.QASM) == "" {
		return nil, unmintable("the plan has no circuit: nothing to run")
	}
	backend, err := jobs.NormalizeBackend(body.Backend)
	if err != nil {
		return nil, unmintable("%v", err)
	}
	body.Backend = backend
	if body.Shots <= 0 {
		body.Shots = 1024
	}
	if body.Seed <= 0 {
		body.Seed = 7
	}
	if backend != "local_simulator" && body.MaxCostUSD <= 0 {
		return nil, unmintable("metered backend %q has no numeric max_cost_usd: text estimates are not a spend bound, so it cannot be proposed", backend)
	}
	if backend == "local_simulator" {
		// Local execution is zero-cost by rail; a model's "$0.00 (Local
		// Simulator)" string is display copy, not a ceiling.
		body.MaxCostUSD = 0
	}

	var rev int64
	if _, currentRev, rerr := s.ledger.GetCampaignStateWithRevision(sessionID); rerr == nil {
		rev = currentRev
	} else if !errors.Is(rerr, sql.ErrNoRows) {
		return nil, fmt.Errorf("campaign basis read failed: %w", rerr)
	}
	calBasis, err := s.calibrationBasis(backend)
	if err != nil {
		return nil, unmintable("%v", err)
	}
	canonical, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("canonicalize proposal: %w", err)
	}
	digest := proposalDigest(canonical)
	// Content-idempotent mint: the same plan in the same basis is one row.
	if existing, ferr := s.ledger.FindOfferedProposal(sessionID, digest); ferr == nil && existing != nil {
		return existing, nil
	}
	return s.ledger.MintProposal(sessionID, messageID, digest, canonical, rev, calBasis, basisTainted)
}

// proposalBodyFromCampaign builds the plan from stored campaign state.
func (s *Server) proposalBodyFromCampaign(sessionID string) (proposalBody, error) {
	raw, _, err := s.ledger.GetCampaignStateWithRevision(sessionID)
	if errors.Is(err, sql.ErrNoRows) || len(raw) == 0 {
		return proposalBody{}, unmintable("this campaign has no stored plan yet: ask for a circuit first")
	}
	if err != nil {
		return proposalBody{}, fmt.Errorf("campaign read failed: %w", err)
	}
	var state struct {
		TargetSystem string `json:"target_system"`
		DesignedBy   string `json:"designed_by"`
		Quantum      struct {
			TargetMolecule     string  `json:"target_molecule"`
			QASM               string  `json:"qasm"`
			RecommendedBackend string  `json:"recommended_backend"`
			EstimatedSpendUSD  float64 `json:"estimated_spend_usd"`
			Shots              int     `json:"shots"`
			Seed               int     `json:"seed"`
		} `json:"quantum_factors"`
		Anchor struct {
			Method string `json:"method"`
		} `json:"classical_anchor"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return proposalBody{}, unmintable("the stored campaign is not readable: %v", err)
	}
	target := state.TargetSystem
	if target == "" {
		target = state.Quantum.TargetMolecule
	}
	return proposalBody{
		Source: "campaign", Target: target,
		QASM: state.Quantum.QASM, Backend: state.Quantum.RecommendedBackend,
		Shots: state.Quantum.Shots, Seed: state.Quantum.Seed,
		MaxCostUSD:      state.Quantum.EstimatedSpendUSD,
		ClassicalAnchor: state.Anchor.Method,
		DesignedBy:      state.DesignedBy,
	}, nil
}

// proposalBodyFromMessage builds the plan from the assistant message's stored
// proposal block. Raw model JSON is never launched directly; it is normalized
// once into a server-owned body.
func (s *Server) proposalBodyFromMessage(sessionID, messageID string) (proposalBody, error) {
	msgs, err := s.sessionMgr.GetMessages(sessionID)
	if err != nil {
		return proposalBody{}, fmt.Errorf("messages read failed: %w", err)
	}
	for _, m := range msgs {
		if m.ID != messageID {
			continue
		}
		if m.Role != "assistant" || m.ProposalJSON == nil {
			return proposalBody{}, unmintable("this message carries no proposal")
		}
		var raw struct {
			Target          string   `json:"target"`
			QASM            string   `json:"qasm"`
			RecommendedTier string   `json:"recommended_tier"`
			SpendEstimate   string   `json:"spend_estimate"`
			ClassicalAnchor string   `json:"classical_anchor"`
			DesignedBy      string   `json:"designed_by"`
			MaxCostUSD      *float64 `json:"max_cost_usd"`
			Shots           int      `json:"shots"`
			Seed            int      `json:"seed"`
		}
		if err := json.Unmarshal(*m.ProposalJSON, &raw); err != nil {
			return proposalBody{}, unmintable("the stored proposal is not readable: %v", err)
		}
		body := proposalBody{
			Source: "message", Target: raw.Target, QASM: raw.QASM,
			Backend: raw.RecommendedTier, Shots: raw.Shots, Seed: raw.Seed,
			ClassicalAnchor: raw.ClassicalAnchor, SpendEstimate: raw.SpendEstimate,
			DesignedBy: raw.DesignedBy,
		}
		if raw.MaxCostUSD != nil {
			body.MaxCostUSD = *raw.MaxCostUSD
		}
		return body, nil
	}
	return proposalBody{}, unmintable("message not found in this session")
}

// handleProposals mints a proposal from server-owned state.
func (s *Server) handleProposals(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
		if sessionID == "" {
			WriteAPIError(w, "bad_request", "session_id is required", nil)
			return
		}
		if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
			return
		}
		limit := 50
		if raw := r.URL.Query().Get("limit"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}
		rows, err := s.ledger.ListProposals(sessionID, limit)
		if err != nil {
			WriteAPIError(w, "internal", "proposal history read failed", nil)
			return
		}
		views := make([]*proposalView, 0, len(rows))
		for i := range rows {
			views = append(views, proposalViewFrom(&rows[i]))
		}
		writeJSON(w, http.StatusOK, map[string]any{"proposals": views, "count": len(views)})
		return
	}
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "proposals are GET (history) or POST (mint)", nil)
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
		Source    string `json:"source"`
		MessageID string `json:"message_id"`
		// Downgrade is a monotone, spend-reducing choice: "simulator" mints a
		// zero-cost local plan from the stored campaign. Nothing else is
		// accepted — a client can never upgrade a plan through this field.
		Downgrade string `json:"downgrade"`
	}
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid request body: %v", err), nil)
		return
	}
	sid, err := s.requireSession(req.SessionID)
	if err != nil {
		WriteAPIError(w, "not_found", err.Error(), nil)
		return
	}
	if _, ok := s.authorizeKnownSession(w, r, sid); !ok {
		return
	}
	if req.Downgrade != "" && req.Downgrade != "simulator" {
		WriteAPIError(w, "bad_request", `downgrade must be "simulator" or omitted`, nil)
		return
	}
	// A message's plan is minted once; later calls return the same proposal
	// (offered or already decided) instead of growing the table.
	if req.Source == "message" && strings.TrimSpace(req.MessageID) != "" {
		if existing, ferr := s.ledger.GetProposalForMessage(req.MessageID); ferr == nil && existing != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "proposal": proposalViewFrom(existing)})
			return
		}
	}
	var body proposalBody
	switch req.Source {
	case "campaign":
		body, err = s.proposalBodyFromCampaign(sid)
		if req.Downgrade == "simulator" {
			body.Backend = "local_simulator"
			body.MaxCostUSD = 0
			body.SpendEstimate = "$0.00 (Local Simulator)"
		}
	case "message":
		if strings.TrimSpace(req.MessageID) == "" {
			WriteAPIError(w, "bad_request", "message proposals need message_id", nil)
			return
		}
		body, err = s.proposalBodyFromMessage(sid, req.MessageID)
	default:
		WriteAPIError(w, "bad_request", `source must be "campaign" or "message"`, nil)
		return
	}
	if err != nil {
		s.writeProposalMintError(w, err)
		return
	}
	p, err := s.mintProposalBody(sid, req.MessageID, body, false)
	if err != nil {
		s.writeProposalMintError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "proposal": proposalViewFrom(p)})
}

func (s *Server) writeProposalMintError(w http.ResponseWriter, err error) {
	var mint *proposalMintError
	if errors.As(err, &mint) {
		WriteAPIError(w, "proposal_unmintable", mint.reason, nil)
		return
	}
	WriteAPIError(w, "internal", "proposal could not be stored", nil)
}

// handleProposalByID serves GET /api/proposals/{id} and
// POST /api/proposals/{id}/dismiss.
func (s *Server) handleProposalByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/proposals/")
	id, action, _ := strings.Cut(rest, "/")
	if id == "" || strings.Contains(action, "/") {
		WriteAPIError(w, "bad_request", "proposal path is /api/proposals/{id}[/dismiss]", nil)
		return
	}
	p, err := s.ledger.GetProposal(id)
	if err != nil {
		WriteAPIError(w, "internal", "proposal read failed", nil)
		return
	}
	if p == nil {
		WriteAPIError(w, "not_found", "unknown proposal", nil)
		return
	}
	if _, ok := s.authorizeKnownSession(w, r, p.SessionID); !ok {
		return
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "proposal": proposalViewFrom(p)})
	case action == "dismiss" && r.Method == http.MethodPost:
		var req struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		ok, err := s.ledger.CASProposalStatus(id, ledger.ProposalOffered, ledger.ProposalDismissed, "")
		if err != nil {
			WriteAPIError(w, "internal", "dismiss failed", nil)
			return
		}
		if !ok {
			WriteAPIError(w, "proposal_not_offered", "only an offered proposal can be dismissed", map[string]any{"status": p.Status})
			return
		}
		_ = s.ledger.AppendEvent(p.SessionID, "proposal_dismissed", map[string]any{
			"proposal_id": id, "reason": strings.TrimSpace(req.Reason), "actor": s.callerActor(r),
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "status": ledger.ProposalDismissed})
	case action == "":
		WriteAPIError(w, "method_not_allowed", "proposal reads are GET; dismissal is POST /dismiss", nil)
	default:
		WriteAPIError(w, "not_found", "unknown proposal action", nil)
	}
}

// loadProposalForLaunch validates identity, digest, and body integrity and
// parses the stored body. It does NOT gate on status or basis: an idempotent
// replay of an already-admitted launch must still return its receipt.
func (s *Server) loadProposalForLaunch(w http.ResponseWriter, sessionID, id, digest string) (*ledger.Proposal, *proposalBody, bool) {
	p, err := s.ledger.GetProposal(id)
	if err != nil {
		WriteAPIError(w, "internal", "proposal read failed", nil)
		return nil, nil, false
	}
	if p == nil || p.SessionID != sessionID {
		// No oracle: an unknown id and another session's id refuse alike.
		WriteAPIError(w, "not_found", "unknown proposal for this session", nil)
		return nil, nil, false
	}
	if digest == "" || digest != p.Digest {
		WriteAPIError(w, "proposal_digest_mismatch", "the digest does not match the server-minted proposal", map[string]any{"proposal_id": p.ID})
		return nil, nil, false
	}
	if proposalDigest([]byte(p.BodyJSON)) != p.Digest {
		WriteAPIError(w, "internal", "stored proposal body no longer matches its digest", nil)
		return nil, nil, false
	}
	var body proposalBody
	if err := json.Unmarshal([]byte(p.BodyJSON), &body); err != nil {
		WriteAPIError(w, "internal", "stored proposal body is unreadable", nil)
		return nil, nil, false
	}
	return p, &body, true
}

// checkProposalLaunchable gates single-use state and basis. It writes the
// typed refusal and reports false when the plan cannot be launched now.
func (s *Server) checkProposalLaunchable(w http.ResponseWriter, p *ledger.Proposal, body *proposalBody) bool {
	if p.Status != ledger.ProposalOffered {
		WriteAPIError(w, "proposal_not_offered", "this proposal is no longer offered", map[string]any{"status": p.Status})
		return false
	}
	// Basis: the plan is only valid on the campaign + calibration it was
	// priced against. A moved basis refuses; it never silently re-prices.
	_, currentRev, revErr := s.ledger.GetCampaignStateWithRevision(p.SessionID)
	if errors.Is(revErr, sql.ErrNoRows) {
		currentRev = 0
	} else if revErr != nil {
		WriteAPIError(w, "internal", "campaign basis read failed", nil)
		return false
	}
	if currentRev != p.BasisCampaignRev {
		WriteAPIError(w, "proposal_stale", "the campaign changed since this plan was priced", map[string]any{
			"basis_campaign_rev": p.BasisCampaignRev, "current_campaign_rev": currentRev,
		})
		return false
	}
	currentCal, calErr := s.calibrationBasis(body.Backend)
	if calErr != nil {
		WriteAPIError(w, "proposal_stale", "the calibration this plan was priced against is gone", map[string]any{
			"reason": calErr.Error(),
		})
		return false
	}
	if currentCal != p.BasisCalibrationDigest {
		WriteAPIError(w, "proposal_stale", "the device calibration changed since this plan was priced", map[string]any{
			"basis_calibration_digest": p.BasisCalibrationDigest, "current_calibration_digest": currentCal,
		})
		return false
	}
	return true
}

// mintProposalForMessage mints the launchable proposal for a stored assistant
// message and links it, so history reloads can launch by id. A plan that
// cannot be priced honestly yields no id and a reason the UI can show — the
// reply itself is never failed for it.
func (s *Server) mintProposalForMessage(sessionID, messageID string, basisTainted bool) (id, digest, reason string) {
	body, err := s.proposalBodyFromMessage(sessionID, messageID)
	if err != nil {
		return "", "", err.Error()
	}
	p, err := s.mintProposalBody(sessionID, messageID, body, basisTainted)
	if err != nil {
		return "", "", err.Error()
	}
	if err := s.ledger.SetMessageProposal(messageID, p.ID, p.Digest); err != nil {
		return "", "", "the plan was priced but could not be linked to this message"
	}
	return p.ID, p.Digest, ""
}
