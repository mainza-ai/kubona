package api_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/experimentresult"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc"
)

// fakeManuscriptBridge composes a deterministic draft from the request and,
// when asked, returns one PDF artifact. It never invents a verdict stronger
// than the evidence it was given, which is the contract under test.
type fakeManuscriptBridge struct {
	pb.QuantumBridgeServiceClient
	recommendation  string
	sectionLatex    string
	citationKeys    []string
	reverseSections bool
	buildCalls      int
}

func (f *fakeManuscriptBridge) ComposeManuscript(_ context.Context, in *pb.ManuscriptRequest, _ ...grpc.CallOption) (*pb.ManuscriptResponse, error) {
	return f.compose(in)
}

func (f *fakeManuscriptBridge) compose(in *pb.ManuscriptRequest) (*pb.ManuscriptResponse, error) {
	var req struct {
		ManuscriptID string `json:"manuscript_id"`
		Title        string `json:"title"`
		Abstract     string `json:"abstract"`
		Build        bool   `json:"build"`
		Campaign     struct {
			Experiments []struct {
				JobID    string  `json:"job_id"`
				Energy   float64 `json:"energy"`
				Measured bool    `json:"measured"`
			} `json:"experiments"`
		} `json:"campaign"`
	}
	if err := json.Unmarshal([]byte(in.RequestJson), &req); err != nil {
		return &pb.ManuscriptResponse{Ok: false, ErrorCode: "invalid_request", ErrorMessage: err.Error()}, nil
	}
	title := req.Title
	if title == "" {
		title = "Composed manuscript"
	}
	abstract := req.Abstract
	if abstract == "" {
		abstract = "Composed from the stored campaign projection."
	}
	rec := f.recommendation
	if rec == "" {
		rec = "REVISE"
	}
	latex := f.sectionLatex
	if latex == "" {
		latex = "Measured rows only."
	}
	cites := make([]map[string]any, 0, len(f.citationKeys))
	for _, key := range f.citationKeys {
		cites = append(cites, map[string]any{"key": key, "title": key, "authors": "A", "year": "2024", "doi": ""})
	}
	sections := []map[string]any{
		{"slug": "results", "title": "Results", "latex": latex},
		{"slug": "methods", "title": "Methods", "latex": "Bridge-executed."},
	}
	if f.reverseSections {
		sections[0], sections[1] = sections[1], sections[0]
	}
	out := map[string]any{
		"ok":            true,
		"manuscript_id": req.ManuscriptID,
		"citations":     cites,
		"title":         title,
		"abstract":      abstract,
		"sections":      sections,
		"figure_sha256": map[string]string{},
		"review":        map[string]any{"recommendation": rec, "mean_score": 6.9, "hygiene": 4.0},
	}
	var artifacts []*pb.ManuscriptArtifact
	if req.Build {
		f.buildCalls++
		out["build"] = map[string]any{"build_hash": "build-hash-1", "pdf_sha256": "pdf-hash"}
		artifacts = append(artifacts,
			&pb.ManuscriptArtifact{
				Name: "main.pdf", Content: []byte("%PDF-1.4 fake"), Sha256: sha256HexTest([]byte("%PDF-1.4 fake")),
			},
			&pb.ManuscriptArtifact{
				Name: "sections/results.tex", Content: []byte("\\section{Results}"),
				Sha256: sha256HexTest([]byte("\\section{Results}")),
			},
		)
	}
	raw, _ := json.Marshal(out)
	return &pb.ManuscriptResponse{Ok: true, ResponseJson: string(raw), Artifacts: artifacts}, nil
}

func sha256HexTest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func setupManuscriptServer(t *testing.T, bridge pb.QuantumBridgeServiceClient) (*api.Server, func()) {
	t.Helper()
	t.Setenv("KUBONA_ALLOW_EPHEMERAL_SIGNER", "1")
	t.Setenv("KUBONA_API_TOKEN", "op-token")
	dbPath := filepath.Join(t.TempDir(), "manuscripts_api.db")
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, bridge, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	srv := api.NewServer(l, sm, jr, sup, wd)
	return srv, func() {
		l.Close()
		os.Remove(dbPath)
	}
}

func doJSON(t *testing.T, srv *api.Server, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func seedCampaign(t *testing.T, srv *api.Server, sessionID string) {
	t.Helper()
	state := agents.CampaignState{
		SessionID:    sessionID,
		TargetSystem: "H2",
		Experiments: []agents.CampaignExperiment{{
			JobID: "job-1", Kind: "vqe", Energy: ptr(1.2), Measured: true, Backend: "aer",
		}},
		ResultReceipts: []agents.CampaignResultRef{{
			JobID: "job-1",
			Receipt: &experimentresult.Receipt{
				Algorithm: "ed25519", ResultSHA256: "abc123", Signed: true, Signature: "sig",
			},
		}},
	}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Ledger().SaveCampaignState(sessionID, raw); err != nil {
		t.Fatalf("save campaign: %v", err)
	}
}

func ptr(v float64) *float64 { return &v }

func createSession(t *testing.T, srv *api.Server, tenant string) string {
	t.Helper()
	if tenant != "default" {
		if _, err := srv.Ledger().DB().Exec(
			`INSERT OR IGNORE INTO tenants (id, display_name, created_at) VALUES (?, ?, ?)`,
			tenant, tenant, time.Now().UTC().Format(time.RFC3339)); err != nil {
			t.Fatalf("register tenant: %v", err)
		}
	}
	sess, err := srv.SessionManager().CreateSession("manuscript drill", 10, tenant)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return sess.ID
}

func TestManuscriptRouteRequiresCampaignAndStoresDraft(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()

	sid := createSession(t, srv, "default")
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{})
	if w.Code != http.StatusConflict {
		t.Fatalf("no campaign = %d, want 409: %s", w.Code, w.Body.String())
	}

	seedCampaign(t, srv, sid)
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{
		"title": "Measured H2 results", "authors": []string{"Operator"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201: %s", w.Code, w.Body.String())
	}
	var created struct {
		Manuscript struct {
			ID          string `json:"id"`
			State       string `json:"state"`
			ContentHash string `json:"content_hash"`
			Sections    []struct {
				Slug string `json:"slug"`
			} `json:"sections"`
			Review struct {
				Recommendation string `json:"recommendation"`
			} `json:"review"`
			Artifacts []any `json:"artifacts"`
		} `json:"manuscript"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	ms := created.Manuscript
	if ms.ID == "" || ms.State != ledger.ManuscriptStateDraft || ms.ContentHash == "" {
		t.Fatalf("draft not stored: %+v", ms)
	}
	if len(ms.Sections) != 2 || ms.Review.Recommendation != "REVISE" {
		t.Fatalf("unexpected draft: %+v", ms)
	}
	if len(ms.Artifacts) != 0 {
		t.Fatalf("draft should not carry artifacts: %+v", ms.Artifacts)
	}

	// A second session cannot read it.
	other := createSession(t, srv, "other")
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+other+"/manuscripts/"+ms.ID, "", nil)
	// The tenant gate may refuse before the existence lookup (401) or the
	// manuscript may be absent for that session (404). Either way the body
	// must not leak the draft.
	if w.Code != http.StatusUnauthorized && w.Code != http.StatusNotFound {
		t.Fatalf("cross-session read = %d, want 401/404", w.Code)
	}
	if strings.Contains(w.Body.String(), "Measured H2 results") {
		t.Fatal("cross-session read leaked the manuscript body")
	}
}

func TestManuscriptBuildSignoffAndDeposit(t *testing.T) {
	bridge := &fakeManuscriptBridge{recommendation: "ACCEPT"}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()

	sid := createSession(t, srv, "default")
	seedCampaign(t, srv, sid)
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{
		"title": "Measured H2 results", "build": true,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create+build = %d: %s", w.Code, w.Body.String())
	}
	var detail struct {
		Manuscript struct {
			ID          string `json:"id"`
			ContentHash string `json:"content_hash"`
			State       string `json:"state"`
			BuildHash   string `json:"build_hash"`
			Artifacts   []struct {
				Name   string `json:"name"`
				SHA256 string `json:"sha256"`
			} `json:"artifacts"`
		} `json:"manuscript"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	ms := detail.Manuscript
	if ms.BuildHash != "build-hash-1" || len(ms.Artifacts) != 2 {
		t.Fatalf("build not installed: %+v", ms)
	}
	names := map[string]bool{}
	for _, a := range ms.Artifacts {
		names[a.Name] = true
	}
	if !names["main.pdf"] || !names["sections/results.tex"] {
		t.Fatalf("artifact set = %+v", names)
	}

	// Artifact download carries the bytes and their recorded hash.
	req := httptest.NewRequest(http.MethodGet,
		"/api/sessions/"+sid+"/manuscripts/"+ms.ID+"/artifacts/main.pdf", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("artifact download = %d %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Kubona-SHA256") == "" {
		t.Fatal("artifact download lacks its hash header")
	}

	// Signoff requires the operator bearer.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+ms.ID+"/signoff", "",
		map[string]any{"content_hash": ms.ContentHash})
	if w.Code != http.StatusForbidden {
		t.Fatalf("anonymous signoff = %d, want 403", w.Code)
	}
	// Wrong hash refuses.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+ms.ID+"/signoff", "op-token",
		map[string]any{"content_hash": "not-the-hash"})
	if w.Code != http.StatusConflict {
		t.Fatalf("mismatched signoff = %d, want 409: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+ms.ID+"/signoff", "op-token",
		map[string]any{"content_hash": ms.ContentHash,
			"expiry": time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339)})
	if w.Code != http.StatusOK {
		t.Fatalf("signoff = %d: %s", w.Code, w.Body.String())
	}
	var signed struct {
		Manuscript struct {
			Signoff *struct {
				SignerPubKey string `json:"signer_pubkey_hex"`
				Signature    string `json:"signature_hex"`
				ContentHash  string `json:"content_hash"`
				Expiry       string `json:"expiry"`
			} `json:"signoff"`
		} `json:"manuscript"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &signed); err != nil {
		t.Fatal(err)
	}
	sig := signed.Manuscript.Signoff
	if sig == nil || sig.ContentHash != ms.ContentHash {
		t.Fatalf("signoff missing: %+v", sig)
	}
	pub, _ := hex.DecodeString(sig.SignerPubKey)
	signature, _ := hex.DecodeString(sig.Signature)
	payload := []byte("manuscript-signoff:" + ms.ID + ":" + ms.ContentHash + ":" + sig.Expiry)
	if !ed25519.Verify(pub, payload, signature) {
		t.Fatal("signoff signature does not verify over the canonical payload")
	}

	// Deposit manifest is staged from built artifacts, never auto-published.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+ms.ID+"/deposit", "op-token", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("deposit = %d: %s", w.Code, w.Body.String())
	}
	var deposit struct {
		Manifest struct {
			State       string              `json:"state"`
			AutoPublish bool                `json:"auto_publish"`
			DOI         any                 `json:"doi"`
			Files       []map[string]string `json:"files"`
		} `json:"manifest"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &deposit); err != nil {
		t.Fatal(err)
	}
	if deposit.Manifest.State != "staged-needs-human-upload" || deposit.Manifest.AutoPublish || deposit.Manifest.DOI != nil {
		t.Fatalf("manifest is not a human-upload staging record: %+v", deposit.Manifest)
	}
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/manuscripts/"+ms.ID, "", nil)
	if !strings.Contains(w.Body.String(), `"state":"deposit_ready"`) {
		t.Fatalf("deposit did not mark the manuscript deposit_ready: %s", w.Body.String())
	}
}

func TestManuscriptDepositRefusesWithoutAcceptReview(t *testing.T) {
	bridge := &fakeManuscriptBridge{recommendation: "REVISE"}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedCampaign(t, srv, sid)
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{"build": true})
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", w.Code, w.Body.String())
	}
	var detail struct {
		Manuscript struct {
			ID          string `json:"id"`
			ContentHash string `json:"content_hash"`
		} `json:"manuscript"`
	}
	json.Unmarshal(w.Body.Bytes(), &detail)
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+detail.Manuscript.ID+"/signoff", "op-token",
		map[string]any{"content_hash": detail.Manuscript.ContentHash})
	if w.Code != http.StatusOK {
		t.Fatalf("signoff = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+detail.Manuscript.ID+"/deposit", "op-token", nil)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "review_not_accepted") {
		t.Fatalf("deposit with REVISE = %d %s", w.Code, w.Body.String())
	}
}

func TestManuscriptBuildRefusesChangedSource(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedCampaign(t, srv, sid)
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{"title": "draft"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", w.Code, w.Body.String())
	}
	var detail struct {
		Manuscript struct {
			ID string `json:"id"`
		} `json:"manuscript"`
	}
	json.Unmarshal(w.Body.Bytes(), &detail)

	// The stored campaign snapshot is unchanged, but a composer that now
	// returns different prose would produce a PDF the signoff does not
	// describe. That must refuse rather than silently rebuild.
	bridge.sectionLatex = "A different source."
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+detail.Manuscript.ID+"/build", "", nil)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "source_changed") {
		t.Fatalf("changed source build = %d %s", w.Code, w.Body.String())
	}
}

func TestManuscriptSignoffRejectsPastExpiry(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedCampaign(t, srv, sid)
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{})
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d", w.Code)
	}
	var detail struct {
		Manuscript struct {
			ID          string `json:"id"`
			ContentHash string `json:"content_hash"`
		} `json:"manuscript"`
	}
	json.Unmarshal(w.Body.Bytes(), &detail)
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+detail.Manuscript.ID+"/signoff", "op-token",
		map[string]any{"content_hash": detail.Manuscript.ContentHash, "expiry": "2020-01-01T00:00:00Z"})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "future") {
		t.Fatalf("past expiry = %d %s", w.Code, w.Body.String())
	}
}

func TestManuscriptArtifactDownloadAcceptsNestedProjectPaths(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedCampaign(t, srv, sid)
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{"build": true})
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", w.Code, w.Body.String())
	}
	var detail struct {
		Manuscript struct {
			ID string `json:"id"`
		} `json:"manuscript"`
	}
	json.Unmarshal(w.Body.Bytes(), &detail)

	for _, path := range []string{
		"/api/sessions/" + sid + "/manuscripts/" + detail.Manuscript.ID + "/artifacts/sections/results.tex",
		"/api/sessions/" + sid + "/manuscripts/" + detail.Manuscript.ID + "/artifacts/sections%2Fresults.tex",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Results") {
			t.Fatalf("nested artifact %q = %d %q", path, rec.Code, rec.Body.String())
		}
	}

	for _, bad := range []string{"../secret", "sections/../secret", "/etc/passwd"} {
		req := httptest.NewRequest(http.MethodGet,
			"/api/sessions/"+sid+"/manuscripts/"+detail.Manuscript.ID+"/artifacts/"+bad, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		// net/http cleans the path before the handler (307/301); the cleaned
		// path is no longer an artifact route, so the only requirement is that
		// the traversal never yields artifact bytes.
		if rec.Code == http.StatusOK {
			t.Fatalf("traversal %q = 200 (%q)", bad, rec.Body.String())
		}
	}
}

func TestManuscriptStoresBridgeEffectiveCitationKeys(t *testing.T) {
	bridge := &fakeManuscriptBridge{citationKeys: []string{"10_1_x"}}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedCampaign(t, srv, sid)
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{
		"citations": []map[string]any{{"key": "10.1/x", "title": "T", "authors": "A", "year": "2024"}},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", w.Code, w.Body.String())
	}
	var detail struct {
		Manuscript struct {
			Citations []struct {
				Key string `json:"key"`
			} `json:"citations"`
			ContentHash string `json:"content_hash"`
		} `json:"manuscript"`
	}
	json.Unmarshal(w.Body.Bytes(), &detail)
	if len(detail.Manuscript.Citations) != 1 || detail.Manuscript.Citations[0].Key != "10_1_x" {
		t.Fatalf("stored citations = %+v", detail.Manuscript.Citations)
	}
}

func TestManuscriptWorkIsRateLimited(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedCampaign(t, srv, sid)
	statuses := []int{}
	for i := 0; i < 6; i++ {
		w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{})
		statuses = append(statuses, w.Code)
		if w.Code == http.StatusTooManyRequests {
			break
		}
	}
	sawLimit := false
	for _, code := range statuses {
		if code == http.StatusTooManyRequests {
			sawLimit = true
		}
	}
	if !sawLimit {
		t.Fatalf("no rate limit after %v", statuses)
	}
}

func TestManuscriptHashCoversSectionOrder(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedCampaign(t, srv, sid)
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{})
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", w.Code, w.Body.String())
	}
	var detail struct {
		Manuscript struct {
			ID string `json:"id"`
		} `json:"manuscript"`
	}
	json.Unmarshal(w.Body.Bytes(), &detail)

	// Same text, same hashes, different order: the rendered document differs,
	// so the signoff hash must differ (and the build must refuse).
	bridge.reverseSections = true
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+detail.Manuscript.ID+"/build", "", nil)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "source_changed") {
		t.Fatalf("reordered sections = %d %s", w.Code, w.Body.String())
	}
}

func TestManuscriptAnnotationsGateSignoff(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedCampaign(t, srv, sid)
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{"title": "review me"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", w.Code, w.Body.String())
	}
	var detail struct {
		Manuscript struct {
			ID          string `json:"id"`
			ContentHash string `json:"content_hash"`
		} `json:"manuscript"`
	}
	json.Unmarshal(w.Body.Bytes(), &detail)
	mid := detail.Manuscript.ID

	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+mid+"/annotations", "",
		map[string]any{"anchor": "results", "body": "Explain the discrepancy."})
	if w.Code != http.StatusCreated {
		t.Fatalf("annotation = %d: %s", w.Code, w.Body.String())
	}
	var ann struct {
		Annotation struct {
			ID string `json:"id"`
		} `json:"annotation"`
	}
	json.Unmarshal(w.Body.Bytes(), &ann)

	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+mid+"/signoff", "op-token",
		map[string]any{"content_hash": detail.Manuscript.ContentHash})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "annotations_open") {
		t.Fatalf("signoff with open note = %d %s", w.Code, w.Body.String())
	}

	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/manuscripts/"+mid, "", nil)
	if !strings.Contains(w.Body.String(), `"open_annotations":1`) {
		t.Fatalf("detail open count missing: %s", w.Body.String())
	}

	w = doJSON(t, srv, http.MethodPatch,
		"/api/sessions/"+sid+"/manuscripts/"+mid+"/annotations/"+ann.Annotation.ID, "",
		map[string]any{"status": "addressed"})
	if w.Code != http.StatusOK {
		t.Fatalf("resolve = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+mid+"/signoff", "op-token",
		map[string]any{"content_hash": detail.Manuscript.ContentHash})
	if w.Code != http.StatusOK {
		t.Fatalf("signoff after resolve = %d: %s", w.Code, w.Body.String())
	}

	// A second lab cannot read the notes.
	other := createSession(t, srv, "other")
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+other+"/manuscripts/"+mid+"/annotations", "", nil)
	if w.Code == http.StatusOK {
		t.Fatalf("cross-tenant annotations = 200: %s", w.Body.String())
	}
}

func TestManuscriptCreateIsIdempotentByRequestID(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedCampaign(t, srv, sid)
	body := map[string]any{"request_id": "ms-create-1", "title": "Once"}
	first := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first create = %d: %s", first.Code, first.Body.String())
	}
	second := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", body)
	if second.Code != http.StatusOK {
		t.Fatalf("replay = %d: %s", second.Code, second.Body.String())
	}
	var a, b struct {
		Manuscript struct {
			ID string `json:"id"`
		} `json:"manuscript"`
	}
	json.Unmarshal(first.Body.Bytes(), &a)
	json.Unmarshal(second.Body.Bytes(), &b)
	if a.Manuscript.ID == "" || a.Manuscript.ID != b.Manuscript.ID {
		t.Fatalf("replay created a second draft: %q vs %q", a.Manuscript.ID, b.Manuscript.ID)
	}
	list := doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/manuscripts", "", nil)
	if !strings.Contains(list.Body.String(), `"id":"`+a.Manuscript.ID+`"`) || strings.Count(list.Body.String(), `"id":"`) != 1 {
		t.Fatalf("idempotent create persisted duplicates: %s", list.Body.String())
	}
}
