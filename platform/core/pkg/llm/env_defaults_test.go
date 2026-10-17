package llm_test

import (
	"database/sql"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
	_ "github.com/mattn/go-sqlite3"
)

func openMem(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// M1: model IDs come from configuration, never code. Fresh boots resolve
// entirely from env; nothing is baked in.
func TestEnvDrivenDefaults(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "test-key")
	t.Setenv("NVIDIA_MODEL", "nvidia/test-model-1")
	t.Setenv("KUBONA_LLM_PROVIDER", "nvidia")
	s := llm.DefaultSettings()
	if s.ActiveProvider != llm.ProviderNVIDIA {
		t.Fatalf("env provider must win, got %s", s.ActiveProvider)
	}
	if s.ActiveModel != "nvidia/test-model-1" {
		t.Fatalf("env model must win, got %q", s.ActiveModel)
	}
	prov, model, err := s.GetModelForRole("copilot")
	if err != nil {
		t.Fatalf("copilot must resolve: %v", err)
	}
	if prov != llm.ProviderNVIDIA || model != "nvidia/test-model-1" {
		t.Fatalf("copilot resolved to %s/%s, want nvidia env model", prov, model)
	}
}

// No env: the default active provider (opencode) is keyless → disabled,
// so resolution refuses naming the provider and the fix. No silent
// fallback, no invented model.
func TestNoEnvFailsLoudly(t *testing.T) {
	s := llm.DefaultSettings()
	if _, _, err := s.GetModelForRole("copilot"); err == nil {
		t.Fatal("copilot with no key and no model configured must refuse")
	} else if got := err.Error(); !contains(got, "opencode") || !contains(got, "disabled") {
		t.Fatalf("refusal must name the provider and its state, got: %v", err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}

// Per-role overrides win over the active defaults.
func TestPerRoleEnvOverride(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "test-key")
	t.Setenv("NVIDIA_MODEL", "nvidia/fleet-model")
	t.Setenv("KUBONA_LLM_PROVIDER", "nvidia")
	t.Setenv("KUBONA_LLM_COPILOT_MODEL", "nvidia/copilot-special")
	s := llm.DefaultSettings()
	_, model, err := s.GetModelForRole("copilot")
	if err != nil {
		t.Fatalf("copilot must resolve: %v", err)
	}
	if model != "nvidia/copilot-special" {
		t.Fatalf("per-role model must win, got %q", model)
	}
	_, model, err = s.GetModelForRole("ansatz-scout")
	if err != nil {
		t.Fatalf("scout must resolve: %v", err)
	}
	if model != "nvidia/fleet-model" {
		t.Fatalf("unoverridden role must use fleet model, got %q", model)
	}
}

// A role pointing at a disabled provider refuses — never silently serves
// the active provider's model.
func TestDisabledRoleProviderRefuses(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "test-key")
	t.Setenv("NVIDIA_MODEL", "nvidia/fleet-model")
	t.Setenv("KUBONA_LLM_PROVIDER", "nvidia")
	db := openMem(t)
	s, err := llm.LoadSettings(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	snap := s.GetSnapshot()
	// anthropic has no key in this test → disabled.
	snap.RoleAssignments["copilot"] = llm.RoleModelAssignment{Role: "copilot", Provider: llm.ProviderAnthropic, Model: "claude-test"}
	if err := s.Update(&snap, db); err != nil {
		t.Fatalf("storing a disabled-provider assignment must be allowed (dormant), got: %v", err)
	}
	if _, _, err := s.GetModelForRole("copilot"); err == nil {
		t.Fatal("resolving through a disabled provider must refuse, not fall back silently")
	}
}

// Update refuses unknown providers and empty models on enabled providers.
func TestUpdateRefusesBadAssignments(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "test-key")
	t.Setenv("NVIDIA_MODEL", "nvidia/fleet-model")
	db := openMem(t)
	s, err := llm.LoadSettings(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	snap := s.GetSnapshot()
	snap.RoleAssignments["ghost"] = llm.RoleModelAssignment{Role: "ghost", Provider: "nope", Model: "x"}
	if err := s.Update(&snap, db); err == nil {
		t.Fatal("unknown provider assignment must refuse")
	}
	snap = s.GetSnapshot()
	snap.RoleAssignments["copilot"] = llm.RoleModelAssignment{Role: "copilot", Provider: llm.ProviderNVIDIA, Model: ""}
	if err := s.Update(&snap, db); err == nil {
		t.Fatal("empty model on enabled provider must refuse")
	}
}

// Schema version: fresh boots stamp current; pre-version blobs adopt the
// stamp without touching values.
func TestSchemaVersionMigration(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "test-key")
	t.Setenv("NVIDIA_MODEL", "nvidia/fleet-model")
	t.Setenv("KUBONA_LLM_PROVIDER", "nvidia")
	db := openMem(t)
	s, err := llm.LoadSettings(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.GetSnapshot().SchemaVersion != llm.CurrentSettingsVersion {
		t.Fatalf("fresh boot must stamp schema v%d", llm.CurrentSettingsVersion)
	}
	// Simulate a pre-version blob: wipe the version, keep the values.
	snap := s.GetSnapshot()
	snap.SchemaVersion = 0
	if err := s.Update(&snap, db); err != nil {
		t.Fatalf("stamping zero must succeed: %v", err)
	}
	re, err := llm.LoadSettings(db)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got := re.GetSnapshot()
	if got.SchemaVersion != llm.CurrentSettingsVersion {
		t.Fatalf("pre-version blob must adopt current schema, got %d", got.SchemaVersion)
	}
	if got.ActiveModel != "nvidia/fleet-model" {
		t.Fatalf("migration must not rewrite values, got %q", got.ActiveModel)
	}
}

// RecordSuccess stamps last-success; snapshot carries the kill-switch.
func TestRecordSuccessAndSnapshot(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "test-key")
	t.Setenv("NVIDIA_MODEL", "nvidia/fleet-model")
	db := openMem(t)
	s, err := llm.LoadSettings(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	snap := s.GetSnapshot()
	if snap.Providers[llm.ProviderNVIDIA].LastSuccessAt != "" {
		t.Fatal("fresh provider must show no success yet")
	}
	s.RecordSuccess(llm.ProviderNVIDIA)
	if got := s.GetSnapshot().Providers[llm.ProviderNVIDIA].LastSuccessAt; got == "" {
		t.Fatal("RecordSuccess must stamp last_success_at")
	}
	if s.GetSnapshot().DisableDSMLFallback {
		t.Fatal("kill-switch defaults off")
	}
	snap = s.GetSnapshot()
	snap.DisableDSMLFallback = true
	if err := s.Update(&snap, db); err != nil {
		t.Fatalf("kill-switch must be settable: %v", err)
	}
	re, err := llm.LoadSettings(db)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !re.GetSnapshot().DisableDSMLFallback {
		t.Fatal("kill-switch must persist and round-trip")
	}
}
