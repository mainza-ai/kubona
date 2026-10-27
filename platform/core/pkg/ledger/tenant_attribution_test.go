package ledger

import "testing"

func TestTenantScopedEventKeepsLabAttribution(t *testing.T) {
	l, cleanup := openVerifyLedger(t)
	defer cleanup()
	if _, err := l.DB().Exec(`INSERT INTO tenants (id, display_name, spend_ceiling_micro_usd) VALUES ('lab-a', 'Lab A', 10000000)`); err != nil {
		t.Fatal(err)
	}
	if err := l.AppendEvent("lab-a", "tenant_ceiling_changed", map[string]any{"tenant_id": "lab-a"}); err != nil {
		t.Fatal(err)
	}
	var tenant, label string
	if err := l.DB().QueryRow(`SELECT tenant_id, conf_label FROM ledger_events WHERE session_id = 'lab-a' ORDER BY id DESC LIMIT 1`).Scan(&tenant, &label); err != nil {
		t.Fatal(err)
	}
	if tenant != "lab-a" || label != "tenant/lab-a" {
		t.Fatalf("tenant event attributed to %q/%q, want lab-a/tenant/lab-a", tenant, label)
	}
}
