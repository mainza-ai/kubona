package ledger

// B1 — server-owned proposals. A proposal is minted by the server (from a
// stored campaign or a stored assistant message), bound to the campaign
// revision and calibration digest it was priced against, and launched by
// {proposal_id, digest, request_id}. The client never supplies the circuit,
// backend, or budget at launch; it only names which minted proposal to run.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Proposal status is a closed vocabulary. "offered" is the only launchable
// state; a launched proposal cannot be launched again, and a dismissed one
// stays visible in history without being runnable.
const (
	ProposalOffered   = "offered"
	ProposalLaunched  = "launched"
	ProposalFailed    = "failed"
	ProposalDismissed = "dismissed"
)

var proposalStatuses = map[string]bool{
	ProposalOffered:   true,
	ProposalLaunched:  true,
	ProposalFailed:    true,
	ProposalDismissed: true,
}

type Proposal struct {
	ID                     string `json:"id"`
	SessionID              string `json:"session_id"`
	MessageID              string `json:"message_id,omitempty"`
	Digest                 string `json:"digest"`
	BodyJSON               string `json:"body_json"`
	BasisCampaignRev       int64  `json:"basis_campaign_rev"`
	BasisCalibrationDigest string `json:"basis_calibration_digest,omitempty"`
	// BasisTainted marks a plan extracted from a turn that consumed untrusted
	// observations: a run allowance may never cover it (C11 taint exclusion).
	BasisTainted  bool      `json:"basis_tainted,omitempty"`
	Status        string    `json:"status"`
	LaunchedJobID string    `json:"launched_job_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func newProposalID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "pr_" + hex.EncodeToString(b[:]), nil
}

// MintProposal inserts a new offered proposal. The body is stored verbatim
// and must be valid JSON; the digest is the caller's hash of exactly those
// bytes (the API layer computes it from the canonical body it built).
func (l *Ledger) MintProposal(sessionID, messageID, digest string, body []byte, basisCampaignRev int64, basisCalibrationDigest string, basisTainted bool) (*Proposal, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("proposal needs an owning session")
	}
	if digest == "" {
		return nil, fmt.Errorf("proposal needs a body digest")
	}
	if len(body) == 0 || !json.Valid(body) {
		return nil, fmt.Errorf("proposal body must be valid JSON")
	}
	id, err := newProposalID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	tainted := 0
	if basisTainted {
		tainted = 1
	}
	_, err = l.DB().Exec(`INSERT INTO proposals
		(id, session_id, message_id, digest, body_json, basis_campaign_rev, basis_calibration_digest, basis_tainted, status, launched_job_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
		id, sessionID, messageID, digest, string(body), basisCampaignRev, basisCalibrationDigest, tainted, ProposalOffered, now, now)
	if err != nil {
		return nil, fmt.Errorf("mint proposal: %w", err)
	}
	return &Proposal{
		ID: id, SessionID: sessionID, MessageID: messageID, Digest: digest,
		BodyJSON: string(body), BasisCampaignRev: basisCampaignRev,
		BasisCalibrationDigest: basisCalibrationDigest,
		BasisTainted:           basisTainted,
		Status:                 ProposalOffered, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// GetProposal loads one proposal by id (nil, nil when absent).
func (l *Ledger) GetProposal(id string) (*Proposal, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	return l.getProposalLocked(id)
}

func (l *Ledger) getProposalLocked(id string) (*Proposal, error) {
	var p Proposal
	var created, updated string
	var basisCal string
	var tainted int
	err := l.DB().QueryRow(`SELECT id, session_id, message_id, digest, body_json,
		basis_campaign_rev, basis_calibration_digest, basis_tainted, status, launched_job_id, created_at, updated_at
		FROM proposals WHERE id = ?`, id).
		Scan(&p.ID, &p.SessionID, &p.MessageID, &p.Digest, &p.BodyJSON,
			&p.BasisCampaignRev, &basisCal, &tainted, &p.Status, &p.LaunchedJobID, &created, &updated)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	p.BasisCalibrationDigest = basisCal
	p.BasisTainted = tainted == 1
	p.CreatedAt = parseLedgerTime(created)
	p.UpdatedAt = parseLedgerTime(updated)
	return &p, nil
}

// CASProposalStatus moves a proposal between states only if it is still in
// the expected state. It is the launch gate: two launches racing on one
// proposal produce exactly one winner.
func (l *Ledger) CASProposalStatus(id, from, to, launchedJobID string) (bool, error) {
	if !proposalStatuses[from] || !proposalStatuses[to] || from == to {
		return false, fmt.Errorf("invalid proposal transition %q → %q", from, to)
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	res, err := l.DB().Exec(`UPDATE proposals SET status = ?, launched_job_id = ?, updated_at = ?
		WHERE id = ? AND status = ?`,
		to, launchedJobID, time.Now().UTC(), id, from)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// SetMessageProposal links a stored assistant message to the proposal minted
// from it, so history reloads can still launch by id.
func (l *Ledger) SetMessageProposal(messageID, proposalID, digest string) error {
	_, err := l.DB().Exec(`UPDATE chat_messages SET proposal_id = ?, proposal_digest = ? WHERE id = ?`,
		proposalID, digest, messageID)
	return err
}

// GetProposalForMessage returns the proposal minted from a message, if any.
func (l *Ledger) GetProposalForMessage(messageID string) (*Proposal, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	var id string
	if err := l.DB().QueryRow(`SELECT proposal_id FROM chat_messages WHERE id = ?`, messageID).Scan(&id); err != nil {
		if errorsIsNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	if id == "" {
		return nil, nil
	}
	return l.getProposalLocked(id)
}

// AttachProposalJob records the admitted job on an already-launched proposal.
func (l *Ledger) AttachProposalJob(id, jobID string) error {
	_, err := l.DB().Exec(`UPDATE proposals SET launched_job_id = ?, updated_at = ?
		WHERE id = ? AND status = ?`, jobID, time.Now().UTC(), id, ProposalLaunched)
	return err
}

// ProposalDismissal is one recorded operator reason for rejecting a plan.
type ProposalDismissal struct {
	ProposalID string
	Reason     string
	CreatedAt  string
}

// RecentProposalDismissals returns newest-first dismissal reasons for a
// session. The copilot prompt consumes these as a ranking signal; they are
// not a verdict on any future plan.
func (l *Ledger) RecentProposalDismissals(sessionID string, limit int) ([]ProposalDismissal, error) {
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	events, err := l.GetRecentEvents(sessionID, 200)
	if err != nil {
		return nil, err
	}
	out := make([]ProposalDismissal, 0, limit)
	for _, event := range events {
		if event.EventType != "proposal_dismissed" {
			continue
		}
		var payload struct {
			ProposalID string `json:"proposal_id"`
			Reason     string `json:"reason"`
		}
		_ = json.Unmarshal(event.Payload, &payload)
		out = append(out, ProposalDismissal{ProposalID: payload.ProposalID, Reason: payload.Reason, CreatedAt: event.Timestamp})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ListProposals returns a session's proposals newest first. The body is kept
// so the history surface can show the target/spend without a second read.
func (l *Ledger) ListProposals(sessionID string, limit int) ([]Proposal, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	l.lock.RLock()
	rows, err := l.db.Query(`SELECT id FROM proposals WHERE session_id = ? ORDER BY created_at DESC, rowid DESC LIMIT ?`, sessionID, limit)
	if err != nil {
		l.lock.RUnlock()
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			l.lock.RUnlock()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	l.lock.RUnlock()
	out := make([]Proposal, 0, len(ids))
	for _, id := range ids {
		p, err := l.GetProposal(id)
		if err != nil {
			return nil, err
		}
		if p != nil {
			out = append(out, *p)
		}
	}
	return out, nil
}

// FindOfferedProposal returns an unlaunched proposal with the same body
// digest for a session. Minting is idempotent on content: a double-click or a
// reload mints one plan, not a pile of identical offered rows.
func (l *Ledger) FindOfferedProposal(sessionID, digest string) (*Proposal, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	var id string
	err := l.DB().QueryRow(`SELECT id FROM proposals WHERE session_id = ? AND digest = ? AND status = ?
		ORDER BY created_at DESC LIMIT 1`, sessionID, digest, ProposalOffered).Scan(&id)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return l.getProposalLocked(id)
}
