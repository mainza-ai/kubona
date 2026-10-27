package session

// S5 — session export/import (single-record migration) + DeleteSession
// cascade fix.
//
// Threat (G9/G6): checkpoint resume and lab-to-lab handoff today mean
// copying raw DB rows — no validation, no single record, silent
// divergence; and DeleteSession orphans 5 of 8 tables. Export sweeps all
// session-scoped tables (plus already-orphaned rows) into a versioned,
// digest-pinned manifest; import validates everything and seals ONE
// ledger record on activation. Content rows carry no filesystem paths, so
// traversal is structurally N/A (stated, not assumed — nothing here is
// ever written to a disk path).

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/autoresearch"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/money"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	pb "github.com/mainza-ai/kubona/core/proto"
)

// W0.1 — job/loop lifecycle sets. SINGLE SOURCE for "is this work over".
// Threat (verified): DeleteBlockers compared against hand-written status
// literals ('FAILED','failed','completed') that never occur in the jobs
// table — failed jobs (stored as JOB_STATUS_FAILED) blocked deletion
// forever. Every query below and in pkg/jobs builds from these sets, so a
// new status defaults to blocking (fail-closed) until classified here.
var (
	// TerminalJobStatuses: a job in one of these states is over and must
	// not block session deletion.
	TerminalJobStatuses = []string{
		pb.JobStatus_JOB_STATUS_COMPLETED.String(),
		pb.JobStatus_JOB_STATUS_FAILED.String(),
	}
	// LiveJobStatuses: jobs in these states are still live work — the
	// CAS-guard live set for cancel/abort sweeps.
	LiveJobStatuses = []string{
		pb.JobStatus_JOB_STATUS_QUEUED.String(),
		pb.JobStatus_JOB_STATUS_RUNNING.String(),
	}
	// TerminalLoopStatuses: a loop in one of these states is over.
	// Sleeping loops are NOT terminal (wake resumes the same session).
	// W0.21: supervisor terminal states included — the loops table is
	// written by two writers (autoresearch engine + supervisor) with two
	// vocabularies; a killed loop blocked deletion forever.
	TerminalLoopStatuses = []string{
		string(autoresearch.StatusCompleted),
		string(autoresearch.StatusAborted),
		supervisor.LoopCompleted,
		supervisor.LoopAborted,
		supervisor.LoopKilled,
	}
)

// Placeholders returns "?, ?, ..." for n bind args.
func Placeholders(n int) string {
	if n <= 0 {
		return "NULL" // IN (NULL) matches nothing — fail-closed, never "all".
	}
	return strings.Repeat("?, ", n-1) + "?"
}

// LiveJobStatusClause returns the SQL "IN (?, ...)" fragment plus bind
// args for the single-source live set, so every CAS sweep in pkg/jobs
// classifies liveness identically to DeleteBlockers' terminal set.
func LiveJobStatusClause() (string, []any) {
	args := make([]any, 0, len(LiveJobStatuses))
	for _, s := range LiveJobStatuses {
		args = append(args, s)
	}
	return "IN (" + Placeholders(len(LiveJobStatuses)) + ")", args
}

// Manifest version. Major bump = incompatible: unknown majors refuse with
// unsupported_manifest_version (a v2 reader must never misread v1).
const ManifestVersion = "1"

// Size caps (single source for migration I/O).
const (
	MaxManifestBytes = 64 * 1024 * 1024
	MaxCanvasBytes   = 256 * 1024
	MaxVersions      = 50
	MaxEventsExport  = 10000
	MaxClaimsExport  = 1000
)

// SessionManifest is the portable session. Digests pin every section
// (CanonicalHash over the section value); import recomputes all of them.
type SessionManifest struct {
	Version         string                   `json:"version"`
	ExportedAt      string                   `json:"exported_at"`
	Session         Session                  `json:"session"`
	Messages        []ChatMessage            `json:"messages"`
	Jobs            []JobRow                 `json:"jobs"`
	Loops           []ledger.LoopPersist     `json:"loops"`
	Events          []ledger.EventRecord     `json:"events"`
	EventsTruncated bool                     `json:"events_truncated"`
	CampaignState   json.RawMessage          `json:"campaign_state"`
	Versions        []ledger.CampaignVersion `json:"versions"`
	Claims          []ledger.ClaimRecord     `json:"claims"`
	ClaimsTruncated bool                     `json:"claims_truncated"`
	CanvasLayout    string                   `json:"canvas_layout"`
	Digests         map[string]string        `json:"digests"`
	// R1: every section sealed as a canonical evidence envelope at
	// export (identity + analysis profile, verifiable by either rail).
	// omitempty: manifests predating envelopes carry no field and still
	// import (digests verify as before); when present, every envelope
	// must verify and agree with the recomputed digest.
	SectionEnvelopes map[string]ledger.Envelope `json:"section_envelopes,omitempty"`
	// S5b/C2: Python-rail sidecar rows for this session (ninth section,
	// digest-pinned + enveloped like the rest). No omitempty: export
	// always writes the array (possibly []) so the digest is stable
	// across JSON round-trips — an omitted-then-nil [] would digest as
	// "null" instead of "[]" and fail import. Pre-S5b manifests without
	// the digest row refuse under the strict roster (decided).
	PythonLedger []PythonRecord `json:"python_ledger"`
	// Operator signature over the canonical manifest (HMAC-SHA256 with
	// KUBONA_IMPORT_HMAC_KEY, falling back to KUBONA_API_TOKEN). Import is
	// operator-only; the signature binds the bytes so a manifest edited
	// on disk cannot be replayed as authority. Production requires it.
	SignatureAlg string `json:"signature_alg,omitempty"`
	Signature    string `json:"signature,omitempty"`
}

// JobRow mirrors the jobs table (session package cannot import jobs:
// jobs imports session — the cycle is resolved by row structs + SQL).
type JobRow struct {
	ID               string  `json:"id"`
	SessionID        string  `json:"session_id"`
	Status           string  `json:"status"`
	JobType          string  `json:"job_type"`
	ParamsJSON       string  `json:"params_json"`
	ResultJSON       string  `json:"result_json"`
	CreatedAt        string  `json:"created_at"`
	CompletedAt      string  `json:"completed_at"`
	ReservedTenant   string  `json:"reserved_tenant"`
	ReservedAmount   float64 `json:"reserved_amount"`
	CompilerUsed     string  `json:"compiler_used"`
	FallbackEstimate int     `json:"fallback_estimate"`
	// R2b: the spend-mandate receipt this job settles against ("" when
	// unmandated). omitempty keeps old manifests shape-stable on write;
	// import tolerates absence (nil reads as "").
	MandateRef string `json:"mandate_ref,omitempty"`
}

func sectionDigest(v any) (string, error) {
	// Normalize through generic JSON first: export hashes typed structs
	// while import (via HTTP) hashes maps — without normalization the
	// same content digests differently (omitempty, int64 vs float64).
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	var norm any
	if err := json.Unmarshal(raw, &norm); err != nil {
		return "", err
	}
	return ledger.CanonicalHash(norm)
}

// importHMACKey is the operator key that seals manifests. It is separate
// from the reviewer key so rotating the audit signer does not invalidate
// old migrations.
func importHMACKey() string {
	if k := strings.TrimSpace(os.Getenv("KUBONA_IMPORT_HMAC_KEY")); k != "" {
		return k
	}
	return strings.TrimSpace(os.Getenv("KUBONA_API_TOKEN"))
}

func signedImportRequired() bool {
	return os.Getenv("KUBONA_ENV") == "production" ||
		os.Getenv("KUBONA_REQUIRE_SIGNED_IMPORT") == "1"
}

func manifestSignature(man *SessionManifest, key string) string {
	// Sign the section digests, not the encoded struct: a JSON round-trip
	// through map[string]any (the HTTP path) may normalise number/slice
	// encodings, but digest names+values are stable. Digests already pin
	// every section; this binds them to the operator key.
	names := make([]string, 0, len(man.Digests))
	for name := range man.Digests {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("kubona-manifest-v1\n")
	fmt.Fprintf(&b, "%s\n%s\n%s\n", man.Version, man.Session.ID, man.Session.TenantID)
	fmt.Fprintf(&b, "events_truncated=%t\nclaims_truncated=%t\n", man.EventsTruncated, man.ClaimsTruncated)
	for _, name := range names {
		fmt.Fprintf(&b, "%s=%s\n", name, man.Digests[name])
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(b.String()))
	return hex.EncodeToString(mac.Sum(nil))
}

// ExportSession sweeps every session-scoped table for id (8 content
// tables). Unknown sessions refuse — exports attest real sessions only.
func (m *Manager) ExportSession(id string) (*SessionManifest, error) {
	sess, err := m.GetSession(id)
	if err != nil {
		return nil, err
	}
	// DB DATETIME precision is seconds: truncate here so re-exports digest
	// identically (nanoseconds would drift across the round-trip).
	sess.CreatedAt = sess.CreatedAt.Truncate(time.Second)
	sess.UpdatedAt = sess.UpdatedAt.Truncate(time.Second)
	msgs, err := m.GetMessages(id)
	if err != nil {
		return nil, fmt.Errorf("export messages: %w", err)
	}
	jobs, err := m.exportJobs(id)
	if err != nil {
		return nil, err
	}
	var loops []ledger.LoopPersist
	if all, err := m.ledger.ListLoopStates(); err != nil {
		return nil, fmt.Errorf("export loops: %w", err)
	} else {
		for _, p := range all {
			if p.SessionID == id {
				p.LastActivity = p.LastActivity.Truncate(time.Second)
				// UpdatedAt is server bookkeeping rewritten on every
				// write — not session content. Zero it so digests are
				// stable across import (which stamps a fresh timestamp).
				p.UpdatedAt = time.Time{}
				loops = append(loops, p)
			}
		}
	}
	events, err := m.ledger.GetRecentEvents(id, MaxEventsExport+1)
	if err != nil {
		return nil, fmt.Errorf("export events: %w", err)
	}
	truncated := false
	if len(events) > MaxEventsExport {
		events = events[:MaxEventsExport]
		truncated = true
	}
	state, err := m.ledger.GetCampaignState(id)
	if err != nil {
		state = nil // no campaign yet — null, not an error
	}
	versions, err := m.ledger.GetCampaignVersions(id, MaxVersions)
	if err != nil {
		return nil, fmt.Errorf("export versions: %w", err)
	}
	claims, err := m.ledger.ListClaimRecords(id, MaxClaimsExport+1)
	if err != nil {
		return nil, fmt.Errorf("export claims: %w", err)
	}
	claimsTruncated := false
	if len(claims) > MaxClaimsExport {
		claims = claims[:MaxClaimsExport]
		claimsTruncated = true
	}
	var canvas string
	_ = m.ledger.DB().QueryRow(`SELECT layout_json FROM canvas_layouts WHERE session_id = ?`, id).Scan(&canvas)

	// S5b: sweep the Python sidecar for this session's rows (missing file
	// = empty section, never an error — sessions without workflow history
	// migrate exactly as before).
	pyRecs, err := exportPythonRecords(id, sess.TenantID)
	if err != nil {
		return nil, fmt.Errorf("export python ledger: %w", err)
	}

	out := &SessionManifest{
		Version:         ManifestVersion,
		ExportedAt:      time.Now().UTC().Format(time.RFC3339),
		Session:         *sess,
		Messages:        msgs,
		Jobs:            jobs,
		Loops:           loops,
		Events:          events,
		EventsTruncated: truncated,
		CampaignState:   json.RawMessage(nullIfEmpty(state)),
		Versions:        versions,
		Claims:          claims,
		ClaimsTruncated: claimsTruncated,
		CanvasLayout:    canvas,
		Digests:         map[string]string{},
		PythonLedger:    pyRecs,
	}
	if out.Messages == nil {
		out.Messages = []ChatMessage{}
	}
	if out.Jobs == nil {
		out.Jobs = []JobRow{}
	}
	if out.Loops == nil {
		out.Loops = []ledger.LoopPersist{}
	}
	if out.Events == nil {
		out.Events = []ledger.EventRecord{}
	}
	if out.Versions == nil {
		out.Versions = []ledger.CampaignVersion{}
	}
	if out.Claims == nil {
		out.Claims = []ledger.ClaimRecord{}
	}
	sections := map[string]any{
		"session": out.Session, "messages": out.Messages, "jobs": out.Jobs,
		"loops": out.Loops, "events": out.Events, "campaign_state": out.CampaignState,
		"versions": out.Versions, "claims": out.Claims, "canvas_layout": out.CanvasLayout,
		"python_ledger": out.PythonLedger,
	}
	for name, sec := range sections {
		h, err := sectionDigest(sec)
		if err != nil {
			return nil, fmt.Errorf("digest %s: %w", name, err)
		}
		out.Digests[name] = h
	}
	// R1: seal each section as an evidence envelope (content-addressed
	// section identity + export profile). Tenantless sessions predate
	// attribution — their sections stay digest-pinned only (import
	// refuses them at the tenant check below regardless).
	if out.Session.TenantID != "" {
		out.SectionEnvelopes = map[string]ledger.Envelope{}
		for name := range sections {
			env, err := ledger.BuildEnvelope(
				map[string]any{"digest": out.Digests[name], "section": name},
				nil,
				map[string]any{"manifest_version": ManifestVersion, "method": "session-export"},
				out.Session.TenantID,
				nil,
			)
			if err != nil {
				return nil, fmt.Errorf("seal section %s: %w", name, err)
			}
			out.SectionEnvelopes[name] = *env
		}
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("manifest marshal: %w", err)
	}
	if len(raw) > MaxManifestBytes {
		return nil, fmt.Errorf("manifest %d bytes exceeds %d cap: session too large to migrate", len(raw), MaxManifestBytes)
	}
	if key := importHMACKey(); key != "" {
		out.SignatureAlg = "hmac-sha256"
		out.Signature = manifestSignature(out, key)
	} else if signedImportRequired() {
		return nil, fmt.Errorf("manifest signing key not configured: refusing an unsigned export in production")
	}
	return out, nil
}

func nullIfEmpty(b []byte) []byte {
	if len(b) == 0 {
		return []byte("null")
	}
	return b
}

func (m *Manager) exportJobs(sessionID string) ([]JobRow, error) {
	rows, err := m.ledger.DB().Query(`SELECT id, session_id, status, job_type, params_json, result_json, created_at, completed_at, reserved_tenant, reserved_amount_micro_usd, compiler_used, fallback_estimate, mandate_ref FROM jobs WHERE session_id = ? ORDER BY created_at`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []JobRow
	for rows.Next() {
		var j JobRow
		var resultJSON, completedAt, createdAt, mandateRef sql.NullString
		var reservedMicro int64
		if err := rows.Scan(&j.ID, &j.SessionID, &j.Status, &j.JobType, &j.ParamsJSON, &resultJSON, &createdAt, &completedAt, &j.ReservedTenant, &reservedMicro, &j.CompilerUsed, &j.FallbackEstimate, &mandateRef); err != nil {
			return nil, err
		}
		j.ReservedAmount = money.MicroUSD(reservedMicro).USD()
		if mandateRef.Valid {
			j.MandateRef = mandateRef.String
		}
		if resultJSON.Valid {
			j.ResultJSON = resultJSON.String
		}
		if createdAt.Valid {
			j.CreatedAt = createdAt.String
		}
		if completedAt.Valid {
			j.CompletedAt = completedAt.String
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// ImportResult reports activation. Existing=true means the session already
// lived here: nothing was written, the current export is returned (replay
// by existence — double-activate replays instead of forking).
type ImportResult struct {
	SessionID string
	Existing  bool
	Manifest  *SessionManifest
	// S5b: sidecar rows appended by this import (0 when the section was
	// empty or every row was already present — idempotent replay).
	PythonReplayed int
}

// ImportSession validates a manifest and activates it with ONE ledger
// record. Imported RUNNING/QUEUED jobs keep their status rows but no
// engine resumes them (no ghost workers — resumption is explicit).
func (m *Manager) ImportSession(man *SessionManifest) (*ImportResult, error) {
	if man == nil {
		return nil, fmt.Errorf("import requires a manifest")
	}
	major := strings.SplitN(man.Version, ".", 2)[0]
	if major != "1" {
		return nil, fmt.Errorf("unsupported_manifest_version %q: this importer reads major 1 only", man.Version)
	}
	if man.Signature != "" {
		key := importHMACKey()
		if key == "" {
			return nil, fmt.Errorf("manifest signature present but no import key configured: refusing")
		}
		if man.SignatureAlg != "" && man.SignatureAlg != "hmac-sha256" {
			return nil, fmt.Errorf("unsupported manifest signature_alg %q", man.SignatureAlg)
		}
		want := manifestSignature(man, key)
		if !hmac.Equal([]byte(want), []byte(man.Signature)) {
			return nil, fmt.Errorf("manifest signature invalid: refusing tampered or foreign bytes")
		}
	} else if signedImportRequired() {
		return nil, fmt.Errorf("unsigned manifest refused: production imports must be operator-signed")
	}
	if man.Session.ID == "" {
		return nil, fmt.Errorf("manifest session has no id")
	}
	if math.IsNaN(man.Session.SpendLimitUSD) || math.IsInf(man.Session.SpendLimitUSD, 0) ||
		math.IsNaN(man.Session.CurrentSpendUSD) || math.IsInf(man.Session.CurrentSpendUSD, 0) ||
		man.Session.SpendLimitUSD < 0 || man.Session.CurrentSpendUSD < 0 {
		return nil, fmt.Errorf("manifest session spend fields must be finite and non-negative")
	}
	if man.Session.CurrentSpendUSD > man.Session.SpendLimitUSD+1e-9 {
		return nil, fmt.Errorf("manifest current_spend_usd %.6f exceeds its ceiling %.6f: refusing negative books",
			man.Session.CurrentSpendUSD, man.Session.SpendLimitUSD)
	}
	// Validate FIRST, before the existence short-circuit below: a
	// tampered manifest must refuse even when its session already lives
	// here (replay-by-existence must never launder corrupt bytes).
	// Caps before content (a hostile manifest fails cheap).
	if len(man.Versions) > MaxVersions {
		return nil, fmt.Errorf("manifest carries %d versions, cap is %d", len(man.Versions), MaxVersions)
	}
	if len(man.CanvasLayout) > MaxCanvasBytes {
		return nil, fmt.Errorf("manifest canvas %d bytes exceeds %d cap", len(man.CanvasLayout), MaxCanvasBytes)
	}
	raw, err := json.Marshal(man)
	if err != nil {
		return nil, fmt.Errorf("manifest marshal: %w", err)
	}
	if len(raw) > MaxManifestBytes {
		return nil, fmt.Errorf("manifest %d bytes exceeds %d cap", len(raw), MaxManifestBytes)
	}
	// Digests pin every section (recompute, never trust). The roster is
	// strict: every known section must carry a digest row and no unknown
	// rows are accepted (S5b — a stripped section + stripped digest row
	// must refuse, not silently migrate partial history; a future
	// section bumps the manifest major instead of smuggling rows past
	// v1 readers, which ignore unknown SECTIONS but never unknown
	// digest rows).
	sections := map[string]any{
		"session": man.Session, "messages": man.Messages, "jobs": man.Jobs,
		"loops": man.Loops, "events": man.Events, "campaign_state": man.CampaignState,
		"versions": man.Versions, "claims": man.Claims, "canvas_layout": man.CanvasLayout,
		"python_ledger": man.PythonLedger,
	}
	for name, sec := range sections {
		want, ok := man.Digests[name]
		if !ok {
			return nil, fmt.Errorf("manifest missing digest row for %s: refusing partial manifest", name)
		}
		h, herr := sectionDigest(sec)
		if herr != nil {
			return nil, fmt.Errorf("digest %s: %w", name, herr)
		}
		if want != h {
			return nil, fmt.Errorf("manifest digest mismatch on %s: tampered or corrupt", name)
		}
	}
	for name := range man.Digests {
		if _, ok := sections[name]; !ok {
			return nil, fmt.Errorf("manifest carries digest row for unknown section %s: refused", name)
		}
	}
	// R1: when section envelopes are present, every one must verify AND
	// agree with the recomputed digest (an envelope for other bytes, or
	// a swapped section name, refuses here — not at replay).
	if man.SectionEnvelopes != nil {
		for name, h := range man.Digests {
			env, ok := man.SectionEnvelopes[name]
			if !ok {
				return nil, fmt.Errorf("section %s has a digest but no envelope: partial envelope set refused", name)
			}
			if err := ledger.VerifyEnvelope(&env); err != nil {
				return nil, fmt.Errorf("section %s envelope invalid: %w", name, err)
			}
			gotSection, _ := env.NormalizedResult["section"].(string)
			gotDigest, _ := env.NormalizedResult["digest"].(string)
			if gotSection != name || gotDigest != h {
				return nil, fmt.Errorf("section %s envelope binds other bytes (section=%q digest=%q)", name, gotSection, gotDigest)
			}
		}
		for name := range man.SectionEnvelopes {
			if _, ok := man.Digests[name]; !ok {
				return nil, fmt.Errorf("envelope for unknown section %s: refused", name)
			}
		}
	}
	// Tenant must resolve (no unattributed imports).
	if man.Session.TenantID == "" {
		return nil, fmt.Errorf("manifest session has no tenant_id")
	}
	var tenantExists int
	if err := m.ledger.DB().QueryRow(`SELECT COUNT(*) FROM tenants WHERE id = ?`, man.Session.TenantID).Scan(&tenantExists); err != nil || tenantExists == 0 {
		return nil, fmt.Errorf("manifest tenant %q does not exist: import refused", man.Session.TenantID)
	}
	for _, rec := range man.PythonLedger {
		if len(rec.Payload) > 0 {
			var payload struct {
				SessionID string `json:"session_id"`
			}
			if err := json.Unmarshal(rec.Payload, &payload); err == nil && payload.SessionID != "" && payload.SessionID != man.Session.ID {
				return nil, fmt.Errorf("manifest python ledger record names foreign session %q", payload.SessionID)
			}
		}
	}

	// Replay by existence (after validation).
	if _, err := m.GetSession(man.Session.ID); err == nil {
		cur, cerr := m.ExportSession(man.Session.ID)
		if cerr != nil {
			return nil, fmt.Errorf("session %q already exists (re-export failed: %v)", man.Session.ID, cerr)
		}
		pyReplayed := 0
		if len(man.PythonLedger) > 0 {
			n, err := replayPythonRecords(man.Session.TenantID, man.PythonLedger)
			if err != nil {
				return nil, fmt.Errorf("replay python ledger: %w", err)
			}
			pyReplayed = n
		}
		return &ImportResult{SessionID: man.Session.ID, Existing: true, Manifest: cur, PythonReplayed: pyReplayed}, nil
	}

	db := m.ledger.DB()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	exec := func(q string, args ...any) error {
		_, e := tx.Exec(q, args...)
		return e
	}
	s := man.Session
	limitMicro, lerr := money.FromUSD(s.SpendLimitUSD)
	if lerr != nil {
		return nil, fmt.Errorf("import session spend limit: %w", lerr)
	}
	spendMicro, serr := money.FromUSD(s.CurrentSpendUSD)
	if serr != nil {
		return nil, fmt.Errorf("import session spend: %w", serr)
	}
	if err := exec(`INSERT INTO sessions (id, title, tenant_id, created_at, updated_at, status, spend_limit_micro_usd, current_spend_micro_usd) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Title, s.TenantID, s.CreatedAt.UTC().Format(time.RFC3339), s.UpdatedAt.UTC().Format(time.RFC3339), s.Status, int64(limitMicro), int64(spendMicro)); err != nil {
		return nil, fmt.Errorf("import session row: %w", err)
	}
	for _, msg := range man.Messages {
		var propStr sql.NullString
		if msg.ProposalJSON != nil && len(*msg.ProposalJSON) > 0 {
			propStr = sql.NullString{String: string(*msg.ProposalJSON), Valid: true}
		}
		var dur any
		if msg.DurationMs != nil {
			dur = *msg.DurationMs
		}
		var usage any
		if msg.UsageJSON != nil && len(*msg.UsageJSON) > 0 {
			usage = string(*msg.UsageJSON)
		}
		if err := exec(`INSERT INTO chat_messages (id, session_id, role, content, proposal_json, created_at, duration_ms, usage_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			msg.ID, s.ID, msg.Role, msg.Content, propStr, msg.CreatedAt, dur, usage); err != nil {
			return nil, fmt.Errorf("import message %s: %w", msg.ID, err)
		}
	}
	for _, j := range man.Jobs {
		if j.SessionID != s.ID {
			return nil, fmt.Errorf("import job %s names foreign session %q", j.ID, j.SessionID)
		}
		// Empty datetimes store as NULL (SubmitJob semantics: nil time, not
		// ""). The driver parses DATETIME columns, so "" would read back
		// as zero-time and break round-trip digests.
		resMicro, merr := money.FromUSD(j.ReservedAmount)
		if merr != nil {
			return nil, fmt.Errorf("import job %s reservation: %w", j.ID, merr)
		}
		if err := exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, result_json, created_at, completed_at, reserved_tenant, reserved_amount_micro_usd, compiler_used, fallback_estimate, mandate_ref) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			j.ID, s.ID, j.Status, j.JobType, j.ParamsJSON, j.ResultJSON, nullDatetime(j.CreatedAt), nullDatetime(j.CompletedAt), j.ReservedTenant, int64(resMicro), j.CompilerUsed, j.FallbackEstimate, j.MandateRef); err != nil {
			return nil, fmt.Errorf("import job %s: %w", j.ID, err)
		}
	}
	for _, p := range man.Loops {
		if p.SessionID != s.ID {
			return nil, fmt.Errorf("import loop %s names foreign session %q", p.ID, p.SessionID)
		}
		resMicro, rerr := money.FromUSD(p.ReservedAmount)
		if rerr != nil {
			return nil, fmt.Errorf("import loop %s reservation: %w", p.ID, rerr)
		}
		consMicro, cerr := money.FromUSD(p.ConsumedAmount)
		if cerr != nil {
			return nil, fmt.Errorf("import loop %s consumed: %w", p.ID, cerr)
		}
		if err := exec(`INSERT INTO loops (id, session_id, harness_id, status, step_count, max_steps, diverged, created_at, updated_at, last_activity, drain, reserved_amount_micro_usd, reserved_tenant, consumed_amount_micro_usd) VALUES (?, ?, ?, ?, ?, ?, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?, ?, ?, ?)`,
			p.ID, s.ID, p.HarnessID, p.Status, p.StepCount, p.MaxSteps, p.LastActivity.UTC().Format(time.RFC3339), boolInt(p.Drain), int64(resMicro), p.ReservedTenant, int64(consMicro)); err != nil {
			return nil, fmt.Errorf("import loop %s: %w", p.ID, err)
		}
	}
	// Import is oldest-first: export returns newest-first (GetRecentEvents),
	// and inserting in that order silently reversed the hash chain. Sort a
	// COPY (never mutate the caller's manifest — its digests are computed
	// over the original order) so a round-trip preserves VerifyChain.
	events := append([]ledger.EventRecord(nil), man.Events...)
	sort.SliceStable(events, func(i, j int) bool { return events[i].ID < events[j].ID })
	for idx, e := range events {
		if e.SessionID != s.ID {
			return nil, fmt.Errorf("import event %d names foreign session %q", e.ID, e.SessionID)
		}
		if !ledger.IsKnownEventType(e.EventType) {
			return nil, fmt.Errorf("import event %d has unregistered type %q", e.ID, e.EventType)
		}
		// entry_hash is deliberately not recomputed here: export/import
		// goes through JSON encoding, which may normalise payload bytes
		// (HTML escaping/compaction), so a byte-exact recompute would
		// reject legitimate manifests. The section digest (validated
		// above) pins content; the link + type + signature checks below
		// pin ordering and authority.
		if idx > 0 && e.PrevHash != events[idx-1].EntryHash {
			return nil, fmt.Errorf("import event %d prev_hash %q does not link to the previous imported event", e.ID, e.PrevHash)
		}
		if ledger.CommitEventTypes[e.EventType] && (e.SignerPubkey == "" || e.Signature == "") {
			return nil, fmt.Errorf("import event %d is commit-class %q but unsigned: refusing forged authority", e.ID, e.EventType)
		}
		var idExists int
		_ = tx.QueryRow(`SELECT COUNT(*) FROM ledger_events WHERE id = ?`, e.ID).Scan(&idExists)
		hver := e.HashVersion
		if hver == 0 {
			hver = 1
		}
		sdom := e.SignatureDomain
		if sdom == "" {
			sdom = "chain"
		}
		var insertErr error
		if idExists > 0 {
			insertErr = exec(`INSERT INTO ledger_events (timestamp, session_id, tenant_id, conf_label, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex, hash_version, signature_domain) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				e.Timestamp, s.ID, e.TenantID, e.ConfLabel, e.EventType, string(e.Payload), e.PrevHash, e.EntryHash, e.SignerPubkey, e.Signature, hver, sdom)
		} else {
			insertErr = exec(`INSERT INTO ledger_events (id, timestamp, session_id, tenant_id, conf_label, event_type, payload, prev_hash, entry_hash, signer_pubkey_hex, signature_hex, hash_version, signature_domain) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				e.ID, e.Timestamp, s.ID, e.TenantID, e.ConfLabel, e.EventType, string(e.Payload), e.PrevHash, e.EntryHash, e.SignerPubkey, e.Signature, hver, sdom)
		}
		if insertErr != nil {
			return nil, fmt.Errorf("import event %d: %w", e.ID, insertErr)
		}
	}
	if len(man.CampaignState) > 0 && string(man.CampaignState) != "null" {
		if err := exec(`INSERT INTO campaign_states (session_id, state_json, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
			s.ID, string(man.CampaignState)); err != nil {
			return nil, fmt.Errorf("import campaign state: %w", err)
		}
	}
	for _, v := range man.Versions {
		if err := exec(`INSERT INTO campaign_versions (session_id, state_json, trigger, created_at) VALUES (?, ?, ?, ?)`,
			s.ID, string(v.StateJSON), v.Trigger, v.CreatedAt); err != nil {
			return nil, fmt.Errorf("import campaign version: %w", err)
		}
	}
	for _, c := range man.Claims {
		if err := exec(`INSERT INTO claim_records (id, session_id, turn_kind, statement, alpha, beta, gamma, verdict, evidence_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, s.ID, c.TurnKind, c.Statement, c.Alpha, c.Beta, c.Gamma, c.Verdict, c.Evidence, c.CreatedAt); err != nil {
			return nil, fmt.Errorf("import claim %s: %w", c.ID, err)
		}
	}
	if man.CanvasLayout != "" {
		if err := exec(`INSERT INTO canvas_layouts (session_id, layout_json, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
			s.ID, man.CanvasLayout); err != nil {
			return nil, fmt.Errorf("import canvas: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("import commit: %w", err)
	}

	pyReplayed := 0
	if len(man.PythonLedger) > 0 {
		n, err := replayPythonRecords(man.Session.TenantID, man.PythonLedger)
		if err != nil {
			return nil, fmt.Errorf("replay python ledger: %w", err)
		}
		pyReplayed = n
	}

	// Single activating record (S5 contract).
	if err := m.ledger.AppendEvent(s.ID, "session_imported", map[string]any{
		"manifest_version": man.Version, "tenant_id": s.TenantID,
	}); err != nil {
		return nil, fmt.Errorf("import activation record failed: %w", err)
	}
	return &ImportResult{SessionID: s.ID, Manifest: man, PythonReplayed: pyReplayed}, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// nullDatetime stores empty datetime strings as NULL (SubmitJob parity).
func nullDatetime(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// DeleteBlockers reports what prevents deletion. Non-terminal jobs/loops
// and outstanding spend reservations refuse first (fail-closed cascade:
// deleting a lab's live work or its money trail is never silent).
func (m *Manager) DeleteBlockers(id string) ([]string, error) {
	var blockers []string
	var n int
	jobQ := `SELECT COUNT(*) FROM jobs WHERE session_id = ? AND status NOT IN (` + Placeholders(len(TerminalJobStatuses)) + `)`
	loopQ := `SELECT COUNT(*) FROM loops WHERE session_id = ? AND status NOT IN (` + Placeholders(len(TerminalLoopStatuses)) + `)`
	type blockerCheck struct {
		query string
		args  []any
		label string
	}
	jobArgs := make([]any, 0, len(TerminalJobStatuses)+1)
	jobArgs = append(jobArgs, id)
	for _, s := range TerminalJobStatuses {
		jobArgs = append(jobArgs, s)
	}
	loopArgs := make([]any, 0, len(TerminalLoopStatuses)+1)
	loopArgs = append(loopArgs, id)
	for _, s := range TerminalLoopStatuses {
		loopArgs = append(loopArgs, s)
	}
	for _, q := range []blockerCheck{
		{jobQ, jobArgs, "non-terminal jobs"},
		{loopQ, loopArgs, "live loops"},
		{`SELECT COUNT(*) FROM jobs WHERE session_id = ? AND reserved_amount_micro_usd > 0`, []any{id}, "outstanding spend reservations"},
		{`SELECT COUNT(*) FROM loops WHERE session_id = ? AND reserved_amount_micro_usd > 0`, []any{id}, "outstanding loop spend reservations"},
	} {
		if err := m.ledger.DB().QueryRow(q.query, q.args...).Scan(&n); err != nil {
			return nil, err
		}
		if n > 0 {
			blockers = append(blockers, q.label)
		}
	}
	return blockers, nil
}

// DeleteSession removes a session and ALL its rows (I4: previously only
// chat + campaign_states + sessions, orphaning the rest). Refuses when
// blockers exist. Implemented in session.go (kept beside the original to
// preserve the deletion-receipt contract); DeleteBlockers lives here.
