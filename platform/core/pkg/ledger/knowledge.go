package ledger

// Cumulative lab-knowledge registry (AG-9). Entries are published from real
// outcomes (rejected/failed plans, collapsed claims, signed reports) with the
// provenance that produced them. Publication is idempotent per tenant + kind +
// normalized statement, and confirmations accumulate instead of duplicating.
// Visibility is tenant-scoped; an owner operator may grant another tenant
// read access to one entry.

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/knowledge"
)

const knowledgeSchema = `
CREATE TABLE IF NOT EXISTS lab_knowledge (
	id TEXT PRIMARY KEY,
	tenant_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	title TEXT NOT NULL,
	statement TEXT NOT NULL,
	fingerprint TEXT NOT NULL,
	evidence_json TEXT NOT NULL DEFAULT '{}',
	confirmations INTEGER NOT NULL DEFAULT 1,
	visibility TEXT NOT NULL DEFAULT 'tenant',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_lab_knowledge_fingerprint
	ON lab_knowledge(tenant_id, kind, fingerprint);
CREATE INDEX IF NOT EXISTS idx_lab_knowledge_tenant
	ON lab_knowledge(tenant_id, created_at DESC);
CREATE TABLE IF NOT EXISTS lab_knowledge_grants (
	knowledge_id TEXT NOT NULL,
	owner_tenant TEXT NOT NULL,
	grantee_tenant TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (knowledge_id, grantee_tenant)
);`

const (
	KnowledgeNegativeResult = "negative_result"
	KnowledgeHypothesis     = "hypothesis"
	KnowledgeClaim          = "claim"
	KnowledgeReport         = "report"
)

type KnowledgeEntry struct {
	ID            string `json:"id"`
	TenantID      string `json:"tenant_id"`
	SessionID     string `json:"session_id"`
	Kind          string `json:"kind"`
	Title         string `json:"title"`
	Statement     string `json:"statement"`
	Fingerprint   string `json:"fingerprint"`
	Evidence      string `json:"evidence_json"`
	Confirmations int64  `json:"confirmations"`
	Visibility    string `json:"visibility"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// KnowledgeMatch is one prior entry with its lexical similarity.
type KnowledgeMatch struct {
	KnowledgeEntry
	Similarity float64 `json:"similarity"`
}

var ErrKnowledgeNotFound = errors.New("knowledge entry not found")
var ErrKnowledgeForbidden = errors.New("knowledge entry is owned by another tenant")

func validKnowledgeKind(k string) bool {
	switch k {
	case KnowledgeNegativeResult, KnowledgeHypothesis, KnowledgeClaim, KnowledgeReport:
		return true
	}
	return false
}

// PublishKnowledge upserts one entry. A duplicate (same tenant, kind, and
// normalized statement) increments confirmations rather than creating a second
// row, so repeated independent negatives remain visible as a count.
func (l *Ledger) PublishKnowledge(e *KnowledgeEntry) (*KnowledgeEntry, error) {
	if e == nil || strings.TrimSpace(e.TenantID) == "" || strings.TrimSpace(e.SessionID) == "" ||
		strings.TrimSpace(e.Statement) == "" || !validKnowledgeKind(e.Kind) {
		return nil, fmt.Errorf("knowledge entry needs tenant, session, kind, and statement")
	}
	if e.ID == "" {
		e.ID = fmt.Sprintf("kn_%s", fingerprintID(e.TenantID+"\x00"+e.Kind+"\x00"+e.Statement))
	}
	fp := knowledge.Fingerprint(e.Statement)
	now := nowUTC()
	_, err := l.DB().Exec(`INSERT INTO lab_knowledge
		(id, tenant_id, session_id, kind, title, statement, fingerprint, evidence_json,
		 confirmations, visibility, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, 'tenant', ?, ?)
		ON CONFLICT(tenant_id, kind, fingerprint) DO UPDATE SET
			confirmations = confirmations + 1,
			evidence_json = excluded.evidence_json,
			updated_at = excluded.updated_at`,
		e.ID, e.TenantID, e.SessionID, e.Kind, e.Title, e.Statement, fp, e.Evidence, now, now)
	if err != nil {
		return nil, err
	}
	var id string
	if err := l.DB().QueryRow(`SELECT id FROM lab_knowledge
		WHERE tenant_id = ? AND kind = ? AND fingerprint = ?`,
		e.TenantID, e.Kind, fp).Scan(&id); err != nil {
		return nil, err
	}
	return l.GetKnowledge(e.TenantID, id)
}

// GetKnowledge reads one entry the tenant owns or has been granted.
func (l *Ledger) GetKnowledge(tenantID, id string) (*KnowledgeEntry, error) {
	var e KnowledgeEntry
	err := l.DB().QueryRow(`SELECT id, tenant_id, session_id, kind, title, statement,
		fingerprint, evidence_json, confirmations, visibility, created_at, updated_at
		FROM lab_knowledge WHERE id = ? AND (tenant_id = ? OR id IN (
			SELECT knowledge_id FROM lab_knowledge_grants WHERE grantee_tenant = ?))`,
		id, tenantID, tenantID).Scan(
		&e.ID, &e.TenantID, &e.SessionID, &e.Kind, &e.Title, &e.Statement,
		&e.Fingerprint, &e.Evidence, &e.Confirmations, &e.Visibility, &e.CreatedAt, &e.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrKnowledgeNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// ListKnowledge returns the tenant's own entries plus entries granted to it.
func (l *Ledger) ListKnowledge(tenantID, kind, query string, limit int) ([]KnowledgeEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	sqlText := `SELECT id, tenant_id, session_id, kind, title, statement, fingerprint,
		evidence_json, confirmations, visibility, created_at, updated_at
		FROM lab_knowledge
		WHERE (tenant_id = ? OR id IN (
			SELECT knowledge_id FROM lab_knowledge_grants WHERE grantee_tenant = ?))`
	args := []any{tenantID, tenantID}
	if strings.TrimSpace(kind) != "" {
		sqlText += ` AND kind = ?`
		args = append(args, kind)
	}
	if q := strings.TrimSpace(query); q != "" {
		sqlText += ` AND (statement LIKE ? OR title LIKE ?)`
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	sqlText += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := l.DB().Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []KnowledgeEntry{}
	for rows.Next() {
		var e KnowledgeEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.SessionID, &e.Kind, &e.Title,
			&e.Statement, &e.Fingerprint, &e.Evidence, &e.Confirmations,
			&e.Visibility, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// LookupKnowledge finds prior negative results visible to the tenant whose
// statements overlap the candidate. Similarity is computed in Go over a
// bounded candidate set so the threshold stays explicit and testable.
func (l *Ledger) LookupKnowledge(tenantID, statement string, limit int) ([]KnowledgeMatch, error) {
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	entries, err := l.ListKnowledge(tenantID, KnowledgeNegativeResult, "", 500)
	if err != nil {
		return nil, err
	}
	out := make([]KnowledgeMatch, 0, len(entries))
	for _, e := range entries {
		sim := knowledge.Similarity(statement, e.Statement)
		if sim <= 0 {
			continue
		}
		out = append(out, KnowledgeMatch{KnowledgeEntry: e, Similarity: sim})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Similarity == out[j].Similarity {
			return out[i].CreatedAt > out[j].CreatedAt
		}
		return out[i].Similarity > out[j].Similarity
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ShareKnowledge grants one grantee tenant read access to an entry the owner
// tenant owns. The owner is verified, not inferred.
func (l *Ledger) ShareKnowledge(ownerTenant, id, granteeTenant string) (*KnowledgeEntry, error) {
	ownerTenant = strings.TrimSpace(ownerTenant)
	granteeTenant = strings.TrimSpace(granteeTenant)
	if ownerTenant == "" || granteeTenant == "" {
		return nil, fmt.Errorf("share needs owner and grantee tenants")
	}
	if ownerTenant == granteeTenant {
		return nil, fmt.Errorf("cannot share with the owning tenant")
	}
	var actualOwner string
	err := l.DB().QueryRow(`SELECT tenant_id FROM lab_knowledge WHERE id = ?`, id).Scan(&actualOwner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrKnowledgeNotFound
	}
	if err != nil {
		return nil, err
	}
	if actualOwner != ownerTenant {
		return nil, fmt.Errorf("%w: %s", ErrKnowledgeForbidden, id)
	}
	_, err = l.DB().Exec(`INSERT INTO lab_knowledge_grants (knowledge_id, owner_tenant, grantee_tenant, created_at)
		VALUES (?, ?, ?, ?) ON CONFLICT(knowledge_id, grantee_tenant) DO NOTHING`,
		id, ownerTenant, granteeTenant, nowUTC())
	if err != nil {
		return nil, err
	}
	return l.GetKnowledge(ownerTenant, id)
}

func fingerprintID(statement string) string {
	fp := knowledge.Fingerprint(statement)
	if len(fp) > 16 {
		fp = fp[:16]
	}
	return fp
}
