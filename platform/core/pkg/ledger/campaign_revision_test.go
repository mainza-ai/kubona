package ledger_test

import "testing"

func TestCampaignRevisionRejectsStaleWrite(t *testing.T) {
	l := mustOpenTempLoop(t)
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, created_at, updated_at, status) VALUES ('s', 's', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'active')`); err != nil {
		t.Fatal(err)
	}
	if err := l.SaveCampaignStateAtRevision("s", []byte(`{"v":1}`), "user-edit", 0); err != nil {
		t.Fatal(err)
	}
	_, rev, err := l.GetCampaignStateWithRevision("s")
	if err != nil || rev != 1 {
		t.Fatalf("revision %d err %v", rev, err)
	}
	if err := l.SaveCampaignStateAtRevision("s", []byte(`{"v":2}`), "user-edit", 0); err == nil {
		t.Fatal("stale revision must not overwrite")
	}
	raw, rev, err := l.GetCampaignStateWithRevision("s")
	if err != nil || rev != 1 || string(raw) != `{"v":1}` {
		t.Fatalf("state %s revision %d err %v", raw, rev, err)
	}
	if err := l.SaveCampaignStateAtRevision("s", []byte(`{"v":2}`), "user-edit", 1); err != nil {
		t.Fatal(err)
	}
	_, rev, err = l.GetCampaignStateWithRevision("s")
	if err != nil || rev != 2 {
		t.Fatalf("revision %d err %v", rev, err)
	}
}
