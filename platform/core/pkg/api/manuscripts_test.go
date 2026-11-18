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
	recommendation string
	sectionLatex   string
	buildCalls     int
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
	out := map[string]any{
		"ok":            true,
		"manuscript_id": req.ManuscriptID,
		"title":         title,
		"abstract":      abstract,
		"sections": []map[string]any{
			{"slug": "results", "title": "Results", "latex": latex},
			{"slug": "methods", "title": "Methods", "latex": "Bridge-executed."},
		},
		"figure_sha256": map[string]string{},
		"review":        map[string]any{"recommendation": rec, "mean_score": 6.9, "hygiene": 4.0},
	}
	var artifacts []*pb.ManuscriptArtifact
	if req.Build {
		f.buildCalls++
		out["build"] = map[string]any{"build_hash": "build-hash-1", "pdf_sha256": "pdf-hash"}
		artifacts = append(artifacts, &pb.ManuscriptArtifact{
			Name: "main.pdf", Content: []byte("%PDF-1.4 fake"), Sha256: sha256HexTest([]byte("%PDF-1.4 fake")),
		})
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
	if ms.BuildHash != "build-hash-1" || len(ms.Artifacts) != 1 || ms.Artifacts[0].Name != "main.pdf" {
		t.Fatalf("build not installed: %+v", ms)
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
