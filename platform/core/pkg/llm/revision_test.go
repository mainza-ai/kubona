package llm_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

func TestSettingsRevisionOperatorWritesAndTelemetry(t *testing.T) {
	db := openMem(t)
	if _, err := db.Exec(`CREATE TABLE llm_settings (
		key TEXT PRIMARY KEY,
		value_json TEXT NOT NULL,
		updated_at TEXT
	)`); err != nil {
		t.Fatal(err)
	}
	s := llm.DefaultSettings()
	snap := s.GetSnapshot()
	if err := s.Update(&snap, db); err != nil {
		t.Fatalf("initial write: %v", err)
	}
	if got := s.GetPublicSnapshot().Revision; got != 1 {
		t.Fatalf("first operator write revision = %d, want 1", got)
	}
	current := s.GetPublicSnapshot().AirGapped
	if _, rev, err := s.SetAirGapped(current, db); err != nil || rev != 1 {
		t.Fatalf("no-op air-gap revision = %d err=%v, want 1", rev, err)
	}
	if _, rev, err := s.SetAirGapped(!current, db); err != nil || rev != 2 {
		t.Fatalf("air-gap flip revision = %d err=%v, want 2", rev, err)
	}
	if s.GetPublicSnapshot().AirGapped == current {
		t.Fatal("air-gap flip did not change the flag")
	}

	stale := s.GetSnapshot()
	stale.AirGapped = current
	stale.Temperature = 0.15
	err := s.UpdateAtRevision(&stale, 1, db)
	var conflict *llm.RevisionConflictError
	if !errors.As(err, &conflict) || conflict.Expected != 1 || conflict.Current != 2 {
		t.Fatalf("stale write = %v, want conflict expected 1 current 2", err)
	}
	pub := s.GetPublicSnapshot()
	if pub.AirGapped == current || pub.Revision != 2 || pub.Temperature == 0.15 {
		t.Fatalf("stale write mutated settings: air_gapped=%v revision=%d temperature=%v", pub.AirGapped, pub.Revision, pub.Temperature)
	}

	var provider llm.ProviderType
	for name := range pub.Providers {
		provider = name
		break
	}
	s.RecordProviderCheck(provider, "valid", "", "2026-11-12T00:00:00Z", db)
	s.RecordSuccess(provider)
	if s.GetPublicSnapshot().Revision != 2 {
		t.Fatalf("health telemetry revision = %d, want 2", s.GetPublicSnapshot().Revision)
	}
}

func TestSettingsPersistFailureRollsBack(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE llm_settings (
		key TEXT PRIMARY KEY,
		value_json TEXT NOT NULL,
		updated_at TEXT
	)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s := llm.DefaultSettings()
	before := s.GetPublicSnapshot()
	snap := s.GetSnapshot()
	snap.Temperature = before.Temperature + 0.2
	err = s.Update(&snap, db)
	var persist *llm.PersistError
	if !errors.As(err, &persist) {
		t.Fatalf("closed database write = %v, want PersistError", err)
	}
	after := s.GetPublicSnapshot()
	if after.Temperature != before.Temperature || after.Revision != before.Revision {
		t.Fatalf("failed persist left memory at temperature=%v revision=%d", after.Temperature, after.Revision)
	}
}
