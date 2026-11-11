package llm_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

func TestKnownRolesIncludesAutoresearchAndMetasearch(t *testing.T) {
	for _, want := range []string{
		"copilot", "codesign",
		"ansatz-scout", "disconfirmation-officer", "device-auditor", "epistemic-scribe",
		"coder", "critic", "supervisor",
	} {
		if !slices.Contains(llm.KnownRoles, want) {
			t.Fatalf("KnownRoles missing %q: got %v", want, llm.KnownRoles)
		}
	}
	s := llm.DefaultSettings()
	for _, role := range []string{"coder", "critic", "supervisor"} {
		if _, ok := s.RoleAssignments[role]; !ok {
			t.Fatalf("DefaultSettings missing role assignment %q", role)
		}
	}
	cat := llm.KnownRoleCatalog()
	if len(cat) != len(llm.KnownRoles) {
		t.Fatalf("KnownRoleCatalog count %d != KnownRoles %d", len(cat), len(llm.KnownRoles))
	}
}

func TestPublicSnapshotCredentialTruthfulnessAndClearKey(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "nv-secret-1")
	t.Setenv("NVIDIA_MODEL", "nvidia/model-a")
	t.Setenv("KUBONA_SECRET_KEY", strings.Repeat("12", 32))

	db := openMem(t)
	s, err := llm.LoadSettings(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	pub := s.GetPublicSnapshot()
	nv := pub.Providers[llm.ProviderNVIDIA]
	if !nv.HasKey || nv.SecretSource != llm.SecretSourceEnv {
		t.Fatalf("expected env secret posture, got has_key=%v source=%q", nv.HasKey, nv.SecretSource)
	}

	// Disabling provider must preserve HasKey truthfulness on public snapshot.
	upd := s.GetPublicSnapshot()
	updNV := upd.Providers[llm.ProviderNVIDIA]
	updNV.Enabled = false
	upd.Providers[llm.ProviderNVIDIA] = updNV
	if err := s.Update(&upd, db); err != nil {
		t.Fatalf("disable update: %v", err)
	}
	pub2 := s.GetPublicSnapshot()
	if !pub2.Providers[llm.ProviderNVIDIA].HasKey {
		t.Fatal("disabled provider still holds server-side key: has_key must remain true")
	}

	// Entering UI key switches source to sealed_db when KUBONA_SECRET_KEY is configured.
	rot := s.GetPublicSnapshot()
	rotNV := rot.Providers[llm.ProviderNVIDIA]
	rotNV.Enabled = true
	rotNV.APIKey = "nv-ui-rotated"
	rot.Providers[llm.ProviderNVIDIA] = rotNV
	if err := s.Update(&rot, db); err != nil {
		t.Fatalf("rotate update: %v", err)
	}
	pub3 := s.GetPublicSnapshot()
	if !pub3.Providers[llm.ProviderNVIDIA].HasKey || pub3.Providers[llm.ProviderNVIDIA].SecretSource != llm.SecretSourceSealedDB {
		t.Fatalf("expected sealed_db after UI write, got has_key=%v source=%q",
			pub3.Providers[llm.ProviderNVIDIA].HasKey, pub3.Providers[llm.ProviderNVIDIA].SecretSource)
	}

	// Reload from DB confirms sealed_db provenance and unsealed plaintext recovery.
	reloaded, err := llm.LoadSettings(db)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	rePub := reloaded.GetPublicSnapshot()
	if !rePub.Providers[llm.ProviderNVIDIA].HasKey || rePub.Providers[llm.ProviderNVIDIA].SecretSource != llm.SecretSourceSealedDB {
		t.Fatalf("reloaded public snapshot mismatch: %+v", rePub.Providers[llm.ProviderNVIDIA])
	}

	// Explicit clear_key removes key cleanly instead of restoring env or stored key.
	clr := reloaded.GetPublicSnapshot()
	clrNV := clr.Providers[llm.ProviderNVIDIA]
	clrNV.ClearKey = true
	clr.Providers[llm.ProviderNVIDIA] = clrNV
	if err := reloaded.Update(&clr, db); err != nil {
		t.Fatalf("clear update: %v", err)
	}
	afterClr := reloaded.GetPublicSnapshot().Providers[llm.ProviderNVIDIA]
	if afterClr.HasKey || afterClr.SecretSource != llm.SecretSourceNone || afterClr.KeyStatus != llm.KeyStatusMissing || afterClr.Enabled {
		t.Fatalf("clear_key must remove key and disable cloud provider, got %+v", afterClr)
	}
}

func TestPlaintextSecretSourceWithoutSecretKey(t *testing.T) {
	t.Setenv("KUBONA_SECRET_KEY", "")
	t.Setenv("KUBONA_ENV", "development")
	db := openMem(t)
	s, err := llm.LoadSettings(db)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	upd := s.GetPublicSnapshot()
	oa := upd.Providers[llm.ProviderOpenAI]
	oa.Enabled = true
	oa.APIKey = "sk-plaintext-dev"
	oa.DefaultModel = "gpt-5-mini"
	upd.Providers[llm.ProviderOpenAI] = oa
	if err := s.Update(&upd, db); err != nil {
		t.Fatalf("update: %v", err)
	}
	pub := s.GetPublicSnapshot().Providers[llm.ProviderOpenAI]
	if !pub.HasKey || pub.SecretSource != llm.SecretSourcePlaintextDB {
		t.Fatalf("expected plaintext_db warning posture when KUBONA_SECRET_KEY unset, got %+v", pub)
	}
}

func TestValidateEndpointURLAndLocalClassifier(t *testing.T) {
	localCases := []string{
		"http://127.0.0.1:8787/v1",
		"http://localhost:11434/v1",
		"http://[::1]:1234/v1",
		"http://[::ffff:127.0.0.1]:8080/v1",
		"http://10.0.0.12:8080/v1",
		"http://172.16.4.2:8000/v1",
		"http://192.168.1.50:11434/v1",
		"http://100.64.0.9:8787/v1",
		"http://host.docker.internal:11434/v1",
		"http://mac-studio.local:8787/v1",
	}
	for _, raw := range localCases {
		cleaned, isLocal, err := llm.ValidateEndpointURL(llm.ProviderOMLX, raw)
		if err != nil || !isLocal || cleaned == "" {
			t.Fatalf("expected valid local endpoint for %q, got cleaned=%q local=%v err=%v", raw, cleaned, isLocal, err)
		}
	}

	remoteOK := "https://api.openai.com/v1/"
	cleaned, isLocal, err := llm.ValidateEndpointURL(llm.ProviderOpenAI, remoteOK)
	if err != nil || isLocal || cleaned != "https://api.openai.com/v1" {
		t.Fatalf("expected normalized remote HTTPS endpoint, got cleaned=%q local=%v err=%v", cleaned, isLocal, err)
	}

	badCases := []string{
		"http://evil.example.com/v1",
		"ftp://127.0.0.1/v1",
		"https://user:pass@api.openai.com/v1",
		"https://api.openai.com/v1?leak=1",
		"https://api.openai.com/v1#frag",
		"not a url",
	}
	for _, bad := range badCases {
		if _, _, err := llm.ValidateEndpointURL(llm.ProviderOpenAI, bad); err == nil {
			t.Fatalf("expected endpoint refusal for %q", bad)
		}
	}
}

func TestUnknownProviderErrorUsesOMLXSpelling(t *testing.T) {
	s := llm.DefaultSettings()
	bad := s.GetSnapshot()
	bad.Providers["unknown-x"] = llm.ProviderConfig{BaseURL: "https://example.com/v1", TimeoutSec: 30}
	err := s.Update(&bad, nil)
	if err == nil || strings.Contains(err.Error(), "olmx") || !strings.Contains(err.Error(), "omlx") {
		t.Fatalf("expected unknown provider error with omlx spelling, got %v", err)
	}
}
