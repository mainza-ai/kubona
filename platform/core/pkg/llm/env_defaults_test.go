package llm_test

import (
	"database/sql"
	"encoding/json"
	"strings"
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
	// Env-selection tests exercise the cloud path: opt out of the
	// air-gap default exactly as an operator does in settings.
	snap := s.GetSnapshot()
	snap.AirGapped = false
	if err := s.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
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

// No env: the default active provider (strata, local) has no model, so
// resolution refuses naming the exact variable to set. No silent fallback,
// no invented model.
func TestNoEnvFailsLoudly(t *testing.T) {
	s := llm.DefaultSettings()
	if s.ActiveProvider != llm.ProviderStrata {
		t.Fatalf("no-env default must be strata, got %s", s.ActiveProvider)
	}
	if _, _, err := s.GetModelForRole("copilot"); err == nil {
		t.Fatal("copilot with no model configured must refuse")
	} else if got := err.Error(); !contains(got, "STRATA_MODEL") {
		t.Fatalf("refusal must name STRATA_MODEL, got: %v", err)
	}
}

// Strata resolves end-to-end from env (default provider + model + base URL
// override); NVIDIA stays explicitly selectable (parked, not removed).
func TestStrataDefaults(t *testing.T) {
	t.Setenv("STRATA_MODEL", "iq3_s")
	s := llm.DefaultSettings()
	if s.ActiveProvider != llm.ProviderStrata {
		t.Fatalf("no-env explicit provider must default to strata, got %s", s.ActiveProvider)
	}
	prov, model, err := s.GetModelForRole("copilot")
	if err != nil {
		t.Fatalf("copilot must resolve on strata env: %v", err)
	}
	if prov != llm.ProviderStrata || model != "iq3_s" {
		t.Fatalf("copilot resolved to %s/%s, want strata/iq3_s", prov, model)
	}
	pc := s.Providers[llm.ProviderStrata]
	if pc.BaseURL != "http://127.0.0.1:47831/v1" {
		t.Fatalf("strata default base URL wrong, got %q", pc.BaseURL)
	}
	if !pc.IsLocal || !pc.Enabled {
		t.Fatal("strata must be local and enabled by default")
	}
}

func TestStrataBaseURLOverride(t *testing.T) {
	t.Setenv("STRATA_MODEL", "iq3_s")
	t.Setenv("STRATA_BASE_URL", "http://127.0.0.1:9999/v1")
	s := llm.DefaultSettings()
	if got := s.Providers[llm.ProviderStrata].BaseURL; got != "http://127.0.0.1:9999/v1" {
		t.Fatalf("STRATA_BASE_URL must win, got %q", got)
	}
}

func TestNvidiaStillExplicitlySelectable(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "test-key")
	t.Setenv("NVIDIA_MODEL", "nvidia/parked-model")
	t.Setenv("KUBONA_LLM_PROVIDER", "nvidia")
	s := llm.DefaultSettings()
	// Same cloud-path opt-out as above: selection, not dispatch, is
	// under test, and selection of a cloud provider needs cloud.
	snap := s.GetSnapshot()
	snap.AirGapped = false
	if err := s.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	prov, model, err := s.GetModelForRole("copilot")
	if err != nil {
		t.Fatalf("explicit nvidia selection must still work (parked, not removed): %v", err)
	}
	if prov != llm.ProviderNVIDIA || model != "nvidia/parked-model" {
		t.Fatalf("got %s/%s, want nvidia parked model", prov, model)
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
	// Role-override tests resolve cloud models: opt out of the air-gap
	// default exactly as an operator does in settings.
	snap := s.GetSnapshot()
	snap.AirGapped = false
	if err := s.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
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

// Backfill: a persisted blob predating Strata gains an enabled local
// entry with the env model (or empty model, which refuses loudly at
// resolve time — never silently). Stored values are untouched.
func TestBackfillsStrataIntoOldBlobs(t *testing.T) {
	t.Setenv("STRATA_MODEL", "iq3_s")
	db := openMem(t)
	s, err := llm.LoadSettings(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	snap := s.GetSnapshot()
	delete(snap.Providers, llm.ProviderStrata)
	blob, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO llm_settings (key, value_json, updated_at) VALUES ('active_settings', ?, CURRENT_TIMESTAMP) ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json, updated_at=CURRENT_TIMESTAMP`, string(blob)); err != nil {
		t.Fatalf("seed old blob: %v", err)
	}
	re, err := llm.LoadSettings(db)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, ok := re.GetSnapshot().Providers[llm.ProviderStrata]; ok {
		t.Fatal("setup broken: blob must lack strata before backfill")
	}
	if !re.EnsureDefaultProviders() {
		t.Fatal("backfill must report adding the missing strata entry")
	}
	got := re.GetSnapshot().Providers[llm.ProviderStrata]
	if !got.Enabled {
		t.Fatal("backfilled strata must be enabled (local needs no key)")
	}
	if !got.IsLocal {
		t.Fatal("backfilled strata must be marked local (production-safe)")
	}
	if got.DefaultModel != "iq3_s" {
		t.Fatalf("backfill must pick up STRATA_MODEL, got %q", got.DefaultModel)
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

// G1 (Phase 0): keys are server-side only. The public snapshot redacts
// values; the internal snapshot keeps them (Update merge still sees stored
// keys), and redaction must not mutate the live settings.
func TestGetPublicSnapshotRedactsKeys(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "test-key-must-never-leave")
	db := openMem(t)
	s, err := llm.LoadSettings(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	internal := s.GetSnapshot().Providers[llm.ProviderNVIDIA]
	if internal.APIKey != "test-key-must-never-leave" {
		t.Fatalf("setup broken: internal snapshot must carry the key, got %q", internal.APIKey)
	}
	pub := s.GetPublicSnapshot().Providers[llm.ProviderNVIDIA]
	if pub.APIKey != "" {
		t.Fatalf("public snapshot must redact the key, got %q", pub.APIKey)
	}
	if !pub.Enabled {
		t.Fatal("redaction must preserve the Enabled signal (existence, not value)")
	}
	after := s.GetSnapshot().Providers[llm.ProviderNVIDIA]
	if after.APIKey != "test-key-must-never-leave" {
		t.Fatal("redaction must not mutate live settings (map aliasing would be a credential-loss bug)")
	}
	raw, err := json.Marshal(s.GetPublicSnapshot())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "test-key-must-never-leave") {
		t.Fatal("serialized public snapshot must not contain the key material")
	}
}

// G13: partial provider writes must refuse, never silently zero fields.
// Only APIKey merges (empty keeps stored); everything else requires the
// full object (GET-modify-PUT, the console's exact flow).
func TestUpdateRefusesPartialProviders(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "test-key")
	t.Setenv("NVIDIA_MODEL", "nvidia/fleet-model")
	db := openMem(t)
	s, err := llm.LoadSettings(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	before := s.GetSnapshot().Providers[llm.ProviderNVIDIA]

	// Key-only fragment: refuses, stored config untouched.
	frag := s.GetSnapshot()
	frag.Providers = map[llm.ProviderType]llm.ProviderConfig{
		llm.ProviderNVIDIA: {APIKey: "rotated-key"},
	}
	if err := s.Update(&frag, db); err == nil {
		t.Fatal("partial provider entry must refuse, not clobber")
	}
	after := s.GetSnapshot().Providers[llm.ProviderNVIDIA]
	if after != before {
		t.Fatalf("refused write must not mutate stored config: %+v -> %+v", before, after)
	}

	// Unknown provider type: refuses.
	unk := s.GetSnapshot()
	unk.Providers["nope"] = llm.ProviderConfig{BaseURL: "https://x.example", TimeoutSec: 30}
	if err := s.Update(&unk, db); err == nil {
		t.Fatal("unknown provider type must refuse")
	}

	// Full-object rotation: succeeds, key updates, nothing else moves.
	rot := s.GetSnapshot()
	full := rot.Providers[llm.ProviderNVIDIA]
	full.APIKey = "rotated-key"
	rot.Providers = map[llm.ProviderType]llm.ProviderConfig{llm.ProviderNVIDIA: full}
	if err := s.Update(&rot, db); err != nil {
		t.Fatalf("full-object rotation must succeed, got: %v", err)
	}
	got := s.GetSnapshot().Providers[llm.ProviderNVIDIA]
	if got.APIKey != "rotated-key" || !got.Enabled || got.BaseURL != before.BaseURL {
		t.Fatalf("rotation must swap only the key: %+v", got)
	}

	// Enable-toggle via full object: the console's real flow.
	dis := s.GetSnapshot()
	cur := dis.Providers[llm.ProviderNVIDIA]
	cur.Enabled = false
	dis.Providers = map[llm.ProviderType]llm.ProviderConfig{llm.ProviderNVIDIA: cur}
	if err := s.Update(&dis, db); err != nil {
		t.Fatalf("enable-toggle must succeed, got: %v", err)
	}
	if s.GetSnapshot().Providers[llm.ProviderNVIDIA].Enabled {
		t.Fatal("enable-toggle must take effect")
	}
}
