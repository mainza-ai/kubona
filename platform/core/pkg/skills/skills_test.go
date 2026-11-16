package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validSkill(name, description, body string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\nrole: copilot\ntools: get_anchor, get_campaign\n---\n" + body + "\n"
}

func writeSkill(t *testing.T, root, dir, content string) {
	t.Helper()
	path := filepath.Join(root, dir)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSeedsLoadWithOnlyNamesAndDescriptionsInPrompt(t *testing.T) {
	store := Load(Options{})
	if got := store.Len(); got != 6 {
		t.Fatalf("seed count = %d, want 6", got)
	}
	if faults := store.Faults(); len(faults) != 0 {
		t.Fatalf("seed faults = %v", faults)
	}
	names := []string{"classical-anchor-check", "hardware-readiness", "literature-review", "mitigation-choice", "pes-scan", "vqe-campaign"}
	for i, skill := range store.ForRole("copilot") {
		if skill.Name != names[i] {
			t.Fatalf("skill[%d] = %q, want %q (sorted)", i, skill.Name, names[i])
		}
		if skill.Source != SourceSeed {
			t.Fatalf("%s source = %q, want seed", skill.Name, skill.Source)
		}
		if len([]rune(skill.Description)) > MaxDescription {
			t.Fatalf("%s description exceeds %d", skill.Name, MaxDescription)
		}
	}
	prompt := store.PromptSection("copilot")
	for _, name := range names {
		if !strings.Contains(prompt, name+":") {
			t.Fatalf("prompt missing %s", name)
		}
	}
	// The body must stay behind load_skill.
	if strings.Contains(prompt, "Potential-energy-surface scan") {
		t.Fatal("prompt leaked a skill body")
	}
	if store.PromptSection("device-auditor") != "" {
		t.Fatal("copilot skills must not be advertised to another role")
	}
}

func TestUserSkillLoadsAndBodyStaysPrivate(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "custom-check", validSkill("custom-check", "A custom check for this lab.", "BODY-SENTINEL"))
	store := Load(Options{UserDir: dir})
	if got := store.Len(); got != 7 {
		t.Fatalf("count = %d, want 7", got)
	}
	body, fault := store.LoadBody("custom-check", "copilot")
	if fault != nil {
		t.Fatalf("LoadBody fault: %+v", fault)
	}
	if body != "BODY-SENTINEL" {
		t.Fatalf("body = %q", body)
	}
	raw, err := json.Marshal(store.Public("copilot"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "BODY-SENTINEL") {
		t.Fatal("public catalogue leaked a skill body")
	}
}

func TestLoaderRefusesSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "outside")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte(validSkill("linked", "linked skill", "body")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	store := Load(Options{UserDir: dir})
	if store.Len() != 6 {
		t.Fatalf("symlinked skill loaded: count %d", store.Len())
	}
	found := false
	for _, fault := range store.Faults() {
		if fault.Code == FaultSymlink && fault.Skill == "linked" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing symlink fault: %v", store.Faults())
	}
}

func TestLoaderReportsClosedFaults(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "Bad Name", validSkill("Bad Name", "bad", "body"))
	writeSkill(t, dir, "long-description", validSkill("long-description", strings.Repeat("x", MaxDescription+1), "body"))
	writeSkill(t, dir, "unknown-field", "---\nname: unknown-field\ndescription: okay\nsecret: nope\n---\nbody")
	writeSkill(t, dir, "pes-scan", validSkill("pes-scan", "duplicate seed", "body"))
	writeSkill(t, dir, "huge", validSkill("huge", "too large", strings.Repeat("y", MaxSkillBytes)))
	writeSkill(t, dir, "bad-tool", "---\nname: bad-tool\ndescription: okay\ntools: get_anchor, ../escape\n---\nbody")

	store := Load(Options{UserDir: dir})
	want := map[FaultCode]bool{
		FaultNameInvalid:        false,
		FaultDescriptionInvalid: false,
		FaultFrontmatterInvalid: false,
		FaultDuplicate:          false,
		FaultTooLarge:           false,
		FaultToolInvalid:        false,
	}
	for _, fault := range store.Faults() {
		if _, ok := want[fault.Code]; ok {
			want[fault.Code] = true
		}
	}
	for code, saw := range want {
		if !saw {
			t.Fatalf("expected fault %s; got %v", code, store.Faults())
		}
	}
	if store.Len() != 6 {
		t.Fatalf("malformed skills must not load: count %d", store.Len())
	}
}

func TestSkillLimitCountsSeeds(t *testing.T) {
	dir := t.TempDir()
	// Seeds occupy 6 of 64 slots, so 59 user skills fit and the 60th faults.
	for i := 0; i < 60; i++ {
		name := "user-" + string(rune('a'+i/26)) + string(rune('a'+i%26))
		writeSkill(t, dir, name, validSkill(name, "bulk skill", "body"))
	}
	store := Load(Options{UserDir: dir})
	if store.Len() != MaxSkills {
		t.Fatalf("count = %d, want %d", store.Len(), MaxSkills)
	}
	found := false
	for _, fault := range store.Faults() {
		if fault.Code == FaultTooMany {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing skill_too_many: %v", store.Faults())
	}
}

func TestLoadBodyRejectsUnknownAndWrongRole(t *testing.T) {
	store := Load(Options{})
	if _, fault := store.LoadBody("not-a-skill", "copilot"); fault == nil || fault.Code != FaultUnknown {
		t.Fatalf("unknown body fault = %+v", fault)
	}
	if _, fault := store.LoadBody("pes-scan", "device-auditor"); fault == nil || fault.Code != FaultUnknown {
		t.Fatalf("role mismatch fault = %+v", fault)
	}
	if _, fault := store.LoadBody("PES-SCAN", "copilot"); fault == nil || fault.Code != FaultNameInvalid {
		t.Fatalf("invalid name fault = %+v", fault)
	}
}

func TestMissingUserDirIsNotAFault(t *testing.T) {
	store := Load(Options{UserDir: filepath.Join(t.TempDir(), "absent")})
	if faults := store.Faults(); len(faults) != 0 {
		t.Fatalf("absent user dir should not fault: %v", faults)
	}
}

func TestNilStoreIsSafe(t *testing.T) {
	var store *Store
	if store.Len() != 0 || store.PromptSection("copilot") != "" {
		t.Fatal("nil store must be inert")
	}
	if _, fault := store.LoadBody("pes-scan", "copilot"); fault == nil || fault.Code != FaultUnknown {
		t.Fatalf("nil store body fault = %+v", fault)
	}
}

func TestDisabledSkillLeavesPromptAndBodyButStaysVisibleInCatalog(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "custom-check", validSkill("custom-check", "check one thing", "body"))
	store := Load(Options{UserDir: dir})
	if err := store.SetEnabled("custom-check", false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(store.PromptSection("copilot"), "custom-check") {
		t.Fatal("disabled skill must not enter the prompt")
	}
	if _, fault := store.LoadBody("custom-check", "copilot"); fault == nil || !strings.Contains(fault.Detail, "disabled") {
		t.Fatalf("disabled body must refuse with a named reason, got %+v", fault)
	}
	found := false
	for _, skill := range store.Public("copilot").Skills {
		if skill.Name == "custom-check" {
			found = true
			if skill.Enabled {
				t.Fatal("catalog must expose enabled=false so Settings can re-enable it")
			}
		}
	}
	if !found {
		t.Fatal("disabled skill disappeared from the catalog")
	}
	if err := store.SetEnabled("custom-check", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(store.PromptSection("copilot"), "custom-check") {
		t.Fatal("re-enabled skill must return to the prompt")
	}
}
