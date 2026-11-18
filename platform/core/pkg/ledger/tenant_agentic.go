package ledger

// Tenant-level default for the session agentic master switch (AG-24). A session
// policy overrides it; when no session policy exists, the session inherits this
// default. The default is operator-owned (see the API), not a lab-admin action.

import "database/sql"

const tenantAgenticSchema = `
CREATE TABLE IF NOT EXISTS tenant_agentic_defaults (
	tenant_id TEXT PRIMARY KEY,
	enabled INTEGER NOT NULL,
	updated_at TEXT NOT NULL
);`

func (l *Ledger) GetTenantAgenticDefault(tenantID string) (*bool, error) {
	var enabled int
	err := l.DB().QueryRow(`SELECT enabled FROM tenant_agentic_defaults WHERE tenant_id = ?`, tenantID).Scan(&enabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	value := enabled != 0
	return &value, nil
}

func (l *Ledger) SetTenantAgenticDefault(tenantID string, enabled bool) error {
	value := 0
	if enabled {
		value = 1
	}
	_, err := l.DB().Exec(`INSERT INTO tenant_agentic_defaults (tenant_id, enabled, updated_at)
		VALUES (?, ?, ?) ON CONFLICT(tenant_id) DO UPDATE SET enabled = excluded.enabled,
		updated_at = excluded.updated_at`, tenantID, value, nowUTC())
	return err
}
