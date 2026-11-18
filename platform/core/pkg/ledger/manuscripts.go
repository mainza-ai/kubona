package ledger

// Publication pipeline storage: manuscript drafts, sections, reviews,
// artifacts, signoffs, and citations.
//
// These rows are tenant-scoped evidence, not scratch UI state: the signoff
// binds a content hash, a build binds artifact hashes, and the API refuses a
// signoff whose hash no longer matches the stored draft. Foreign keys are
// deliberately absent: deleting a session must not silently delete the
// publication record (ledger retention rules), so the API checks the session
// and tenant explicitly on every read/write.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

const manuscriptsSchema = `
CREATE TABLE IF NOT EXISTS manuscripts (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	tenant_id TEXT NOT NULL DEFAULT '',
	title TEXT NOT NULL,
	abstract TEXT NOT NULL DEFAULT '',
	state TEXT NOT NULL DEFAULT 'draft',
	request_id TEXT NOT NULL DEFAULT '',
	content_hash TEXT NOT NULL DEFAULT '',
	build_hash TEXT NOT NULL DEFAULT '',
	review_json TEXT NOT NULL DEFAULT '{}',
	campaign_json TEXT NOT NULL DEFAULT '{}',
	evidence_json TEXT NOT NULL DEFAULT '{}',
	authors_json TEXT NOT NULL DEFAULT '[]',
	figures_json TEXT NOT NULL DEFAULT '[]',
	figure_sha_json TEXT NOT NULL DEFAULT '{}',
	source_date_epoch INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_manuscripts_session
	ON manuscripts(session_id, created_at DESC);

CREATE TABLE IF NOT EXISTS manuscript_sections (
	manuscript_id TEXT NOT NULL,
	slug TEXT NOT NULL,
	title TEXT NOT NULL,
	latex TEXT NOT NULL,
	position INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (manuscript_id, slug)
);

CREATE TABLE IF NOT EXISTS manuscript_reviews (
	id TEXT PRIMARY KEY,
	manuscript_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	tenant_id TEXT NOT NULL DEFAULT '',
	reviewer TEXT NOT NULL,
	recommendation TEXT NOT NULL,
	acceptable INTEGER NOT NULL DEFAULT 0,
	mean_score REAL NOT NULL DEFAULT 0,
	hygiene REAL NOT NULL DEFAULT 0,
	evidence_json TEXT NOT NULL DEFAULT '{}',
	content_hash TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_manuscript_reviews
	ON manuscript_reviews(manuscript_id, created_at DESC);

CREATE TABLE IF NOT EXISTS manuscript_artifacts (
	manuscript_id TEXT NOT NULL,
	name TEXT NOT NULL,
	sha256 TEXT NOT NULL,
	content BLOB NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (manuscript_id, name)
);

CREATE TABLE IF NOT EXISTS manuscript_signoffs (
	manuscript_id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	tenant_id TEXT NOT NULL DEFAULT '',
	signer_pubkey_hex TEXT NOT NULL,
	signature_hex TEXT NOT NULL,
	content_hash TEXT NOT NULL,
	expiry TEXT NOT NULL,
	signed_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS manuscript_citations (
	manuscript_id TEXT NOT NULL,
	cite_key TEXT NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	authors TEXT NOT NULL DEFAULT '',
	year TEXT NOT NULL DEFAULT '',
	doi TEXT NOT NULL DEFAULT '',
	claim_id TEXT NOT NULL DEFAULT '',
	source_id TEXT NOT NULL DEFAULT '',
	verified INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (manuscript_id, cite_key)
);

CREATE TABLE IF NOT EXISTS manuscript_annotations (
	id TEXT PRIMARY KEY,
	manuscript_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	tenant_id TEXT NOT NULL DEFAULT '',
	author TEXT NOT NULL,
	anchor TEXT NOT NULL,
	body TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'open',
	content_hash TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_manuscript_annotations
	ON manuscript_annotations(manuscript_id, created_at);`

// Manuscript states. A draft may be reviewed and signed; a build may happen
// before or after signoff, but a content change invalidates both. The API
// owns transition policy; the ledger stores the state.
const (
	ManuscriptStateDraft        = "draft"
	ManuscriptStateReviewed     = "reviewed"
	ManuscriptStateSigned       = "signed"
	ManuscriptStateBuilt        = "built"
	ManuscriptStateDepositReady = "deposit_ready"
)

var (
	ErrManuscriptNotFound = errors.New("manuscript not found")
	ErrManuscriptState    = errors.New("manuscript state does not allow this operation")
)

type ManuscriptSection struct {
	Slug     string
	Title    string
	LaTeX    string
	Position int
}

type ManuscriptCitation struct {
	Key      string
	Title    string
	Authors  string
	Year     string
	DOI      string
	ClaimID  string
	SourceID string
	// Verified means the claim this citation is bound to exists in the
	// ledger; an unbound citation is caller-supplied reference text.
	Verified bool
}

type ManuscriptArtifact struct {
	Name      string `json:"name"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
}

type ManuscriptSignoff struct {
	SessionID    string
	TenantID     string
	SignerPubKey string
	Signature    string
	ContentHash  string
	Expiry       string
	SignedAt     string
}

type Manuscript struct {
	ID              string
	SessionID       string
	TenantID        string
	Title           string
	Abstract        string
	State           string
	RequestID       string
	ContentHash     string
	BuildHash       string
	ReviewJSON      string
	CampaignJSON    string
	EvidenceJSON    string
	AuthorsJSON     string
	FiguresJSON     string
	FigureSHAJSON   string
	SourceDateEpoch int64
	CreatedAt       string
	UpdatedAt       string
	Sections        []ManuscriptSection
	Citations       []ManuscriptCitation
	Artifacts       []ManuscriptArtifact
	Signoff         *ManuscriptSignoff
}

type ManuscriptSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	State       string `json:"state"`
	ContentHash string `json:"content_hash"`
	BuildHash   string `json:"build_hash,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

// CreateManuscript inserts a draft and its sections/citations atomically.
func (l *Ledger) CreateManuscript(m *Manuscript) error {
	if m == nil || m.ID == "" || m.SessionID == "" || m.Title == "" {
		return fmt.Errorf("manuscript needs id, session, and title")
	}
	if m.State == "" {
		m.State = ManuscriptStateDraft
	}
	now := nowUTC()
	if m.CreatedAt == "" {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	if m.ReviewJSON == "" {
		m.ReviewJSON = "{}"
	}
	tx, err := l.DB().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`INSERT INTO manuscripts
		(id, session_id, tenant_id, title, abstract, state, request_id, content_hash, build_hash,
		 review_json, campaign_json, evidence_json, authors_json, figures_json,
		 figure_sha_json, source_date_epoch, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.SessionID, m.TenantID, m.Title, m.Abstract, m.State, m.RequestID, m.ContentHash,
		m.BuildHash, m.ReviewJSON, m.CampaignJSON, m.EvidenceJSON, m.AuthorsJSON,
		m.FiguresJSON, m.FigureSHAJSON, m.SourceDateEpoch, m.CreatedAt, m.UpdatedAt); err != nil {
		return fmt.Errorf("insert manuscript: %w", err)
	}
	for i, sec := range m.Sections {
		pos := sec.Position
		if pos == 0 {
			pos = i
		}
		if _, err := tx.Exec(`INSERT INTO manuscript_sections
			(manuscript_id, slug, title, latex, position) VALUES (?, ?, ?, ?, ?)`,
			m.ID, sec.Slug, sec.Title, sec.LaTeX, pos); err != nil {
			return fmt.Errorf("insert section: %w", err)
		}
	}
	for _, c := range m.Citations {
		if _, err := tx.Exec(`INSERT INTO manuscript_citations
			(manuscript_id, cite_key, title, authors, year, doi, claim_id, source_id, verified)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.ID, c.Key, c.Title, c.Authors, c.Year, c.DOI, c.ClaimID, c.SourceID, boolInt(c.Verified)); err != nil {
			return fmt.Errorf("insert citation: %w", err)
		}
	}
	return tx.Commit()
}

func (l *Ledger) ListManuscripts(sessionID string) ([]ManuscriptSummary, error) {
	rows, err := l.DB().Query(`SELECT id, title, state, content_hash, build_hash, created_at, updated_at
		FROM manuscripts WHERE session_id = ? ORDER BY created_at DESC, id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ManuscriptSummary{}
	for rows.Next() {
		var s ManuscriptSummary
		if err := rows.Scan(&s.ID, &s.Title, &s.State, &s.ContentHash, &s.BuildHash,
			&s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (l *Ledger) GetManuscript(sessionID, id string) (*Manuscript, error) {
	var m Manuscript
	err := l.DB().QueryRow(`SELECT id, session_id, tenant_id, title, abstract, state,
		request_id, content_hash, build_hash, review_json, campaign_json, evidence_json, authors_json,
		figures_json, figure_sha_json, source_date_epoch, created_at, updated_at
		FROM manuscripts WHERE session_id = ? AND id = ?`, sessionID, id).Scan(
		&m.ID, &m.SessionID, &m.TenantID, &m.Title, &m.Abstract, &m.State,
		&m.RequestID, &m.ContentHash, &m.BuildHash, &m.ReviewJSON, &m.CampaignJSON, &m.EvidenceJSON,
		&m.AuthorsJSON, &m.FiguresJSON, &m.FigureSHAJSON, &m.SourceDateEpoch,
		&m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrManuscriptNotFound
	}
	if err != nil {
		return nil, err
	}
	sections, err := l.manuscriptSections(id)
	if err != nil {
		return nil, err
	}
	m.Sections = sections
	citations, err := l.manuscriptCitations(id)
	if err != nil {
		return nil, err
	}
	m.Citations = citations
	artifacts, err := l.ListManuscriptArtifacts(id)
	if err != nil {
		return nil, err
	}
	m.Artifacts = artifacts
	signoff, err := l.GetManuscriptSignoff(id)
	if err != nil {
		return nil, err
	}
	m.Signoff = signoff
	return &m, nil
}

func (l *Ledger) manuscriptSections(id string) ([]ManuscriptSection, error) {
	rows, err := l.DB().Query(`SELECT slug, title, latex, position FROM manuscript_sections
		WHERE manuscript_id = ? ORDER BY position, slug`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ManuscriptSection{}
	for rows.Next() {
		var s ManuscriptSection
		if err := rows.Scan(&s.Slug, &s.Title, &s.LaTeX, &s.Position); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (l *Ledger) manuscriptCitations(id string) ([]ManuscriptCitation, error) {
	rows, err := l.DB().Query(`SELECT cite_key, title, authors, year, doi, claim_id, source_id, verified
		FROM manuscript_citations WHERE manuscript_id = ? ORDER BY cite_key`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ManuscriptCitation{}
	for rows.Next() {
		var c ManuscriptCitation
		var verified int
		if err := rows.Scan(&c.Key, &c.Title, &c.Authors, &c.Year, &c.DOI,
			&c.ClaimID, &c.SourceID, &verified); err != nil {
			return nil, err
		}
		c.Verified = verified != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateManuscriptDraft replaces a draft's content. Only drafts are
// editable: a reviewed/signed/built manuscript is immutable evidence, and a
// content change must invalidate every downstream hash rather than mutate it.
func (l *Ledger) UpdateManuscriptDraft(m *Manuscript) error {
	if m == nil || m.ID == "" || m.SessionID == "" {
		return fmt.Errorf("manuscript needs id and session")
	}
	var state string
	if err := l.DB().QueryRow(`SELECT state FROM manuscripts WHERE session_id = ? AND id = ?`,
		m.SessionID, m.ID).Scan(&state); errors.Is(err, sql.ErrNoRows) {
		return ErrManuscriptNotFound
	} else if err != nil {
		return err
	}
	if state != ManuscriptStateDraft {
		return fmt.Errorf("%w: %s manuscript is immutable", ErrManuscriptState, state)
	}
	tx, err := l.DB().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE manuscripts SET title = ?, abstract = ?, content_hash = ?,
		review_json = ?, campaign_json = ?, evidence_json = ?, authors_json = ?,
		figures_json = ?, figure_sha_json = ?, updated_at = ?
		WHERE session_id = ? AND id = ?`,
		m.Title, m.Abstract, m.ContentHash, m.ReviewJSON, m.CampaignJSON, m.EvidenceJSON,
		m.AuthorsJSON, m.FiguresJSON, m.FigureSHAJSON, nowUTC(), m.SessionID, m.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM manuscript_sections WHERE manuscript_id = ?`, m.ID); err != nil {
		return err
	}
	for i, sec := range m.Sections {
		pos := sec.Position
		if pos == 0 {
			pos = i
		}
		if _, err := tx.Exec(`INSERT INTO manuscript_sections
			(manuscript_id, slug, title, latex, position) VALUES (?, ?, ?, ?, ?)`,
			m.ID, sec.Slug, sec.Title, sec.LaTeX, pos); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM manuscript_citations WHERE manuscript_id = ?`, m.ID); err != nil {
		return err
	}
	for _, c := range m.Citations {
		if _, err := tx.Exec(`INSERT INTO manuscript_citations
			(manuscript_id, cite_key, title, authors, year, doi, claim_id, source_id, verified)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.ID, c.Key, c.Title, c.Authors, c.Year, c.DOI, c.ClaimID, c.SourceID, boolInt(c.Verified)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetManuscriptReviewState records the current review JSON and moves a draft
// to reviewed. Recomputing a review on a built manuscript is allowed (the
// review is over source content, not the PDF), but never downgrades the state.
func (l *Ledger) SetManuscriptReviewState(sessionID, id, reviewJSON string) error {
	state := ManuscriptStateDraft
	if err := l.DB().QueryRow(`SELECT state FROM manuscripts WHERE session_id = ? AND id = ?`,
		sessionID, id).Scan(&state); errors.Is(err, sql.ErrNoRows) {
		return ErrManuscriptNotFound
	} else if err != nil {
		return err
	}
	next := state
	if state == ManuscriptStateDraft {
		next = ManuscriptStateReviewed
	}
	_, err := l.DB().Exec(`UPDATE manuscripts SET review_json = ?, state = ?, updated_at = ?
		WHERE session_id = ? AND id = ?`, reviewJSON, next, nowUTC(), sessionID, id)
	return err
}

type ManuscriptReviewRecord struct {
	ID             string  `json:"id"`
	Reviewer       string  `json:"reviewer"`
	Recommendation string  `json:"recommendation"`
	Acceptable     bool    `json:"acceptable"`
	MeanScore      float64 `json:"mean_score"`
	Hygiene        float64 `json:"hygiene"`
	EvidenceJSON   string  `json:"evidence_json"`
	ContentHash    string  `json:"content_hash"`
	CreatedAt      string  `json:"created_at"`
}

// AddManuscriptReview appends an immutable review row. The caller must have
// verified that ContentHash matches the manuscript's current content.
func (l *Ledger) AddManuscriptReview(sessionID, manuscriptID string, r ManuscriptReviewRecord) error {
	if r.ID == "" || r.Reviewer == "" || r.Recommendation == "" {
		return fmt.Errorf("review needs id, reviewer, and recommendation")
	}
	if r.ContentHash == "" {
		return fmt.Errorf("review needs a content hash")
	}
	var tenant string
	if err := l.DB().QueryRow(`SELECT tenant_id FROM manuscripts WHERE session_id = ? AND id = ?`,
		sessionID, manuscriptID).Scan(&tenant); errors.Is(err, sql.ErrNoRows) {
		return ErrManuscriptNotFound
	} else if err != nil {
		return err
	}
	_, err := l.DB().Exec(`INSERT INTO manuscript_reviews
		(id, manuscript_id, session_id, tenant_id, reviewer, recommendation, acceptable,
		 mean_score, hygiene, evidence_json, content_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, manuscriptID, sessionID, tenant, r.Reviewer, r.Recommendation,
		boolInt(r.Acceptable), r.MeanScore, r.Hygiene, r.EvidenceJSON, r.ContentHash, nowUTC())
	if err != nil {
		return fmt.Errorf("insert manuscript review: %w", err)
	}
	return nil
}

func (l *Ledger) ListManuscriptReviews(manuscriptID string) ([]ManuscriptReviewRecord, error) {
	rows, err := l.DB().Query(`SELECT id, reviewer, recommendation, acceptable, mean_score,
		hygiene, evidence_json, content_hash, created_at
		FROM manuscript_reviews WHERE manuscript_id = ? ORDER BY created_at DESC, id`, manuscriptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ManuscriptReviewRecord{}
	for rows.Next() {
		var r ManuscriptReviewRecord
		var acceptable int
		if err := rows.Scan(&r.ID, &r.Reviewer, &r.Recommendation, &acceptable, &r.MeanScore,
			&r.Hygiene, &r.EvidenceJSON, &r.ContentHash, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Acceptable = acceptable != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// ReplaceManuscriptArtifacts installs a build's artifacts atomically and
// records the build hash. Old artifacts are replaced: an artifact set is one
// build, not an append-only log of builds.
func (l *Ledger) ReplaceManuscriptArtifacts(sessionID, id, buildHash string, artifacts map[string][]byte) error {
	tx, err := l.DB().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`UPDATE manuscripts SET build_hash = ?, state = ?, updated_at = ?
		WHERE session_id = ? AND id = ?`, buildHash, ManuscriptStateBuilt, nowUTC(), sessionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrManuscriptNotFound
	}
	if _, err := tx.Exec(`DELETE FROM manuscript_artifacts WHERE manuscript_id = ?`, id); err != nil {
		return err
	}
	now := nowUTC()
	for name, content := range artifacts {
		sum := sha256Hex(content)
		if _, err := tx.Exec(`INSERT INTO manuscript_artifacts
			(manuscript_id, name, sha256, content, created_at) VALUES (?, ?, ?, ?, ?)`,
			id, name, sum, content, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (l *Ledger) ListManuscriptArtifacts(manuscriptID string) ([]ManuscriptArtifact, error) {
	rows, err := l.DB().Query(`SELECT name, sha256, LENGTH(content), created_at
		FROM manuscript_artifacts WHERE manuscript_id = ? ORDER BY name`, manuscriptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ManuscriptArtifact{}
	for rows.Next() {
		var a ManuscriptArtifact
		if err := rows.Scan(&a.Name, &a.SHA256, &a.Size, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetManuscriptArtifact returns one artifact's bytes and recorded hash. The
// session/manuscript pair is checked so a caller cannot read another lab's
// artifact by guessing a name.
func (l *Ledger) GetManuscriptArtifact(sessionID, manuscriptID, name string) ([]byte, string, error) {
	var exists int
	if err := l.DB().QueryRow(`SELECT COUNT(*) FROM manuscripts WHERE session_id = ? AND id = ?`,
		sessionID, manuscriptID).Scan(&exists); err != nil {
		return nil, "", err
	}
	if exists == 0 {
		return nil, "", ErrManuscriptNotFound
	}
	var content []byte
	var sum string
	err := l.DB().QueryRow(`SELECT content, sha256 FROM manuscript_artifacts
		WHERE manuscript_id = ? AND name = ?`, manuscriptID, name).Scan(&content, &sum)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrManuscriptNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if sha256Hex(content) != sum {
		return nil, "", fmt.Errorf("artifact %q failed its recorded hash", name)
	}
	return content, sum, nil
}

// SetManuscriptSignoff records a verified signoff. The API verifies the
// signature and the content hash before calling this; the ledger still
// refuses a signoff for a manuscript whose stored hash differs.
func (l *Ledger) SetManuscriptSignoff(sessionID, manuscriptID string, s ManuscriptSignoff) error {
	if s.SignerPubKey == "" || s.Signature == "" || s.ContentHash == "" || s.Expiry == "" {
		return fmt.Errorf("signoff needs signer, signature, content hash, and expiry")
	}
	var tenant, currentHash string
	if err := l.DB().QueryRow(`SELECT tenant_id, content_hash FROM manuscripts
		WHERE session_id = ? AND id = ?`, sessionID, manuscriptID).Scan(&tenant, &currentHash); errors.Is(err, sql.ErrNoRows) {
		return ErrManuscriptNotFound
	} else if err != nil {
		return err
	}
	if currentHash != s.ContentHash {
		return fmt.Errorf("%w: signed content hash does not match the stored manuscript", ErrManuscriptState)
	}
	now := nowUTC()
	tx, err := l.DB().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`INSERT INTO manuscript_signoffs
		(manuscript_id, session_id, tenant_id, signer_pubkey_hex, signature_hex,
		 content_hash, expiry, signed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(manuscript_id) DO UPDATE SET
			signer_pubkey_hex = excluded.signer_pubkey_hex,
			signature_hex = excluded.signature_hex,
			content_hash = excluded.content_hash,
			expiry = excluded.expiry,
			signed_at = excluded.signed_at`,
		manuscriptID, sessionID, tenant, s.SignerPubKey, s.Signature, s.ContentHash, s.Expiry, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE manuscripts SET state = CASE WHEN state = 'built' THEN state ELSE ? END,
		updated_at = ? WHERE session_id = ? AND id = ?`,
		ManuscriptStateSigned, now, sessionID, manuscriptID); err != nil {
		return err
	}
	return tx.Commit()
}

func (l *Ledger) GetManuscriptSignoff(manuscriptID string) (*ManuscriptSignoff, error) {
	var s ManuscriptSignoff
	err := l.DB().QueryRow(`SELECT session_id, tenant_id, signer_pubkey_hex, signature_hex,
		content_hash, expiry, signed_at FROM manuscript_signoffs WHERE manuscript_id = ?`,
		manuscriptID).Scan(&s.SessionID, &s.TenantID, &s.SignerPubKey, &s.Signature,
		&s.ContentHash, &s.Expiry, &s.SignedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Manuscript annotation statuses (closed vocabulary).
const (
	AnnotationOpen      = "open"
	AnnotationAddressed = "addressed"
	AnnotationDismissed = "dismissed"
)

// ManuscriptAnnotation is one human review note anchored to a section of a
// specific draft. Annotations are evidence of review, not part of the
// rendered document, so they never enter the content hash; an open annotation
// blocks signoff.
type ManuscriptAnnotation struct {
	ID           string `json:"id"`
	ManuscriptID string `json:"manuscript_id"`
	Author       string `json:"author"`
	Anchor       string `json:"anchor"`
	Body         string `json:"body"`
	Status       string `json:"status"`
	ContentHash  string `json:"content_hash"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func validAnnotationStatus(status string) bool {
	switch status {
	case AnnotationOpen, AnnotationAddressed, AnnotationDismissed:
		return true
	}
	return false
}

func (l *Ledger) manuscriptTenant(sessionID, manuscriptID string) (string, string, error) {
	var tenant, contentHash string
	err := l.DB().QueryRow(`SELECT tenant_id, content_hash FROM manuscripts
		WHERE session_id = ? AND id = ?`, sessionID, manuscriptID).Scan(&tenant, &contentHash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrManuscriptNotFound
	}
	if err != nil {
		return "", "", err
	}
	return tenant, contentHash, nil
}

func (l *Ledger) AddManuscriptAnnotation(sessionID, manuscriptID string, a ManuscriptAnnotation) error {
	if a.ID == "" || a.Author == "" || a.Body == "" {
		return fmt.Errorf("annotation needs id, author, and body")
	}
	if a.Anchor == "" {
		a.Anchor = "general"
	}
	if a.Status == "" {
		a.Status = AnnotationOpen
	}
	if !validAnnotationStatus(a.Status) {
		return fmt.Errorf("annotation status %q is not open|addressed|dismissed", a.Status)
	}
	tenant, contentHash, err := l.manuscriptTenant(sessionID, manuscriptID)
	if err != nil {
		return err
	}
	now := nowUTC()
	_, err = l.DB().Exec(`INSERT INTO manuscript_annotations
		(id, manuscript_id, session_id, tenant_id, author, anchor, body, status,
		 content_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, manuscriptID, sessionID, tenant, a.Author, a.Anchor, a.Body, a.Status,
		contentHash, now, now)
	if err != nil {
		return fmt.Errorf("insert manuscript annotation: %w", err)
	}
	return nil
}

func (l *Ledger) ListManuscriptAnnotations(sessionID, manuscriptID string) ([]ManuscriptAnnotation, error) {
	if _, _, err := l.manuscriptTenant(sessionID, manuscriptID); err != nil {
		return nil, err
	}
	rows, err := l.DB().Query(`SELECT id, manuscript_id, author, anchor, body, status,
		content_hash, created_at, updated_at FROM manuscript_annotations
		WHERE manuscript_id = ? ORDER BY created_at, id`, manuscriptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ManuscriptAnnotation{}
	for rows.Next() {
		var a ManuscriptAnnotation
		if err := rows.Scan(&a.ID, &a.ManuscriptID, &a.Author, &a.Anchor, &a.Body,
			&a.Status, &a.ContentHash, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (l *Ledger) SetManuscriptAnnotationStatus(sessionID, manuscriptID, annotationID, status string) error {
	status = strings.ToLower(strings.TrimSpace(status))
	if !validAnnotationStatus(status) {
		return fmt.Errorf("annotation status %q is not open|addressed|dismissed", status)
	}
	if _, _, err := l.manuscriptTenant(sessionID, manuscriptID); err != nil {
		return err
	}
	res, err := l.DB().Exec(`UPDATE manuscript_annotations SET status = ?, updated_at = ?
		WHERE manuscript_id = ? AND id = ?`, status, nowUTC(), manuscriptID, annotationID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrManuscriptNotFound
	}
	return nil
}

func (l *Ledger) CountOpenAnnotations(sessionID, manuscriptID string) (int, error) {
	if _, _, err := l.manuscriptTenant(sessionID, manuscriptID); err != nil {
		return 0, err
	}
	var n int
	if err := l.DB().QueryRow(`SELECT COUNT(*) FROM manuscript_annotations
		WHERE manuscript_id = ? AND status = ?`, manuscriptID, AnnotationOpen).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// GetManuscriptByRequestID resolves an idempotent create. Empty ids never
// match: only explicit request ids participate.
func (l *Ledger) GetManuscriptByRequestID(sessionID, requestID string) (*Manuscript, error) {
	if strings.TrimSpace(requestID) == "" {
		return nil, ErrManuscriptNotFound
	}
	var id string
	err := l.DB().QueryRow(`SELECT id FROM manuscripts WHERE session_id = ? AND request_id = ?`,
		sessionID, requestID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrManuscriptNotFound
	}
	if err != nil {
		return nil, err
	}
	return l.GetManuscript(sessionID, id)
}

// SetManuscriptState records an explicit state transition (used by deposit
// staging to distinguish "built" from "staged"). Unknown states refuse.
func (l *Ledger) SetManuscriptState(sessionID, id, state string) error {
	switch state {
	case ManuscriptStateDraft, ManuscriptStateReviewed, ManuscriptStateSigned,
		ManuscriptStateBuilt, ManuscriptStateDepositReady:
	default:
		return fmt.Errorf("unknown manuscript state %q", state)
	}
	res, err := l.DB().Exec(`UPDATE manuscripts SET state = ?, updated_at = ?
		WHERE session_id = ? AND id = ?`, state, nowUTC(), sessionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrManuscriptNotFound
	}
	return nil
}

// ListManuscriptsPage returns a bounded page and whether more rows exist.
func (l *Ledger) ListManuscriptsPage(sessionID string, limit, offset int) ([]ManuscriptSummary, bool, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := l.DB().Query(`SELECT id, title, state, content_hash, build_hash, created_at, updated_at
		FROM manuscripts WHERE session_id = ? ORDER BY created_at DESC, id LIMIT ? OFFSET ?`,
		sessionID, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []ManuscriptSummary{}
	for rows.Next() {
		var s ManuscriptSummary
		if err := rows.Scan(&s.ID, &s.Title, &s.State, &s.ContentHash, &s.BuildHash,
			&s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, false, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	return out, hasMore, nil
}

// DeleteManuscript removes one manuscript and every child row. The caller
// records the deletion event and hashes before calling; this method is the
// storage destruction only.
func (l *Ledger) DeleteManuscript(sessionID, id string) error {
	tx, err := l.DB().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`DELETE FROM manuscripts WHERE session_id = ? AND id = ?`, sessionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrManuscriptNotFound
	}
	for _, stmt := range []string{
		`DELETE FROM manuscript_sections WHERE manuscript_id = ?`,
		`DELETE FROM manuscript_reviews WHERE manuscript_id = ?`,
		`DELETE FROM manuscript_artifacts WHERE manuscript_id = ?`,
		`DELETE FROM manuscript_signoffs WHERE manuscript_id = ?`,
		`DELETE FROM manuscript_citations WHERE manuscript_id = ?`,
		`DELETE FROM manuscript_annotations WHERE manuscript_id = ?`,
	} {
		if _, err := tx.Exec(stmt, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (l *Ledger) ManuscriptArtifactBytesForSession(sessionID string) (int64, error) {
	var n int64
	err := l.DB().QueryRow(`SELECT COALESCE(SUM(LENGTH(content)), 0)
		FROM manuscript_artifacts a JOIN manuscripts m ON m.id = a.manuscript_id
		WHERE m.session_id = ?`, sessionID).Scan(&n)
	return n, err
}

func (l *Ledger) ManuscriptArtifactBytesForManuscript(manuscriptID string) (int64, error) {
	var n int64
	err := l.DB().QueryRow(`SELECT COALESCE(SUM(LENGTH(content)), 0)
		FROM manuscript_artifacts WHERE manuscript_id = ?`, manuscriptID).Scan(&n)
	return n, err
}

// CountEvidenceManuscripts reports signed/built/deposit-ready manuscripts in a
// session. Session deletion uses this to require an explicit acknowledgement
// after an export, so publication evidence cannot vanish silently.
func (l *Ledger) CountEvidenceManuscripts(sessionID string) (int, error) {
	var n int
	err := l.DB().QueryRow(`SELECT COUNT(*) FROM manuscripts
		WHERE session_id = ? AND state IN (?, ?, ?)`,
		sessionID, ManuscriptStateSigned, ManuscriptStateBuilt, ManuscriptStateDepositReady).Scan(&n)
	return n, err
}
