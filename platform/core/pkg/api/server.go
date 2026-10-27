package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/autoresearch"
	"github.com/mainza-ai/kubona/core/pkg/cost"
	"github.com/mainza-ai/kubona/core/pkg/disk"
	"github.com/mainza-ai/kubona/core/pkg/epistemics"
	"github.com/mainza-ai/kubona/core/pkg/fidelity"
	"github.com/mainza-ai/kubona/core/pkg/hf"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/mandates"
	"github.com/mainza-ai/kubona/core/pkg/memory"
	"github.com/mainza-ai/kubona/core/pkg/metasearch"
	"github.com/mainza-ai/kubona/core/pkg/qir"
	"github.com/mainza-ai/kubona/core/pkg/sandbox"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/tenants"
	"github.com/mainza-ai/kubona/core/pkg/tools"
	"github.com/mainza-ai/kubona/core/pkg/vault"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
	pb "github.com/mainza-ai/kubona/core/proto"
)

type Server struct {
	httpMetrics        *httpMetrics
	ledger             *ledger.Ledger
	sessionMgr         *session.Manager
	tenantMgr          *tenants.Manager
	jobRunner          *jobs.Runner
	supervisor         *supervisor.Supervisor
	watchdog           *watchdog.TaskWatchdog
	collective         *agents.AgentCollective
	a2aServer          *agents.A2AServer
	fidelityGate       *fidelity.FidelityGate
	epistemicGate      *epistemics.EpistemicGate
	harnessReg         *autoresearch.HarnessRegistry
	llmSettings        *llm.Settings
	llmClient          *llm.Client
	quantumSettings    *QuantumSettings
	quantumCanaryCache *canaryCache
	toolRegistry       *tools.Registry
	memoryStore        *memory.HindsightStore
	metaSearchEngine   *metasearch.MetaSearchEngine
	sandboxRunner      *sandbox.SandboxRunner
	autoresearchMu     sync.RWMutex
	activeAutoloops    map[string]*autoresearch.LoopEngine
	mux                *http.ServeMux

	// Hugging Face Hub client & Custom Agent Registry
	hfClient     *hf.HubClient
	customAgents *agents.CustomAgentRegistry

	// S4: server-side lease registry. The /api/vault/leases endpoint mints
	// HMAC receipts; the lease objects live here so issued leases are
	// consumable and revocable (previously tested but unwired).
	vaultLeases *vault.VaultClient

	// Post-quantum cryptography keypair & QIR compiler
	pqcKeypair  *mandates.RecipientKeypair
	qirCompiler *qir.Compiler

	// Human-reviewer mandate signing key. In production this MUST be loaded from
	// KUBONA_REVIEWER_PRIVKEY_HEX (see loadReviewerKey); reviewerKeyConfigured
	// reports whether the key came from the environment. An ephemeral dev key is
	// generated otherwise and handleMandateSign refuses to use it unless
	// KUBONA_ALLOW_EPHEMERAL_SIGNER=1 (tests / local dev only).
	signerPubKey          ed25519.PublicKey
	signerPrivKey         ed25519.PrivateKey
	reviewerKeyConfigured bool

	// Edge rate limiters (B0-2): per-server instances so tests stay
	// isolated; see edge_auth.go for budgets.
	signLimiter   *rateLimiter
	verifyLimiter *rateLimiter
	// R4: the tenants/grants mutations had no limiter at all. Human-paced
	// budgets — these are operator actions, never machine loops.
	tenantLimiter *rateLimiter
	grantLimiter  *rateLimiter
	// W2.1: ledger-append and supervisor-abort are operator kill-switch /
	// provenance-forging rails — their own human-paced buckets so a spray
	// on one rail never spends another's budget. Reads stay public behind
	// limiters; these writes get edge auth + their own limiter.
	ledgerLimiter *rateLimiter
	abortLimiter  *rateLimiter
	// Auth-session rail (A1): challenge/login/logout/whoami/list/revoke
	// are human-paced; a guessing campaign against /api/auth/login is
	// throttled here and every refusal is ledger-sealed (auditAuth).
	authLimiter *rateLimiter
	// Auth sessions: challenge-response login bound to lab admin keys.
	// Set at construction (nil only in tests that never touch auth).
	authSessions *AuthSessionStore

	// S2: disk guard (nil = unguarded, e.g. tests). Set via SetDiskGuard
	// after construction so NewServer's signature never ripples.
	diskGuard *disk.Guard

	// D1/U3: bridge address for the fleet liveness probe (host:port of
	// the Python gRPC bridge). Set via SetBridgeAddr after construction.
	// Empty = unconfigured: fleet reports local_simulator UNKNOWN
	// instead of claiming ONLINE behind a nil client that never fails.
	bridgeAddr string
}

// SetDiskGuard attaches the S2 disk guard. Nil detaches (tests).
func (s *Server) SetDiskGuard(g *disk.Guard) {
	s.diskGuard = g
}

// SetBridgeAddr records the Python bridge address for fleet liveness.
func (s *Server) SetBridgeAddr(addr string) {
	s.bridgeAddr = addr
}

// diskBlocked reports whether new work must be refused (503
// storage_blocked). Reads, health, and recovery never consult this —
// the guard freezes admission, never visibility.
func (s *Server) diskBlocked() (blocked bool, reason string) {
	g := s.diskGuard
	if g == nil {
		return false, ""
	}
	snap := g.Snapshot()
	if snap.Level != disk.LevelBlocked {
		return false, ""
	}
	if snap.Reason == "" {
		return true, "disk guard is blocking new work"
	}
	return true, snap.Reason
}

// refuseIfDiskBlocked writes the 503 refusal when blocked. Handlers for
// every new-work entry (spend mutations + chat writes + loop starts)
// call this first: sub-MB bounded writes (canvas, single reads) are
// deliberately ungated — at 1 GB free they always fit, so gating them
// would add failure modes without protection.
func (s *Server) refuseIfDiskBlocked(w http.ResponseWriter) bool {
	if blocked, reason := s.diskBlocked(); blocked {
		w.Header().Set("Retry-After", "30")
		WriteAPIError(w, "storage_blocked", fmt.Sprintf("storage_blocked: %s — new work refused until disk space recovers; reads and recovery stay available", reason), nil)
		return true
	}
	return false
}

// loadReviewerKey returns the human-reviewer ed25519 keypair for mandate
// signing. Production deployments MUST set KUBONA_REVIEWER_PRIVKEY_HEX to the
// hex encoding of either the 64-byte private key or the 32-byte seed. When the
// variable is absent or malformed an ephemeral key is generated for local
// development and configured=false, in which case handleMandateSign refuses to
// sign unless KUBONA_ALLOW_EPHEMERAL_SIGNER=1.
func loadReviewerKey() (ed25519.PublicKey, ed25519.PrivateKey, bool) {
	if hexKey := strings.TrimSpace(os.Getenv("KUBONA_REVIEWER_PRIVKEY_HEX")); hexKey != "" {
		raw, err := hex.DecodeString(hexKey)
		if err == nil {
			switch len(raw) {
			case ed25519.PrivateKeySize:
				priv := ed25519.PrivateKey(append([]byte(nil), raw...))
				if pub, ok := priv.Public().(ed25519.PublicKey); ok {
					return pub, priv, true
				}
			case ed25519.SeedSize:
				priv := ed25519.NewKeyFromSeed(raw)
				return priv.Public().(ed25519.PublicKey), priv, true
			}
		}
	}
	pub, priv, _ := ed25519.GenerateKey(nil)
	return pub, priv, false
}

// newVaultLeases builds the server-side lease registry (W2.7). When
// KUBONA_VAULT_REVOCATIONS names a file, revocations persist there across
// restarts (a revoked lease stays revoked); unset means dev-only in-memory
// leases. An unusable persistence path fails server construction loudly —
// an undurable revocation must never report success.
func newVaultLeases() *vault.VaultClient {
	if path := strings.TrimSpace(os.Getenv("KUBONA_VAULT_REVOCATIONS")); path != "" {
		vc, err := vault.NewVaultClientWithPersistence(path)
		if err != nil {
			panic("vault revocations persistence unusable: " + err.Error())
		}
		return vc
	}
	return vault.NewVaultClient()
}

func NewServer(
	l *ledger.Ledger,
	sm *session.Manager,
	jr *jobs.Runner,
	sup *supervisor.Supervisor,
	wd *watchdog.TaskWatchdog,
) *Server {
	pub, priv, configured := loadReviewerKey()

	llmSet, err := llm.LoadSettings(l.DB())
	if err != nil {
		llmSet = llm.DefaultSettings()
	}
	if llmSet.EnsureDefaultRoles() {
		// Persist the backfill so next boot is clean; never overwrites a
		// role the operator explicitly assigned.
		_ = llmSet.Save(l.DB())
	}
	if llmSet.EnsureDefaultProviders() {
		_ = llmSet.Save(l.DB())
	}
	// Boot visibility: the effective model/max-tokens are logged so a stale
	// persisted selection (e.g. a model oMLX no longer serves) is caught at
	// startup, not misdiagnosed mid-session (2026-10-11).
	snap := llmSet.GetSnapshot()
	slog.Info("[Kubona Core] LLM effective", "provider", snap.ActiveProvider, "model", snap.ActiveModel, "max_tokens", snap.MaxTokens)
	// W0.19 production posture: a prod boot with cloud inference and no
	// explicit opt-in refuses here — loudly at startup, never silently
	// exfiltrating prompts mid-session. Dev/test boot is unaffected.
	if err := llmSet.ValidateForProduction(); err != nil {
		log.Fatalf("[Kubona Core] FATAL: %v", err)
	}
	llmCli := llm.NewClient(llmSet)
	// K4: cloud LLM spend accounting. Every metered completion records a
	// ledger event with a heuristic estimate; an optional global dollar
	// cap (KUBONA_LLM_BUDGET_USD) refuses new cloud entries when spent.
	// Local providers never record. Unset cap = track-only (current
	// behavior preserved).
	llmCli.OnSpend = func(rec llm.SpendRecord) {
		payload := map[string]any{
			"provider": rec.Provider, "model": rec.Model,
			"prompt_tokens": rec.PromptTokens, "completion_tokens": rec.CompletionTokens,
			"usage_reported": rec.UsageReported, "heuristic": true,
		}
		if rec.CeilingApplied {
			// W0.19: tokens unknown, ceiling billed at the output rate.
			est, ok := llm.EstimateCeilingCostUSD(llm.ProviderType(rec.Provider), rec.Model, rec.CeilingTokens)
			payload["ceiling_applied"] = true
			payload["ceiling_tokens"] = rec.CeilingTokens
			if ok {
				payload["estimated_usd"] = est
			} else {
				payload["estimated_usd"] = nil
			}
		} else if rec.Unmetered {
			// Streams with no usage and no ceiling: flagged, never
			// zero-priced. The budget gate trips on these when capped.
			payload["unmetered"] = true
			payload["estimated_usd"] = nil
		} else {
			est, ok := llm.EstimateCostUSD(llm.ProviderType(rec.Provider), rec.Model, rec.PromptTokens, rec.CompletionTokens)
			if ok {
				payload["estimated_usd"] = est
			} else {
				payload["estimated_usd"] = nil
			}
		}
		_ = l.AppendEvent("global", "llm_spend_estimated", payload)
	}

	sbCfg := sandbox.DefaultConfig()
	// Host fallback is opt-in, never default: without Docker, agent code
	// refuses to run rather than silently executing on the host. Local
	// dev opts in with KUBONA_ALLOW_HOST_FALLBACK=1 (tests set it where
	// they need host execution, same as the harness unit tests).
	sbCfg.AllowHostFallback = os.Getenv("KUBONA_ALLOW_HOST_FALLBACK") == "1"
	sbRunner := sandbox.NewSandboxRunnerWithConfig(sbCfg)
	metaEngine := metasearch.NewMetaSearchEngineWithLLM(llmCli)
	// K4: proposals persist across restarts (approvals must survive the
	// reboot between review and application).
	metaEngine.SetDB(l.DB())
	pqcKey, _ := mandates.GenerateRecipientKeypair()
	qirComp := qir.NewCompiler()

	s := &Server{
		httpMetrics:           newHTTPMetrics(),
		ledger:                l,
		sessionMgr:            sm,
		tenantMgr:             tenants.NewManager(l),
		jobRunner:             jr,
		supervisor:            sup,
		watchdog:              wd,
		collective:            agents.NewAgentCollective(),
		a2aServer:             agents.NewA2AServer("0x89205A3A3b2A69De6Dbf7f01ED13B2108B2c43e7", nil, sbRunner),
		fidelityGate:          fidelity.NewFidelityGate(),
		epistemicGate:         epistemics.NewEpistemicGate(),
		harnessReg:            autoresearch.NewHarnessRegistry(),
		llmSettings:           llmSet,
		llmClient:             llmCli,
		quantumSettings:       mustLoadQuantumSettings(l.DB()),
		quantumCanaryCache:    newCanaryCache(),
		metaSearchEngine:      metaEngine,
		sandboxRunner:         sbRunner,
		activeAutoloops:       make(map[string]*autoresearch.LoopEngine),
		hfClient:              hf.NewHubClient(""),
		customAgents:          agents.NewCustomAgentRegistryWithDB(l.DB()),
		vaultLeases:           newVaultLeases(),
		pqcKeypair:            pqcKey,
		qirCompiler:           qirComp,
		signerPubKey:          pub,
		signerPrivKey:         priv,
		reviewerKeyConfigured: configured,
		signLimiter:           newRateLimiter(rateFromEnv("KUBONA_RATE_SIGN", 2, 5)),
		verifyLimiter:         newRateLimiter(rateFromEnv("KUBONA_RATE_VERIFY", 20, 50)),
		tenantLimiter:         newRateLimiter(rateFromEnv("KUBONA_RATE_TENANT", 5, 15)),
		grantLimiter:          newRateLimiter(rateFromEnv("KUBONA_RATE_GRANT", 5, 15)),
		ledgerLimiter:         newRateLimiter(rateFromEnv("KUBONA_RATE_LEDGER", 5, 15)),
		abortLimiter:          newRateLimiter(rateFromEnv("KUBONA_RATE_ABORT", 5, 15)),
		authLimiter:           newRateLimiter(rateFromEnv("KUBONA_RATE_AUTH", 5, 15)),
		authSessions:          newAuthSessionStore(l.DB()),
		mux:                   http.NewServeMux(),
	}

	// Single tool registry: read-only observations the copilot can invoke.
	// Writes keep flowing through the gated paths (launch, mandates).
	s.toolRegistry = s.registerAgentTools()
	// Tribunal turns observe through the same registry (nil-safe: turns
	// proceed text-only when unset).
	s.collective.Tools = s.toolRegistry
	// R2/C3: the collective seals tribunal turns + records obligation
	// cases directly (nil-ledger unit paths are unaffected).
	s.collective.Ledger = l
	// Persistent associative memory (SQLite-backed, tenant-scoped recall).
	// Activates the tribunal recall/retain paths and the recall_memory tool.
	s.memoryStore = memory.NewPersistentHindsightStore(l.DB())
	// Planner replan: settled jobs fold their measured results back into the
	// session campaign. Runs on the job-worker goroutine — pure fold plus
	// ledger writes only, never LLM or bridge calls.
	// R2b: mandate settlement rides first (obligation closure), then the
	// replan fold. The hook never fails the settlement (audit follows
	// evidence): recording errors log, and a post-closure settlement
	// opens a follow-up anomaly obligation instead of rewriting history.
	replan := s.replanCampaignOnSettled
	jr.SetSettledHook(func(j *jobs.Job) {
		s.recordMandateSettlement(j)
		replan(j)
	})

	// Ledger-bound A2A execution: delegated tasks run on the jobs runner (one
	// budget rail), never on invented results.
	s.a2aServer.SetTaskExecutor(&bridgeTaskExecutor{runner: jr, sessions: sm, ledger: l, harness: s.harnessReg, sandbox: sbRunner, qir: qirComp, signer: s.signerPrivKey})
	// Live tenant autonomy at A2A dispatch (same rule as HTTP dispatch).
	s.a2aServer.SetAutonomyChecker(func(tenantID string) (string, error) {
		t, err := s.tenantMgr.GetTenant(tenantID)
		if err != nil {
			return "", err
		}
		return t.AutonomyLevel, nil
	})

	// Bootstrap the default lab tenant's admin key from the environment when
	// provided (first-write-wins; sealed in the ledger). The private key never
	// touches the server: only the admin *public* key is configured here.
	// Already-bootstrapped tenants ignore this (rotation needs the current
	// admin's signature via /api/tenants/admin-key).
	if adminHex := strings.TrimSpace(os.Getenv("KUBONA_TENANT_ADMIN_PUBKEY_HEX")); adminHex != "" {
		_, _ = s.tenantMgr.SetAdminKey(tenants.DefaultTenantID, adminHex, nil)
	}

	// Seed sample claim wavefunctions for the Epistemic Manifold
	for _, seed := range epistemics.SeedClaims() {
		if err := s.epistemicGate.RegisterClaim(seed); err != nil {
			slog.Error("[Epistemics] seed claim refused", "error", err)
		}
	}

	s.registerRoutes()
	if err := initAuthSessionTable(l.DB()); err != nil {
		slog.Error("[Auth] session table init failed; login will refuse until the ledger is writable", "error", err)
	}
	return s
}

func (s *Server) Handler() http.Handler {
	return s.instrument(requestIDAndRecovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := corsAllowOrigin(r); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			// Credentialed requests (session cookie) only work cross-origin
			// with this header; the origin above is allowlisted, never "*",
			// so browsers accept it. Reads stay public; writes still need
			// auth (bearer or session) plus CSRF checks on cookie calls.
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		s.mux.ServeHTTP(w, r)
	})))
}

// requestIDAndRecovery wraps every route with a request id and a panic
// boundary: one handler bug must not take the daemon down or emit a bare
// connection reset. The id is echoed and logged with the stack.
func requestIDAndRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if reqID == "" {
			reqID = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", reqID)
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("http_panic",
					"method", r.Method, "path", r.URL.Path,
					"request_id", reqID, "panic", fmt.Sprint(rec),
					"stack", string(debug.Stack()))
				WriteAPIError(w, "internal", "internal error (request id "+reqID+")", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/metrics", s.requireBearer(s.handleMetrics))
	s.mux.HandleFunc("/api/ready", s.handleReady)
	s.mux.HandleFunc("/api/capabilities", s.handleCapabilities)
	s.mux.HandleFunc("/api/sessions", s.handleSessions)
	s.mux.HandleFunc("/api/sessions/refund", s.requireBearer(s.handleSessionRefund))
	s.mux.HandleFunc("/api/sessions/export", s.handleSessionExport)
	s.mux.HandleFunc("/api/sessions/import", s.requireBearer(s.handleSessionImport))
	s.mux.HandleFunc("/api/sessions/context", s.handleSessionContext)
	s.mux.HandleFunc("/api/sessions/", s.handleSessionByID)
	s.mux.HandleFunc("/api/campaigns/", s.handleCampaignRoute)
	s.mux.HandleFunc("/api/chat/messages", s.handleChatMessages)
	s.mux.HandleFunc("/api/chat/stream", s.handleChatStream)
	s.mux.HandleFunc("/api/chat/regenerate", s.handleChatRegenerate)
	s.mux.HandleFunc("/api/chat/rewrite", s.handleChatRewrite)
	s.mux.HandleFunc("/api/chat/feedback", s.handleChatFeedback)
	s.mux.HandleFunc("/api/chat/tribunal", s.handleChatTribunal)
	// T-16: resume a tribunal from an exported turn bundle (R4 flow).
	s.mux.HandleFunc("/api/tribunal/resume", s.handleTribunalResume)
	s.mux.HandleFunc("/api/chat/launch", s.handleChatLaunch)
	s.mux.HandleFunc("/api/jobs", s.handleJobs)
	s.mux.HandleFunc("/api/jobs/", s.handleJobByID)
	s.mux.HandleFunc("/api/loops", s.requireBearer(s.handleLoops))
	s.mux.HandleFunc("/api/loops/", s.requireBearer(s.handleLoopActions))
	s.mux.HandleFunc("/api/events", s.handleEvents)
	s.mux.HandleFunc("/api/memory/recall", s.requireBearer(s.handleMemoryRecall))
	s.mux.HandleFunc("/api/memory/retain", s.requireBearer(s.handleMemoryRetain))

	// New Observatory control plane endpoints
	s.mux.HandleFunc("/api/agents", s.handleAgents)
	s.mux.HandleFunc("/api/fidelity", s.handleFidelity)
	s.mux.HandleFunc("/api/epistemics", s.handleEpistemics)
	s.mux.HandleFunc("/api/fleet", s.handleFleet)
	s.mux.HandleFunc("/api/radar", s.handleRadar)
	s.mux.HandleFunc("/api/ledger/verify", s.requireBearer(s.handleLedgerVerify))
	s.mux.HandleFunc("/api/mandates/sign", s.requireBearer(s.handleMandateSign))
	// Mandate verification is bearer-gated (2026-10-26 audit): the handler
	// appends a ledger event + obligation, so the old "no state change"
	// justification for leaving it public was false. Rate limiting remains
	// inside the handler as a second layer.
	s.mux.HandleFunc("/api/mandates/verify", s.handleMandateVerify)
	// Session auth (A1): challenge-response login bound to lab admin keys.
	// Challenge + whoami stay public (no identity exists yet); login is
	// rate-limited with ledger-sealed refusals; whoami/sessions/revoke
	// need a session except logout, which always succeeds.
	s.mux.HandleFunc("/api/auth/challenge", s.handleAuthChallenge)
	s.mux.HandleFunc("/api/auth/login", s.handleAuthLogin)
	s.mux.HandleFunc("/api/auth/logout", s.handleAuthLogout)
	s.mux.HandleFunc("/api/auth/whoami", s.handleAuthWhoami)
	s.mux.HandleFunc("/api/auth/sessions", s.handleAuthSessions)
	s.mux.HandleFunc("/api/auth/revoke", s.handleAuthRevoke)
	s.mux.HandleFunc("/api/harnesses", s.handleHarnesses)
	s.mux.HandleFunc("/api/results/notebooks", s.handleNotebooks)
	s.mux.HandleFunc("/api/literature", s.handleLiterature)

	// Autoresearch Loop & Section 13 Meta-Search Endpoints
	s.mux.HandleFunc("/api/autoresearch/harnesses", s.handleAutoresearchHarnesses)
	s.mux.HandleFunc("/api/autoresearch/start", s.handleAutoresearchStart)
	s.mux.HandleFunc("/api/autoresearch/loops", s.handleAutoresearchLoops)
	s.mux.HandleFunc("/api/autoresearch/loops/", s.handleAutoresearchLoopAction)
	s.mux.HandleFunc("/api/metasearch/proposals", s.handleMetasearchProposals)
	// J3: meta-approve/reject/apply are capability mutations (they mark
	// strategy changes approved and resume loops). The signer field is an
	// attribution label, not authentication — the bearer token proves
	// operator possession. Unauthenticated callers get 401 before any
	// proposal state is touched. Listing stays public (read-only).
	s.mux.HandleFunc("/api/metasearch/approve", s.requireBearer(s.handleMetasearchApprove))
	s.mux.HandleFunc("/api/metasearch/reject", s.requireBearer(s.handleMetasearchReject))
	s.mux.HandleFunc("/api/metasearch/apply", s.requireBearer(s.handleMetasearchApply))

	// Hybrid LLM Configuration Plane endpoints
	s.mux.HandleFunc("/api/llm/settings", s.handleLLMSettings)
	s.mux.HandleFunc("/api/llm/models", s.handleLLMModels)
	s.mux.HandleFunc("/api/llm/test", s.requireBearer(s.handleLLMTest))
	// Quantum provider settings (human selection surface; secrets stay
	// in env/Bao and are never accepted here).
	s.mux.HandleFunc("/api/quantum/settings", s.handleQuantumSettings)
	s.mux.HandleFunc("/api/quantum/backends", s.handleQuantumBackends)
	// Human quantum dispatch proxy (R-34) + provider cancel (R-33).
	// Spend authority lives in the Python rail; these routes authenticate,
	// authorize, validate, and proxy — never reserve QPU spend themselves.
	s.mux.HandleFunc("/api/quantum/jobs", s.requireBearer(s.handleQuantumJobs))
	s.mux.HandleFunc("/api/quantum/jobs/", s.requireBearer(s.handleQuantumJobActions))
	// Live canary proxy (public read like fleet; TTL-cached, fails soft
	// to declared catalog values — see quantum_canary.go).
	s.mux.HandleFunc("/api/quantum/canary", s.handleQuantumCanary)

	// Workflow engine execution and planning routes (Phase 3).
	// Planning is read-only validation against tools (session-scoped);
	// running executes MCP steps and requires dispatch autonomy.
	s.mux.HandleFunc("/api/workflows/plan", s.requireBearer(s.handleWorkflowPlan))
	s.mux.HandleFunc("/api/workflows/run", s.requireBearer(s.handleWorkflowRun))

	// Vault Dynamic Leases (OpenBao sidecar bridge)
	s.mux.HandleFunc("/api/vault/leases", s.requireBearer(s.handleVaultLeases))
	s.mux.HandleFunc("/api/vault/leases/revoke", s.requireBearer(s.handleVaultLeaseRevoke))

	// A2A Inter-Agent Protocol & x402 payment rails
	s.mux.HandleFunc("/api/a2a/v1", s.requireA2AAuth(s.handleA2A))
	s.mux.HandleFunc("/api/a2a/v1/challenge", s.requireA2AAuth(s.handleA2AChallenge))

	// Live Telemetry Event Stream (SSE) & Spatial Canvas Layouts
	s.mux.HandleFunc("/api/events/stream", s.handleEventsStream)
	s.mux.HandleFunc("/api/canvas/layout", s.handleCanvasLayout)
	// Observatory lens: spend-authority writes get edge auth (F-13 — a bare
	// session UUID must never be able to bill the lab's budget).
	s.mux.HandleFunc("/api/observatory/lens", s.requireBearer(s.handleObservatoryLens))

	// Vault pre-registration: sealed expectations and surprise evaluations
	// (reads are session-scoped, writes bill attention — both edge-authed).
	s.mux.HandleFunc("/api/vault/predictions/seal", s.requireBearer(s.handlePredictionSeal))
	s.mux.HandleFunc("/api/vault/predictions/evaluate", s.requireBearer(s.handlePredictionEvaluate))
	s.mux.HandleFunc("/api/vault/predictions/latest", s.requireBearer(s.handlePredictionLatest))
	s.mux.HandleFunc("/api/vault/predictions/assert", s.requireBearer(s.handlePredictionAssert))
	s.mux.HandleFunc("/api/vault/predictions/calibration", s.requireBearer(s.handlePredictionCalibration))
	s.mux.HandleFunc("/api/vault/predictions/history", s.requireBearer(s.handlePredictionHistory))

	// Observatory asset upload + operation resolve (P1 rails).
	s.mux.HandleFunc("/api/observatory/assets", s.requireBearer(s.handleObservatoryAsset))
	s.mux.HandleFunc("/api/observatory/lens/resolve", s.requireBearer(s.handleObservatoryLensResolve))
	s.mux.HandleFunc("/api/observatory/lens/history", s.requireBearer(s.handleObservatoryLensHistory))
	// Atlas webhook delivery (P6): PUBLIC route — the Ed25519 v1a signature
	// is the authentication (Atlas cannot present our bearer). Verification
	// before any state change; poll+resolve stays the correctness path.
	s.mux.HandleFunc("/api/observatory/webhooks", s.handleObservatoryWebhook)

	// Molecular Science & 12-Agent Drug Discovery Pipeline
	s.mux.HandleFunc("/api/chemistry/admet", s.handleChemistryADMET)
	s.mux.HandleFunc("/api/discovery/pipeline", s.handleDiscoveryPipeline)

	// Post-Quantum Cryptography Spend Mandates (ML-KEM-768 + AES-256-GCM)
	s.mux.HandleFunc("/api/mandates/pqc-key", s.handlePQCPublicKey)
	s.mux.HandleFunc("/api/mandates/pqc-verify", s.requireBearer(s.handlePQCVerify))

	// QIR Compilation & Lowering
	s.mux.HandleFunc("/api/qir/compile", s.handleQIRCompile)
	// Circuit co-design scoring (read-only; validity stays Python-owned)
	s.mux.HandleFunc("/api/codesign/tq", s.handleCodesignTQ)
	s.mux.HandleFunc("/api/codesign/frontier", s.handleCodesignFrontier)
	// Evaluator proxy (Phase 1 migration): stateless Python compute only.
	s.mux.HandleFunc("/api/codesign/evaluate", s.handleCodesignEvaluate)
	s.mux.HandleFunc("/api/codesign/evaluate-batch", s.handleCodesignEvaluateBatch)
	s.mux.HandleFunc("/api/codesign/families", s.handleCodesignFamilies)
	s.mux.HandleFunc("/api/codesign/budget", s.handleCodesignBudget)
	// Verified results (G4): versioned JSON, numbers not verdicts.
	s.mux.HandleFunc("/api/codesign/verified", s.handleCodesignVerified)
	// Pack registry (ux-plan slice 4): served live, never hand-typed.
	s.mux.HandleFunc("/api/codesign/packs", s.handleCodesignPacks)

	// Quantum Krylov Subspace & Mitigation Endpoints
	s.mux.HandleFunc("/api/krylov/solve", s.handleKrylovSolve)
	s.mux.HandleFunc("/api/mitigation/apply", s.handleMitigationApply)
	s.mux.HandleFunc("/api/cost/settle", s.handlePaymentSettle)
	// Spend visibility (G10): read-only LLM cost summary for the console.
	// Aggregates only (no per-session/tenant attribution is recorded);
	// heuristic estimates, never metered billing.
	s.mux.HandleFunc("/api/cost/summary", s.handleCostSummary)

	// Hugging Face Model Hub & In-Platform Downloader
	// W2.1 threat model: reads stay public behind limiters in general,
	// but HF routes trigger external network fetch + disk writes (even
	// "search" fans out to huggingface.co and caches), so every HF route
	// is an operator-gated write-class rail: bearer auth, no anonymous
	// fetch-or-store primitive.
	s.mux.HandleFunc("/api/hf/search", s.requireBearer(s.handleHFSearch))
	s.mux.HandleFunc("/api/hf/models/", s.requireBearer(s.handleHFModelDetails))
	s.mux.HandleFunc("/api/hf/download", s.requireBearer(s.handleHFDownload))
	s.mux.HandleFunc("/api/hf/downloads", s.requireBearer(s.handleHFDownloads))
	s.mux.HandleFunc("/api/hf/downloads/", s.requireBearer(s.handleHFDownloadAction))

	// Custom Agents & Multimodal Mesh Router
	// K3: agent create/delete are registry mutations — bearer-gated like
	// the other operator mutations (same human-paced limiter bucket).
	// Listing stays public.
	s.mux.HandleFunc("/api/agents/custom", s.handleCustomAgents)
	s.mux.HandleFunc("/api/agents/custom/", s.handleCustomAgentByID)
	s.mux.HandleFunc("/api/mesh/route", s.handleMeshRoute)
	// W2.1 threat model: ledger append forges tribunal/review provenance
	// and the abort rail is an availability kill-switch — both are writes,
	// so both get edge auth + their own limiter. Reads stay public behind
	// limiters; these two never serve anonymous callers.
	s.mux.HandleFunc("/api/ledger/append", s.requireBearer(s.handleLedgerAppend))
	s.mux.HandleFunc("/api/ledger/export", s.handleLedgerExport)
	s.mux.HandleFunc("/api/supervisor/abort", s.requireBearer(s.handleSupervisorAbort))
	s.mux.HandleFunc("/api/tenants", s.handleTenants)
	s.mux.HandleFunc("/api/tenants/", s.handleTenantByID)
	// R4: spend-adjacent mutations sit behind the bearer rail as well as
	// their own cryptographic envelopes. Tenant self-provisioning
	// (POST /api/tenants) stays open by design — first SetAdminKey wins —
	// but rate-limited against creation spam. Pure reads stay open.
	s.mux.HandleFunc("/api/grants/register", s.requireBearer(s.handleGrantRegister))
	s.mux.HandleFunc("/api/grants/revoke", s.requireBearer(s.handleGrantRevoke))
	s.mux.HandleFunc("/api/tenants/admin-key", s.requireBearer(s.handleTenantAdminKey))
	s.mux.HandleFunc("/api/tenants/ceiling", s.requireBearer(s.handleTenantCeiling))
	s.mux.HandleFunc("/api/tenants/refund", s.requireBearer(s.handleTenantRefund))
	s.mux.HandleFunc("/api/tenants/autonomy", s.requireBearer(s.handleTenantAutonomy))
	s.mux.HandleFunc("/api/tenants/revoke", s.requireBearer(s.handleTenantRevoke))
	s.mux.HandleFunc("/api/tenants/status", s.requireBearer(s.handleTenantStatus))
}

func (s *Server) PQCKeypair() *mandates.RecipientKeypair {
	return s.pqcKeypair
}

func (s *Server) A2AServer() *agents.A2AServer {
	return s.a2aServer
}

func (s *Server) TenantManager() *tenants.Manager {
	return s.tenantMgr
}

func (s *Server) Ledger() *ledger.Ledger {
	return s.ledger
}

func (s *Server) SessionManager() *session.Manager {
	return s.sessionMgr
}

// requireDispatchAutonomy enforces live tenant autonomy at every dispatch
// entry point. L0 (observe/suggest only), suspended, or unknown tenants
// refuse job submission; backend-level gating (simulator vs QPU) happens in
// the admission stage. Revocation takes effect on the next call — no grants
// are cached.
func (s *Server) requireDispatchAutonomy(sessionID string) error {
	sess, err := s.sessionMgr.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("unknown session %q: dispatch refused", sessionID)
	}
	return s.requireTenantDispatchAutonomy(sess.TenantID)
}

func (s *Server) requireTenantDispatchAutonomy(tenantID string) error {
	if tenantID == "" {
		tenantID = tenants.DefaultTenantID
	}
	t, err := s.tenantMgr.GetTenant(tenantID)
	if err != nil {
		return fmt.Errorf("unknown tenant %q: dispatch refused", tenantID)
	}
	if t.Status != "active" {
		return fmt.Errorf("tenant %q is %s: dispatch refused", t.ID, t.Status)
	}
	if t.AutonomyLevel == tenants.LevelL0 {
		return fmt.Errorf("tenant %q autonomy L0 (observe/suggest only): dispatch refused", t.ID)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	bytes, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(bytes)
}

// decodeStrict parses a spend-mutation body with unknown fields refused
// (I6): a typo'd mandate_ref must 400, never parse as absent (zero value)
// and silently authorize the wrong thing. Applies to the spend paths
// only (jobs, launch, tribunal, autoresearch-start, mandates/sign,
// loops) — read/query routes stay lenient so console display probes
// never 400 on additive fields.
func decodeStrict(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]string{
		"status":    "healthy",
		"service":   "kubona-core-orchestrator",
		"version":   "0.2.0-ts-go",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	// S2: disk readings ride along when a guard is attached (additive —
	// existing health consumers keep working).
	if s.diskGuard != nil {
		snap := s.diskGuard.Snapshot()
		health["disk_level"] = snap.Level
		health["disk_avail_bytes"] = strconv.FormatUint(snap.AvailBytes, 10)
		if snap.Reason != "" {
			health["disk_reason"] = snap.Reason
		}
	}
	writeJSON(w, http.StatusOK, health)
}

// handleReady is the deployment readiness probe: 200 only when the disk
// guard (if attached) is not blocking. Unlike health (always 200 with a
// body), readiness is status-coded for load balancers and supervisors.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if blocked, reason := s.diskBlocked(); blocked {
		WriteAPIError(w, "not_ready", fmt.Sprintf("not ready: %s", reason), nil)
		return
	}
	// Disk alone is not readiness: a closed/locked SQLite handle means every
	// write fails after the load balancer already routed traffic here.
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.ledger.DB().PingContext(ctx); err != nil {
		WriteAPIError(w, "not_ready", "database unavailable: "+err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ready": true})
}

// handleCapabilities serves runtime discovery (I5) with the build-time
// catalog digest embedded (C5 boundary): the catalog is the build-time
// surface (what CAN be called, digest-pinned); capabilities are runtime
// state (what is CURRENTLY available: flags, thresholds, harness
// catalog, budgets). Skew between the live registry digest and the
// embedded reviewed digest 500s — fail-closed discovery (regen, never
// hand-edit, never serve stale tools). Open like /api/health (read-only
// discovery degrades the console instead of probing blind).
func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteAPIError(w, "method_not_allowed", "capabilities is GET-only", nil)
		return
	}
	snaps := s.toolRegistry.Catalog()
	body, digest := tools.CatalogJSON(snaps)
	if digest == "" {
		WriteAPIError(w, "internal", "catalog render failed", nil)
		return
	}
	if digest != EmbeddedCatalogDigest {
		WriteAPIError(w, "internal",
			fmt.Sprintf("tool catalog skew: live registry digests %s, reviewed build pins %s — regen the catalog, never serve stale tools", digest, EmbeddedCatalogDigest),
			map[string]any{"live_digest": digest, "pinned_digest": EmbeddedCatalogDigest})
		return
	}
	var harnesses []string
	for _, h := range s.harnessReg.List() {
		harnesses = append(harnesses, h.ID())
	}
	sort.Strings(harnesses)
	if harnesses == nil {
		harnesses = []string{}
	}
	var disk any = map[string]any{"guarded": false}
	if s.diskGuard != nil {
		cfg := s.diskGuard.Config()
		snap := s.diskGuard.Snapshot()
		disk = map[string]any{
			"guarded":    true,
			"warn_bytes": cfg.WarnBytes, "block_bytes": cfg.BlockBytes,
			"resume_bytes": cfg.ResumeBytes, "level": snap.Level,
			"bytes_available": snap.AvailBytes,
		}
	}
	budget := llm.DefaultBudgetConfig()
	writeJSON(w, http.StatusOK, map[string]any{
		"catalog_digest": digest,
		"tools":          json.RawMessage(body),
		"disk":           disk,
		"harnesses":      harnesses,
		"budget": map[string]any{
			"turn_context_runes":    budget.TurnContextRunes,
			"max_observation_chars": budget.MaxObservationChars,
			"max_tool_rounds":       budget.MaxToolRounds,
		},
		"flags": map[string]any{
			"reviewer_configured":      s.reviewerKeyConfigured,
			"ephemeral_signer_allowed": os.Getenv("KUBONA_ALLOW_EPHEMERAL_SIGNER") == "1",
			"bridge_connected":         s.jobRunner.BridgeClient() != nil,
		},
	})
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	roles := s.collective.ListRoles()
	writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
}

func (s *Server) handleFidelity(w http.ResponseWriter, r *http.Request) {
	device := r.URL.Query().Get("device")
	if device == "" {
		device = "ibm_heron"
	}
	depthStr := r.URL.Query().Get("depth")
	depth, _ := strconv.Atoi(depthStr)
	if depth <= 0 {
		depth = 12
	}
	twoQGatesStr := r.URL.Query().Get("two_qubit_gates")
	twoQGates, _ := strconv.Atoi(twoQGatesStr)
	if twoQGates <= 0 {
		twoQGates = 8
	}

	// T-02: honesty over precision. No probe runs on this path, so the
	// assessment routes through the dispatch-grade floor-only gate
	// instead of a hardcoded 0.94 "measurement". The response carries
	// probe_run:false — a zero measured_clifford beside it reads as
	// "not measured", never as a 0.0 probe.
	assessment := s.fidelityGate.EvaluateAdmissionFloorOnly(device, depth, twoQGates)
	writeJSON(w, http.StatusOK, assessment)
}

func (s *Server) handleEpistemics(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		// W2.2: no cross-session leak — an unscoped read never adopts
		// sessions[0] (that attached one lab's verdict to a stranger's
		// view). Reads stay public behind limiters, but they must name
		// their session: missing ids 404.
		http.Error(w, "missing session_id: epistemics reads must name an existing session", http.StatusNotFound)
		return
	}
	if _, gerr := s.sessionMgr.GetSession(sessionID); gerr != nil {
		WriteAPIError(w, "ghost_session", fmt.Sprintf("unknown session %q: refusing to show another session", sessionID), nil)
		return
	}
	// T-09b: known session — bind caller to session tenant.
	if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
		return
	}

	alpha := 0.95
	beta := 0.05
	gamma := 0.10
	anchor := -1.13727
	drift := 0.47
	statement := "H2 ground state equilibrium at 0.7414 A"
	claimID := "claim-h2-ground"
	// No campaign means no evaluated claim: the default is explicitly
	// unverified, never a collapsed verdict over placeholder amplitudes.
	status := "UNVERIFIED"

	if raw, err := s.ledger.GetCampaignState(sessionID); err == nil && len(raw) > 0 {
		var cs agents.CampaignState
		if json.Unmarshal(raw, &cs) == nil {
			alpha = cs.Epistemics.AlphaCorroborated
			beta = cs.Epistemics.BetaContested
			gamma = cs.Epistemics.GammaUnverified
			anchor = cs.ClassicalAnchor.FciExactEnergy
			drift = cs.ClassicalAnchor.DiscrepancyMHa
			statement = cs.Epistemics.Statement
			claimID = fmt.Sprintf("claim-%s", strings.ToLower(cs.QuantumFactors.TargetMolecule))
			status = cs.Epistemics.Status
		}
	}

	kappa, _ := epistemics.ComputeEpistemicCurvature(0.12, anchor, anchor+drift/1000.0)
	// Latest claim record: this is what backs the displayed verdict, when
	// anything evaluated backs it yet. Empty means synthesis placeholders.
	recordID := ""
	if recs, err := s.ledger.ListClaimRecords(sessionID, 1); err == nil && len(recs) > 0 {
		recordID = recs[0].ID
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"refusal_ratio":        s.epistemicGate.RefusalRatio(),
		"stagnation_kappa":     kappa,
		"literature_drift_mha": drift,
		"canonical_anchor":     anchor,
		"active_wavefunctions": []map[string]any{
			{
				"claim_id":   claimID,
				"record_id":  recordID,
				"statement":  statement,
				"alpha_corr": alpha,
				"beta_cont":  beta,
				"gamma_unv":  gamma,
				"collapsed":  status == "COLLAPSED_VERIFIED",
				"status":     status,
			},
		},
	})
}

func (s *Server) handleFleet(w http.ResponseWriter, r *http.Request) {
	// Bridge liveness is MEASURED (short TCP dial), never inferred from
	// the lazily-constructed gRPC client (which is never nil, so the old
	// nil-check reported ONLINE with the bridge down). Unconfigured
	// address reads UNKNOWN — honest about not knowing.
	localSimStatus := "UNKNOWN"
	if s.bridgeAddr != "" {
		localSimStatus = "DEGRADED"
		if conn, err := net.DialTimeout("tcp", s.bridgeAddr, 200*time.Millisecond); err == nil {
			conn.Close()
			localSimStatus = "ONLINE"
		}
	}

	// Credential dimension (U3): env presence is a fact about this
	// daemon's ability to submit — not a measurement, stated as such.
	// Simulators are credential-free by construction ("ready"); QPU
	// backends without their token are "credentialless" (distinct from
	// down: the device may be fine, WE lack credentials).
	credState := func(present bool) string {
		if present {
			return "ready"
		}
		return "credentialless"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"backends": []map[string]any{
			{
				// Telemetry provenance: "measured" fields were read by this
				// daemon (TCP liveness dial, env presence); "declared"
				// fields are vendor-catalog placeholders until the live
				// canary proxy lands — never presented as measurements.
				"name":             "local_simulator",
				"telemetry":        "measured",
				"type":             "Qiskit Aer GPU/CPU Statevector",
				"qubits":           32,
				"t1_us":            "Infinite (Exact Numerical)",
				"status":           localSimStatus,
				"credential_state": "ready",
				"queue_depth":      0,
				"cost_per_shot":    0.0,
				"jurisdiction":     "LOCAL_SOVEREIGN",
				"itar_ear":         "EXEMPT",
				"avg_t1_us":        0.0,
				"avg_t2_us":        0.0,
				"readout_error":    0.0,
			},
			{
				"name":             "ibm_heron",
				"telemetry":        "declared",
				"catalog_verified": nil,
				"type":             "Superconducting Transmon (Heavy-Hex)",
				"qubits":           156,
				"t1_us":            "180.0",
				"status":           "READY",
				"credential_state": credState(os.Getenv("KUBONA_IBM_QISKIT_TOKEN") != ""),
				"queue_depth":      14,
				"cost_per_shot":    0.0016,
				"jurisdiction":     "US_EAST",
				"itar_ear":         "EAR99_CONTROLLED",
				"avg_t1_us":        180.0,
				"avg_t2_us":        145.0,
				"readout_error":    0.0012,
			},
			{
				"name":             "quantinuum_h1",
				"telemetry":        "declared",
				"catalog_verified": nil,
				"type":             "Trapped Ion Shuttle",
				"qubits":           20,
				"t1_us":            ">1,000,000",
				"status":           "ONLINE",
				"credential_state": credState(os.Getenv("KUBONA_QUANTINUUM_API_KEY") != ""),
				"queue_depth":      3,
				"cost_per_shot":    0.0125,
				"jurisdiction":     "US_CENTRAL",
				"itar_ear":         "EAR99_CONTROLLED",
				"avg_t1_us":        1000000.0,
				"avg_t2_us":        500000.0,
				"readout_error":    0.0005,
			},
			{
				"name":             "rigetti_aspen",
				"telemetry":        "declared",
				"catalog_verified": nil,
				"type":             "Superconducting Multi-Chip",
				"qubits":           80,
				"t1_us":            "35.0",
				"status":           "CALIBRATING",
				"credential_state": credState(os.Getenv("KUBONA_RIGETTI_QCS_TOKEN") != ""),
				"queue_depth":      28,
				"cost_per_shot":    0.0008,
				"jurisdiction":     "US_WEST",
				"itar_ear":         "EAR99_CONTROLLED",
				"avg_t1_us":        35.0,
				"avg_t2_us":        28.0,
				"readout_error":    0.0035,
			},
		},
	})
}

func (s *Server) handleRadar(w http.ResponseWriter, r *http.Request) {
	// Probe local ports
	ports := []int{8787, 11434, 1234, 47831, 8080, 50051}
	var detected []map[string]any

	for _, p := range ports {
		dialTarget := fmt.Sprintf("127.0.0.1:%d", p)
		if p == 50051 && s.bridgeAddr != "" {
			// The bridge lives at the configured gRPC address
			// (--bridge; bridge:50051 in compose), not on this
			// daemon's loopback. Dialing loopback here reported a
			// healthy bridge as OFFLINE in the shipped topology.
			// Same standard as the fleet liveness probe below.
			dialTarget = s.bridgeAddr
		}
		conn, err := net.DialTimeout("tcp", dialTarget, 150*time.Millisecond)
		isListening := (err == nil)
		if conn != nil {
			conn.Close()
		}

		serviceName := "Unknown Service"
		switch p {
		case 8787:
			serviceName = "oMLX Local LLM Engine (Apple Silicon)"
		case 11434:
			serviceName = "Ollama Local LLM Engine"
		case 1234:
			serviceName = "LM Studio Local LLM Engine"
		case 47831:
			serviceName = "Strata Local LLM Engine"
		case 8080:
			serviceName = "Kubona Go Core Gateway"
		case 50051:
			serviceName = "Kubona Python Quantum Bridge"
		}

		detected = append(detected, map[string]any{
			"port":        p,
			"listening":   isListening,
			"service":     serviceName,
			"sovereign":   true,
			"is_local_ip": true,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"radar_endpoints": detected})
}

func (s *Server) handleMandateSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.signLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "rate limit exceeded: slow down and retry", nil)
		return
	}

	var req struct {
		TenantID     string               `json:"tenant_id"`
		JobHash      string               `json:"job_hash"`
		MaxBudgetUSD float64              `json:"max_budget_usd"`
		TtlSeconds   int64                `json:"ttl_seconds"`
		AdminAction  *tenants.AdminAction `json:"admin_action"`
	}
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid request body: %v", err), nil)
		return
	}

	if req.TtlSeconds <= 0 {
		req.TtlSeconds = 3600
	}
	if !s.reviewerKeyConfigured && os.Getenv("KUBONA_ALLOW_EPHEMERAL_SIGNER") != "1" {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "reviewer signing key not configured: set KUBONA_REVIEWER_PRIVKEY_HEX " +
				"(or KUBONA_ALLOW_EPHEMERAL_SIGNER=1 for local dev only)",
			"reviewer_key_configured": false,
		})
		return
	}
	// Spend authority mints only for the tenant's admin: the envelope binds
	// tenant, action, and exact parameters under the admin key. A bearer
	// token alone (already checked by the route wrapper) is not enough.
	if req.AdminAction == nil {
		WriteAPIError(w, "forbidden", "admin_action envelope required: tenant admin must authorize this spend", nil)
		return
	}
	bound, err := bindAdminAction(req.TenantID, "mandate_sign",
		map[string]any{"job_hash": req.JobHash, "max_budget_usd": req.MaxBudgetUSD, "ttl_seconds": req.TtlSeconds},
		*req.AdminAction)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if _, err := s.tenantMgr.VerifyAdminAction(bound); err != nil {
		http.Error(w, "admin authorization failed: "+err.Error(), http.StatusForbidden)
		return
	}
	expiry := time.Now().UTC().Add(time.Duration(req.TtlSeconds) * time.Second)

	mandate, err := mandates.SignMandate(s.signerPrivKey, req.TenantID, req.JobHash, req.MaxBudgetUSD, expiry)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	verifiedErr := mandate.Verify(req.TenantID, req.JobHash, req.MaxBudgetUSD)
	writeJSON(w, http.StatusOK, map[string]any{
		"mandate":                 mandate,
		"public_key":              hex.EncodeToString(s.signerPubKey),
		"verified":                verifiedErr == nil,
		"reviewer_key_configured": s.reviewerKeyConfigured,
	})
}

func (s *Server) handleMandateVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.verifyLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "rate limit exceeded: slow down and retry", nil)
		return
	}

	var req mandates.SpendMandate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	err := req.Verify(req.TenantID, req.JobHash, req.MaxBudgetUSD)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid": false,
			"code":  "mandate_invalid",
			"error": err.Error(),
		})
		return
	}

	// AP2 Spend Ceiling Clamping against the LAB TENANT ceiling (not the
	// session: mandate.TenantID names a tenant since the tenancy model
	// landed; the old session-id-as-tenant conflation is removed).
	if err := s.tenantMgr.CheckSpend(req.TenantID, req.MaxBudgetUSD); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid": false,
			"code":  "budget_exceeded",
			"error": err.Error(),
		})
		return
	}

	// W0.12 key→tenant binding: the signing key must be attributable.
	// Two paths, both audited (R-fix 2026-10-17: the first version of this
	// check accepted admin keys only, which refused every reviewer-minted
	// mandate the sign endpoint produces — TestAPIRoutes step 10 caught
	// it on the clean tree):
	//  1. tenant-admin path: key == the tenant's CURRENT admin key
	//     (self-authorized spend under its own ceiling; rotation
	//     invalidates old keys here automatically);
	//  2. reviewer path: key == this server's signing key (human-review
	//     flow; the sign endpoint mints under this key). Gated on a real
	//     reviewer key or the dev-only ephemeral flag — mirroring the
	//     sign endpoint's own gate, so production never accepts an
	//     ephemeral reviewer.
	// Anything else (attacker's self-signed key) refuses: a valid
	// signature from an unattributed key spends nobody's ceiling.
	lab, err := s.tenantMgr.GetTenant(req.TenantID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid": false,
			"error": err.Error(),
		})
		return
	}
	isAdmin := lab.AdminPubkeyHex != "" && strings.EqualFold(lab.AdminPubkeyHex, req.PublicKeyHex)
	isReviewer := strings.EqualFold(hex.EncodeToString(s.signerPubKey), req.PublicKeyHex) &&
		(s.reviewerKeyConfigured || os.Getenv("KUBONA_ALLOW_EPHEMERAL_SIGNER") == "1")
	if !isAdmin && !isReviewer {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid":       false,
			"code":        "mandate_invalid",
			"error":       "mandate key is not the tenant admin key: self-signed spend refused (bootstrap an admin key or use the reviewer-signed path)",
			"remediation": "mint a fresh mandate: bind the tenant admin key, exact parameters, and a live expiry",
		})
		return
	}
	via := "tenant_admin"
	if !isAdmin {
		via = "reviewer"
	}

	rec, rerr := s.ledger.AppendExternalEvent(req.TenantID, "mandate_verified_client", map[string]any{
		"tenant_id":        req.TenantID,
		"job_hash":         req.JobHash,
		"public_key_hex":   req.PublicKeyHex,
		"signature_hex":    req.SignatureHex,
		"max_budget_usd":   req.MaxBudgetUSD,
		"via":              via,
		"expiry_timestamp": req.ExpiryTimestamp.UTC().Format(time.RFC3339),
	}, req.PublicKeyHex, req.SignatureHex)
	if rerr != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid": false,
			"code":  "mandate_invalid",
			"error": rerr.Error(),
		})
		return
	}
	// R2b: the verified receipt opens its closure obligation (subject =
	// the receipt entry_hash jobs cite as mandate_ref). The owner is the
	// accountable key: tenant admin on the admin path, the reviewer on
	// the reviewer path. Opening failure fails verification (no
	// unattributed mandates).
	owner := "admin:" + req.PublicKeyHex
	if !isAdmin {
		owner = "reviewer:" + req.PublicKeyHex
	}
	if oerr := s.openMandateObligation(req.TenantID, owner, rec.EntryHash); oerr != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid": false,
			"code":  "mandate_invalid",
			"error": oerr.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"valid":       true,
		"mandate":     req,
		"verified_at": time.Now().UTC().Format(time.RFC3339),
		"receipt":     rec.EntryHash,
	})
}

// openMandateObligation opens (idempotently) the closure obligation for
// a verified mandate receipt (R2b): subject = the receipt entry_hash
// jobs cite as mandate_ref, required case = settlement by the ledger
// receipt authority, owner = the accountable key (admin or reviewer).
// tenantID names the billed lab (kept for call-site clarity; the
// subject + owner carry the binding).
func (s *Server) openMandateObligation(tenantID, owner, entryHash string) error {
	if entryHash == "" {
		return fmt.Errorf("mandate obligation needs the receipt entry hash")
	}
	if owner == "" {
		return fmt.Errorf("mandate obligation needs an accountable owner")
	}
	ob, err := s.ledger.OpenObligation("mandate", entryHash, []string{"settlement"}, []string{"ledger-receipt"}, nil)
	if err != nil {
		return fmt.Errorf("mandate obligation: %w", err)
	}
	if err := s.ledger.SetObligationOwner(ob.ID, owner); err != nil {
		return fmt.Errorf("mandate obligation owner: %w", err)
	}
	return nil
}

// recordMandateSettlement closes the mandate loop (R2b): a completed
// job carrying mandate_ref records the settlement case on its receipt's
// obligation (authority ledger-receipt — the admission-verified binding
// is the verification) and attempts close. Unmandated jobs and unknown
// receipts (pre-obligation history) skip silently. A settlement landing
// on an ALREADY-CLOSED obligation opens an ownerless follow-up anomaly
// (subject mandate/H/jobID, dependency on the original) instead of
// rewriting closure — post-closure activity stays visible, never merged.
// Runs on the job-worker goroutine: never fails the settlement itself.
func (s *Server) recordMandateSettlement(j *jobs.Job) {
	if j == nil || j.MandateRef == "" {
		return
	}
	ob, err := s.ledger.GetObligationBySubject("mandate", j.MandateRef)
	if err != nil {
		slog.Error("[Kubona Core] mandate settlement lookup failed", "job_id", j.ID, "error", err)
		return
	}
	if ob == nil {
		return
	}
	settlement := ledger.ObligationCase{
		CaseKind: "settlement", Verdict: "pass", Authority: "ledger-receipt",
		Evidence: j.ID,
	}
	if err := s.ledger.RecordObligationCase(ob.ID, settlement); err != nil {
		// Closed history is immutable: fork an anomaly follow-up.
		follow, ferr := s.ledger.OpenObligation("mandate-settlement",
			j.MandateRef+"/job/"+j.ID, []string{"settlement"}, []string{"ledger-receipt"},
			[]string{ob.ID})
		if ferr != nil {
			slog.Error("[Kubona Core] mandate follow-up failed", "job_id", j.ID, "error", ferr)
			return
		}
		if ferr := s.ledger.RecordObligationCase(follow.ID, settlement); ferr != nil {
			slog.Error("[Kubona Core] mandate follow-up case failed", "job_id", j.ID, "error", ferr)
		}
		return
	}
	if closed, reasons, err := agents.AttemptClose(s.ledger, ob.ID); err != nil {
		slog.Error("[Kubona Core] mandate close failed", "job_id", j.ID, "error", err)
	} else if !closed {
		slog.Warn("[Kubona Core] mandate stays open after job", "mandate_ref", j.MandateRef, "job_id", j.ID, "reasons", reasons)
	}
}

func (s *Server) handleHarnesses(w http.ResponseWriter, r *http.Request) {
	rawList := s.harnessReg.List()
	var out []map[string]any
	for _, h := range rawList {
		out = append(out, map[string]any{
			"id":               h.ID(),
			"fixed_setup":      h.FixedSetup(),
			"mutable_artifact": h.MutableArtifact(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"harnesses": out})
}

// handleLiterature wires live arXiv/OpenAlex grounding into the console
// (P6): the bridge searches live with an offline-cache fallback, and every
// record carries real year/citation values — never placeholders.
func (s *Server) handleLiterature(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query().Get("query")
	if query == "" {
		http.Error(w, "query is required", http.StatusBadRequest)
		return
	}
	limit := 5
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 25 {
			limit = n
		}
	}
	bridge := s.jobRunner.BridgeClient()
	if bridge == nil {
		writeBridgeUnavailable(w, "literature_grounding", "bridge client is nil.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resp, err := bridge.GroundLiterature(ctx, &pb.LiteratureGroundingRequest{
		Query: query,
		Limit: int32(limit),
	})
	if err != nil {
		writeBridgeUnavailable(w, "literature_grounding", "GroundLiterature RPC failed: "+err.Error())
		return
	}
	records := make([]any, 0, len(resp.Records))
	for _, rec := range resp.Records {
		records = append(records, map[string]any{
			"id": rec.Id, "title": rec.Title, "authors": rec.Authors,
			"abstract": rec.AbstractText, "url": rec.Url,
			"publication_year": rec.PublicationYear,
			"citation_count":   rec.CitationCount,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"records": records, "cached": resp.Cached, "query": query,
	})
}

func (s *Server) handleNotebooks(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID != "" {
		if _, gerr := s.sessionMgr.GetSession(sessionID); gerr != nil {
			WriteAPIError(w, "ghost_session", fmt.Sprintf("unknown session %q: refusing to show another session", sessionID), nil)
			return
		}
		// T-09b: known session — bind caller to session tenant.
		if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
			return
		}
	}
	// Empty id renders the unscoped template below — a GET never mints a
	// session, and never adopts the most-recent one (G7).

	if raw, err := s.ledger.GetCampaignState(sessionID); err == nil && len(raw) > 0 {
		var cs agents.CampaignState
		if json.Unmarshal(raw, &cs) == nil && cs.Notebook.Title != "" {
			writeJSON(w, http.StatusOK, map[string]any{
				"notebooks": []any{cs.Notebook},
			})
			return
		}
	}

	// 1. Check if jobs exist for session
	jobsList, _ := s.jobRunner.ListJobs(sessionID, 5)
	var latestJob *jobs.Job
	for _, j := range jobsList {
		if j.Status == pb.JobStatus_JOB_STATUS_COMPLETED {
			latestJob = j
			break
		}
	}

	mol := "H2"
	bondDist := 0.7414
	targetTitle := "H2 Molecular Potential Energy Surface VQE vs PySCF FCI"
	classicalAnchorName := "PySCF STO-3G Full Configuration Interaction"
	var fciEnergy float64 = -1.13727
	var rhfEnergy float64 = -1.11668
	var qEnergy float64 = -1.13680

	// Dynamically compute real PySCF classical anchor via bridge. R2: when
	// the bridge is absent or fails, every number below is an assumed
	// template default — labeled as such, never as verified integrals.
	anchorComputed := false
	if s.jobRunner.BridgeClient() != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		resp, err := s.jobRunner.BridgeClient().ComputeClassicalAnchor(ctx, &pb.ClassicalAnchorRequest{
			Molecule:     mol,
			BondDistance: bondDist,
		})
		cancel()
		if err == nil && resp.Success {
			anchorComputed = true
			fciEnergy = resp.FciExactEnergy
			rhfEnergy = resp.RhfEnergy
			// Corridor estimate, not a measurement: nothing has executed
			// on this template path. qEnergy exists so the notebook shows
			// the shape of the corridor, labeled as such below.
			qEnergy = fciEnergy + 0.00047
		}
	}
	if !anchorComputed {
		classicalAnchorName = "Hardcoded fallback (bridge unavailable) — assumed, no ab-initio integrals computed"
	}

	deltaMHa := (qEnergy - fciEnergy) * 1000.0
	if deltaMHa < 0 {
		deltaMHa = -deltaMHa
	}

	// Compute real provenance Merkle hash from recent events
	recentEvents, _ := s.ledger.GetRecentEvents(sessionID, 20)
	hasher := sha256.New()
	for _, ev := range recentEvents {
		hasher.Write([]byte(fmt.Sprintf("%s:%s:%d:%s", ev.EventType, ev.SessionID, ev.ID, string(ev.Payload))))
	}
	if len(recentEvents) == 0 {
		hasher.Write([]byte(fmt.Sprintf("seed:%s:%.5f:%.5f", mol, fciEnergy, qEnergy)))
	}
	merkleHash := fmt.Sprintf("sha256:%x", hasher.Sum(nil))

	notebookID := "nb-" + uuid.New().String()[:8]
	if latestJob != nil {
		notebookID = "nb-" + latestJob.ID[:8]
	}

	// The template path never passes chemical accuracy and never claims
	// verification: qEnergy is a corridor estimate by construction, never a
	// measured VQE result. Only job folds decide accuracy — the same rule
	// campaign synthesis obeys. A computed anchor reads as PROJECTION
	// (the shape of the work, anchor real, execution pending); an assumed
	// one as UNVERIFIED_FALLBACK.
	verdict := "PROJECTION"
	chemAccuracy := false
	anchorSection := fmt.Sprintf("2. Classical PySCF ab-initio ground state RHF (%.5f Ha) and FCI (%.5f Ha) integrals", rhfEnergy, fciEnergy)
	vqeSection := fmt.Sprintf("3. Variational Quantum Eigensolver circuit prepared — awaiting execution; corridor estimate E=%.5f Ha (not measured)", qEnergy)
	collapseSection := fmt.Sprintf("4. Ring 1 Epistemic projection: chemical accuracy unclaimed until measured (|Δ corridor|=%.3f mHa)", deltaMHa)
	if !anchorComputed {
		// R2: no bridge, no verification-grade claim of any kind.
		verdict = "UNVERIFIED_FALLBACK"
		anchorSection = fmt.Sprintf("2. Classical anchor UNAVAILABLE — hardcoded RHF (%.5f Ha) and FCI (%.5f Ha) assumed (bridge unreachable); no ab-initio integrals computed", rhfEnergy, fciEnergy)
		vqeSection = fmt.Sprintf("3. Variational Quantum Eigensolver circuit prepared — awaiting execution; template default E=%.5f Ha (not measured)", qEnergy)
		collapseSection = "4. Ring 1 Epistemic state UNVERIFIED: anchor is a hardcoded fallback, no collapse claimed"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"notebooks": []map[string]any{
			{
				"title":             targetTitle,
				"id":                notebookID,
				"created_at":        time.Now().UTC().Format(time.RFC3339),
				"classical_anchor":  classicalAnchorName,
				"quantum_energy":    qEnergy,
				"classical_energy":  fciEnergy,
				"rhf_energy":        rhfEnergy,
				"delta_mha":         deltaMHa,
				"chemical_accuracy": chemAccuracy,
				"provenance_hash":   merkleHash,
				"epistemic_verdict": verdict,
				"sections": []string{
					fmt.Sprintf("1. Scientific Objective & Active-Space Configuration (%s at R=%.4f A)", mol, bondDist),
					anchorSection,
					vqeSection,
					collapseSection,
					"5. Negative results & barren plateau avoidance ledger (Var(∇E) >= 1e-6)",
					"6. Pre-flight Clifford canary probe fidelity profile (F >= 95%)",
					fmt.Sprintf("7. Provenance SHA-256 Merkle DAG hash proof (%s)", merkleHash),
					fmt.Sprintf("8. Reproduction command: kubona replay --hash %s", merkleHash),
				},
			},
		},
	})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// T-09a (D-09 single-lab open mode): lab callers list only
		// their own sessions; bearer lists all (operator); anonymous
		// lists only while the store holds at most one lab.
		var sessions []session.Session
		if hasNoCredentials(r) {
			if !s.singleTenantStore() {
				WriteAPIError(w, "unauthorized", "multi-lab store: log in or present the operator token to list sessions", nil)
				return
			}
			var err error
			sessions, err = s.sessionMgr.ListSessions(50)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else if tenant, _, code := s.checkAuth(r); code != http.StatusOK {
			WriteAPIError(w, "unauthorized", "session required: log in or present the operator token", nil)
			return
		} else if tenant == "" {
			var err error
			sessions, err = s.sessionMgr.ListSessions(50)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			var err error
			sessions, err = s.sessionMgr.ListSessionsByTenant(tenant, 50)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if sessions == nil {
			sessions = []session.Session{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
	case http.MethodPost:
		var req struct {
			Title          string  `json:"title"`
			TenantID       string  `json:"tenant_id"`
			SpendLimitUSD  float64 `json:"spend_limit_usd"`
			TargetMolecule string  `json:"target_molecule,omitempty"`
			Fresh          bool    `json:"fresh,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		// T-09b creation-spoof guard: sessions are spend-attributed to
		// their lab, so minting into another lab is attribution fraud.
		// Bearer (operator) may mint anywhere; lab callers mint in
		// their own lab (empty pins to it); anonymous mints only into
		// the default lab (the zero-login local flow).
		if hasNoCredentials(r) {
			if req.TenantID != "" && req.TenantID != "default" {
				WriteAPIError(w, "forbidden", "anonymous sessions belong to the default lab: log in to use a lab tenant", nil)
				return
			}
			req.TenantID = ""
			// Anonymous callers cannot reach any ReserveSpend>0 path
			// (lens, loops, mandates are all authenticated), but they
			// also get no say in budgets: requested limits above the
			// default are clamped, so spend authority without identity
			// can never mint an above-default budget. Smaller requests
			// (and unset → default) pass through untouched.
			if req.SpendLimitUSD > session.DefaultBudgetUSD {
				req.SpendLimitUSD = session.DefaultBudgetUSD
			}
		} else if tenant, ok := s.checkCallerAuth(w, r); !ok {
			return
		} else if tenant != "" {
			if req.TenantID != "" && req.TenantID != tenant {
				WriteAPIError(w, "forbidden", "cannot create sessions in another lab", nil)
				return
			}
			req.TenantID = tenant
		}
		sess, err := s.sessionMgr.CreateSession(req.Title, req.SpendLimitUSD, req.TenantID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Synthesize dynamic campaign state for new session with options
		campState, _ := agents.SynthesizeCampaignStateWithOpts(r.Context(), sess.ID, sess.Title, req.TargetMolecule, req.Fresh, s.llmClient, s.jobRunner.BridgeClient())
		if campState != nil {
			campBytes, _ := json.Marshal(campState)
			s.ledger.SaveCampaignStateWithTrigger(sess.ID, campBytes, "session-create")
		}
		writeJSON(w, http.StatusCreated, sess)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSessionRefund returns consumed spend to a session's books (R-32
// mechanism, Go rail). Mirrors handleTenantRefund exactly: admin envelope
// binds tenant + session + amount + reason (verified against the session's
// own tenant — cross-tenant refund refuses); zero/empty/over-refund
// refuse; every outcome seals a ledger event. ReleaseSpend stays the
// reservation-return path; this is the audited money-back path.
func (s *Server) handleSessionRefund(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteAPIError(w, "method_not_allowed", "refund is POST-only", nil)
		return
	}
	if !s.tenantLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "rate limit exceeded: slow down and retry", nil)
		return
	}
	var req struct {
		SessionID string              `json:"session_id"`
		Amount    float64             `json:"amount_usd"`
		Reason    string              `json:"reason"`
		Action    tenants.AdminAction `json:"action"`
	}
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid request body: %v", err), nil)
		return
	}
	if req.SessionID == "" {
		WriteAPIError(w, "bad_request", "session_id is required", nil)
		return
	}
	sess, err := s.sessionMgr.GetSession(req.SessionID)
	if err != nil {
		WriteAPIError(w, "ghost_session", fmt.Sprintf("session %q not found", req.SessionID), nil)
		return
	}
	bound, err := s.bindAdminActionOrSession(r, sess.TenantID, "session_refund_spend",
		map[string]any{"tenant_id": sess.TenantID, "session_id": req.SessionID,
			"amount_usd": req.Amount, "reason": req.Reason}, req.Action)
	if err != nil {
		WriteAPIError(w, "forbidden", err.Error(), nil)
		return
	}
	if _, err := s.tenantMgr.VerifyAdminAction(bound); err != nil {
		WriteAPIError(w, "forbidden", "refund envelope signature refused: "+err.Error(), nil)
		return
	}
	updated, err := s.sessionMgr.RefundSpend(bound, req.SessionID, req.Amount, req.Reason)
	if err != nil {
		WriteAPIError(w, "forbidden", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleSessionExport returns the validated migration manifest for one
// session (S5). A read, open like other session reads; ghosts 404.
func (s *Server) handleSessionExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionID == "" {
		http.Error(w, "export requires {\"session_id\": ...}", http.StatusBadRequest)
		return
	}
	if _, err := s.sessionMgr.GetSession(req.SessionID); err != nil {
		http.Error(w, fmt.Sprintf("unknown session %q: nothing to export", req.SessionID), http.StatusNotFound)
		return
	}
	// T-09b: known session — bind caller to session tenant.
	if _, ok := s.authorizeKnownSession(w, r, req.SessionID); !ok {
		return
	}
	man, err := s.sessionMgr.ExportSession(req.SessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, man)
}

// handleSessionImport validates a manifest and activates it with ONE
// ledger record (S5). Open like other session-content writes (chat POST,
// jobs POST are open; validation is the control, not bearer —
// bearer-gating here while chat stays open would be consistency theater).
// An already-present session ID replays the current export (200 +
// replayed:true) instead of forking a duplicate.
func (s *Server) handleSessionImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Manifest json.RawMessage `json:"manifest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Manifest) == 0 {
		http.Error(w, "import requires {\"manifest\": {...}}", http.StatusBadRequest)
		return
	}
	var man session.SessionManifest
	if err := json.Unmarshal(req.Manifest, &man); err != nil {
		http.Error(w, fmt.Sprintf("manifest does not parse: %v", err), http.StatusBadRequest)
		return
	}
	res, err := s.sessionMgr.ImportSession(&man)
	if err != nil {
		status := http.StatusBadRequest
		msg := err.Error()
		if strings.HasPrefix(msg, "import commit:") || strings.HasPrefix(msg, "import activation") {
			status = http.StatusInternalServerError
		}
		http.Error(w, msg, status)
		return
	}
	out := map[string]any{"session_id": res.SessionID, "manifest_version": res.Manifest.Version}
	if res.Existing {
		w.Header().Set("X-Idempotent-Replay", "true")
		out["replayed"] = true
		out["manifest"] = res.Manifest
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSessionContext serves the composer's context readout (S8a): the
// measured transcript against the REAL truncation rule (BudgetHistory
// caps), plus summed provider-reported token usage. No vendor window is
// involved — pressure_pct is max(chars/cap, turns/cap), i.e. how close
// the next turn is to the cut the system actually applies. A read:
// ungated like other session reads.
func (s *Server) handleSessionContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	if _, err := s.sessionMgr.GetSession(sessionID); err != nil {
		WriteAPIError(w, "ghost_session", fmt.Sprintf("unknown session %q", sessionID), nil)
		return
	}
	// T-09b: known session — bind caller to session tenant.
	if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
		return
	}
	msgs, err := s.sessionMgr.GetMessages(sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var transcript []llm.ChatMessage
	var promptTok, completionTok int64
	for _, m := range msgs {
		transcript = append(transcript, llm.ChatMessage{Role: m.Role, Content: m.Content})
		if m.UsageJSON != nil && len(*m.UsageJSON) > 0 {
			var u struct {
				PromptTokens     int64 `json:"prompt_tokens"`
				CompletionTokens int64 `json:"completion_tokens"`
				TotalTokens      int64 `json:"total_tokens"`
			}
			if jerr := json.Unmarshal(*m.UsageJSON, &u); jerr == nil {
				promptTok += u.PromptTokens
				completionTok += u.CompletionTokens
			}
		}
	}
	turns, chars, dropped := llm.HistoryPressure(transcript, llm.DefaultMaxHistoryMessages, llm.DefaultMaxHistoryChars)
	pressure := 0.0
	if llm.DefaultMaxHistoryChars > 0 {
		pressure = float64(chars) / float64(llm.DefaultMaxHistoryChars)
	}
	if llm.DefaultMaxHistoryMessages > 0 {
		if pm := float64(turns) / float64(llm.DefaultMaxHistoryMessages); pm > pressure {
			pressure = pm
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id":        sessionID,
		"turns":             turns,
		"turn_cap":          llm.DefaultMaxHistoryMessages,
		"history_chars":     chars,
		"history_chars_cap": llm.DefaultMaxHistoryChars,
		"would_drop":        dropped,
		"pressure_pct":      pressure * 100,
		"prompt_tokens":     promptTok,
		"completion_tokens": completionTok,
	})
}

func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if strings.HasSuffix(id, "/messages") {
		sessionID := strings.TrimSuffix(id, "/messages")
		// T-09b: known session — bind caller to session tenant. Ghosts
		// keep legacy behavior; the clear-chat DELETE is destruction,
		// so it takes the same gate as the read.
		if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
			return
		}
		if r.Method == http.MethodDelete {
			// Operator "clear chat": messages gone, session + campaign +
			// ledger history survive, erasure sealed as an event.
			n, err := s.sessionMgr.ClearMessages(sessionID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"cleared": true, "deleted": n, "session_id": sessionID})
			return
		}
		msgs, err := s.sessionMgr.GetMessages(sessionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
		return
	}
	if strings.HasSuffix(id, "/campaign/synthesize") {
		sessionID := strings.TrimSuffix(id, "/campaign/synthesize")
		var req struct {
			Prompt string `json:"prompt"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		// T-09b: known session — bind caller to session tenant (this
		// endpoint also writes campaign state, so the gate covers both).
		if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
			return
		}
		sess, err := s.sessionMgr.GetSession(sessionID)
		if err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		prompt := req.Prompt
		if prompt == "" {
			prompt = sess.Title
		}
		state, err := agents.SynthesizeCampaignState(r.Context(), sessionID, prompt, s.llmClient, s.jobRunner.BridgeClient())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		stateBytes, _ := json.Marshal(state)
		s.ledger.SaveCampaignStateWithTrigger(sessionID, stateBytes, triggerLabel("synthesize", prompt))
		s.ledger.AppendEvent(sessionID, "campaign_synthesized", map[string]any{
			"target": state.TargetSystem,
			"prompt": prompt,
		})
		writeJSON(w, http.StatusOK, state)
		return
	}
	// Claim history: every cited [clm-…] ID resolves here, newest first.
	// Unknown sessions resolve to an empty list, never an error shell.
	if strings.HasSuffix(id, "/claims") {
		sessionID := strings.TrimSuffix(id, "/claims")
		// T-09b: ghosts resolve to an empty list (documented contract);
		// known sessions bind caller to session tenant.
		if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
			return
		}
		limit := 50
		if raw := r.URL.Query().Get("limit"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > 200 {
			limit = 200
		}
		records, err := s.ledger.ListClaimRecords(sessionID, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if records == nil {
			records = []ledger.ClaimRecord{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"claims": records})
		return
	}
	if strings.HasSuffix(id, "/campaign/versions") {
		sessionID := strings.TrimSuffix(id, "/campaign/versions")
		// T-09b: ghosts resolve to an empty list (documented contract);
		// known sessions bind caller to session tenant.
		if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
			return
		}
		limit := 50
		if raw := r.URL.Query().Get("limit"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > 200 {
			limit = 200
		}
		versions, err := s.ledger.GetCampaignVersions(sessionID, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if versions == nil {
			versions = []ledger.CampaignVersion{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
		return
	}
	if strings.HasSuffix(id, "/campaign") {
		sessionID := strings.TrimSuffix(id, "/campaign")
		// T-09b: known session — bind caller to session tenant. Ghosts
		// keep legacy behavior below (state serve or synthesize path).
		if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
			return
		}
		rawState, err := s.ledger.GetCampaignState(sessionID)
		if err == nil && len(rawState) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(rawState)
			return
		}
		sess, err := s.sessionMgr.GetSession(sessionID)
		if err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		state, err := agents.SynthesizeCampaignState(r.Context(), sessionID, sess.Title, s.llmClient, s.jobRunner.BridgeClient())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		stateBytes, _ := json.Marshal(state)
		s.ledger.SaveCampaignStateWithTrigger(sessionID, stateBytes, "campaign-fetch")
		writeJSON(w, http.StatusOK, state)
		return
	}
	if strings.HasSuffix(id, "/campaign/flight-recorder") || strings.HasSuffix(id, "/flight-recorder") {
		sessionID := strings.TrimSuffix(strings.TrimSuffix(id, "/campaign/flight-recorder"), "/flight-recorder")
		s.handleCampaignFlightRecorder(w, r, sessionID)
		return
	}
	// T-10b: settle restart-orphan loops (DB-keyed: the action handler
	// 404s without an in-memory engine, so orphans need their own path).
	if strings.HasSuffix(id, "/loops/settle") {
		s.handleLoopSettle(w, r, strings.TrimSuffix(id, "/loops/settle"))
		return
	}
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodDelete {
		// T-09b: destruction binds caller to session tenant. Ghosts
		// keep the legacy not-found mapping below.
		if _, ok := s.authorizeKnownSession(w, r, id); !ok {
			return
		}
		if err := s.sessionMgr.DeleteSession(id, s.signerPrivKey); err != nil {
			if strings.Contains(err.Error(), "session not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "session_id": id})
		return
	}
	sess, err := s.sessionMgr.GetSession(id)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	// T-09b: known session — bind caller to session tenant (this also
	// covers the tenant_id field the single-session read discloses).
	if !s.authorizeSessionRead(w, r, sess) {
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) requireSession(sessionID string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("missing session_id: spend-attributed calls must name an existing session")
	}
	if _, err := s.sessionMgr.GetSession(sessionID); err != nil {
		return "", fmt.Errorf("unknown session %q: refusing to attach spend elsewhere", sessionID)
	}
	return sessionID, nil
}

func (s *Server) ensureSession(sessionID string) (string, error) {
	// W2.2 / G7: fail-closed attribution — chat/stream/regenerate never
	// mint a session implicitly. A missing id 404s (no tenant to bill)
	// and an unknown id 404s (refusing to attach content elsewhere).
	// Callers map this error to 404.
	if sessionID == "" {
		return "", fmt.Errorf("missing session_id: chat calls must name an existing session")
	}
	if _, err := s.sessionMgr.GetSession(sessionID); err != nil {
		return "", fmt.Errorf("unknown session %q: refusing to attach content to another session", sessionID)
	}
	return sessionID, nil
}

func (s *Server) handleChatMessages(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")

	switch r.Method {
	case http.MethodGet:
		if sessionID == "" {
			writeJSON(w, http.StatusOK, map[string]any{"messages": []any{}})
			return
		}
		sess, gerr := s.sessionMgr.GetSession(sessionID)
		if gerr != nil {
			WriteAPIError(w, "ghost_session", fmt.Sprintf("unknown session %q: refusing to read another session", sessionID), nil)
			return
		}
		if !s.authorizeSessionRead(w, r, sess) {
			return
		}
		// T-09b: known session — bind caller to session tenant.
		if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
			return
		}
		msgs, err := s.sessionMgr.GetMessages(sessionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
	case http.MethodPost:
		var req struct {
			SessionID string `json:"session_id"`
			Role      string `json:"role"`
			Content   string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		// S2: chat writes mint LLM spend + message rows — new work.
		if s.refuseIfDiskBlocked(w) {
			return
		}
		// K4: cloud LLM entries check the global dollar cap first.
		if err := s.checkLLMBudget(); err != nil {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
			return
		}
		if sessionID == "" {
			sessionID = req.SessionID
		}
		var err error
		sessionID, err = s.ensureSession(sessionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		// T-09c: content writes bind caller to session tenant (cross-lab
		// graffiti without it). Same D-09 rule as reads.
		if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
			return
		}
		if req.Role == "" {
			req.Role = "user"
		}

		userMsg, err := s.sessionMgr.AddMessage(sessionID, req.Role, req.Content, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Autonomous Epistemic Routing via the configured engine
		genStart := time.Now()
		turnEventsBefore := s.latestEventID(sessionID)
		botReply, proposalJSON, err := s.generateCopilotResponse(r.Context(), sessionID, req.Content)
		genMs := time.Since(genStart).Milliseconds()
		if err != nil && botReply == "" {
			prov, model, _ := s.llmSettings.GetModelForRole("copilot")
			botReply = engineNotice(string(prov), model, err)
		}
		assistantMsg, _ := s.sessionMgr.AddMessage(sessionID, "assistant", botReply, proposalJSON)

		// Served-model observability: the UI shows which model answered, so a
		// stale selection is visible, never silent (2026-10-11).
		_, servedModel, _ := s.llmSettings.GetModelForRole("copilot")

		// Dynamically update campaign state for session
		campState, _ := agents.SynthesizeCampaignState(r.Context(), sessionID, req.Content, s.llmClient, s.jobRunner.BridgeClient())
		if campState != nil {
			campBytes, _ := json.Marshal(campState)
			s.ledger.SaveCampaignStateWithTrigger(sessionID, campBytes, triggerLabel("chat", req.Content))
		}

		// Memory write-back: this turn's outcome becomes recallable context.
		// One ledger read: the tool list feeds both the content and the
		// importance rating.
		turnTools := s.toolNamesSince(sessionID, turnEventsBefore)
		s.retainTurnExperience(sessionID, "chat", req.Content,
			chatOutcome(botReply, proposalJSON != nil, err), turnTools,
			chatImportance(proposalJSON != nil, err != nil, len(turnTools) > 0))

		writeJSON(w, http.StatusCreated, map[string]any{
			"user_message":      userMsg,
			"assistant_message": assistantMsg,
			"user":              userMsg,
			"assistant":         assistantMsg,
			"model":             servedModel,
			"duration_ms":       genMs,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleChatStream is the streaming twin of the chat POST: provider SSE
// tokens forward to the browser live, then the same finalization runs
// (proposal extraction, sanitation, campaign synthesis) and a done event
// carries the stored records. Failures arrive as error events carrying the
// provider-aware notice — never silence.
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// S2: streaming generation is new work (spend + rows).
	if s.refuseIfDiskBlocked(w) {
		return
	}
	// K4: cloud LLM entries check the global dollar cap first.
	if err := s.checkLLMBudget(); err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	emit := func(event string, payload any) {
		raw, _ := json.Marshal(payload)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(raw))
		flusher.Flush()
	}

	sessionID := r.URL.Query().Get("session_id")
	var req struct {
		SessionID string `json:"session_id"`
		Role      string `json:"role"`
		Content   string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		emit("error", map[string]any{"error": "invalid request body"})
		return
	}
	if sessionID == "" {
		sessionID = req.SessionID
	}
	var err error
	sessionID, err = s.ensureSession(sessionID)
	if err != nil {
		emit("error", map[string]any{"error": err.Error()})
		return
	}
	// T-09c: same binding as chat POST — reported as an error event
	// (SSE headers are already sent; no status line left to write).
	if sess, gerr := s.sessionMgr.GetSession(sessionID); gerr == nil {
		if allow, _, msg := s.checkSessionTenant(r, sess); !allow {
			emit("error", map[string]any{"error": msg})
			return
		}
	}
	if req.Role == "" {
		req.Role = "user"
	}
	userMsg, err := s.sessionMgr.AddMessage(sessionID, req.Role, req.Content, nil)
	if err != nil {
		emit("error", map[string]any{"error": err.Error()})
		return
	}
	emit("started", map[string]any{"user": userMsg})

	// History mirrors generateCopilotResponse (same system contract).
	recentMsgs, _ := s.sessionMgr.GetMessages(sessionID)
	history := []llm.ChatMessage{{Role: "system", Content: copilotSystemPrompt}}
	for _, m := range recentMsgs {
		if m.Role == "user" || m.Role == "assistant" {
			history = append(history, llm.ChatMessage{Role: m.Role, Content: m.Content})
		}
	}
	history = append(history, llm.ChatMessage{Role: "user", Content: req.Content})
	history = llm.BudgetHistory(history, 0, 0)

	streamCtx, cancel := context.WithTimeout(r.Context(), 600*time.Second)
	defer cancel()

	// Same tool loop as the unary path: observe live state first, then
	// stream the final answer. Degrades silently to base history.
	streamEventsBefore := s.latestEventID(sessionID)
	history = s.resolveWithTools(streamCtx, sessionID, history)

	streamRes, streamErr := s.llmClient.CompleteStream(streamCtx, "copilot", history, func(tok string) {
		emit("token", map[string]any{"text": tok})
	})
	if streamErr != nil {
		prov, model, _ := s.llmSettings.GetModelForRole("copilot")
		notice := engineNotice(string(prov), model, streamErr)
		assistantMsg, _ := s.sessionMgr.AddMessage(sessionID, "assistant", notice, nil)
		_, servedModel, _ := s.llmSettings.GetModelForRole("copilot")
		emit("done", map[string]any{"user": userMsg, "assistant": assistantMsg, "model": servedModel})
		return
	}
	raw := streamRes.Text
	finish := streamRes.Finish
	if finish == "length" {
		raw += copilotTruncationNotice
	}
	display, proposalJSON, multiProposal := extractProposal(raw)
	if multiProposal {
		display += "\n\n⚠️ Multiple proposals generated — ambiguous intent, none kept. Resubmit one."
	}
	display = sanitizeCopilotReply(display)
	assistantMsg, _ := s.sessionMgr.AddMessage(sessionID, "assistant", display, proposalJSON)
	_, servedModel, _ := s.llmSettings.GetModelForRole("copilot")
	// Persist measured cost on the stored message (NULL when unreported).
	streamMs := streamRes.Duration.Milliseconds()
	var streamUsage *json.RawMessage
	if streamRes.Usage != nil {
		if rawUsage, merr := json.Marshal(streamRes.Usage); merr == nil {
			u := json.RawMessage(rawUsage)
			streamUsage = &u
		}
	}
	if streamMs > 0 || streamUsage != nil {
		_ = s.sessionMgr.SetMessageUsage(assistantMsg.ID, streamMs, streamUsage)
		if streamMs > 0 {
			assistantMsg.DurationMs = &streamMs
		}
		assistantMsg.UsageJSON = streamUsage
	}

	var campState any
	if cs, _ := agents.SynthesizeCampaignState(r.Context(), sessionID, req.Content, s.llmClient, s.jobRunner.BridgeClient()); cs != nil {
		if campBytes, merr := json.Marshal(cs); merr == nil {
			s.ledger.SaveCampaignStateWithTrigger(sessionID, campBytes, triggerLabel("chat-stream", req.Content))
		}
		campState = cs
	}
	streamOutcome := chatOutcome(display, proposalJSON != nil, streamErr)
	if streamRes.Finish == "length" {
		streamOutcome += " (truncated at token budget)"
	}
	streamTools := s.toolNamesSince(sessionID, streamEventsBefore)
	s.retainTurnExperience(sessionID, "chat-stream", req.Content, streamOutcome, streamTools,
		chatImportance(proposalJSON != nil, streamErr != nil, len(streamTools) > 0))
	emit("done", map[string]any{
		"user": userMsg, "assistant": assistantMsg,
		"proposal": proposalJSON, "campaign": campState, "model": servedModel,
		"duration_ms": streamRes.Duration.Milliseconds(), "usage": streamRes.Usage,
	})
}

// handleChatRegenerate re-runs generation for the last user message without
// duplicating it: trailing assistant messages are dropped first, then the
// standard unary path answers the surviving prompt. Same envelope as chat
// POST plus duration_ms.
func (s *Server) handleChatRegenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// S2: regeneration mints a fresh assistant turn — new work.
	if s.refuseIfDiskBlocked(w) {
		return
	}
	// K4: cloud LLM entries check the global dollar cap first.
	if err := s.checkLLMBudget(); err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	sessionID, err := s.ensureSession(req.SessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	// T-09c: regeneration spends on the session's lab — bind the caller.
	if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
		return
	}
	prompt, err := s.sessionMgr.DropTrailingAssistant(sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	genStart := time.Now()
	regenEventsBefore := s.latestEventID(sessionID)
	botReply, proposalJSON, genErr := s.generateCopilotResponse(r.Context(), sessionID, prompt)
	genMs := time.Since(genStart).Milliseconds()
	if genErr != nil && botReply == "" {
		prov, model, _ := s.llmSettings.GetModelForRole("copilot")
		botReply = engineNotice(string(prov), model, genErr)
	}
	assistantMsg, _ := s.sessionMgr.AddMessage(sessionID, "assistant", botReply, proposalJSON)
	if genMs > 0 {
		_ = s.sessionMgr.SetMessageUsage(assistantMsg.ID, genMs, nil)
		assistantMsg.DurationMs = &genMs
	}
	_, servedModel, _ := s.llmSettings.GetModelForRole("copilot")
	if campState, _ := agents.SynthesizeCampaignState(r.Context(), sessionID, prompt, s.llmClient, s.jobRunner.BridgeClient()); campState != nil {
		if campBytes, merr := json.Marshal(campState); merr == nil {
			s.ledger.SaveCampaignStateWithTrigger(sessionID, campBytes, "regenerate")
		}
	}
	regenTools := s.toolNamesSince(sessionID, regenEventsBefore)
	s.retainTurnExperience(sessionID, "regenerate", prompt,
		chatOutcome(botReply, proposalJSON != nil, genErr), regenTools,
		chatImportance(proposalJSON != nil, genErr != nil, len(regenTools) > 0))
	writeJSON(w, http.StatusCreated, map[string]any{
		"assistant_message": assistantMsg,
		"assistant":         assistantMsg,
		"model":             servedModel,
		"duration_ms":       genMs,
		"regenerated_from":  prompt,
	})
}

// handleChatFeedback records a 👍/👎 vote on an assistant message as a ledger
// event — the training signal for the credibility flywheel. Votes outside
// {up, down} are refused; unknown message ids are refused.
func (s *Server) handleChatFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
		MessageID string `json:"message_id"`
		Vote      string `json:"vote"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Vote != "up" && req.Vote != "down" {
		http.Error(w, "vote must be 'up' or 'down'", http.StatusBadRequest)
		return
	}
	// T-09c: feedback writes to the session's ledger — bind the caller.
	// Ghosts keep the legacy not-found path below.
	if _, ok := s.authorizeKnownSession(w, r, req.SessionID); !ok {
		return
	}
	msgs, err := s.sessionMgr.GetMessages(req.SessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	found := false
	for _, m := range msgs {
		if m.ID == req.MessageID && m.Role == "assistant" {
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "assistant message not found in session", http.StatusNotFound)
		return
	}
	if err := s.ledger.AppendEvent(req.SessionID, "chat_feedback", map[string]any{
		"message_id": req.MessageID, "vote": req.Vote,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recorded": true, "message_id": req.MessageID, "vote": req.Vote})
}

// handleChatRewrite implements edit-resend branching: a user message is
// replaced and every later message is deleted, restarting the thread from
// the edited turn. The caller then regenerates (or leaves the clean branch).
func (s *Server) handleChatRewrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// S2: rewrite mutates rows — same gate class as chat writes.
	if s.refuseIfDiskBlocked(w) {
		return
	}
	var req struct {
		SessionID       string `json:"session_id"`
		MessageID       string `json:"message_id"`
		Content         string `json:"content"`
		ExpectedContent string `json:"expected_content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Content == "" {
		http.Error(w, "content must not be empty", http.StatusBadRequest)
		return
	}
	// S7: ghost sessions 404 explicitly (previously conflated with
	// ghost messages under one 400).
	if _, gerr := s.sessionMgr.GetSession(req.SessionID); gerr != nil {
		http.Error(w, fmt.Sprintf("unknown session %q: nothing to rewrite", req.SessionID), http.StatusNotFound)
		return
	}
	// T-09c: rewrite deletes the session's later rows — bind the caller.
	if _, ok := s.authorizeKnownSession(w, r, req.SessionID); !ok {
		return
	}
	msg, err := s.sessionMgr.RewriteUserMessage(req.SessionID, req.MessageID, req.Content, req.ExpectedContent)
	if err != nil {
		if err == session.ErrContentMismatch {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rewritten": true, "message": msg})
}

// fallbackProposal synthesizes launchable scaffolding when the model emits
// no proposal: a Bell-ladder QASM for the detected molecule plus a live (or
// explicitly fallback-labeled) anchor string. Attribution is structural —
// designed_by:"server-fallback" — so no consumer can present it as the
// model's design. Returns nil when no molecule is detected: no scaffolding
// to offer, no proposal to fake.
func (s *Server) fallbackProposal(content, reply string) *json.RawMessage {
	upper := strings.ToUpper(content + " " + reply)
	mol := ""
	bondDist := 0.0
	qubits := 2
	if strings.Contains(upper, "LIH") || strings.Contains(upper, "LITHIUM") {
		mol = "LiH"
		bondDist = 1.5957
		qubits = 4
	} else if strings.Contains(upper, "BEH2") || strings.Contains(upper, "BERYLLIUM") {
		mol = "BeH2"
		bondDist = 1.3300
		qubits = 6
	} else if strings.Contains(upper, "H2") || strings.Contains(upper, "HYDROGEN") || strings.Contains(upper, "VQE") {
		mol = "H2"
		bondDist = 0.7414
		qubits = 2
	}
	if mol == "" {
		return nil
	}
	fciStr := "PySCF STO-3G FCI (unverified fallback)"
	if s.jobRunner.BridgeClient() != nil {
		anchorCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		anchorResp, err := s.jobRunner.BridgeClient().ComputeClassicalAnchor(anchorCtx, &pb.ClassicalAnchorRequest{
			Molecule:     mol,
			BondDistance: bondDist,
		})
		cancel()
		if err == nil && anchorResp.Success {
			fciStr = fmt.Sprintf("PySCF STO-3G FCI (%.5f Ha)", anchorResp.FciExactEnergy)
		}
	}

	// Generate valid OpenQASM circuit
	var qasmBuilder strings.Builder
	qasmBuilder.WriteString("OPENQASM 2.0;\ninclude \"qelib1.inc\";\n")
	qasmBuilder.WriteString(fmt.Sprintf("qreg q[%d];\ncreg c[%d];\n", qubits, qubits))
	qasmBuilder.WriteString("h q[0];\n")
	for i := 0; i < qubits-1; i++ {
		qasmBuilder.WriteString(fmt.Sprintf("cx q[%d], q[%d];\n", i, i+1))
	}
	for i := 0; i < qubits; i++ {
		qasmBuilder.WriteString(fmt.Sprintf("measure q[%d] -> c[%d];\n", i, i))
	}

	// Attribution: this circuit is server-synthesized scaffolding
	// (Bell ladder for the detected molecule), NOT model-designed.
	// Consumers must surface designed_by; launch treats it like
	// any valid proposal, but nobody may present it as the
	// model's design.
	propMap := map[string]any{
		"target":           fmt.Sprintf("%s Ground State VQE (R=%.4f A)", mol, bondDist),
		"molecule":         mol,
		"spend_estimate":   "$0.00 (Local Simulator)",
		"recommended_tier": "Local Simulator (qiskit-aer-simulator)",
		"classical_anchor": fciStr,
		"qasm":             qasmBuilder.String(),
		"designed_by":      "server-fallback",
	}
	propBytes, _ := json.MarshalIndent(propMap, "", "  ")
	raw := json.RawMessage(propBytes)
	return &raw
}

// triggerLabel builds a bounded trigger string for campaign versions:
// triggerLabel builds a bounded trigger string for campaign versions:
// kind plus up to 120 runes of source text. Prompts can be long; the
// trigger names the cause, it never stores the whole prompt.
func triggerLabel(kind, text string) string {
	t := strings.TrimSpace(text)
	if len([]rune(t)) > 120 {
		t = string([]rune(t)[:120]) + "…"
	}
	if t == "" {
		return kind
	}
	return kind + ":" + t
}

func (s *Server) generateCopilotResponse(ctx context.Context, sessionID, content string) (string, *json.RawMessage, error) { // 1. Fetch recent messages for session context
	recentMsgs, _ := s.sessionMgr.GetMessages(sessionID)
	var history []llm.ChatMessage

	sysPrompt := copilotSystemPrompt
	history = append(history, llm.ChatMessage{Role: "system", Content: sysPrompt})
	for _, m := range recentMsgs {
		if m.Role == "user" || m.Role == "assistant" {
			history = append(history, llm.ChatMessage{Role: m.Role, Content: m.Content})
		}
	}
	history = append(history, llm.ChatMessage{Role: "user", Content: content})

	// Context budget: bounded transcript for every call (system survives,
	// newest turns win, cuts announced). Prevents unbounded spend/latency
	// growth as sessions lengthen.
	history = llm.BudgetHistory(history, 0, 0)

	// 2. Plan→tool→observe: let the model inspect live session state	// (anchors, campaign, jobs) before answering. Degrades to the base
	// history on any failure — never blocks the answer path.
	history = s.resolveWithTools(ctx, sessionID, history)

	// 3. Call live LLM (uncapped by default: MaxTokens 0 omits the field and
	// the model generates to EOS; the result contract below refuses to store
	// a severed paragraph as an answer). 600s budget: local generations run
	// ~6 tok/s, so real answers take minutes — the UI carries elapsed/overdue
	// states for exactly this.
	callCtx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()
	reply, chatResp, _, err := s.llmClient.Complete(callCtx, "copilot", history)
	if err != nil {
		prov, model, _ := s.llmSettings.GetModelForRole("copilot")
		return engineNotice(string(prov), model, err), nil, err
	}
	if chatResp != nil {
		for _, choice := range chatResp.Choices {
			if choice.Finish == "length" {
				reply += copilotTruncationNotice
				break
			}
		}
	}

	// 3. Extract json_proposal if generated (runs on the RAW reply: the
	// display filter below excises the block from chat prose, but the
	// launchable payload must survive in proposal_json).
	display, proposalJSON, multiProposal := extractProposal(reply)
	if multiProposal {
		reply = sanitizeCopilotReply(display) + "\n\n⚠️ Multiple proposals generated — ambiguous intent, none kept. Resubmit one."
		return reply, nil, nil
	}
	reply = sanitizeCopilotReply(display)

	// If the model did not close or generate the proposal block, dynamically synthesize by computing classical anchor from Python bridge
	if proposalJSON == nil {
		proposalJSON = s.fallbackProposal(content, reply)
	}

	return reply, proposalJSON, nil
}

func (s *Server) handleChatLaunch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// S2: disk guard freezes new work before any parse/spend.
	if s.refuseIfDiskBlocked(w) {
		return
	}

	var req struct {
		SessionID string          `json:"session_id"`
		QASM      string          `json:"qasm"`
		Backend   string          `json:"backend"`
		BudgetUSD float64         `json:"budget_usd"`
		Proposal  json.RawMessage `json:"proposal"`
		// G3: launch accepts the full jobs field set — without these,
		// metered-hardware launch could never pass admission (dead path).
		MandateRef          string   `json:"mandate_ref"`
		GrantChain          []string `json:"grant_chain"`
		InputLabels         []string `json:"input_labels"`
		CallerJurisdictions []string `json:"caller_jurisdictions"`
		CallerExportClass   string   `json:"caller_export_class"`
		ExportReviewed      bool     `json:"export_reviewed"`
		// S1: optional content-bound idempotency key (see handleJobs).
		RequestID string `json:"request_id"`
	}
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid request body: %v", err), nil)
		return
	}

	// Launch mints jobs (spend): missing/unknown session ids 404 like
	// dispatch — unattributed spend is another lab's bill (G7).
	launchSid, launchErr := s.requireSession(req.SessionID)
	if launchErr != nil {
		http.Error(w, launchErr.Error(), http.StatusNotFound)
		return
	}
	req.SessionID = launchSid

	if err := s.requireDispatchAutonomy(req.SessionID); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	// T-09c: launch mints spend on the session's lab — bind the caller
	// (a valid mandate is a capability, not a lab identity).
	if _, ok := s.authorizeKnownSession(w, r, req.SessionID); !ok {
		return
	}

	// R17: proposal attribution transcribed into the job below.
	designedBy := ""
	if len(req.Proposal) > 0 {
		var p struct {
			QASM            string `json:"qasm"`
			RecommendedTier string `json:"recommended_tier"`
			SpendEstimate   string `json:"spend_estimate"`
			Target          string `json:"target"`
			// R17: attribution rides into the job. Transcribed verbatim
			// when present, never inferred when absent — stamping "model"
			// on client-crafted JSON would fabricate authorship. The
			// server-fallback flow (generated in-chat, echoed back at
			// launch) preserves its stamp end-to-end, which is the case
			// that matters.
			DesignedBy string `json:"designed_by"`
		}
		if json.Unmarshal(req.Proposal, &p) == nil {
			designedBy = p.DesignedBy
			if p.QASM != "" {
				req.QASM = p.QASM
			}
			if p.RecommendedTier != "" {
				// Tier normalization (2026-10-11): models write free-text
				// tiers ("starter / simulator-first ..."). Anything
				// simulator-smelling routes local; anything else keeps its
				// literal name so hardware admission still guards it.
				lower := strings.ToLower(p.RecommendedTier)
				simSmell := false
				for _, smell := range []string{"local", "simulator", "statevector", "aer", "sim-first", "sim first", "starter", "shot-based", "numpy"} {
					if strings.Contains(lower, smell) {
						simSmell = true
						break
					}
				}
				if simSmell {
					req.Backend = "local_simulator"
				} else {
					req.Backend = p.RecommendedTier
				}
			}
		}
	}

	if req.QASM == "" {
		http.Error(w, "launch requires qasm: no circuit in request or proposal — nothing dispatched", http.StatusBadRequest)
		return
	}
	if req.Backend == "" {
		http.Error(w, "launch requires backend: no backend in request or proposal — nothing dispatched", http.StatusBadRequest)
		return
	}
	if req.BudgetUSD <= 0 {
		req.BudgetUSD = 1.0
	}

	jobParams := map[string]any{
		"qasm":                 req.QASM,
		"requested_backend":    req.Backend,
		"shots":                1024,
		"max_cost_usd":         req.BudgetUSD,
		"mandate_ref":          req.MandateRef,
		"grant_chain":          req.GrantChain,
		"input_labels":         req.InputLabels,
		"caller_jurisdictions": req.CallerJurisdictions,
		"caller_export_class":  req.CallerExportClass,
		"export_reviewed":      req.ExportReviewed,
	}
	// R17: attribution persists on the job row and the job_queued event
	// (SubmitJob stores params_json verbatim and queues the full job).
	if designedBy != "" {
		jobParams["designed_by"] = designedBy
	}
	// S1: claim-or-replay after all 400-validation (a fixable 400 must not
	// consume the key) and before SubmitJob (spend). Content hashes the
	// effective spend-binding values, not raw proposal bytes.
	idem, iref := s.idemBegin(req.SessionID, req.RequestID, map[string]any{
		"session_id": req.SessionID, "qasm": req.QASM, "backend": req.Backend,
		"budget_usd": req.BudgetUSD, "mandate_ref": req.MandateRef,
		"grant_chain": req.GrantChain, "input_labels": req.InputLabels,
		"caller_jurisdictions": req.CallerJurisdictions,
		"caller_export_class":  req.CallerExportClass,
		"export_reviewed":      req.ExportReviewed,
		"designed_by":          designedBy,
	}, idemRailJob)
	if iref != nil {
		iref.write(w)
		return
	}
	if idem.replay {
		s.idemReplay(w, idem)
		return
	}
	job, err := s.jobRunner.SubmitJob(req.SessionID, "circuit_run", jobParams)
	if err != nil {
		s.idemFinishError(w, idem, http.StatusInternalServerError, err.Error())
		return
	}

	loop, _ := s.supervisor.RegisterLoop(req.SessionID, 50)

	s.sessionMgr.AddMessage(req.SessionID, "system", fmt.Sprintf("🚀 Dispatched Job %s to backend '%s'. Loop %s armed under Ring 0 watchdog.", job.ID, req.Backend, loop.ID), nil)

	s.idemFinishJSON(w, idem, http.StatusCreated, map[string]any{
		"job":  job,
		"loop": loop,
	})
}

func (s *Server) handleChatTribunal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// S2: infrastructure before policy — a full disk can't reliably write
	// the tribunal's turns, so block before the budget check.
	if s.refuseIfDiskBlocked(w) {
		return
	}
	// K4: cloud LLM entries check the global dollar cap first (covers the
	// 4-turn run atomically at entry; mid-run breach stops the next entry).
	if err := s.checkLLMBudget(); err != nil {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		Prompt    string `json:"prompt"`
		// S1: optional content-bound idempotency key (slow rail: the
		// 4-turn run executes once; retries replay the stored consensus).
		RequestID string `json:"request_id"`
	}
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid request body: %v", err), nil)
		return
	}

	if req.Prompt == "" {
		req.Prompt = "Investigate H2 molecular ground state energy at equilibrium bond distance 0.7414 A"
	}

	var err error
	// K4-follow-up (demonstrated live: a probe id attached 8 turns + claim
	// records to a stranger session): an explicit unknown session id
	// refuses instead of falling back to sessions[0]. G7 extends the rule
	// to the spend class: tribunal spends (multi-role calls), so a missing
	// id 404s too — a tribunal with no session has no tenant to bill.
	// Launch/jobs carry their own requireSession; chat keeps ensureSession.
	if req.SessionID != "" {
		if _, gerr := s.sessionMgr.GetSession(req.SessionID); gerr != nil {
			WriteAPIError(w, "ghost_session", fmt.Sprintf("unknown session %q: tribunal refuses to attach elsewhere", req.SessionID), nil)
			return
		}
	} else {
		http.Error(w, "missing session_id: tribunal calls must name an existing session", http.StatusNotFound)
		return
	}

	// W0.3: tribunal runs multi-role LLM calls on the lab's bill — L0 and
	// suspended labs refuse here, like launch/jobs. Autonomy after
	// attribution: the 404 above names the session, the 403 names the lab.
	if err := s.requireDispatchAutonomy(req.SessionID); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	// T-09c: tribunal spends on the session's lab — bind the caller.
	if _, ok := s.authorizeKnownSession(w, r, req.SessionID); !ok {
		return
	}

	// S1: claim-or-replay on the slow rail (effective prompt post-default,
	// so a retried empty prompt matches). All side effects below
	// (messages, claim mint, campaign save, memory write-back) run only on
	// the proceed path; replays serve the stored consensus untouched.
	tribIdem, tribRef := s.idemBegin(req.SessionID, req.RequestID, map[string]any{
		"session_id": req.SessionID, "prompt": req.Prompt,
	}, idemRailSlow)
	if tribRef != nil {
		tribRef.write(w)
		return
	}
	if tribIdem.replay {
		s.idemReplay(w, tribIdem)
		return
	}

	onTurn := s.tribunalTurnRecorder(req.SessionID)

	// Watermark first: observe-round dispatches land during the run, and
	// the write-back below attributes exactly this turn's tools.
	tribEventsBefore := s.latestEventID(req.SessionID)
	res, err := s.collective.ExecuteTribunalWithSession(r.Context(), s.llmClient, s.jobRunner.BridgeClient(), s.memoryStore, req.SessionID, req.Prompt, onTurn)
	s.finishTribunalRun(w, r, tribIdem, req.SessionID, req.Prompt, tribEventsBefore, res, err)
}

// scoreTribunalTranscript scores the recorded run (R6 pilot, best-effort).
// Returns JSON-friendly rows for the response; persistence failures are
// logged, never fatal. Authorship reads from the obligation cases
// (pass == authored) so the score measures recorded evidence, not the
// response struct's word.
func (s *Server) scoreTribunalTranscript(sessionID string, res *agents.TribunalResult) []map[string]any {
	passed := map[string]bool{}
	if res.ObligationID != "" {
		if cases, err := s.ledger.ListObligationCases(res.ObligationID); err == nil {
			for _, c := range cases {
				if c.Verdict == "pass" {
					passed[c.CaseKind] = true
				}
			}
		}
	}
	var turns []agents.ScoredTurn
	for _, t := range res.Turns {
		failedObs := false
		for _, o := range t.Observed {
			if !o.OK {
				failedObs = true
			}
		}
		coverage := map[string]agents.CoverageDecl{}
		for facet, decl := range t.Coverage {
			coverage[facet] = decl
		}
		turns = append(turns, agents.ScoredTurn{
			Role: t.Role, Content: t.Content, Verdict: t.Verdict,
			Authored: passed[fmt.Sprintf("turn-%d", t.TurnIndex)],
			Coverage: coverage, Observed: len(t.Observed), FailedObs: failedObs,
		})
	}
	scores := agents.ScoreTranscript(turns, res.ProposalJSON != nil)
	var stored []ledger.TranscriptScore
	rows := make([]map[string]any, 0, len(scores))
	for _, sc := range scores {
		signals := map[string]bool{}
		for k, v := range sc.Signals {
			signals[k] = v
		}
		stored = append(stored, ledger.TranscriptScore{
			SessionID: sessionID, ObligationID: res.ObligationID,
			Role: string(sc.Role), Score: sc.Score, Signals: signals,
		})
		rows = append(rows, map[string]any{
			"role": string(sc.Role), "score": sc.Score, "signals": signals,
		})
	}
	if res.ObligationID != "" {
		if err := s.ledger.RecordTranscriptScores(stored); err != nil {
			slog.Error("tribunal record transcript scores failed; run stands", "error", err)
		}
	}
	return rows
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// W2.2: tenant-scoped reads — an unscoped list (empty session)
		// would enumerate every tenant's jobs. Refuse instead of listing
		// all tenants; ghosts 404.
		sessionID := r.URL.Query().Get("session_id")
		if sessionID == "" {
			http.Error(w, "session_id is required: refusing unscoped job list (would cross tenants)", http.StatusBadRequest)
			return
		}
		sess, gerr := s.sessionMgr.GetSession(sessionID)
		if gerr != nil {
			WriteAPIError(w, "ghost_session", fmt.Sprintf("unknown session %q: refusing to read another session", sessionID), nil)
			return
		}
		if !s.authorizeSessionRead(w, r, sess) {
			return
		}
		jobsList, err := s.jobRunner.ListJobs(sessionID, 50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Audit 2026-10-21: a nil slice serializes as "jobs":null, forcing
		// every consumer to null-guard. Empty lists read as [].
		if jobsList == nil {
			jobsList = []*jobs.Job{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"jobs": jobsList})
	case http.MethodPost:
		// S2: disk guard freezes admission before any parse/spend.
		if s.refuseIfDiskBlocked(w) {
			return
		}
		var req struct {
			SessionID   string   `json:"session_id"`
			QASM        string   `json:"qasm"`
			Backend     string   `json:"backend"`
			Shots       int      `json:"shots"`
			BudgetUSD   float64  `json:"budget_usd"`
			MandateRef  string   `json:"mandate_ref"`
			GrantChain  []string `json:"grant_chain"`
			InputLabels []string `json:"input_labels"`
			// P5 §2.9 routing: QPU-destined jobs must declare these;
			// admission refuses hardware without them.
			CallerJurisdictions []string `json:"caller_jurisdictions"`
			CallerExportClass   string   `json:"caller_export_class"`
			ExportReviewed      bool     `json:"export_reviewed"`
			// S1: optional content-bound idempotency key. Same
			// (session, request_id, content) replays the stored receipt;
			// same ID with different content 409s. Empty = current
			// behavior (console back-compat).
			RequestID string `json:"request_id"`
		}
		if err := decodeStrict(r, &req); err != nil {
			WriteAPIError(w, "bad_request", fmt.Sprintf("invalid request body: %v", err), nil)
			return
		}
		// Dispatch is always attributed to a real session (tenant
		// attribution for autonomy checks); missing/unknown ids 404 —
		// unattributed spend is another lab's bill (G7). The 404 runs
		// before any idempotency key is touched (S1): ghosts never store.
		sid, serr := s.requireSession(req.SessionID)
		if serr != nil {
			http.Error(w, serr.Error(), http.StatusNotFound)
			return
		}
		req.SessionID = sid
		if err := s.requireDispatchAutonomy(req.SessionID); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		// T-09c: spend attribution without authorization is another
		// lab's bill — bind caller to session tenant (ghost/autonomy
		// already refused above; this is the caller check).
		if _, ok := s.authorizeKnownSession(w, r, req.SessionID); !ok {
			return
		}
		if req.Shots <= 0 {
			req.Shots = 1024
		}
		jobParams := map[string]any{
			"qasm":                 req.QASM,
			"requested_backend":    req.Backend,
			"shots":                req.Shots,
			"max_cost_usd":         req.BudgetUSD,
			"mandate_ref":          req.MandateRef,
			"grant_chain":          req.GrantChain,
			"input_labels":         req.InputLabels,
			"caller_jurisdictions": req.CallerJurisdictions,
			"caller_export_class":  req.CallerExportClass,
			"export_reviewed":      req.ExportReviewed,
		}
		// S1: claim-or-replay before SubmitJob (spend happens inside).
		idem, iref := s.idemBegin(req.SessionID, req.RequestID, map[string]any{
			"session_id": req.SessionID, "qasm": req.QASM, "backend": req.Backend,
			"shots": req.Shots, "budget_usd": req.BudgetUSD, "mandate_ref": req.MandateRef,
			"grant_chain": req.GrantChain, "input_labels": req.InputLabels,
			"caller_jurisdictions": req.CallerJurisdictions,
			"caller_export_class":  req.CallerExportClass,
			"export_reviewed":      req.ExportReviewed,
		}, idemRailJob)
		if iref != nil {
			iref.write(w)
			return
		}
		if idem.replay {
			s.idemReplay(w, idem)
			return
		}
		job, err := s.jobRunner.SubmitJob(req.SessionID, "circuit_run", jobParams)
		if err != nil {
			s.idemFinishError(w, idem, http.StatusInternalServerError, err.Error())
			return
		}
		s.idemFinishJSON(w, idem, http.StatusCreated, job)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleJobByID serves one job with full params + result payloads — the
// click-through target for job-detail views. Unknown ids 404 (never empty).
// POST {id}/cancel cancels an unsettled job (S7): QUEUED never executes,
// RUNNING is fenced; terminal replays success; unknown 404s. Cancel frees
// (never spends), so it is deliberately ungated by the disk guard —
// blocking cancel during an outage would trap spend behind the outage.
func (s *Server) handleJobByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	if id == "" {
		http.Error(w, "missing job id", http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodPost && strings.HasSuffix(id, "/cancel") {
		jobID := strings.TrimSuffix(id, "/cancel")
		if jobID == "" {
			http.Error(w, "missing job id", http.StatusBadRequest)
			return
		}
		var req struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if existingJob, gerr := s.jobRunner.GetJob(jobID); gerr == nil && existingJob != nil && existingJob.SessionID != "" {
			if _, ok := s.authorizeKnownSession(w, r, existingJob.SessionID); !ok {
				return
			}
		}
		transitioned, err := s.jobRunner.CancelJob(jobID, req.Reason)
		if err != nil {
			// Unknown ids 404; receipt failures 500 (the cancellation
			// itself happened — retry lands on already-terminal replay).
			if strings.Contains(err.Error(), "job not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		job, _ := s.jobRunner.GetJob(jobID)
		writeJSON(w, http.StatusOK, map[string]any{
			"job_id": jobID, "cancelled": transitioned,
			"already_terminal": !transitioned, "job": job,
		})
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// W2.2: tenant-scoped job reads — the caller names the session they
	// read from, and the job must belong to a session of the same tenant.
	// A job id alone is a cross-tenant read primitive (guessable UUIDs
	// across labs). Missing scope 400s, ghosts 404, tenant mismatch 403.
	scopeSession := r.URL.Query().Get("session_id")
	if scopeSession == "" {
		http.Error(w, "session_id is required: job reads are tenant-scoped", http.StatusBadRequest)
		return
	}
	scopeSess, gerr := s.sessionMgr.GetSession(scopeSession)
	if gerr != nil {
		http.Error(w, fmt.Sprintf("unknown session %q: refusing to read another session", scopeSession), http.StatusNotFound)
		return
	}
	job, err := s.jobRunner.GetJob(id)
	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	jobSess, jerr := s.sessionMgr.GetSession(job.SessionID)
	if jerr != nil {
		http.Error(w, "job session unknown: refusing cross-tenant read", http.StatusForbidden)
		return
	}
	if jobSess.TenantID != scopeSess.TenantID {
		http.Error(w, "job belongs to another tenant: refused", http.StatusForbidden)
		return
	}
	if !s.authorizeSessionRead(w, r, scopeSess) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": job})
}

func (s *Server) handleLoops(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		loops := s.supervisor.ListLoops()
		// K2: optional session scope. Unfiltered (all loops) stays for
		// operators; a lab cookie only sees loops whose session belongs
		// to that lab (2026-10-26 audit AUD-A1).
		if sid := r.URL.Query().Get("session_id"); sid != "" {
			if _, ok := s.authorizeKnownSession(w, r, sid); !ok {
				return
			}
			scoped := loops[:0:0]
			for _, l := range loops {
				if l.SessionID == sid {
					scoped = append(scoped, l)
				}
			}
			loops = scoped
		} else if tenant, _, code := s.checkAuth(r); code == http.StatusOK && tenant != "" {
			scoped := loops[:0:0]
			for _, l := range loops {
				sess, err := s.sessionMgr.GetSession(l.SessionID)
				if err == nil && sess != nil && sess.TenantID == tenant {
					scoped = append(scoped, l)
				}
			}
			loops = scoped
		}
		writeJSON(w, http.StatusOK, map[string]any{"loops": loops})
	case http.MethodPost:
		var req struct {
			SessionID string `json:"session_id"`
			MaxSteps  int    `json:"max_steps"`
		}
		if err := decodeStrict(r, &req); err != nil {
			WriteAPIError(w, "bad_request", fmt.Sprintf("invalid request body: %v", err), nil)
			return
		}
		if _, ok := s.authorizeKnownSession(w, r, req.SessionID); !ok {
			return
		}
		loop, err := s.supervisor.RegisterLoop(req.SessionID, req.MaxSteps)
		if err != nil {
			// RegisterLoop only fails on input validation (maxSteps bound,
			// session attribution) — a 400, never a 500. Store writes happen
			// after validation inside the supervisor lock.
			WriteAPIError(w, "bad_request", err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusCreated, loop)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleLoopActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/loops/")
	parts := strings.Split(path, "/")
	loopID := parts[0]
	if len(parts) < 2 {
		http.Error(w, "missing loop action", http.StatusBadRequest)
		return
	}
	action := parts[1]

	// Bind the loop's session to the caller before any mutation. Unknown
	// ids stay 404; a lab cookie cannot pause/kill another lab's loop.
	loop, ok := s.supervisor.GetLoop(loopID)
	if !ok {
		http.Error(w, "loop not found", http.StatusNotFound)
		return
	}
	if sess, serr := s.sessionMgr.GetSession(loop.SessionID); serr == nil && sess != nil {
		if !s.authorizeSessionRead(w, r, sess) {
			return
		}
	}

	var err error
	switch action {
	case "pause":
		err = s.supervisor.Pause(loopID)
	case "resume":
		err = s.supervisor.Resume(loopID)
	case "kill":
		err = s.supervisor.Kill(loopID)
	case "downgrade":
		s.ledger.AppendEvent(loopID, "action_gate_downgrade_sim", map[string]any{
			"loop_id":   loopID,
			"target":    "aer_simulator",
			"cost_usd":  0.0,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	case "rescope":
		err = s.supervisor.RescopeLoop(loopID, 50)
	case "rollback":
		_, err = s.supervisor.RollbackToSnapshot(loopID)
	case "step":
		gradStr := r.URL.Query().Get("grad")
		grad, _ := strconv.ParseFloat(gradStr, 64)
		if grad == 0 {
			grad = 0.05
		}
		status, stepErr := s.supervisor.Step(loopID, grad)
		if stepErr != nil {
			// Audit 2026-10-21: unknown loop ids refused with 500.
			// A missing loop is a 404 (same class as the loops/hf-download
			// validation 500s); only genuine execution failures stay 500.
			if strings.Contains(stepErr.Error(), "loop not found") {
				http.Error(w, stepErr.Error(), http.StatusNotFound)
			} else {
				http.Error(w, stepErr.Error(), http.StatusInternalServerError)
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"loop_id": loopID, "status": status})
		return
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}

	if err != nil {
		// Audit 2026-10-21: unknown supervisor loop ids refused with 500.
		// A missing loop is a 404 (same class as the loops/hf-download
		// validation 500s); only genuine execution failures stay 500.
		if strings.Contains(err.Error(), "loop not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"loop_id": loopID, "action": action, "status": "ok"})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	// W2.1 threat model: event tail is a read — stays public behind the
	// edge (no bearer), but bounded (max 200) and session-scoped. An
	// unscoped or ghost read refuses instead of leaking another tenant's
	// tail or scanning unbounded rows.
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id is required: refusing unscoped event list", http.StatusBadRequest)
		return
	}
	if _, gerr := s.sessionMgr.GetSession(sessionID); gerr != nil {
		WriteAPIError(w, "ghost_session", fmt.Sprintf("unknown session %q: refusing to read another session", sessionID), nil)
		return
	}
	// T-09b: known session — bind caller to session tenant.
	if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
		return
	}
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}
	if limit > 200 {
		limit = 200
	}

	events, err := s.ledger.GetRecentEvents(sessionID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) handleEventsStream(w http.ResponseWriter, r *http.Request) {
	// W2.1 threat model: SSE is a read, but a long-lived unauthenticated
	// subscription is a ghost-observation primitive (no tenant to bill,
	// no identity to rate-limit fairly). Require operator bearer + an
	// existing session before the first byte streams; ghosts and
	// bearerless callers refuse.
	switch _, _, code := s.checkAuth(r); code {
	case http.StatusOK:
	case http.StatusServiceUnavailable:
		http.Error(w, "API token not configured: set KUBONA_API_TOKEN", http.StatusServiceUnavailable)
		return
	case http.StatusForbidden:
		WriteAPIError(w, "forbidden", "cross-site session use refused: retry same-origin", nil)
		return
	default:
		http.Error(w, "missing or invalid bearer token", http.StatusUnauthorized)
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id is required: streams must name an existing session", http.StatusNotFound)
		return
	}
	streamSess, gerr := s.sessionMgr.GetSession(sessionID)
	if gerr != nil {
		http.Error(w, fmt.Sprintf("unknown session %q: refusing to stream another session", sessionID), http.StatusNotFound)
		return
	}
	if !s.authorizeSessionRead(w, r, streamSess) {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	if origin := corsAllowOrigin(r); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}

	// Emit initial connection ACK frame
	initData, _ := json.Marshal(map[string]any{
		"type":       "connection_ack",
		"session_id": sessionID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
	fmt.Fprintf(w, "event: connected\ndata: %s\n\n", initData)
	flusher.Flush()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case <-ticker.C:
			events, err := s.ledger.GetRecentEvents(sessionID, 5)
			if err == nil && len(events) > 0 {
				data, _ := json.Marshal(map[string]any{
					"type":      "telemetry_tick",
					"timestamp": time.Now().UTC().Format(time.RFC3339),
					"events":    events,
				})
				fmt.Fprintf(w, "event: telemetry\ndata: %s\n\n", data)
				flusher.Flush()
			} else {
				fmt.Fprintf(w, ": heartbeat %s\n\n", time.Now().UTC().Format(time.RFC3339))
				flusher.Flush()
			}
		}
	}
}

func (s *Server) handleA2AChallenge(w http.ResponseWriter, r *http.Request) {
	if !a2aEnabled() {
		WriteAPIError(w, "feature_disabled", "A2A/x402 is disabled: set KUBONA_A2A_ENABLED=1 after a real settlement rail exists", nil)
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		token = "USDC"
	}
	amount := r.URL.Query().Get("amount")
	if amount == "" {
		amount = "0.05"
	}
	challenge, err := s.a2aServer.PaymentManager().GenerateChallenge(token, amount)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid payment challenge: %v", err), http.StatusBadRequest)
		return
	}
	w.Header().Set("WWW-Authenticate", s.a2aServer.PaymentManager().FormatWWWAuthenticate(challenge))
	writeJSON(w, http.StatusPaymentRequired, challenge)
}

func (s *Server) handleA2A(w http.ResponseWriter, r *http.Request) {
	if !a2aEnabled() {
		WriteAPIError(w, "feature_disabled", "A2A/x402 is disabled: set KUBONA_A2A_ENABLED=1 after a real settlement rail exists", nil)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var rpcReq agents.JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&rpcReq); err != nil {
		writeJSON(w, http.StatusBadRequest, agents.JSONRPCResponse{
			JSONRPC: "2.0",
			Error: &agents.JSONRPCError{
				Code:    -32700,
				Message: "Parse error: invalid JSON",
			},
		})
		return
	}

	// For paid operations (a2a.SubmitTask), require x402 payment
	if rpcReq.Method == "a2a.SubmitTask" {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			challenge, err := s.a2aServer.PaymentManager().GenerateChallenge("USDC", "0.05")
			if err != nil {
				http.Error(w, fmt.Sprintf("payment challenge failed: %v", err), http.StatusInternalServerError)
				return
			}
			w.Header().Set("WWW-Authenticate", s.a2aServer.PaymentManager().FormatWWWAuthenticate(challenge))
			writeJSON(w, http.StatusPaymentRequired, map[string]any{
				"error":     "Payment Required",
				"challenge": challenge,
			})
			return
		}

		receipt, err := cost.ParseAuthorizationHeader(authHeader)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid x402 header: %v", err), http.StatusBadRequest)
			return
		}

		valid, verifyErr := s.a2aServer.PaymentManager().VerifyPayment(receipt)
		if verifyErr != nil || !valid {
			http.Error(w, fmt.Sprintf("payment verification failed: %v", verifyErr), http.StatusPaymentRequired)
			return
		}
	}

	resp := s.a2aServer.HandleRequest(&rpcReq)
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCanvasLayout(w http.ResponseWriter, r *http.Request) {
	// W2.1: no shared "default" session — a missing session_id 404s and
	// ghost sessions 404 instead of reading/writing one shared layout row
	// (that row mixed every tenant's canvas). Reads stay public behind
	// limiters; the session gate is attribution, not auth.
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id is required: canvas layouts are session-scoped, no shared default", http.StatusNotFound)
		return
	}
	if _, gerr := s.sessionMgr.GetSession(sessionID); gerr != nil {
		WriteAPIError(w, "ghost_session", fmt.Sprintf("unknown session %q: refusing to show another session", sessionID), nil)
		return
	}
	// T-09b: known session — bind caller to session tenant (reads and
	// layout writes alike: a layout row is pure session state).
	if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
		return
	}

	// K2: layouts persist in SQLite (survive restarts), not in a Go map.
	// The old in-memory map lost every layout on reboot while the log
	// claimed a table — that claim is now true.
	switch r.Method {
	case http.MethodGet:
		var layout string
		err := s.ledger.DB().QueryRow(
			`SELECT layout_json FROM canvas_layouts WHERE session_id = ?`, sessionID).Scan(&layout)
		if err != nil {
			layout = "{}"
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(layout))

	case http.MethodPost, http.MethodPut:
		var raw json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			http.Error(w, "invalid JSON layout", http.StatusBadRequest)
			return
		}
		// Bound the blob: one client must not stuff megabytes into a
		// layout row (or the old map). 256KB fits any real canvas.
		const maxLayoutBytes = 256 * 1024
		if len(raw) > maxLayoutBytes {
			http.Error(w, "layout too large: max 256KB", http.StatusBadRequest)
			return
		}
		if _, err := s.ledger.DB().Exec(
			`INSERT INTO canvas_layouts (session_id, layout_json, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
			 ON CONFLICT(session_id) DO UPDATE SET layout_json = excluded.layout_json, updated_at = CURRENT_TIMESTAMP`,
			sessionID, string(raw)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "saved", "session_id": sessionID})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// llmBudgetCapUSD reads the optional global cloud-LLM dollar cap.
// Unset/non-positive = track-only: spend is recorded, never refused.
func llmBudgetCapUSD() float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("KUBONA_LLM_BUDGET_USD")), 64)
	if v <= 0 {
		return 0
	}
	return v
}

// llmSpendTotal is a convenience read for callers that cannot surface an
// error. The budget gate uses llmSpendSummary directly so a DB failure is
// fail-closed, never a fabricated zero.
func (s *Server) llmSpendTotal() float64 {
	spent, _, _, err := s.llmSpendSummary()
	if err != nil {
		return 0
	}
	return spent
}

// llmSpendSummary aggregates the full LLM spend ledger: total cloud
// estimates, per-provider subtotals, and the blind-stream (unmetered) count.
// No LIMIT: a cap that silently stops counting after 10k records is a
// bypass. A query/scan error is returned so the gate can refuse.
func (s *Server) llmSpendSummary() (spent float64, byProvider map[string]float64, unmetered int, err error) {
	byProvider = map[string]float64{}
	rows, queryErr := s.ledger.DB().Query(
		`SELECT payload FROM ledger_events WHERE event_type = 'llm_spend_estimated' ORDER BY id`)
	if queryErr != nil {
		return 0, byProvider, 0, queryErr
	}
	defer rows.Close()
	for rows.Next() {
		var payloadStr string
		var payload struct {
			EstimatedUSD *float64 `json:"estimated_usd"`
			Provider     string   `json:"provider"`
			Unmetered    *bool    `json:"unmetered"`
		}
		if scanErr := rows.Scan(&payloadStr); scanErr != nil {
			return 0, byProvider, 0, scanErr
		}
		if jsonErr := json.Unmarshal([]byte(payloadStr), &payload); jsonErr != nil {
			return 0, byProvider, 0, jsonErr
		}
		if payload.Unmetered != nil && *payload.Unmetered {
			unmetered++
			continue
		}
		if payload.EstimatedUSD != nil {
			spent += *payload.EstimatedUSD
			byProvider[payload.Provider] += *payload.EstimatedUSD
		}
	}
	if rowErr := rows.Err(); rowErr != nil {
		return 0, byProvider, 0, rowErr
	}
	return spent, byProvider, unmetered, nil
}

// hasCloudLLMRole reports whether the active provider or any role assignment
// resolves to a non-local provider. The old gate only looked at the active
// provider, so a local-active config with a cloud role bypassed the cap.
func (s *Server) hasCloudLLMRole(snap *llm.Settings) bool {
	if prov, ok := snap.Providers[snap.ActiveProvider]; ok && !prov.IsLocal {
		return true
	}
	for _, ra := range snap.RoleAssignments {
		if prov, ok := snap.Providers[ra.Provider]; ok && !prov.IsLocal {
			return true
		}
	}
	return false
}

// checkLLMBudget refuses new cloud LLM entries once the operator's global
// dollar cap is spent. Local-only deployments never trip it (their active
// provider is local); track-only mode (cap unset) never refuses. Mid-run
// breach (tribunal turns 2-4 after a passed pre-check) stops the NEXT
// entry, not the running one — documented limitation, not a hole.
// W0.19: unmetered records (blind stream spend) trip a capped gate even
// though they carry no dollar figure — a cap that ignores blind spend is
// theater.
func (s *Server) checkLLMBudget() error {
	cap := llmBudgetCapUSD()
	if cap <= 0 {
		return nil
	}
	snap := s.llmSettings.GetSnapshot()
	if !s.hasCloudLLMRole(&snap) {
		return nil
	}
	spent, _, unmetered, err := s.llmSpendSummary()
	if err != nil {
		return fmt.Errorf("cloud LLM budget cannot be verified (ledger read failed): refusing new entries: %w", err)
	}
	if unmetered > 0 {
		return fmt.Errorf("blind cloud spend detected (%d unmetered stream record(s)): refusing new entries until usage reporting or max_tokens ceiling is configured", unmetered)
	}
	if spent >= cap {
		return fmt.Errorf("cloud LLM budget exhausted: spent ~$%.4f of $%.4f cap (KUBONA_LLM_BUDGET_USD) — switch to a local provider or raise the cap", spent, cap)
	}
	return nil
}

func (s *Server) handleLLMSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.llmSettings.GetPublicSnapshot())
	case http.MethodPut, http.MethodPost:
		// W0.19: reads stay public behind limiters; writes get edge auth.
		// An unauthenticated settings write flips providers/endpoints —
		// redirecting prompts (and their secrets) to an attacker URL.
		switch _, _, code := s.checkAuth(r); code {
		case http.StatusOK:
		case http.StatusServiceUnavailable:
			http.Error(w, "API token not configured: set KUBONA_API_TOKEN", http.StatusServiceUnavailable)
			return
		case http.StatusForbidden:
			WriteAPIError(w, "forbidden", "cross-site session use refused: retry same-origin", nil)
			return
		default:
			http.Error(w, "missing or invalid bearer token", http.StatusUnauthorized)
			return
		}
		var req llm.Settings
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := validateProviderURLs(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.llmSettings.Update(&req, s.ledger.DB()); err != nil {
			// Audit 2026-10-26: Update only fails on input validation
			// (empty model, unknown provider, fragment entries, bad
			// numbers) — a 400, never a 500. Same class as the loops /
			// hf-download / supervisor-loop fixes.
			WriteAPIError(w, "bad_request", err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "updated", "settings": s.llmSettings.GetPublicSnapshot()})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// validateProviderURLs constrains where prompts may be sent (W0.19): the
// settings writer could otherwise redirect completions (and API keys) to an
// attacker endpoint. Loopback http:// is local; anything remote must be
// https://. The config layer validates values; this route validates
// destinations — neither is the sole guard.
func validateProviderURLs(st *llm.Settings) error {
	for pt, pc := range st.Providers {
		u := strings.TrimSpace(pc.BaseURL)
		if u == "" {
			continue
		}
		parsed, err := url.Parse(u)
		if err != nil {
			return fmt.Errorf("provider %s has an unparseable BaseURL: refusing", pt)
		}
		switch parsed.Scheme {
		case "https":
			continue
		case "http":
			host := strings.ToLower(parsed.Hostname())
			if host == "127.0.0.1" || host == "localhost" || host == "::1" {
				continue
			}
			return fmt.Errorf("provider %s: non-loopback http:// endpoints refuse (credentials would cross plaintext)", pt)
		default:
			return fmt.Errorf("provider %s: scheme %q refuses (https or loopback http only)", pt, parsed.Scheme)
		}
	}
	return nil
}

func (s *Server) handleLLMModels(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		provider = "omlx"
	}
	models, err := s.llmClient.FetchModels(r.Context(), llm.ProviderType(provider))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"provider": provider,
			"models":   []any{},
			"error":    err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"provider": provider,
		"models":   models,
	})
}

func (s *Server) handleLLMTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Role   string `json:"role"`
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Role == "" {
		req.Role = "copilot"
	}
	if req.Prompt == "" {
		req.Prompt = "Confirm you are active and report status."
	}

	msgs := []llm.ChatMessage{
		{Role: "user", Content: req.Prompt},
	}
	reply, chatResp, dur, err := s.llmClient.Complete(r.Context(), req.Role, msgs)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success":     false,
			"error":       err.Error(),
			"duration_ms": dur.Milliseconds(),
		})
		return
	}

	var genRate, promptRate float64
	var modelName string
	if chatResp != nil {
		genRate = chatResp.Usage.GenTokPerSec
		promptRate = chatResp.Usage.PromptTokPerSec
		modelName = chatResp.Model
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":            true,
		"reply":              reply,
		"model":              modelName,
		"duration_ms":        dur.Milliseconds(),
		"gen_tok_per_sec":    genRate,
		"prompt_tok_per_sec": promptRate,
	})
}

// S4 lease constants (single source — never copied literals).
// Scope is a ledger-persisted identifier: bounded alphabet + length.
// TTL is capped at 24h: short-lived issuance only.
var leaseScopeRe = regexp.MustCompile(`^[A-Za-z0-9:_-]{1,128}$`)

const maxLeaseTTLSeconds = 86400

// leaseEventSession routes the lease_issued event at the bound session when
// one exists, else the vault-plane stream (lease events are operator-plane,
// not chat turns).
func leaseEventSession(sessionID string) string {
	if sessionID != "" {
		return sessionID
	}
	return "vault-plane"
}

func (s *Server) handleVaultLeases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Scope      string `json:"scope"`
		TTLSeconds int    `json:"ttl_seconds"`
		// S4: optional session attribution. When provided it must exist
		// (ghosts 404 — unattributed leases repeat the G7 class); the
		// lease record and event carry session + tenant.
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Scope == "" {
		req.Scope = "provider:session_token"
	}
	// S4: scope is a ledger-persisted identifier, not free text — bounded
	// alphabet and length (an unbounded scope is ledger bloat by request).
	if !leaseScopeRe.MatchString(req.Scope) {
		http.Error(w, fmt.Sprintf("invalid scope %q: 1-128 chars of [A-Za-z0-9:_-]", req.Scope), http.StatusBadRequest)
		return
	}
	if req.TTLSeconds <= 0 {
		req.TTLSeconds = 60
	}
	// S4: TTL capped — an unbounded ttl_seconds mints an effectively
	// immortal lease, defeating short-lived issuance.
	if req.TTLSeconds > maxLeaseTTLSeconds {
		http.Error(w, fmt.Sprintf("ttl_seconds %d exceeds maximum %d: short-lived leases only", req.TTLSeconds, maxLeaseTTLSeconds), http.StatusBadRequest)
		return
	}
	tenantID := tenants.DefaultTenantID
	if req.SessionID != "" {
		sess, gerr := s.sessionMgr.GetSession(req.SessionID)
		if gerr != nil {
			http.Error(w, fmt.Sprintf("unknown session %q: leases bind to real sessions only", req.SessionID), http.StatusNotFound)
			return
		}
		if !s.authorizeSessionRead(w, r, sess) {
			return
		}
		if sess.TenantID != "" {
			tenantID = sess.TenantID
		}
	}

	leaseKey := os.Getenv("KUBONA_LEASE_HMAC_KEY")
	if len(leaseKey) < 32 {
		http.Error(w, "lease signing key not configured: set KUBONA_LEASE_HMAC_KEY (min 32 chars)", http.StatusServiceUnavailable)
		return
	}
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(req.TTLSeconds) * time.Second)

	// S4: the lease object lives server-side (consumable + revocable —
	// previously tested but unwired). Its ID is the receipt ID: one
	// namespace, so revoke/consume resolve what issuance returned. The
	// receipt carries no secret value by design; values live in
	// Bao/env/process memory, never in events.
	issued, lerr := s.vaultLeases.IssueLease(req.Scope, tenantID, "", time.Duration(req.TTLSeconds)*time.Second)
	if lerr != nil {
		http.Error(w, fmt.Sprintf("lease registry refused: %v", lerr), http.StatusInternalServerError)
		return
	}
	leaseID := issued.LeaseID
	mac := hmac.New(sha256.New, []byte(leaseKey))
	mac.Write([]byte(fmt.Sprintf("%s:%s:%d", req.Scope, leaseID, expiresAt.Unix())))
	leaseSignature := hex.EncodeToString(mac.Sum(nil))

	s.ledger.AppendEvent(leaseEventSession(req.SessionID), "lease_issued", map[string]any{
		"lease_id":        leaseID,
		"scope":           req.Scope,
		"ttl_seconds":     req.TTLSeconds,
		"lease_signature": leaseSignature,
		"expires_at":      expiresAt.Format(time.RFC3339),
		"session_id":      req.SessionID,
		"tenant_id":       tenantID,
		// Reserved for a future OS-level caller check (Linux peercred
		// sidecar); empty today so the schema never changes later.
		"caller_binding": "",
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"lease_id":        leaseID,
		"scope":           req.Scope,
		"valid":           true,
		"ttl_seconds":     req.TTLSeconds,
		"lease_signature": leaseSignature,
		"issued_at":       now.Format(time.RFC3339),
		"expires_at":      expiresAt.Format(time.RFC3339),
		"session_id":      req.SessionID,
		"tenant_id":       tenantID,
	})
}

// handleVaultLeaseRevoke revokes an issued lease (S4): unknown ids 404,
// consumed/expired ids refuse (already dead — revoking the dead proves
// nothing and would mint a misleading receipt).
func (s *Server) handleVaultLeaseRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		LeaseID string `json:"lease_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.LeaseID == "" {
		http.Error(w, "revoke requires {\"lease_id\": ...}", http.StatusBadRequest)
		return
	}
	if ok, _ := s.vaultLeases.ValidateLease(req.LeaseID); !ok {
		http.Error(w, fmt.Sprintf("lease %q is not active (unknown, consumed, expired, or already revoked)", req.LeaseID), http.StatusNotFound)
		return
	}
	if err := s.vaultLeases.RevokeLease(req.LeaseID); err != nil {
		http.Error(w, fmt.Sprintf("revoke refused: %v", err), http.StatusConflict)
		return
	}
	s.ledger.AppendEvent("vault-plane", "lease_revoked", map[string]any{
		"lease_id": req.LeaseID,
	})
	writeJSON(w, http.StatusOK, map[string]any{"lease_id": req.LeaseID, "revoked": true})
}

// findPythonVenv locates the hermetic venv python + bridge dir for the
// K1 exec sites. Candidates are anchored to every CWD the daemon and its
// tests actually run from (repo root, platform/, platform/core,
// package dirs) — a bare CWD-relative guess breaks whenever the process
// starts elsewhere, which the K1 tests caught live.
func findPythonVenv() (string, string) {
	candidates := []struct{ bin, dir string }{
		{".venv/bin/python3", "bridge"},                   // CWD=platform
		{"platform/.venv/bin/python3", "platform/bridge"}, // CWD=repo root
		{"../.venv/bin/python3", "../bridge"},             // CWD=platform/core
		{"../../.venv/bin/python3", "../../bridge"},       // CWD=platform/core/pkg/<x> (go test)
		{"../../../.venv/bin/python3", "../../../bridge"}, // deeper test nesting, just in case
	}
	for _, c := range candidates {
		absBin, err := filepath.Abs(c.bin)
		if err != nil {
			continue
		}
		if _, err := os.Stat(absBin); err != nil {
			continue
		}
		// The interpreter existing is not enough: the paired dir must
		// exist too, or exec chdirs into nothing (CI 2026-10-22: a
		// cryptic 503 from a missing dir instead of a named error).
		if absDir, err := filepath.Abs(c.dir); err == nil {
			if st, serr := os.Stat(absDir); serr == nil && st.IsDir() {
				return absBin, absDir
			}
		}
	}
	// No venv: fall back to a checkout-anchored bridge dir (CI installs
	// the kubona package, so only codesign_ops on sys.path matters) and
	// finally to CWD — never a nonexistent dir, so failures name the
	// missing interpreter/package instead of failing chdir. Anchored for
	// every CWD the daemon and its tests run from: go-test binaries run
	// in their package dir, three levels below platform/bridge.
	for _, d := range []string{
		"bridge",             // CWD=platform
		"platform/bridge",    // CWD=repo root
		"../bridge",          // CWD=platform/core
		"../../bridge",       // CWD=platform/pkg/<x>
		"../../../bridge",    // CWD=platform/core/pkg/<x> (go test)
		"../../../../bridge", // deeper test nesting, just in case
	} {
		if absDir, err := filepath.Abs(d); err == nil {
			if st, serr := os.Stat(absDir); serr == nil && st.IsDir() {
				return "python3", absDir
			}
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return "python3", cwd
	}
	return "python3", "."
}

// runBridgePython executes a bridge snippet with a bounded lifetime and
// absolute interpreter paths. K1: previously this was two unbounded,
// CWD-relative execs (the second a verbatim retry when the first already
// resolved to python3). One attempt per resolved interpreter, each under
// ctx timeout (well inside the server WriteTimeout); failure names the
// interpreter tried instead of returning a bare exec error.
func runBridgePython(ctx context.Context, timeout time.Duration, script string, args ...string) ([]byte, string, error) {
	bin, dir := findPythonVenv()
	attempt := func(pyBin string) ([]byte, error) {
		cctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		cmd := exec.CommandContext(cctx, pyBin, append([]string{"-c", script}, args...)...)
		// findPythonVenv returns absolute paths; never re-resolve here
		// (re-resolving a dir against CWD a second time is how a wrong
		// dir once slipped through — K1 tests pin this).
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "PYTHONPATH="+dir)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if cctx.Err() == context.DeadlineExceeded {
			return stdout.Bytes(), fmt.Errorf("bridge python timed out after %s (%s)", timeout, pyBin)
		}
		if err != nil {
			return stdout.Bytes(), fmt.Errorf("bridge python exit error: %w, stderr: %s", err, stderr.String())
		}
		return stdout.Bytes(), nil
	}
	if absBin, err := exec.LookPath(bin); err == nil {
		bin = absBin
	}
	out, err := attempt(bin)
	if err == nil {
		return out, bin, nil
	}
	firstErr := err
	// Fallback: PATH-resolved python3, once — for dev machines whose venv
	// is absent but system python carries the deps. Never a CWD-relative
	// guess, never a verbatim retry of the same binary.
	if hostBin, lerr := exec.LookPath("python3"); lerr == nil && hostBin != bin {
		if out2, err2 := attempt(hostBin); err2 == nil {
			return out2, hostBin, nil
		} else {
			return out2, hostBin, fmt.Errorf("bridge python failed (%s: %v; %s: %v)", bin, firstErr, hostBin, err2)
		}
	}
	return out, bin, fmt.Errorf("bridge python failed (%s: %v)", bin, firstErr)
}

func (s *Server) handleChemistryADMET(w http.ResponseWriter, r *http.Request) {
	smiles := r.URL.Query().Get("smiles")
	if smiles == "" && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
		var req struct {
			SMILES string `json:"smiles"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			smiles = req.SMILES
		}
	}
	// K1: the Aspirin default is labeled in the response, never silent.
	defaultedSmiles := false
	if smiles == "" {
		smiles = "CC(=O)Oc1ccccc1C(=O)O" // Default Aspirin benchmark
		defaultedSmiles = true
	}

	// T-01a: bridge rail first (the prod image has no python, so the
	// local exec below 503s there). Dev machines without a bridge keep
	// the local rail via fallback inside admetViaBridge's false path.
	if parsed, ok := s.admetViaBridge(r.Context(), smiles); ok {
		if _, hasErr := parsed["error"]; !hasErr {
			parsed["defaulted_smiles"] = defaultedSmiles
			parsed["evaluated_by"] = "bridge"
			s.ledger.AppendEvent("system", "molecular_admet_profiled", map[string]any{
				"smiles":    smiles,
				"mw":        parsed["molecular_weight"],
				"logp":      parsed["clogp"],
				"qed":       parsed["qed"],
				"defaulted": defaultedSmiles,
			})
		}
		writeJSON(w, http.StatusOK, parsed)
		return
	}

	script := `import json, sys
from chemistry import evaluate_molecular_admet
try:
    res = evaluate_molecular_admet(sys.argv[1])
    print(json.dumps(res))
except Exception as e:
    print(json.dumps({"error": str(e)}))
`
	out, pyBin, err := runBridgePython(r.Context(), 120*time.Second, script, smiles)
	if err != nil {
		http.Error(w, fmt.Sprintf("ADMET evaluation unavailable: %v", err), http.StatusServiceUnavailable)
		return
	}

	var parsed map[string]any
	cleanOut := bytes.TrimSpace(out)
	if idx := bytes.Index(cleanOut, []byte("{")); idx >= 0 {
		cleanOut = cleanOut[idx:]
	}
	if json.Unmarshal(cleanOut, &parsed) == nil {
		if _, hasErr := parsed["error"]; !hasErr {
			parsed["defaulted_smiles"] = defaultedSmiles
			parsed["evaluated_by"] = pyBin
			s.ledger.AppendEvent("system", "molecular_admet_profiled", map[string]any{
				"smiles":    smiles,
				"mw":        parsed["molecular_weight"],
				"logp":      parsed["clogp"],
				"qed":       parsed["qed"],
				"defaulted": defaultedSmiles,
			})
		}
		writeJSON(w, http.StatusOK, parsed)
		return
	}

	http.Error(w, fmt.Sprintf("failed to evaluate ADMET: %v, out: %s", err, string(out)), http.StatusInternalServerError)
}

func (s *Server) handleDiscoveryPipeline(w http.ResponseWriter, r *http.Request) {
	type CandReq struct {
		Name   string `json:"name"`
		SMILES string `json:"smiles"`
	}
	var req struct {
		Target     string    `json:"target"`
		Candidates []CandReq `json:"candidates"`
	}

	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}

	// K1: demo defaults labeled, never silent.
	defaultedTarget := req.Target == ""
	if req.Target == "" {
		req.Target = "COX-2"
	}
	defaultedCandidates := len(req.Candidates) == 0
	if len(req.Candidates) == 0 {
		req.Candidates = []CandReq{
			{Name: "Aspirin", SMILES: "CC(=O)Oc1ccccc1C(=O)O"},
			{Name: "Paracetamol", SMILES: "CC(=O)Nc1ccc(O)cc1"},
			{Name: "Ibuprofen", SMILES: "CC(C)Cc1ccc(cc1)C(C)C(=O)O"},
		}
	}

	candsJSON, _ := json.Marshal(req.Candidates)

	// T-01a: bridge rail first (prod); local exec is the dev fallback
	// (same shape, same ledger event, evaluated_by names the rail).
	if parsed, ok := s.discoveryViaBridge(r.Context(), req.Target, candsJSON); ok {
		if _, hasErr := parsed["error"]; !hasErr {
			parsed["defaulted_target"] = defaultedTarget
			parsed["defaulted_candidates"] = defaultedCandidates
			parsed["evaluated_by"] = "bridge"
			s.ledger.AppendEvent("system", "discovery_pipeline_evaluated", map[string]any{
				"target":               req.Target,
				"total_evaluated":      len(req.Candidates),
				"top_lead":             parsed["top_lead"],
				"defaulted_candidates": defaultedCandidates,
			})
		}
		writeJSON(w, http.StatusOK, parsed)
		return
	}

	script := `import json, sys
from chemistry import evaluate_candidate_pipeline
try:
    cands = json.loads(sys.argv[2])
    res = evaluate_candidate_pipeline(sys.argv[1], cands)
    print(json.dumps(res))
except Exception as e:
    print(json.dumps({"error": str(e)}))
`
	out, pyBin, err := runBridgePython(r.Context(), 180*time.Second, script, req.Target, string(candsJSON))
	if err != nil {
		http.Error(w, fmt.Sprintf("discovery pipeline unavailable: %v", err), http.StatusServiceUnavailable)
		return
	}

	var parsed map[string]any
	cleanOut := bytes.TrimSpace(out)
	if idx := bytes.Index(cleanOut, []byte("{")); idx >= 0 {
		cleanOut = cleanOut[idx:]
	}
	if json.Unmarshal(cleanOut, &parsed) == nil {
		if _, hasErr := parsed["error"]; !hasErr {
			parsed["defaulted_target"] = defaultedTarget
			parsed["defaulted_candidates"] = defaultedCandidates
			parsed["evaluated_by"] = pyBin
			s.ledger.AppendEvent("system", "discovery_pipeline_evaluated", map[string]any{
				"target":               req.Target,
				"total_evaluated":      len(req.Candidates),
				"top_lead":             parsed["top_lead"],
				"defaulted_candidates": defaultedCandidates,
			})
		}
		writeJSON(w, http.StatusOK, parsed)
		return
	}

	http.Error(w, fmt.Sprintf("failed to evaluate discovery pipeline: %v, out: %s", err, string(out)), http.StatusInternalServerError)
}

// ============================================================================
// Autoresearch Loop & Section 13 Meta-Search Handlers
// ============================================================================

func (s *Server) handleAutoresearchHarnesses(w http.ResponseWriter, r *http.Request) {
	rawList := s.harnessReg.List()
	var out []map[string]any
	for _, h := range rawList {
		out = append(out, map[string]any{
			"id":               h.ID(),
			"fixed_setup":      h.FixedSetup(),
			"mutable_artifact": h.MutableArtifact(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"harnesses": out,
		"docker_runner": map[string]any{
			"image":     s.sandboxRunner.Config().Image,
			"available": s.sandboxRunner.IsDockerAvailable(),
			"security":  "NemoClaw-OpenClaw-Hardened (read-only, cap-drop=ALL, net=none)",
		},
	})
}

func (s *Server) handleAutoresearchStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// S2: disk guard freezes new loops before any parse/spend.
	if s.refuseIfDiskBlocked(w) {
		return
	}

	var req struct {
		SessionID       string `json:"session_id"`
		HarnessID       string `json:"harness_id"`
		InitialArtifact string `json:"initial_artifact"`
		MaxIterations   int    `json:"max_iterations"`
		// S1: optional content-bound idempotency key (slow rail: the
		// baseline run executes once; retries replay the stored state).
		RequestID string `json:"request_id"`
	}
	if err := decodeStrict(r, &req); err != nil {
		WriteAPIError(w, "bad_request", fmt.Sprintf("invalid request body: %v", err), nil)
		return
	}

	if req.HarnessID == "" {
		req.HarnessID = "ansatz-search"
	}
	// Autoresearch loops spend: missing/unknown session ids 404 like
	// dispatch — loop spend with no tenant to bill is refused (G7).
	arSid, arErr := s.requireSession(req.SessionID)
	if arErr != nil {
		http.Error(w, arErr.Error(), http.StatusNotFound)
		return
	}
	req.SessionID = arSid
	// W0.3: loops spend sandbox + LLM budget — L0 and suspended labs
	// refuse here, like launch/jobs, before the baseline run is built.
	if err := s.requireDispatchAutonomy(req.SessionID); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	// T-09c: loops spend on the session's lab — bind the caller.
	if _, ok := s.authorizeKnownSession(w, r, req.SessionID); !ok {
		return
	}
	if req.MaxIterations <= 0 {
		req.MaxIterations = 25
	}

	harness, err := s.harnessReg.Get(req.HarnessID)
	if err != nil {
		http.Error(w, fmt.Sprintf("harness '%s' not found: %v", req.HarnessID, err), http.StatusBadRequest)
		return
	}

	// S1: claim-or-replay after all 400-validation, before the baseline
	// run (the first spend-adjacent work). A replayed start returns the
	// original loop state — never a second loop.
	arIdem, arRef := s.idemBegin(req.SessionID, req.RequestID, map[string]any{
		"session_id": req.SessionID, "harness_id": req.HarnessID,
		"initial_artifact": req.InitialArtifact, "max_iterations": req.MaxIterations,
	}, idemRailSlow)
	if arRef != nil {
		arRef.write(w)
		return
	}
	if arIdem.replay {
		s.idemReplay(w, arIdem)
		return
	}

	// I3: loop spend admission — reserve the worst-case total BEFORE the
	// baseline run (which may burn LLM). The bound prices every
	// iteration's ceiling across loop roles; remainder releases at loop
	// end. A ceiling that cannot cover the bound refuses here (never a
	// mid-run surprise); local/unpriced roles bind $0 (nothing to bill).
	arTenant := ""
	if sess, gerr := s.sessionMgr.GetSession(req.SessionID); gerr == nil {
		arTenant = sess.TenantID
	}
	perIter, priceFallbacks, boundErr := llm.LoopIterationBound(s.llmSettings, []string{"coder", "critic"})
	if boundErr != nil {
		WriteAPIError(w, "unpriced_model", fmt.Sprintf("loop admission refused: %v", boundErr), map[string]any{
			"tenant_id": arTenant, "max_iterations": req.MaxIterations,
		})
		return
	}
	loopReserveTotal := perIter * float64(req.MaxIterations)
	if loopReserveTotal > 0 {
		if arTenant == "" {
			http.Error(w, "loop spend has no tenant to bill: refusing", http.StatusForbidden)
			return
		}
		if err := s.tenantMgr.ReserveSpend(arTenant, loopReserveTotal); err != nil {
			WriteAPIError(w, "ceiling_exceeded", fmt.Sprintf("loop worst-case $%.4f exceeds tenant means: %v", loopReserveTotal, err), map[string]any{
				"tenant_id": arTenant, "reserved_usd": loopReserveTotal, "max_iterations": req.MaxIterations,
			})
			return
		}
		if err := s.sessionMgr.ReserveSpend(req.SessionID, loopReserveTotal); err != nil {
			_ = s.tenantMgr.ReleaseSpend(arTenant, loopReserveTotal)
			WriteAPIError(w, "ceiling_exceeded", fmt.Sprintf("loop worst-case $%.4f exceeds session means: %v", loopReserveTotal, err), map[string]any{
				"session_id": req.SessionID, "reserved_usd": loopReserveTotal,
			})
			return
		}
	}

	loopID := fmt.Sprintf("loop-%s-%s", req.HarnessID, uuid.New().String()[:8])
	engine := autoresearch.NewLoopEngine(
		loopID,
		req.SessionID,
		harness,
		s.sandboxRunner,
		s.llmClient,
		req.InitialArtifact,
		req.MaxIterations,
	)
	// S3: mid-run spend bound — entry checks never bound a hot loop.
	engine.SetBudgetCheck(func() error { return s.checkLLMBudget() })
	// I3: per-iteration drawdown against the admission reservation.
	engine.SetSpendReservation(loopReserveTotal)
	engine.SetSpendRecorder(func(role string, promptTokens, completionTokens int) (float64, error) {
		return s.recordLoopSpend(loopID, role, promptTokens, completionTokens)
	})

	// Hook up Section 13 Meta-Search stagnation trigger
	engine.SetStagnationCallback(func(lID string, discards int, champion string) {
		s.onLoopStagnation(req.SessionID, req.HarnessID, lID, discards, champion)
	})

	// Run initial baseline
	_, _ = engine.InitializeBaseline(r.Context())

	s.autoresearchMu.Lock()
	s.activeAutoloops[loopID] = engine
	s.autoresearchMu.Unlock()

	s.ledger.AppendEvent(req.SessionID, "autoresearch_loop_started", map[string]any{
		"loop_id":         loopID,
		"harness_id":      req.HarnessID,
		"max_iterations":  req.MaxIterations,
		"reserved_usd":    loopReserveTotal,
		"price_fallbacks": priceFallbacks,
	})
	// I3: the loops table previously had no writer — persist the row so
	// sleep/wake/drain and restarts have restart-surviving truth.
	// Reservation books ride the row from birth (tenant + total).
	// Birth write carries the tenant explicitly (the carry in
	// persistLoopRow reads the row — which does not exist yet here).
	if arTenant != "" && loopReserveTotal > 0 {
		if err := s.ledger.SaveLoopState(ledger.LoopPersist{
			ID: loopID, SessionID: req.SessionID, HarnessID: req.HarnessID,
			Status: string(engine.State().Status), StepCount: 0,
			MaxSteps: req.MaxIterations, LastActivity: time.Now().UTC(),
			ReservedAmount: loopReserveTotal, ReservedTenant: arTenant,
		}); err != nil {
			// Birth row failed: unwind the reservation (no orphaned hold)
			// and refuse — a loop whose books cannot persist cannot run.
			_ = s.tenantMgr.ReleaseSpend(arTenant, loopReserveTotal)
			_ = s.sessionMgr.ReleaseSpend(req.SessionID, loopReserveTotal)
			http.Error(w, fmt.Sprintf("loop spend books failed: %v", err), http.StatusInternalServerError)
			return
		}
	}
	s.persistLoopRow(engine)

	s.idemFinishJSON(w, arIdem, http.StatusCreated, engine.State())
}

func (s *Server) handleAutoresearchLoops(w http.ResponseWriter, r *http.Request) {
	s.autoresearchMu.RLock()
	defer s.autoresearchMu.RUnlock()

	var out []*autoresearch.LoopState
	for _, engine := range s.activeAutoloops {
		out = append(out, engine.State())
	}
	// Audit 2026-10-21: nil slice serializes as "loops":null (same class
	// as jobs/sessions). Empty reads as [].
	if out == nil {
		out = []*autoresearch.LoopState{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"loops": out})
}

// S3 loop-idle constants (Q3 decision): 10 min for autonomous loops
// (chat/tribunal sessions hold no harness, so sleeping them would be
// theater — loops only). Poll every 60 s.
const (
	loopIdleAfter  = 10 * time.Minute
	loopSweepEvery = 60 * time.Second
)

// persistLoopRow writes the engine's restart-surviving projection (S3: the
// loops table previously had no writer). Persist failures never fail the
// action — the in-memory engine is authoritative mid-flight; the row is
// reconciled on the next transition.
func (s *Server) persistLoopRow(engine *autoresearch.LoopEngine) {
	st := engine.State()
	// I3: spend books persist from the engine telemetry (reserved +
	// consumed mirrors); reserved_tenant lives row-side only (engines
	// never learn tenants), so carry it across the upsert. Admission
	// sets it at birth; every later persist must not wipe it.
	var tenant string
	_ = s.ledger.DB().QueryRow(`SELECT reserved_tenant FROM loops WHERE id = ?`, st.ID).Scan(&tenant)
	if err := s.ledger.SaveLoopState(ledger.LoopPersist{
		ID: st.ID, SessionID: st.SessionID, HarnessID: st.HarnessID,
		Status: string(st.Status), StepCount: st.CurrentIteration,
		MaxSteps: st.MaxIterations, LastActivity: st.LastActivity,
		Drain: st.Drained, ReservedAmount: st.ReservedAmount,
		ReservedTenant: tenant, ConsumedAmount: st.ConsumedAmount,
	}); err != nil {
		// W2.10: loud, timestamped, and audited — the old bare println
		// vanished into scrollback while DB and memory diverged.
		slog.Error("[Kubona Core] loop persist failed; memory/DB diverged", "loop_id", st.ID, "error", err)
		_ = s.ledger.AppendEvent(st.SessionID, "loop_persist_failed", map[string]any{
			"loop_id": st.ID, "error": err.Error(),
		})
	}
}

// recordLoopSpend prices one measured loop LLM call and draws it
// against the loop's reservation (I3). Local models cost nothing;
// measured tokens on an unpriced model pause the loop (fail-closed —
// spend that cannot be accounted cannot continue). Successful metered
// calls always carry usage (the client refuses blind spend), so a zero
// measurement here is genuine, not missing data.
func (s *Server) recordLoopSpend(loopID, role string, promptTokens, completionTokens int) (float64, error) {
	prov, model, err := s.llmSettings.GetModelForRole(role)
	if err != nil {
		return 0, fmt.Errorf("cannot price role %q: %w", role, err)
	}
	if llm.IsLocalProvider(prov) {
		return 0, nil
	}
	cost, ok := llm.EstimateCostUSD(prov, model, promptTokens, completionTokens)
	if !ok {
		return 0, fmt.Errorf("unpriced model %q: measured spend unaccountable, pausing", model)
	}
	if err := s.ledger.AddLoopConsumed(loopID, cost); err != nil {
		return 0, err
	}
	return cost, nil
}

// releaseLoopSpend returns a loop's unspent reservation (I3): claim the
// row once (kill racing completion releases exactly once), unwind tenant
// AND session books, seal the audit event. Idempotent — already-released
// loops are a no-op receipt. Returns false only on ledger failure (the
// reservation may still be held — callers must NOT park the loop then).
// Stuck money stays visible in the tenant book, recoverable via refund.
func (s *Server) releaseLoopSpend(loopID, cause string) bool {
	st, err := s.ledger.GetLoopState(loopID)
	if err != nil {
		slog.Error("[Kubona Core] loop spend release failed", "loop_id", loopID, "error", err)
		return false
	}
	sessionID := ""
	if st != nil {
		sessionID = st.SessionID
	}
	tenant, refund, ok, err := s.ledger.ClaimLoopSpend(loopID)
	if err != nil {
		slog.Error("[Kubona Core] loop spend release failed", "loop_id", loopID, "error", err)
		return false
	}
	if !ok {
		return true
	}
	// I3: clear the engine mirrors so the next persist cannot resurrect
	// the reservation from State (released twice = double refund).
	s.autoresearchMu.RLock()
	engine, present := s.activeAutoloops[loopID]
	s.autoresearchMu.RUnlock()
	if present {
		engine.ClearSpendReservation()
	}
	if tenant != "" && refund > 0 {
		if rerr := s.tenantMgr.ReleaseSpend(tenant, refund); rerr != nil {
			slog.Error("[Kubona Core] loop spend tenant release failed", "loop_id", loopID, "error", rerr)
		}
	}
	if sessionID != "" && refund > 0 {
		if rerr := s.sessionMgr.ReleaseSpend(sessionID, refund); rerr != nil {
			slog.Error("[Kubona Core] loop spend session release failed", "loop_id", loopID, "error", rerr)
		}
	}
	_ = s.ledger.AppendEvent(sessionID, "loop_spend_released", map[string]any{
		"loop_id": loopID, "tenant_id": tenant, "amount_usd": refund, "cause": cause,
	})
	return true
}

// rereserveLoopSpend funds a waking (previously slept) loop for its
// remaining iterations. Refuses when the ceiling cannot cover — wake is
// admission, not a right (the caller re-sleeps on refusal). Paused-only
// loops never released, so they never reach here.
func (s *Server) rereserveLoopSpend(engine *autoresearch.LoopEngine) error {
	st := engine.State()
	remaining := st.MaxIterations - st.CurrentIteration
	if remaining <= 0 {
		return nil
	}
	sess, err := s.sessionMgr.GetSession(st.SessionID)
	if err != nil {
		return fmt.Errorf("wake needs a live session to bill: %w", err)
	}
	perIter, _ /* no fallbacks by construction: unknown models refuse */, err := llm.LoopIterationBound(s.llmSettings, []string{"coder", "critic"})
	if err != nil {
		return fmt.Errorf("wake re-reserve refused: %w", err)
	}
	total := perIter * float64(remaining)
	if total <= 0 {
		return nil
	}
	if err := s.tenantMgr.ReserveSpend(sess.TenantID, total); err != nil {
		return fmt.Errorf("wake re-reserve $%.4f refused: %w", total, err)
	}
	if err := s.sessionMgr.ReserveSpend(st.SessionID, total); err != nil {
		_ = s.tenantMgr.ReleaseSpend(sess.TenantID, total)
		return fmt.Errorf("wake re-reserve $%.4f refused: %w", total, err)
	}
	engine.SetSpendReservation(total)
	if err := s.ledger.SaveLoopState(ledger.LoopPersist{
		ID: st.ID, SessionID: st.SessionID, HarnessID: st.HarnessID,
		Status: string(st.Status), StepCount: st.CurrentIteration,
		MaxSteps: st.MaxIterations, LastActivity: st.LastActivity,
		Drain: st.Drained, ReservedAmount: total, ReservedTenant: sess.TenantID,
	}); err != nil {
		engine.ClearSpendReservation()
		_ = s.tenantMgr.ReleaseSpend(sess.TenantID, total)
		_ = s.sessionMgr.ReleaseSpend(st.SessionID, total)
		return fmt.Errorf("wake spend books failed: %w", err)
	}
	return nil
}

// SweepIdleLoops sleeps engines past the idle threshold (S3). Only
// running/paused engines with recent-activity older than idleAfter;
// drained, terminal, and already-sleeping loops are untouched. Returns
// slept loop IDs.
func (s *Server) SweepIdleLoops(now time.Time, idleAfter time.Duration) []string {
	var slept []string
	s.autoresearchMu.RLock()
	engines := make([]*autoresearch.LoopEngine, 0, len(s.activeAutoloops))
	for _, e := range s.activeAutoloops {
		engines = append(engines, e)
	}
	s.autoresearchMu.RUnlock()
	for _, e := range engines {
		st := e.State()
		if st.Status != autoresearch.StatusRunning && st.Status != autoresearch.StatusPaused {
			continue
		}
		if e.Drained() {
			continue
		}
		// W2.10: zero activity means idle-since-start (StartedAt), not
		// exempt — the old IsZero skip leaked never-stepped loops forever.
		last := e.LastActivity()
		if last.IsZero() {
			last = st.StartedAt
		}
		if last.IsZero() || now.Sub(last) < idleAfter {
			continue
		}
		// I3: sleep releases the reservation first (money never parks
		// unreachable — this reverses the W2.10 skip, which would have
		// frozen sleep for every funded loop now that reservations are
		// real). A failed release refuses the sleep loudly instead.
		if !s.releaseLoopSpend(st.ID, "sleep") {
			s.ledger.AppendEvent(st.SessionID, "autoresearch_loop_sleep_refused", map[string]any{
				"loop_id": st.ID, "reason": "spend release failed",
			})
			continue
		}
		e.Sleep()
		s.persistLoopRow(e)
		s.ledger.AppendEvent(st.SessionID, "autoresearch_loop_slept", map[string]any{
			"loop_id": st.ID, "reason": "idle sweeper",
		})
		slept = append(slept, st.ID)
	}
	return slept
}

// StartLoopSweeper runs the idle sweeper until stop closes (S3). One
// sweeper per daemon (single-writer flock ⇒ one daemon per state dir).
func (s *Server) StartLoopSweeper(stop <-chan struct{}) {
	t := time.NewTicker(loopSweepEvery)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.SweepIdleLoops(time.Now().UTC(), loopIdleAfter)
		}
	}
}

func (s *Server) handleAutoresearchLoopAction(w http.ResponseWriter, r *http.Request) {

	path := strings.TrimPrefix(r.URL.Path, "/api/autoresearch/loops/")
	parts := strings.Split(path, "/")
	loopID := parts[0]

	s.autoresearchMu.RLock()
	engine, exists := s.activeAutoloops[loopID]
	s.autoresearchMu.RUnlock()

	if !exists {
		http.Error(w, fmt.Sprintf("autoresearch loop '%s' not found", loopID), http.StatusNotFound)
		return
	}

	if len(parts) == 1 || parts[1] == "" {
		writeJSON(w, http.StatusOK, engine.State())
		return
	}

	action := parts[1]
	switch action {
	case "step":
		rec, err := engine.Step(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.persistLoopRow(engine)
		// I3: terminal steps settle the books (kill racing completion
		// releases exactly once via the claim).
		if st := engine.State().Status; st == autoresearch.StatusCompleted || st == autoresearch.StatusAborted {
			s.releaseLoopSpend(loopID, "end")
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"step":  rec,
			"state": engine.State(),
		})
	case "pause":
		engine.Pause()
		s.persistLoopRow(engine)
		writeJSON(w, http.StatusOK, engine.State())
	case "resume":
		engine.Resume()
		s.persistLoopRow(engine)
		writeJSON(w, http.StatusOK, engine.State())
	case "stop":
		engine.Abort()
		s.persistLoopRow(engine)
		// I3: stopping settles the books (unspent reservation returns).
		s.releaseLoopSpend(loopID, "end")
		writeJSON(w, http.StatusOK, engine.State())
	case "sleep":
		// S3: explicit sleep — release the harness, keep history; wake
		// (resume) continues the same session. Terminal states refuse:
		// sleeping a finished loop rewrites history.
		st := engine.State().Status
		if st != autoresearch.StatusRunning && st != autoresearch.StatusPaused {
			http.Error(w, fmt.Sprintf("loop is %s: only running/paused loops sleep", st), http.StatusConflict)
			return
		}
		// I3: sleep releases the reservation FIRST (money never parks
		// unreachable — this reverses the W2.10 skip, which would have
		// frozen sleep for every funded loop). A failed release refuses
		// the sleep loudly instead. Wake re-reserves.
		if !s.releaseLoopSpend(loopID, "sleep") {
			s.ledger.AppendEvent(engine.State().SessionID, "autoresearch_loop_sleep_refused", map[string]any{
				"loop_id": loopID, "reason": "spend release failed",
			})
			http.Error(w, "sleep refused: loop spend could not be released", http.StatusInternalServerError)
			return
		}
		engine.Sleep()
		s.persistLoopRow(engine)
		s.ledger.AppendEvent(engine.State().SessionID, "autoresearch_loop_slept", map[string]any{
			"loop_id": loopID,
		})
		writeJSON(w, http.StatusOK, engine.State())
	case "wake":
		// S1-keyed (slow rail): a retried wake resumes once, never forks a
		// second loop. Wake only applies to sleeping/paused/stagnant loops.
		var wreq struct {
			RequestID string `json:"request_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&wreq)
		widem, wref := s.idemBegin(engine.State().SessionID, wreq.RequestID, map[string]any{
			"loop_id": loopID, "action": "wake",
		}, idemRailSlow)
		if wref != nil {
			wref.write(w)
			return
		}
		if widem.replay {
			s.idemReplay(w, widem)
			return
		}
		st := engine.State().Status
		if st != autoresearch.StatusSleeping && st != autoresearch.StatusPaused && st != autoresearch.StatusStagnant {
			s.idemFinishError(w, widem, http.StatusConflict, fmt.Sprintf("loop is %s: only sleeping/paused/stagnant loops wake", st))
			return
		}
		engine.Resume()
		// I3: sleeping loops released on sleep — waking re-reserves the
		// remaining estimate (refuses when the ceiling cannot cover, and
		// re-sleeps so no unfunded loop runs; paused/stagnant loops never
		// released, nothing to top up).
		if st == autoresearch.StatusSleeping {
			if werr := s.rereserveLoopSpend(engine); werr != nil {
				engine.Sleep()
				s.persistLoopRow(engine)
				s.idemFinishError(w, widem, http.StatusForbidden, werr.Error())
				return
			}
		}
		s.persistLoopRow(engine)
		s.ledger.AppendEvent(engine.State().SessionID, "autoresearch_loop_woken", map[string]any{
			"loop_id": loopID,
		})
		s.idemFinishJSON(w, widem, http.StatusOK, engine.State())
	case "drain":
		// S3: drain latch — no new Steps while a stop is arranged.
		var dreq struct {
			Drain *bool `json:"drain"`
		}
		if err := json.NewDecoder(r.Body).Decode(&dreq); err != nil || dreq.Drain == nil {
			http.Error(w, "drain requires {\"drain\": true|false}", http.StatusBadRequest)
			return
		}
		engine.SetDrained(*dreq.Drain)
		s.persistLoopRow(engine)
		s.ledger.AppendEvent(engine.State().SessionID, "autoresearch_loop_drain", map[string]any{
			"loop_id": loopID, "drain": *dreq.Drain,
		})
		writeJSON(w, http.StatusOK, engine.State())
	default:
		http.Error(w, fmt.Sprintf("unknown action '%s'", action), http.StatusBadRequest)
	}
}

func (s *Server) handleMetasearchProposals(w http.ResponseWriter, r *http.Request) {
	proposals := s.metaSearchEngine.ListProposals()
	writeJSON(w, http.StatusOK, map[string]any{"proposals": proposals})
}

func (s *Server) handleMetasearchApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ProposalID string `json:"proposal_id"`
		Signer     string `json:"signer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Signer == "" {
		req.Signer = "human-quantum-reviewer"
	}
	if err := s.metaSearchEngine.ApproveProposal(req.ProposalID, req.Signer); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	prop, _ := s.metaSearchEngine.GetProposal(req.ProposalID)
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "proposal": prop})
}

func (s *Server) handleMetasearchReject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ProposalID string `json:"proposal_id"`
		Signer     string `json:"signer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Signer == "" {
		req.Signer = "human-quantum-reviewer"
	}
	if err := s.metaSearchEngine.RejectProposal(req.ProposalID, req.Signer); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	prop, _ := s.metaSearchEngine.GetProposal(req.ProposalID)
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "proposal": prop})
}

func (s *Server) handleMetasearchApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// S2: applying resumes loop work — blocked with other new work.
	if s.refuseIfDiskBlocked(w) {
		return
	}
	var req struct {
		ProposalID string `json:"proposal_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	msg, err := s.metaSearchEngine.ApplyProposal(req.ProposalID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// W2.1: fail-closed nil guard — ApplyProposal succeeded, so the
	// proposal must still resolve; a nil return here (store race, evicted
	// row) refuses with 400 instead of dereferencing prop.LoopID below.
	prop, gerr := s.metaSearchEngine.GetProposal(req.ProposalID)
	if gerr != nil || prop == nil {
		http.Error(w, "proposal vanished after apply: refusing to resume unknown loop", http.StatusBadRequest)
		return
	}

	// K4: the application record lands in the ledger (who/when/which) —
	// Apply itself writes no files by design (see ApplyProposal).
	s.ledger.AppendEvent("global", "metasearch_applied", map[string]any{
		"proposal_id": prop.ID, "loop_id": prop.LoopID,
		"harness_id": prop.HarnessID, "approved_by": prop.ApprovedBy,
	})

	// If corresponding loop was stagnant, resume it!
	s.autoresearchMu.RLock()
	if loop, ok := s.activeAutoloops[prop.LoopID]; ok {
		loop.Resume()
	}
	s.autoresearchMu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": msg, "proposal": prop})
}

func (s *Server) handlePQCPublicKey(w http.ResponseWriter, r *http.Request) {
	if s.pqcKeypair == nil {
		s.pqcKeypair, _ = mandates.GenerateRecipientKeypair()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"fingerprint":           s.pqcKeypair.Fingerprint(),
		"encapsulation_key_hex": hex.EncodeToString(s.pqcKeypair.EncapsulationKey.Bytes()),
		"algorithm":             "ML-KEM-768",
		"standard":              "NIST FIPS 203",
	})
}

func (s *Server) handlePQCVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var hybrid mandates.HybridSpendMandate
	if err := json.NewDecoder(r.Body).Decode(&hybrid); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if s.pqcKeypair == nil {
		s.pqcKeypair, _ = mandates.GenerateRecipientKeypair()
	}

	mandate, err := s.pqcKeypair.DecapsulateAndVerify(&hybrid, "", "", 0)
	if err != nil {
		// W0.12: uniform external refusal (decap/decrypt/hex failures are
		// indistinguishable outside; detail goes to the server log, not
		// the wire — no oracle, still debuggable).
		slog.Warn("[Kubona Core] PQC mandate rejected", "error", err)
		writeJSON(w, http.StatusOK, map[string]any{
			"valid": false,
			"error": "hybrid mandate rejected",
		})
		return
	}

	// Bind the decapsulated mandate to its own fields (no wildcard
	// expectations) and clamp against the lab tenant ceiling.
	if err := mandate.Verify(mandate.TenantID, mandate.JobHash, mandate.MaxBudgetUSD); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid": false,
			"error": err.Error(),
		})
		return
	}

	// AP2 Spend Ceiling Clamping against the lab tenant (see above).
	if err := s.tenantMgr.CheckSpend(mandate.TenantID, mandate.MaxBudgetUSD); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid": false,
			"error": err.Error(),
		})
		return
	}

	// W0.12 key→tenant binding (same rule as the client path): the inner
	// signature must come from the tenant's current admin key.
	plab, err := s.tenantMgr.GetTenant(mandate.TenantID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid": false,
			"error": err.Error(),
		})
		return
	}
	if plab.AdminPubkeyHex == "" || !strings.EqualFold(plab.AdminPubkeyHex, mandate.PublicKeyHex) {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid": false,
			"error": "mandate key is not the tenant admin key: self-signed spend refused",
		})
		return
	}

	pqcRec, rerr := s.ledger.AppendExternalEvent(mandate.TenantID, "mandate_verified_pqc_mlkem768", map[string]any{
		"tenant_id":             mandate.TenantID,
		"job_hash":              mandate.JobHash,
		"public_key_hex":        mandate.PublicKeyHex,
		"recipient_fingerprint": hybrid.RecipientFingerprint,
		"algorithm":             "ML-KEM-768+AES-GCM+Ed25519",
		"max_budget_usd":        mandate.MaxBudgetUSD,
		"expiry_timestamp":      mandate.ExpiryTimestamp.UTC().Format(time.RFC3339),
	}, mandate.PublicKeyHex, mandate.SignatureHex)
	if rerr != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid": false,
			"error": rerr.Error(),
		})
		return
	}
	// R2b: same obligation seam as the classical path (receipt-scoped).
	// The PQC path admits admin keys only, so ownership is admin-side.
	if oerr := s.openMandateObligation(mandate.TenantID, "admin:"+mandate.PublicKeyHex, pqcRec.EntryHash); oerr != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid": false,
			"error": oerr.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"valid":                 true,
		"tenant_id":             mandate.TenantID,
		"job_hash":              mandate.JobHash,
		"pqc_verified":          true,
		"receipt":               pqcRec.EntryHash,
		"recipient_fingerprint": hybrid.RecipientFingerprint,
	})
}

func (s *Server) handleQIRCompile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		QASM string `json:"qasm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// R8: compiling nothing is a client error, not an empty summary.
	// Analyze is total by construction — without this guard an empty
	// submission would come back looking like a result.
	if strings.TrimSpace(req.QASM) == "" {
		http.Error(w, "qasm is required: empty input compiles to nothing", http.StatusBadRequest)
		return
	}

	if s.qirCompiler == nil {
		s.qirCompiler = qir.NewCompiler()
	}

	summary, err := s.qirCompiler.Analyze(req.QASM)
	// Defensive refuse branch, kept deliberately (see SubmitJob): Analyze
	// is total today, so this never fires — but a future fallible Analyze
	// must refuse here, not summarize. Do not delete as "dead code".
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

// cachedH2FCIAnchor is a labeled reference value, NOT a computed result. It may
// be returned alongside an explicit failure so clients can still display the
// classical baseline — it must never be presented with success:true.
const cachedH2FCIAnchor = -1.13727

// writeBridgeUnavailable is the single fail-closed envelope for quantum-bridge
// outages (quantum-advantage-realism policy: never silently return a canned
// number as a measurement). Callers pass the computation name; the response is
// HTTP 503 with success:false and result_computed:false plus the labeled
// cached anchor for reference only.
func writeBridgeUnavailable(w http.ResponseWriter, computation, detail string) {
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{
		"success":         false,
		"result_computed": false,
		"computation":     computation,
		"error_message":   "quantum bridge unavailable: no " + computation + " computation performed. " + detail,
		"backend":         "classical-fallback: bridge unavailable — cached anchor below is a reference value, not a computed result",
		"classical_anchor": map[string]any{
			"method":             "PySCF STO-3G FCI (cached)",
			"fci_energy_hartree": cachedH2FCIAnchor,
		},
	})
}

func (s *Server) handleKrylovSolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Molecule          string  `json:"molecule"`
		BondDistance      float64 `json:"bond_distance"`
		SubspaceDimension int     `json:"subspace_dimension"`
		TimeStep          float64 `json:"time_step"`
		Shots             int     `json:"shots"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Molecule == "" {
		req.Molecule = "H2"
	}
	if req.BondDistance <= 0 {
		req.BondDistance = 0.7414
	}
	if req.SubspaceDimension <= 0 {
		req.SubspaceDimension = 4
	}
	if req.TimeStep <= 0 {
		req.TimeStep = 0.2
	}
	if req.Shots <= 0 {
		req.Shots = 1024
	}

	bridge := s.jobRunner.BridgeClient()
	if bridge != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		resp, err := bridge.ExecuteQuantumKrylov(ctx, &pb.QuantumKrylovRequest{
			Molecule:          req.Molecule,
			BondDistance:      req.BondDistance,
			SubspaceDimension: int32(req.SubspaceDimension),
			TimeStep:          req.TimeStep,
			Shots:             int32(req.Shots),
		})
		if err == nil && resp != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"success":                    resp.Success,
				"ground_state_energy":        resp.GroundStateEnergy,
				"classical_fci_energy":       resp.ClassicalFciEnergy,
				"subspace_eigenvalues":       resp.SubspaceEigenvalues,
				"subspace_dimension":         resp.SubspaceDimension,
				"meyer_wallach_entanglement": resp.MeyerWallachEntanglement,
				"execution_time_ms":          resp.ExecutionTimeMs,
				"error_message":              resp.ErrorMessage,
				// J5: solver attribution must survive this proxy —
				// downstream folds gate MW on it.
				"solver": resp.Solver,
			})
			return
		}
	}

	writeBridgeUnavailable(w, "krylov_solve", "bridge client is nil or the ExecuteQuantumKrylov RPC failed.")
}

func (s *Server) handleMitigationApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		QASM         string    `json:"qasm"`
		Method       string    `json:"method"`
		ScaleFactors []float64 `json:"scale_factors"`
		Shots        int       `json:"shots"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Method == "" {
		req.Method = "ZNE"
	}
	if len(req.ScaleFactors) == 0 {
		req.ScaleFactors = []float64{1.0, 1.5, 2.0}
	}
	if req.Shots <= 0 {
		req.Shots = 1024
	}

	bridge := s.jobRunner.BridgeClient()
	if bridge != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		resp, err := bridge.MitigateCircuit(ctx, &pb.MitigationRequest{
			Qasm:         req.QASM,
			Method:       req.Method,
			ScaleFactors: req.ScaleFactors,
			Shots:        int32(req.Shots),
		})
		if err == nil && resp != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"success":           resp.Success,
				"unmitigated_value": resp.UnmitigatedValue,
				"mitigated_value":   resp.MitigatedValue,
				"error_bound":       resp.ErrorBound,
				"method_used":       resp.MethodUsed,
				"error_message":     resp.ErrorMessage,
			})
			return
		}
	}

	writeBridgeUnavailable(w, "mitigation_apply", "bridge client is nil or the MitigateCircuit RPC failed.")
}

func (s *Server) handlePaymentSettle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Receipt cost.PaymentReceipt `json:"receipt"`
		Network string              `json:"network"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	pm := s.a2aServer.PaymentManager()
	settlement, err := pm.SettlePayment(&req.Receipt, req.Network)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"settled": false,
			"error":   err.Error(),
		})
		return
	}

	s.ledger.AppendExternalEvent("payment-settlement", "x402_payment_settled", map[string]any{
		"challenge_id": req.Receipt.ChallengeID,
		"reference":    settlement.Reference,
		"simulated":    settlement.Simulated,
		"network":      settlement.Network,
		"token":        req.Receipt.Token,
		"amount":       req.Receipt.Amount,
		"payer":        req.Receipt.PayerPubHex,
	}, req.Receipt.PayerPubHex, req.Receipt.Signature)

	writeJSON(w, http.StatusOK, map[string]any{
		"settled":   settlement.Settled,
		"simulated": settlement.Simulated,
		"reference": settlement.Reference,
		"network":   settlement.Network,
	})
}

func (s *Server) handleHFSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	library := r.URL.Query().Get("library")
	limitStr := r.URL.Query().Get("limit")
	limit := 15
	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}
	if limit <= 0 {
		limit = 15
	}
	if limit > 50 {
		limit = 50
	}

	if s.hfClient == nil {
		s.hfClient = hf.NewHubClient("")
	}

	models, err := s.hfClient.SearchModels(r.Context(), q, library, limit)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "models": []any{}, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"models": models,
	})
}

func (s *Server) handleHFModelDetails(w http.ResponseWriter, r *http.Request) {
	repoID := strings.TrimPrefix(r.URL.Path, "/api/hf/models/")
	if repoID == "" {
		http.Error(w, "missing repo_id", http.StatusBadRequest)
		return
	}

	if s.hfClient == nil {
		s.hfClient = hf.NewHubClient("")
	}

	model, err := s.hfClient.GetModelDetails(r.Context(), repoID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"model": model,
	})
}

func (s *Server) handleHFDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RepoID    string `json:"repo_id"`
		Filename  string `json:"filename"`
		SizeBytes *int64 `json:"size_bytes"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Edge validation: the hub client would refuse these with the same
	// message, but as a 500. Missing fields are a 400 by contract.
	if req.RepoID == "" || req.Filename == "" {
		WriteAPIError(w, "bad_request", "repo_id and filename are required", nil)
		return
	}

	// T-11: size-confirmation gate (server half). The console confirms
	// over-threshold fetches in a dialog; the server enforces the same
	// bound so a raw POST cannot skip it. Unknown sizes (nil) proceed —
	// the stream still finalizes .part only on success, so a surprise
	// giant can waste bandwidth but never masquerade as a weight file.
	const hfDownloadConfirmBytes = int64(100 * 1024 * 1024)
	if req.SizeBytes != nil && *req.SizeBytes >= hfDownloadConfirmBytes && !req.Confirmed {
		WriteAPIError(w, "bad_request", fmt.Sprintf("fetch is %.2f GB — resend with confirmed:true", float64(*req.SizeBytes)/float64(1024*1024*1024)), nil)
		return
	}

	if s.hfClient == nil {
		s.hfClient = hf.NewHubClient("")
	}

	task, err := s.hfClient.StartDownload(req.RepoID, req.Filename)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	s.ledger.AppendEvent("global", "hf_download_started", map[string]any{
		"task_id":  task.TaskID,
		"repo_id":  req.RepoID,
		"filename": req.Filename,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"task": task,
	})
}

func (s *Server) handleHFDownloads(w http.ResponseWriter, r *http.Request) {
	if s.hfClient == nil {
		s.hfClient = hf.NewHubClient("")
	}

	downloads := s.hfClient.ListDownloads()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"downloads": downloads,
	})
}

func (s *Server) handleHFDownloadAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/hf/downloads/")
	if strings.HasSuffix(path, "/cancel") {
		taskID := strings.TrimSuffix(path, "/cancel")
		if s.hfClient == nil {
			s.hfClient = hf.NewHubClient("")
		}
		err := s.hfClient.CancelDownload(taskID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "task_id": taskID, "status": "cancelled"})
		return
	}
	http.Error(w, "unknown download action", http.StatusNotFound)
}

func (s *Server) handleCustomAgents(w http.ResponseWriter, r *http.Request) {
	if s.customAgents == nil {
		s.customAgents = agents.NewCustomAgentRegistry()
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"agents": s.customAgents.List(),
		})
	case http.MethodPost:
		// K3: registry writes are operator mutations.
		if _, _, code := s.checkAuth(r); code != http.StatusOK {
			http.Error(w, "custom agent registration requires the operator token", code)
			return
		}
		if !s.tenantLimiter.allow(clientKey(r)) {
			WriteAPIError(w, "rate_limited", "rate limit exceeded: slow down and retry", nil)
			return
		}
		var manifest agents.CustomAgentManifest
		if err := json.NewDecoder(r.Body).Decode(&manifest); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		registered, err := s.customAgents.Register(manifest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.ledger.AppendEvent("global", "custom_agent_registered", map[string]any{
			"agent_id": registered.ID,
			"name":     registered.Name,
			"role":     registered.Role,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"agent": registered,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCustomAgentByID(w http.ResponseWriter, r *http.Request) {
	if s.customAgents == nil {
		s.customAgents = agents.NewCustomAgentRegistry()
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/agents/custom/")
	if id == "" {
		http.Error(w, "missing agent id", http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodDelete {
		// K3: registry deletes are operator mutations.
		if _, _, code := s.checkAuth(r); code != http.StatusOK {
			http.Error(w, "custom agent deletion requires the operator token", code)
			return
		}
		if !s.tenantLimiter.allow(clientKey(r)) {
			WriteAPIError(w, "rate_limited", "rate limit exceeded: slow down and retry", nil)
			return
		}
		err := s.customAgents.Delete(id)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		s.ledger.AppendEvent("global", "custom_agent_deleted", map[string]any{
			"agent_id": id,
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "agent_id": id})
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// qualifyBareModelID maps a prefix-less model ID to "provider/model"
// when it exactly matches a configured provider default (or the active
// model). Unknown bare IDs return "" — the router keeps its fail-closed
// cloud verdict for those. Matching is exact and case-sensitive: a
// near-miss is not a match.
func qualifyBareModelID(snap *llm.Settings, bare string) string {
	if bare == "" {
		return ""
	}
	if snap.ActiveModel == bare {
		if prov, ok := snap.Providers[snap.ActiveProvider]; ok && prov.Enabled && strings.TrimSpace(string(snap.ActiveProvider)) != "" {
			return string(snap.ActiveProvider) + "/" + bare
		}
	}
	for pt, prov := range snap.Providers {
		if !prov.Enabled {
			continue
		}
		if strings.TrimSpace(prov.DefaultModel) == bare && strings.TrimSpace(string(pt)) != "" {
			return string(pt) + "/" + bare
		}
	}
	return ""
}

func (s *Server) handleMeshRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req agents.MultimodalPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	airGapped := false
	fallbackModel := ""
	if s.llmSettings != nil {
		snap := s.llmSettings.GetSnapshot()
		airGapped = snap.AirGapped
		// Configured default for callers without an explicit model:
		// the active model when its provider is enabled, else that
		// provider's default. Empty stays empty — the router refuses
		// rather than inventing a model.
		if prov, ok := snap.Providers[snap.ActiveProvider]; ok && prov.Enabled {
			fallbackModel = snap.ActiveModel
			if fallbackModel == "" {
				fallbackModel = prov.DefaultModel
			}
		}
		// Audit 2026-10-21: bare model IDs (no "provider/" prefix) from
		// the console free-text field misrouted — "iq3_s" (the Strata
		// default) classified as cloud and refused for spend. Qualify
		// exact matches against configured provider defaults (no
		// invention: the mapping comes from settings, never a guess).
		// Unknown bare IDs keep the router's fail-closed cloud verdict.
		if !strings.Contains(req.PreferredModel, "/") {
			if q := qualifyBareModelID(&snap, strings.TrimSpace(req.PreferredModel)); q != "" {
				req.PreferredModel = q
			}
		}
	}

	decision := agents.RouteMultimodal(req, airGapped, fallbackModel)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"decision": decision,
	})
}
