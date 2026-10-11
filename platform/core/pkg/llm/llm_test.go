package llm_test

import (
	"database/sql"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
	_ "github.com/mattn/go-sqlite3"
)

func TestLLMSettingsPersistenceAndAirGap(t *testing.T) {
	t.Setenv("OPENCODE_API_KEY", "test-key")
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	defer db.Close()

	settings, err := llm.LoadSettings(db)
	if err != nil {
		t.Fatalf("failed to load settings: %v", err)
	}

	// Development default (operator-directed 2026-10-11): cloud model for
	// every role; oMLX stays enabled as the air-gapped fallback.
	if settings.ActiveModel != "deepseek-v4.1-flash" {
		t.Errorf("expected default model deepseek-v4.1-flash, got %s", settings.ActiveModel)
	}
	if settings.ActiveProvider != llm.ProviderOpenCode {
		t.Errorf("expected default provider opencode, got %s", settings.ActiveProvider)
	}

	prov, model, err := settings.GetModelForRole("copilot")
	if err != nil {
		t.Fatalf("failed to get model for copilot: %v", err)
	}
	if prov != llm.ProviderOpenCode {
		t.Errorf("expected provider opencode, got %s", prov)
	}
	if model != "deepseek-v4.1-flash" {
		t.Errorf("expected model deepseek-v4.1-flash, got %s", model)
	}

	// Air-gap enforcement still holds when enabled: non-local blocked.
	snap := settings.GetSnapshot()
	snap.AirGapped = true
	if err := settings.Update(&snap, db); err != nil {
		t.Fatalf("failed to update settings: %v", err)
	}
	if _, _, err := settings.GetModelForRole("copilot"); err == nil {
		t.Error("expected air-gap to block non-local copilot provider")
	}

	// Update settings
	snap = settings.GetSnapshot()
	snap.AirGapped = false
	snap.Temperature = 0.35
	if err := settings.Update(&snap, db); err != nil {
		t.Fatalf("failed to update settings: %v", err)
	}

	loaded, err := llm.LoadSettings(db)
	if err != nil {
		t.Fatalf("failed to reload settings: %v", err)
	}
	if loaded.Temperature != 0.35 {
		t.Errorf("expected persisted temperature 0.35, got %f", loaded.Temperature)
	}
}

func TestUpdatePreservesAPIKey(t *testing.T) {
	t.Setenv("OPENCODE_API_KEY", "env-key")
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	defer db.Close()

	settings, err := llm.LoadSettings(db)
	if err != nil {
		t.Fatalf("failed to load settings: %v", err)
	}
	// Simulate a UI save that carries no secret: stored/env key survives.
	snap := settings.GetSnapshot()
	for pt, pc := range snap.Providers {
		pc.APIKey = ""
		snap.Providers[pt] = pc
	}
	if err := settings.Update(&snap, db); err != nil {
		t.Fatalf("failed to update settings: %v", err)
	}
	after := settings.GetSnapshot()
	if after.Providers[llm.ProviderOpenCode].APIKey != "env-key" {
		t.Errorf("empty incoming APIKey wiped the stored secret")
	}
}
