package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

func TestAccessLocalPrivate(t *testing.T) {
	srv, _, cleanup := replanHookTestServer(t, "test_access_local.db")
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/access", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("access: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Mode           string `json:"mode"`
		Role           string `json:"role"`
		SingleLab      bool   `json:"single_lab"`
		AnonymousChat  bool   `json:"anonymous_chat"`
		LLMLocal       bool   `json:"llm_local"`
		CampaignCreate bool   `json:"campaign_create"`
		AuthRequired   bool   `json:"auth_required"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Mode != "local_private" || body.Role != "local_researcher" || !body.SingleLab || !body.AnonymousChat || !body.LLMLocal || !body.CampaignCreate || body.AuthRequired {
		t.Fatalf("local access posture wrong: %+v", body)
	}
}

func TestAccessMultiLabRefusesAnonymousCampaignCreate(t *testing.T) {
	t.Setenv("KUBONA_ENV", "production")
	t.Setenv("KUBONA_SINGLE_LAB", "0")
	srv, _, cleanup := replanHookTestServer(t, "test_access_multi.db")
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/sessions",
		bytes.NewBufferString(`{"title":"should not exist","target_molecule":"H2"}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("multi-lab anonymous create must refuse, got %d %s", rec.Code, rec.Body.String())
	}

	accessReq := httptest.NewRequest(http.MethodGet, "/api/access", nil)
	accessRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(accessRec, accessReq)
	var body struct {
		Mode          string `json:"mode"`
		Role          string `json:"role"`
		SingleLab     bool   `json:"single_lab"`
		AnonymousChat bool   `json:"anonymous_chat"`
		AuthRequired  bool   `json:"auth_required"`
	}
	if err := json.Unmarshal(accessRec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Mode != "multi_lab" || body.Role != "anonymous" || body.SingleLab || body.AnonymousChat || !body.AuthRequired {
		t.Fatalf("multi access posture wrong: %+v", body)
	}
}

func TestAnonymousChatRefusedWhenAnyRoleIsCloud(t *testing.T) {
	srv, _, cleanup := replanHookTestServer(t, "test_access_cloud.db")
	defer cleanup()

	snap := srv.llmSettings.GetSnapshot()
	pc := snap.Providers[llm.ProviderOpenAI]
	pc.Enabled = true
	pc.IsLocal = false
	pc.APIKey = "cloud-test-key"
	pc.DefaultModel = "cloud-model"
	snap.Providers[llm.ProviderOpenAI] = pc
	snap.RoleAssignments["copilot"] = llm.RoleModelAssignment{
		Role: "copilot", Provider: llm.ProviderOpenAI, Model: "cloud-model",
	}
	if err := srv.llmSettings.Update(&snap, nil); err != nil {
		t.Fatalf("cloud settings update: %v", err)
	}
	if srv.anonymousChatAllowed() {
		t.Fatal("anonymous chat must be refused when a cloud role is configured")
	}
	req := httptest.NewRequest(http.MethodPost, "/api/chat/messages",
		bytes.NewBufferString(`{"session_id":"ghost","role":"user","content":"hi"}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("cloud anonymous chat must 401 before session resolution, got %d %s", rec.Code, rec.Body.String())
	}
}
