package ledger

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFreshLedgerUsesIncrementalAutoVacuum(t *testing.T) {
	l, err := OpenLedger(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	var mode int
	if err := l.DB().QueryRow(`PRAGMA auto_vacuum`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != 2 {
		t.Fatalf("auto_vacuum=%d want 2 (INCREMENTAL)", mode)
	}
	if got := l.DB().Stats().MaxOpenConnections; got != sqliteMaxOpenConns() {
		t.Fatalf("MaxOpenConnections=%d want %d", got, sqliteMaxOpenConns())
	}
}

func TestSessionListUsesOrderingIndex(t *testing.T) {
	l, err := OpenLedger(filepath.Join(t.TempDir(), "idx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	rows, err := l.DB().Query(`EXPLAIN QUERY PLAN SELECT id, title FROM sessions ORDER BY updated_at DESC LIMIT 50`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail)
		plan.WriteString("\n")
	}
	if !strings.Contains(plan.String(), "idx_sessions_updated") {
		t.Fatalf("session list plan does not use idx_sessions_updated:\n%s", plan.String())
	}
}

func TestRunRetentionDeletesOnlyExpiredRows(t *testing.T) {
	l, err := OpenLedger(filepath.Join(t.TempDir(), "ret.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	now := time.Now().UTC()
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, tenant_id, created_at, updated_at, status) VALUES ('s1','t','default',?,?, 'active')`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := l.DB().Exec(`INSERT INTO chat_messages (id, session_id, role, content, created_at) VALUES ('old','s1','user','old',?)`, now.AddDate(0, 0, -100)); err != nil {
		t.Fatal(err)
	}
	if _, err := l.DB().Exec(`INSERT INTO chat_messages (id, session_id, role, content, created_at) VALUES ('new','s1','user','new',?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := l.DB().Exec(`CREATE TABLE hindsight_memories (id TEXT PRIMARY KEY, content TEXT, retracted_at DATETIME, retract_reason TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.DB().Exec(`INSERT INTO hindsight_memories (id, content, retracted_at) VALUES ('old-t','[retracted]',?)`, now.AddDate(0, 0, -40)); err != nil {
		t.Fatal(err)
	}
	if _, err := l.DB().Exec(`INSERT INTO hindsight_memories (id, content, retracted_at) VALUES ('new-t','[retracted]',?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := l.DB().Exec(`INSERT INTO hindsight_memories (id, content, retracted_at) VALUES ('active','live',NULL)`); err != nil {
		t.Fatal(err)
	}
	beforeEvents := 0
	_ = l.DB().QueryRow(`SELECT COUNT(*) FROM ledger_events`).Scan(&beforeEvents)

	res, err := l.RunRetention(DefaultRetentionConfig())
	if err != nil {
		t.Fatal(err)
	}
	if res.ChatMessages != 1 || res.Tombstones != 1 {
		t.Fatalf("retention=%+v want chat=1 tombstone=1", res)
	}
	for _, tc := range []struct {
		query string
		want  int
	}{
		{`SELECT COUNT(*) FROM chat_messages WHERE id='old'`, 0},
		{`SELECT COUNT(*) FROM chat_messages WHERE id='new'`, 1},
		{`SELECT COUNT(*) FROM hindsight_memories WHERE id='old-t'`, 0},
		{`SELECT COUNT(*) FROM hindsight_memories WHERE id='new-t'`, 1},
		{`SELECT COUNT(*) FROM hindsight_memories WHERE id='active'`, 1},
	} {
		var got int
		if err := l.DB().QueryRow(tc.query).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("%s = %d want %d", tc.query, got, tc.want)
		}
	}
	afterEvents := 0
	_ = l.DB().QueryRow(`SELECT COUNT(*) FROM ledger_events`).Scan(&afterEvents)
	if afterEvents <= beforeEvents {
		t.Fatalf("retention did not seal an event: before=%d after=%d", beforeEvents, afterEvents)
	}
}
