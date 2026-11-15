package skills

import (
	"strings"
	"testing"
)

func TestLoadSectionReturnsOnlyTheNamedPart(t *testing.T) {
	dir := t.TempDir()
	body := "Preamble text.\n\n## Setup\n\nCall get_anchor first.\n\n## Run\n\nSubmit one job.\n"
	writeSkill(t, dir, "sectioned", validSkill("sectioned", "sectioned test skill", body))
	store := Load(Options{UserDir: dir})
	section, fault := store.LoadSection("sectioned", "copilot", "Setup")
	if fault != nil {
		t.Fatalf("fault: %+v", fault)
	}
	if !strings.Contains(section, "Call get_anchor first.") || strings.Contains(section, "Submit one job.") {
		t.Fatalf("section body = %q", section)
	}
	if !strings.HasPrefix(section, "## Setup") {
		t.Fatalf("section lost its heading: %q", section)
	}
	// Case and leading #s are normalized.
	if _, fault := store.LoadSection("sectioned", "copilot", "## run"); fault != nil {
		t.Fatalf("case-insensitive section: %+v", fault)
	}
	full, fault := store.LoadSection("sectioned", "copilot", "")
	if fault != nil || !strings.Contains(full, "Preamble text.") || !strings.Contains(full, "Submit one job.") {
		t.Fatalf("full body = %q fault %+v", full, fault)
	}
}

func TestLoadSectionUnknownListsAvailableHeadings(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "sectioned", validSkill("sectioned", "sectioned test skill", "## Setup\nA\n## Run\nB\n"))
	store := Load(Options{UserDir: dir})
	_, fault := store.LoadSection("sectioned", "copilot", "Deploy")
	if fault == nil || fault.Code != FaultSectionUnknown {
		t.Fatalf("fault = %+v", fault)
	}
	if !strings.Contains(fault.Detail, "Setup") || !strings.Contains(fault.Detail, "Run") {
		t.Fatalf("fault detail = %q", fault.Detail)
	}
}

func TestSeedsExposeTheirTopHeading(t *testing.T) {
	store := Load(Options{})
	names := SectionNames(mustBody(t, store, "pes-scan"))
	if len(names) == 0 || !strings.Contains(strings.ToLower(names[0]), "potential-energy-surface") {
		t.Fatalf("pes-scan headings = %v", names)
	}
	if _, fault := store.LoadSection("pes-scan", "copilot", names[0]); fault != nil {
		t.Fatalf("top heading load: %+v", fault)
	}
}

func mustBody(t *testing.T, store *Store, name string) string {
	t.Helper()
	body, fault := store.LoadBody(name, "copilot")
	if fault != nil {
		t.Fatalf("LoadBody(%s): %+v", name, fault)
	}
	return body
}

func TestSectionNamesIgnoresProseHashes(t *testing.T) {
	body := "not a heading # inline\n## Real\nbody\n"
	names := SectionNames(body)
	if len(names) != 1 || names[0] != "Real" {
		t.Fatalf("names = %v", names)
	}
}
