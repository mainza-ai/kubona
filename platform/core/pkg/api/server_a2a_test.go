package api_test

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/cost"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

func TestA2A_PaymentRequiredAndSettlement(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	// 1. GetAgentCard does not require payment
	cardReq := agents.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "a2a.GetAgentCard",
	}
	body, _ := json.Marshal(cardReq)
	resp, err := http.Post(server.URL+"/api/a2a/v1", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to call a2a.GetAgentCard: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for GetAgentCard, got %d", resp.StatusCode)
	}

	var cardResp agents.JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&cardResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if cardResp.Error != nil {
		t.Fatalf("unexpected error in card response: %+v", cardResp.Error)
	}

	// 2. SubmitTask WITHOUT payment header returns HTTP 402
	taskParams, _ := json.Marshal(agents.TaskSubmitParams{
		TaskType:     "molecular_ground_state_pyscf",
		Payload:      json.RawMessage(`{"molecule": "H2"}`),
		Scopes:       []string{"molecular_ground_state_pyscf"},
		MaxBudgetUSD: 5.0,
	})
	taskReq := agents.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "a2a.SubmitTask",
		Params:  taskParams,
	}
	body, _ = json.Marshal(taskReq)
	resp402, err := http.Post(server.URL+"/api/a2a/v1", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to submit task: %v", err)
	}
	defer resp402.Body.Close()
	if resp402.StatusCode != http.StatusPaymentRequired {
		t.Fatalf("expected 402 Payment Required without auth header, got %d", resp402.StatusCode)
	}

	authHeader := resp402.Header.Get("WWW-Authenticate")
	if !strings.HasPrefix(authHeader, "x402 ") {
		t.Fatalf("expected WWW-Authenticate x402 header, got: %s", authHeader)
	}

	var challengeBody struct {
		Challenge cost.PaymentChallenge `json:"challenge"`
	}
	if err := json.NewDecoder(resp402.Body).Decode(&challengeBody); err != nil {
		t.Fatalf("failed to decode 402 challenge body: %v", err)
	}
	challenge := challengeBody.Challenge

	// 3. Client signs the challenge payload with Ed25519 keypair
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 keypair: %v", err)
	}
	msgHash := sha256.Sum256([]byte(challenge.Payload))
	sig := ed25519.Sign(priv, msgHash[:])

	// 4. Submit task WITH valid signed x402 Authorization header
	client := &http.Client{}
	req, _ := http.NewRequest("POST", server.URL+"/api/a2a/v1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf(`x402 challenge="%s", token="USDC", amount="0.05", payer="%s", signature="%s"`,
		challenge.ChallengeID, hex.EncodeToString(pub), hex.EncodeToString(sig)))

	respPaid, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to send paid request: %v", err)
	}
	defer respPaid.Body.Close()

	if respPaid.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK after valid x402 payment, got %d", respPaid.StatusCode)
	}

	var paidResp agents.JSONRPCResponse
	if err := json.NewDecoder(respPaid.Body).Decode(&paidResp); err != nil {
		t.Fatalf("failed to decode paid response: %v", err)
	}
	if paidResp.Error != nil {
		t.Fatalf("unexpected RPC error in paid response: %+v", paidResp.Error)
	}
}

func TestSSE_EventsStreamHandshake(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/events/stream?session_id=sess-test")
	if err != nil {
		t.Fatalf("failed to connect to SSE stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for SSE stream, got %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("expected text/event-stream Content-Type, got: %s", resp.Header.Get("Content-Type"))
	}

	// Read initial SSE frame
	reader := bufio.NewReader(resp.Body)
	line1, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read line 1: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(line1), "event: connected") {
		t.Fatalf("expected 'event: connected', got: %s", line1)
	}

	line2, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read line 2: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(line2), "data:") {
		t.Fatalf("expected 'data: ...', got: %s", line2)
	}
}

func TestCanvas_LayoutPersistence(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	// Initial layout is empty object
	respGet, err := http.Get(server.URL + "/api/canvas/layout?session_id=s1")
	if err != nil {
		t.Fatalf("failed GET layout: %v", err)
	}
	defer respGet.Body.Close()
	var initialLayout map[string]any
	_ = json.NewDecoder(respGet.Body).Decode(&initialLayout)
	if len(initialLayout) != 0 {
		t.Fatalf("expected empty layout, got: %+v", initialLayout)
	}

	// Save layout
	layoutData := `{"nodes":[{"id":"node-1","position":{"x":100,"y":200}}],"zoom":1.5}`
	respPost, err := http.Post(server.URL+"/api/canvas/layout?session_id=s1", "application/json", strings.NewReader(layoutData))
	if err != nil {
		t.Fatalf("failed POST layout: %v", err)
	}
	defer respPost.Body.Close()
	if respPost.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", respPost.StatusCode)
	}

	// Retrieve saved layout
	respGet2, err := http.Get(server.URL + "/api/canvas/layout?session_id=s1")
	if err != nil {
		t.Fatalf("failed GET layout after save: %v", err)
	}
	defer respGet2.Body.Close()
	var savedLayout struct {
		Nodes []map[string]any `json:"nodes"`
		Zoom  float64          `json:"zoom"`
	}
	if err := json.NewDecoder(respGet2.Body).Decode(&savedLayout); err != nil {
		t.Fatalf("failed to decode saved layout: %v", err)
	}
	if len(savedLayout.Nodes) != 1 || savedLayout.Zoom != 1.5 {
		t.Fatalf("saved layout mismatch: %+v", savedLayout)
	}
}

// K2: canvas layouts survive a server restart (SQLite, not memory), and
// megabyte blobs refuse instead of landing in a row.
func TestCanvas_LayoutSurvivesRestart(t *testing.T) {
	dbPath := "test_canvas_restart.db"
	cleanup := func() {
		os.Remove(dbPath)
		os.Remove(dbPath + ".lock")
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}
	defer cleanup()

	serve := func() (*api.Server, func()) {
		t.Helper()
		l, err := ledger.OpenLedger(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		sm := session.NewManager(l)
		jr := jobs.NewRunner(l, nil, 1)
		sup := supervisor.NewSupervisor(l)
		wd := watchdog.NewTaskWatchdog(l, sup)
		return api.NewServer(l, sm, jr, sup, wd), func() { l.Close() }
	}
	put := func(srv *api.Server, body string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/canvas/layout?session_id=s9", strings.NewReader(body))
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w.Code
	}
	get := func(srv *api.Server) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/canvas/layout?session_id=s9", nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w.Body.String()
	}

	srv1, close1 := serve()
	if code := put(srv1, `{"nodes":[{"id":"n1"}],"zoom":2}`); code != http.StatusOK {
		t.Fatalf("save: %d", code)
	}
	close1()

	// Fresh server, same DB file: layout must still be there.
	srv2, close2 := serve()
	defer close2()
	if got := get(srv2); !strings.Contains(got, "n1") {
		t.Errorf("layout must survive restart, got %q", got)
	}

	// Oversize blobs refuse.
	big := `{"pad":"` + strings.Repeat("x", 300*1024) + `"}`
	if code := put(srv2, big); code != http.StatusBadRequest {
		t.Errorf("300KB layout must 400, got %d", code)
	}
}
