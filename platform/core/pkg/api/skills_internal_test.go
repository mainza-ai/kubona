package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/skills"
	"github.com/mainza-ai/kubona/core/pkg/tools"
)

func skillTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{skillStore: skills.Load(skills.Options{UserDir: t.TempDir()})}
}

func TestCopilotPromptAdvertisesSkillNamesNotBodies(t *testing.T) {
	s := skillTestServer(t)
	prompt := s.copilotSystemPrompt()
	if !strings.Contains(prompt, "pes-scan:") || !strings.Contains(prompt, "mitigation-choice:") {
		t.Fatalf("prompt missing skill names: %q", prompt)
	}
	if strings.Contains(prompt, "Potential-energy-surface scan") {
		t.Fatal("prompt leaked a skill body")
	}
}

func TestLoadSkillDispatchReturnsBodyAndRefusesUnknown(t *testing.T) {
	s := skillTestServer(t)
	reg := s.registerAgentTools()
	out, err := reg.Dispatch(context.Background(), CopilotRole, tools.Call{
		ID: "1", Name: "load_skill", Arguments: `{"name":"pes-scan"}`,
	})
	if err != nil {
		t.Fatalf("load_skill: %v", err)
	}
	payload, ok := out.(map[string]any)
	if !ok || !strings.Contains(payload["body"].(string), "Potential-energy-surface scan") {
		t.Fatalf("body = %#v", out)
	}
	if _, err := reg.Dispatch(context.Background(), CopilotRole, tools.Call{
		ID: "2", Name: "load_skill", Arguments: `{"name":"not-a-skill"}`,
	}); err == nil || !strings.Contains(err.Error(), "skill_unknown") {
		t.Fatalf("unknown skill error = %v", err)
	}
	// B11 section addressing: a known heading returns only that part; an
	// unknown heading names the available ones.
	out, err = reg.Dispatch(context.Background(), CopilotRole, tools.Call{
		ID: "4", Name: "load_skill", Arguments: `{"name":"pes-scan","section":"Potential-energy-surface scan"}`,
	})
	if err != nil {
		t.Fatalf("section load: %v", err)
	}
	sectionPayload := out.(map[string]any)
	if sectionPayload["section"] != "Potential-energy-surface scan" || !strings.Contains(sectionPayload["body"].(string), "Potential-energy-surface scan") {
		t.Fatalf("section payload = %#v", out)
	}
	if _, err := reg.Dispatch(context.Background(), CopilotRole, tools.Call{
		ID: "5", Name: "load_skill", Arguments: `{"name":"pes-scan","section":"Deploy"}`,
	}); err == nil || !strings.Contains(err.Error(), "skill_section_unknown") {
		t.Fatalf("unknown section error = %v", err)
	}
	// The tribunal roles observe but do not get user skills.
	if _, err := reg.Dispatch(context.Background(), "device-auditor", tools.Call{
		ID: "3", Name: "load_skill", Arguments: `{"name":"pes-scan"}`,
	}); err == nil {
		t.Fatal("tribunal role should not call load_skill")
	}
}

func TestSkillsEndpointReturnsCatalogueWithoutBodies(t *testing.T) {
	s := skillTestServer(t)
	rec := httptest.NewRecorder()
	s.handleSkills(rec, httptest.NewRequest(http.MethodGet, "/api/skills", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Skills []map[string]any `json:"skills"`
		Faults []map[string]any `json:"faults"`
		Limits map[string]any   `json:"limits"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Body.String(), `"faults":[]`) {
		t.Fatalf("faults must be an empty array, not null: %s", rec.Body.String())
	}
	if len(body.Skills) != 6 {
		t.Fatalf("skills = %d, want 6", len(body.Skills))
	}
	if len(body.Faults) != 0 {
		t.Fatalf("faults = %v", body.Faults)
	}
	if body.Limits["max_skills"] != float64(skills.MaxSkills) || body.Limits["max_skill_bytes"] != float64(skills.MaxSkillBytes) {
		t.Fatalf("limits = %v", body.Limits)
	}
	for _, skill := range body.Skills {
		if _, leaked := skill["body"]; leaked {
			t.Fatal("skills endpoint leaked a body field")
		}
	}
	// POST is not a write path here.
	rec = httptest.NewRecorder()
	s.handleSkills(rec, httptest.NewRequest(http.MethodPost, "/api/skills", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", rec.Code)
	}
}

func TestSkillTogglePersistsAndLeavesPrompt(t *testing.T) {
	l, err := ledger.OpenLedger(t.TempDir() + "/skills.db")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	s := &Server{skillStore: skills.Load(skills.Options{UserDir: t.TempDir()}), ledger: l}
	req := httptest.NewRequest(http.MethodPut, "/api/skills/pes-scan", strings.NewReader(`{"enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleSkillByID(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("toggle status = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(s.copilotSystemPrompt(), "pes-scan:") {
		t.Fatal("disabled skill still advertised in the prompt")
	}
	disabled, err := l.DisabledSkills()
	if err != nil || len(disabled) != 1 || disabled[0] != "pes-scan" {
		t.Fatalf("persisted disabled = %v err=%v", disabled, err)
	}
	// A fresh load applies the persisted set, so restart does not silently
	// re-enable the skill.
	fresh := skills.Load(skills.Options{UserDir: t.TempDir()})
	fresh.SetDisabled(disabled)
	if strings.Contains(fresh.PromptSection(CopilotRole), "pes-scan:") {
		t.Fatal("restart did not apply the disabled set")
	}
}
