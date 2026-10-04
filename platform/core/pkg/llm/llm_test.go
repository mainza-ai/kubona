package llm_test

import (
	"database/sql"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
	_ "github.com/mattn/go-sqlite3"
)

func TestLLMSettingsPersistenceAndAirGap(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	defer db.Close()

	settings, err := llm.LoadSettings(db)
	if err != nil {
		t.Fatalf("failed to load settings: %v", err)
	}

	if settings.ActiveModel != "Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit" {
		t.Errorf("expected default model Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit, got %s", settings.ActiveModel)
	}

	if !settings.AirGapped {
		t.Errorf("expected air_gapped to default to true for sovereignty")
	}

	// Test Air-gap enforcement
	prov, model, err := settings.GetModelForRole("copilot")
	if err != nil {
		t.Fatalf("failed to get model for copilot: %v", err)
	}
	if prov != llm.ProviderOMLX {
		t.Errorf("expected provider omlx, got %s", prov)
	}
	if model != "Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit" {
		t.Errorf("expected model Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit, got %s", model)
	}

	// Update settings
	snap := settings.GetSnapshot()
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
