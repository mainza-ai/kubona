package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/autonomy"
	"github.com/mainza-ai/kubona/core/pkg/hypotheses"
	"github.com/mainza-ai/kubona/core/pkg/qec"
)

func TestPhase0_B4B5_CodesignPromptIncludesSpecAndFailureFeedback(t *testing.T) {
	spec := codesignSpecMeta{OK: true, SpecKind: "maj3", NumQubits: 4}
	table := json.RawMessage(`{"inputs":[0,1,2],"outputs":[3]}`)
	best := &codesignTrial{Toffoli: 2, PeakQubits: 4, TQ: json.Number("8"), Candidate: "2ops:ccx,cx"}

	prompt := buildCodesignPrompt(spec, table, 1, 64, nil, "output bit 3 mismatch on input 110", best)
	if !strings.Contains(prompt, `spec_kind="maj3"`) {
		t.Fatalf("expected spec_kind in prompt, got: %s", prompt)
	}
	if !strings.Contains(prompt, `"inputs":[0,1,2]`) {
		t.Fatalf("expected spec_table in prompt, got: %s", prompt)
	}
	if strings.Contains(prompt, "A valid AND2 result is exactly") {
		t.Fatalf("prompt must not leak hardcoded AND2 worked answer: %s", prompt)
	}
	if !strings.Contains(prompt, "output bit 3 mismatch on input 110") {
		t.Fatalf("expected last failure reason in prompt, got: %s", prompt)
	}
	if !strings.Contains(prompt, "T*Q=8") {
		t.Fatalf("expected best frontier summary in prompt, got: %s", prompt)
	}
}

func TestPhase0_B6_NoveltyRefusesStructuralDomains(t *testing.T) {
	refs := []hypotheses.PriorArt{{ID: "W1", Title: "Bivariate bicycle codes", Abstract: "qLDPC distance 12"}}
	for _, domain := range []string{"qec", "qec_code", "code_novelty", "circuit", "schedule", "decoder"} {
		_, _, ok, err := hypotheses.MeasureNoveltyForDomain(domain, "new gross code", refs)
		if err == nil || ok {
			t.Fatalf("domain %q: expected fail-closed refusal for lexical novelty, got ok=%v err=%v", domain, ok, err)
		}
	}
	dist, method, ok, err := hypotheses.MeasureNoveltyForDomain("literature", "novel quantum chemistry ansatz", refs)
	if err != nil || !ok || method != hypotheses.NoveltyMethodLexicalContainmentV1 || dist < 0 || dist > 1 {
		t.Fatalf("literature domain should delegate to lexical novelty: dist=%v method=%q ok=%v err=%v", dist, method, ok, err)
	}
}

func TestPhase0_B7_DiscoveryPlannerKindsSubsetOfExecutableJobs(t *testing.T) {
	if len(discoveryPlannerKinds) == 0 {
		t.Fatal("discoveryPlannerKinds must not be empty")
	}
	for _, kind := range discoveryPlannerKinds {
		if !allowedDiscoveryKind(kind) {
			t.Fatalf("planner kind %q is not accepted by allowedDiscoveryKind", kind)
		}
		if jt, ok := experimentJobType(kind); !ok || jt == "" {
			t.Fatalf("planner kind %q has no executable job type in experimentJobType", kind)
		}
	}
	if allowedDiscoveryKind("conjecture_check") {
		t.Fatal("conjecture_check has no job mapping and must not be allowed")
	}
}

func TestPhase0_B9_QECEstimatorMarkedIllustrative(t *testing.T) {
	est := qec.NewResourceEstimator()
	spec := qec.QECResourceSpec{NumLogicalQubits: 12, LogicalTCount: 100, LogicalClifford: 500, PhysicalErrorRate: 0.001}
	na, err := est.EstimateNeutralAtom(spec)
	if err != nil || !na.Illustrative || na.MethodologyNote == "" {
		t.Fatalf("neutral-atom estimate must be marked illustrative: %+v err=%v", na, err)
	}
	bb, err := est.EstimateBivariateBicycle(spec)
	if err != nil || !bb.Illustrative || bb.MethodologyNote == "" {
		t.Fatalf("BB estimate must be marked illustrative: %+v err=%v", bb, err)
	}
}

func TestPhase0_B11_DiscoveryRunRefusesSuggestExecuteStage(t *testing.T) {
	s, _, cleanup := replanHookTestServer(t, "test_phase0_b11.db")
	defer cleanup()

	sess, err := s.sessionMgr.CreateSession("phase0-b11", 10, "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	pol := autonomy.DefaultPolicy()
	pol.Stages[autonomy.StageExecute] = autonomy.ModeSuggest
	polRaw, _ := json.Marshal(pol)
	if err := s.ledger.SetAutonomyPolicy(sess.ID, string(polRaw)); err != nil {
		t.Fatalf("save autonomy policy: %v", err)
	}

	body := map[string]any{
		"session_id": sess.ID,
		"goal":       "run H2 VQE",
		"steps": []map[string]any{
			{"kind": "vqe", "spec": map[string]any{"molecule": "H2", "bond_distance": 0.74, "shots": 100}},
		},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/discovery/run", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleDiscoveryRun(w, req)

	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "autonomy_suggest_only") {
		t.Fatalf("expected 403 autonomy_suggest_only for suggest-stage session, got %d: %s", w.Code, w.Body.String())
	}
}
