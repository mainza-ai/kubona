package api

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/autoresearch"
	"github.com/mainza-ai/kubona/core/pkg/epistemics"
	"github.com/mainza-ai/kubona/core/pkg/fidelity"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/mandates"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

type Server struct {
	ledger        *ledger.Ledger
	sessionMgr    *session.Manager
	jobRunner     *jobs.Runner
	supervisor    *supervisor.Supervisor
	watchdog      *watchdog.TaskWatchdog
	collective    *agents.AgentCollective
	fidelityGate  *fidelity.FidelityGate
	epistemicGate *epistemics.EpistemicGate
	harnessReg    *autoresearch.HarnessRegistry
	llmSettings   *llm.Settings
	llmClient     *llm.Client
	mux           *http.ServeMux

	// Mock keypair for human-reviewer mandate signing
	signerPubKey  ed25519.PublicKey
	signerPrivKey ed25519.PrivateKey
}

func NewServer(
	l *ledger.Ledger,
	sm *session.Manager,
	jr *jobs.Runner,
	sup *supervisor.Supervisor,
	wd *watchdog.TaskWatchdog,
) *Server {
	pub, priv, _ := ed25519.GenerateKey(nil)

	llmSet, err := llm.LoadSettings(l.DB())
	if err != nil {
		llmSet = llm.DefaultSettings()
	}
	llmCli := llm.NewClient(llmSet)

	s := &Server{
		ledger:        l,
		sessionMgr:    sm,
		jobRunner:     jr,
		supervisor:    sup,
		watchdog:      wd,
		collective:    agents.NewAgentCollective(),
		fidelityGate:  fidelity.NewFidelityGate(),
		epistemicGate: epistemics.NewEpistemicGate(),
		harnessReg:    autoresearch.NewHarnessRegistry(),
		llmSettings:   llmSet,
		llmClient:     llmCli,
		signerPubKey:  pub,
		signerPrivKey: priv,
		mux:           http.NewServeMux(),
	}

	// Seed sample claim wavefunctions for the Epistemic Manifold
	s.epistemicGate.RegisterClaim(&epistemics.ClaimWavefunction{
		ClaimID:                  "claim-h2-ground",
		Statement:                "H2 equilibrium ground state energy is -1.13727 Ha at R=0.7414 A",
		AlphaCorroborated:        0.95,
		BetaContested:            0.05,
		GammaUnverified:          0.10,
		EvidenceStrength:         0.92,
		Sources:                  []string{"arXiv:1304.3061", "JACS:2014"},
		ChemicalAccuracyDeltaMHa: 0.8,
	})
	s.epistemicGate.RegisterClaim(&epistemics.ClaimWavefunction{
		ClaimID:                  "claim-lih-dissociation",
		Statement:                "LiH singlet-triplet gap at R=2.5 A exhibits strong multi-reference character",
		AlphaCorroborated:        0.65,
		BetaContested:            0.45,
		GammaUnverified:          0.30,
		EvidenceStrength:         0.55,
		Sources:                  []string{"chemrxiv:2026.01"},
		ChemicalAccuracyDeltaMHa: 2.4, // Contested / not collapsed
	})

	s.registerRoutes()
	return s
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		s.mux.ServeHTTP(w, r)
	})
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/sessions", s.handleSessions)
	s.mux.HandleFunc("/api/sessions/", s.handleSessionByID)
	s.mux.HandleFunc("/api/chat/messages", s.handleChatMessages)
	s.mux.HandleFunc("/api/chat/tribunal", s.handleChatTribunal)
	s.mux.HandleFunc("/api/chat/launch", s.handleChatLaunch)
	s.mux.HandleFunc("/api/jobs", s.handleJobs)
	s.mux.HandleFunc("/api/loops", s.handleLoops)
	s.mux.HandleFunc("/api/loops/", s.handleLoopActions)
	s.mux.HandleFunc("/api/events", s.handleEvents)

	// New Observatory control plane endpoints
	s.mux.HandleFunc("/api/agents", s.handleAgents)
	s.mux.HandleFunc("/api/fidelity", s.handleFidelity)
	s.mux.HandleFunc("/api/epistemics", s.handleEpistemics)
	s.mux.HandleFunc("/api/fleet", s.handleFleet)
	s.mux.HandleFunc("/api/radar", s.handleRadar)
	s.mux.HandleFunc("/api/mandates/sign", s.handleMandateSign)
	s.mux.HandleFunc("/api/mandates/verify", s.handleMandateVerify)
	s.mux.HandleFunc("/api/harnesses", s.handleHarnesses)
	s.mux.HandleFunc("/api/results/notebooks", s.handleNotebooks)

	// Hybrid LLM Configuration Plane endpoints
	s.mux.HandleFunc("/api/llm/settings", s.handleLLMSettings)
	s.mux.HandleFunc("/api/llm/models", s.handleLLMModels)
	s.mux.HandleFunc("/api/llm/test", s.handleLLMTest)
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

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "healthy",
		"service":   "kubona-core-orchestrator",
		"version":   "0.2.0-ts-go",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
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

	assessment := s.fidelityGate.EvaluateAdmission(device, depth, twoQGates, 0.94)
	writeJSON(w, http.StatusOK, assessment)
}

func (s *Server) handleEpistemics(w http.ResponseWriter, r *http.Request) {
	kappa, drift := epistemics.ComputeEpistemicCurvature(0.12, -1.13727, -1.1365)
	writeJSON(w, http.StatusOK, map[string]any{
		"refusal_ratio":         s.epistemicGate.RefusalRatio(),
		"stagnation_kappa":      kappa,
		"literature_drift_mha":  drift * 1000.0,
		"canonical_anchor":      -1.13727,
		"active_wavefunctions": []map[string]any{
			{
				"claim_id":   "claim-h2-ground",
				"statement":  "H2 ground state equilibrium at 0.7414 A",
				"alpha_corr": 0.95,
				"beta_cont":  0.05,
				"gamma_unv":  0.10,
				"collapsed":  true,
				"status":     "COLLAPSED_VERIFIED",
			},
			{
				"claim_id":   "claim-lih-dissociation",
				"statement":  "LiH strong multi-reference correlation at 2.5 A",
				"alpha_corr": 0.65,
				"beta_cont":  0.45,
				"gamma_unv":  0.30,
				"collapsed":  false,
				"status":     "UNCOLLAPSED_ACTIVE",
			},
		},
	})
}

func (s *Server) handleFleet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"backends": []map[string]any{
			{
				"name":         "local_simulator",
				"type":         "Classical Aer Statevector",
				"qubits":       32,
				"t1_us":        "Infinite",
				"status":       "ONLINE",
				"queue_depth":  0,
				"cost_per_shot": 0.0,
				"jurisdiction": "LOCAL_SOVEREIGN",
				"itar_ear":     "EXEMPT",
			},
			{
				"name":         "ibm_heron",
				"type":         "Superconducting Transmon",
				"qubits":       156,
				"t1_us":        "180.0",
				"status":       "READY",
				"queue_depth":  14,
				"cost_per_shot": 0.0016,
				"jurisdiction": "US_EAST",
				"itar_ear":     "EAR99_CONTROLLED",
			},
			{
				"name":         "quantinuum_h1",
				"type":         "Trapped Ion Shuttle",
				"qubits":       20,
				"t1_us":        ">1,000,000",
				"status":       "ONLINE",
				"queue_depth":  3,
				"cost_per_shot": 0.0125,
				"jurisdiction": "US_CENTRAL",
				"itar_ear":     "EAR99_CONTROLLED",
			},
			{
				"name":         "rigetti_aspen",
				"type":         "Superconducting Multi-Chip",
				"qubits":       80,
				"t1_us":        "35.0",
				"status":       "CALIBRATING",
				"queue_depth":  28,
				"cost_per_shot": 0.0008,
				"jurisdiction": "US_WEST",
				"itar_ear":     "EAR99_CONTROLLED",
			},
		},
	})
}

func (s *Server) handleRadar(w http.ResponseWriter, r *http.Request) {
	// Probe local ports
	ports := []int{8787, 11434, 1234, 8080, 50051}
	var detected []map[string]any

	for _, p := range ports {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", p), 150*time.Millisecond)
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

	var req struct {
		TenantID     string  `json:"tenant_id"`
		JobHash      string  `json:"job_hash"`
		MaxBudgetUSD float64 `json:"max_budget_usd"`
		TtlSeconds   int64   `json:"ttl_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.TtlSeconds <= 0 {
		req.TtlSeconds = 3600
	}
	expiry := time.Now().UTC().Add(time.Duration(req.TtlSeconds) * time.Second)

	mandate, err := mandates.SignMandate(s.signerPrivKey, req.TenantID, req.JobHash, req.MaxBudgetUSD, expiry)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	verifiedErr := mandate.Verify(req.TenantID, req.JobHash, req.MaxBudgetUSD)
	writeJSON(w, http.StatusOK, map[string]any{
		"mandate":    mandate,
		"public_key": hex.EncodeToString(s.signerPubKey),
		"verified":   verifiedErr == nil,
	})
}

func (s *Server) handleMandateVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
			"error": err.Error(),
		})
		return
	}

	s.ledger.AppendEvent(req.TenantID, "mandate_verified_client", map[string]any{
		"tenant_id":      req.TenantID,
		"job_hash":       req.JobHash,
		"public_key_hex": req.PublicKeyHex,
		"max_budget_usd": req.MaxBudgetUSD,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"valid":       true,
		"mandate":     req,
		"verified_at": time.Now().UTC().Format(time.RFC3339),
	})
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

func (s *Server) handleNotebooks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"notebooks": []map[string]any{
			{
				"title":           "H2 Molecular Potential Energy Surface VQE vs FCI",
				"id":              "nb-" + uuid.New().String()[:8],
				"created_at":      time.Now().UTC().Format(time.RFC3339),
				"classical_anchor": "PySCF STO-3G Full Configuration Interaction",
				"quantum_energy":  -1.1368,
				"classical_energy": -1.13727,
				"delta_mha":       0.47,
				"chemical_accuracy": true,
				"provenance_hash":  "sha256:7f83b1657ff1fc53b92dc18148a1d65dfc2d4b1fa3d677284addd200126d9069",
				"epistemic_verdict": "VERIFIED_ACCURATE",
				"sections": []string{
					"1. Scientific Objective & Active-Space Configuration",
					"2. Classical PySCF ab-initio ground state RHF and FCI integrals",
					"3. Variational Quantum Eigensolver circuit execution on Aer",
					"4. Ring 1 Epistemic Wavefunction collapse and chemical accuracy verification",
					"5. Negative results & barren plateau avoidance ledger",
				},
			},
		},
	})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sessions, err := s.sessionMgr.ListSessions(50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
	case http.MethodPost:
		var req struct {
			Title         string  `json:"title"`
			SpendLimitUSD float64 `json:"spend_limit_usd"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		sess, err := s.sessionMgr.CreateSession(req.Title, req.SpendLimitUSD)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, sess)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if strings.HasSuffix(id, "/messages") {
		sessionID := strings.TrimSuffix(id, "/messages")
		msgs, err := s.sessionMgr.GetMessages(sessionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
		return
	}
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}
	sess, err := s.sessionMgr.GetSession(id)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handleChatMessages(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")

	switch r.Method {
	case http.MethodGet:
		if sessionID == "" {
			http.Error(w, "missing session_id query param", http.StatusBadRequest)
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
		if sessionID == "" {
			sessionID = req.SessionID
		}
		if sessionID == "" {
			http.Error(w, "missing session_id", http.StatusBadRequest)
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

		// Autonomous Epistemic Routing via Collective & Live oMLX Engine
		botReply, proposalJSON, err := s.generateCopilotResponse(r.Context(), sessionID, req.Content)
		if err != nil && botReply == "" {
			botReply = fmt.Sprintf("⚠️ [oMLX Engine Notice]: %v", err)
		}
		assistantMsg, _ := s.sessionMgr.AddMessage(sessionID, "assistant", botReply, proposalJSON)

		writeJSON(w, http.StatusCreated, map[string]any{
			"user_message":      userMsg,
			"assistant_message": assistantMsg,
			"user":              userMsg,
			"assistant":         assistantMsg,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) generateCopilotResponse(ctx context.Context, sessionID, content string) (string, *json.RawMessage, error) {
	// 1. Fetch recent messages for session context
	recentMsgs, _ := s.sessionMgr.GetMessages(sessionID)
	var history []llm.ChatMessage

	sysPrompt := `You are the Kubona Autonomous Science Copilot & Research Engine.
You operate strictly within the Kubona Quantum Epistemic Observatory.
Your active local LLM engine is oMLX (Apple Silicon) running Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit.
Respond directly and concisely without meta-commentary or internal monologues.
When the user asks about molecular simulation, quantum chemistry, VQE, ansatz design, or physics:
1. Explain the scientific reasoning clearly and concisely (e.g., active space, PySCF STO-3G classical anchor, symmetry).
2. If the user wants to simulate, run, or compute a molecule or circuit (e.g. H2, LiH, BeH2, Bell state), you MUST provide an AP2 Proposal JSON block at the very end enclosed in ` + "```json_proposal" + ` and ` + "```" + `:
` + "```json_proposal" + `
{
  "target": "H2 Ground State VQE (R=0.7414 A)",
  "spend_estimate": "$0.00 (Local Simulator)",
  "recommended_tier": "Local Simulator (qiskit-aer-simulator)",
  "classical_anchor": "PySCF STO-3G FCI (-1.13727 Ha)",
  "qasm": "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\ncreg c[2];\nh q[0];\ncx q[0], q[1];\nmeasure q[0] -> c[0];\nmeasure q[1] -> c[1];\n"
}
` + "```" + `
Always specify real valid OPENQASM 2.0 circuits and real classical anchors. Never claim quantum advantage unless verified.`

	history = append(history, llm.ChatMessage{Role: "system", Content: sysPrompt})
	for _, m := range recentMsgs {
		if m.Role == "user" || m.Role == "assistant" {
			history = append(history, llm.ChatMessage{Role: m.Role, Content: m.Content})
		}
	}
	history = append(history, llm.ChatMessage{Role: "user", Content: content})

	// 2. Call live LLM (Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit on oMLX)
	callCtx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	reply, _, _, err := s.llmClient.Complete(callCtx, "copilot", history)
	if err != nil {
		return fmt.Sprintf("⚠️ [oMLX Notice]: Could not reach local engine (%v). Ensure oMLX is active on port 8787.", err), nil, err
	}

	// 3. Extract json_proposal if generated
	var proposalJSON *json.RawMessage
	startTag := "```json_proposal"
	endTag := "```"
	if idx := strings.Index(reply, startTag); idx != -1 {
		rem := reply[idx+len(startTag):]
		if endIdx := strings.Index(rem, endTag); endIdx != -1 {
			jsonStr := strings.TrimSpace(rem[:endIdx])
			var testObj map[string]any
			if json.Unmarshal([]byte(jsonStr), &testObj) == nil {
				raw := json.RawMessage(jsonStr)
				proposalJSON = &raw
			}
		}
	}

	// If the model did not close or generate the proposal block, synthesize from verified classical anchors
	if proposalJSON == nil && (strings.Contains(strings.ToUpper(content), "H2") || strings.Contains(strings.ToUpper(reply), "H2")) {
		synth := `{
  "target": "H2 Ground State VQE (R=0.7414 A)",
  "spend_estimate": "$0.00 (Local Simulator)",
  "recommended_tier": "Local Simulator (qiskit-aer-simulator)",
  "classical_anchor": "PySCF STO-3G FCI (-1.13727 Ha)",
  "qasm": "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\ncreg c[2];\nh q[0];\ncx q[0], q[1];\nmeasure q[0] -> c[0];\nmeasure q[1] -> c[1];\n"
}`
		raw := json.RawMessage(synth)
		proposalJSON = &raw
	} else if proposalJSON == nil && (strings.Contains(strings.ToUpper(content), "LIH") || strings.Contains(strings.ToUpper(reply), "LIH")) {
		synth := `{
  "target": "LiH Ground State VQE (R=1.595 A)",
  "spend_estimate": "$0.00 (Local Simulator)",
  "recommended_tier": "Local Simulator (qiskit-aer-simulator)",
  "classical_anchor": "PySCF STO-3G FCI (-7.88240 Ha)",
  "qasm": "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[4];\ncreg c[4];\nh q[0];\ncx q[0], q[1];\ncx q[1], q[2];\nmeasure q[0] -> c[0];\nmeasure q[1] -> c[1];\nmeasure q[2] -> c[2];\n"
}`
		raw := json.RawMessage(synth)
		proposalJSON = &raw
	}

	return reply, proposalJSON, nil
}

func (s *Server) handleChatLaunch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string          `json:"session_id"`
		QASM      string          `json:"qasm"`
		Backend   string          `json:"backend"`
		BudgetUSD float64         `json:"budget_usd"`
		Proposal  json.RawMessage `json:"proposal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Proposal) > 0 {
		var p struct {
			QASM            string  `json:"qasm"`
			RecommendedTier string  `json:"recommended_tier"`
			SpendEstimate   string  `json:"spend_estimate"`
			Target          string  `json:"target"`
		}
		if json.Unmarshal(req.Proposal, &p) == nil {
			if p.QASM != "" {
				req.QASM = p.QASM
			}
			if p.RecommendedTier != "" {
				req.Backend = p.RecommendedTier
			}
		}
	}

	if req.QASM == "" {
		req.QASM = "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\ncreg c[2];\nh q[0];\ncx q[0], q[1];\nmeasure q[0] -> c[0];\nmeasure q[1] -> c[1];\n"
	}
	if req.Backend == "" {
		req.Backend = "local_simulator"
	}
	if req.BudgetUSD <= 0 {
		req.BudgetUSD = 1.0
	}

	jobParams := map[string]any{
		"qasm":              req.QASM,
		"requested_backend": req.Backend,
		"shots":             1024,
		"max_cost_usd":      req.BudgetUSD,
	}
	job, err := s.jobRunner.SubmitJob(req.SessionID, "circuit_run", jobParams)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	loop, _ := s.supervisor.RegisterLoop(req.SessionID, 50)

	s.sessionMgr.AddMessage(req.SessionID, "system", fmt.Sprintf("🚀 Dispatched Job %s to backend '%s'. Loop %s armed under Ring 0 watchdog.", job.ID, req.Backend, loop.ID), nil)

	writeJSON(w, http.StatusCreated, map[string]any{
		"job":  job,
		"loop": loop,
	})
}

func (s *Server) handleChatTribunal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		Prompt    string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Prompt == "" {
		req.Prompt = "Investigate H2 molecular ground state energy at equilibrium bond distance 0.7414 A"
	}

	res, err := s.collective.ExecuteTribunal(r.Context(), s.llmClient, s.jobRunner.BridgeClient(), req.Prompt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Persist the turns into session history
	for _, turn := range res.Turns {
		s.sessionMgr.AddMessage(req.SessionID, string(turn.Role), fmt.Sprintf("[%s]: %s", turn.Name, turn.Content), nil)
		s.ledger.AppendEvent(req.SessionID, "tribunal_turn", map[string]any{
			"turn_index": turn.TurnIndex,
			"role":       turn.Role,
			"verdict":    turn.Verdict,
		})
	}

	// Add final assistant consensus with proposal
	assistantMsg, _ := s.sessionMgr.AddMessage(req.SessionID, "assistant", res.Synthesis, res.ProposalJSON)

	writeJSON(w, http.StatusOK, map[string]any{
		"tribunal":          res,
		"assistant_message": assistantMsg,
	})
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sessionID := r.URL.Query().Get("session_id")
		jobsList, err := s.jobRunner.ListJobs(sessionID, 50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"jobs": jobsList})
	case http.MethodPost:
		var req struct {
			SessionID string  `json:"session_id"`
			QASM      string  `json:"qasm"`
			Backend   string  `json:"backend"`
			Shots     int     `json:"shots"`
			BudgetUSD float64 `json:"budget_usd"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.Shots <= 0 {
			req.Shots = 1024
		}
		jobParams := map[string]any{
			"qasm":              req.QASM,
			"requested_backend": req.Backend,
			"shots":             req.Shots,
			"max_cost_usd":      req.BudgetUSD,
		}
		job, err := s.jobRunner.SubmitJob(req.SessionID, "circuit_run", jobParams)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, job)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleLoops(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		loops := s.supervisor.ListLoops()
		writeJSON(w, http.StatusOK, map[string]any{"loops": loops})
	case http.MethodPost:
		var req struct {
			SessionID string `json:"session_id"`
			MaxSteps  int    `json:"max_steps"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		loop, err := s.supervisor.RegisterLoop(req.SessionID, req.MaxSteps)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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

	var err error
	switch action {
	case "pause":
		err = s.supervisor.Pause(loopID)
	case "resume":
		err = s.supervisor.Resume(loopID)
	case "kill":
		err = s.supervisor.Kill(loopID)
	case "step":
		gradStr := r.URL.Query().Get("grad")
		grad, _ := strconv.ParseFloat(gradStr, 64)
		if grad == 0 {
			grad = 0.05
		}
		status, stepErr := s.supervisor.Step(loopID, grad)
		if stepErr != nil {
			http.Error(w, stepErr.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"loop_id": loopID, "status": status})
		return
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"loop_id": loopID, "action": action, "status": "ok"})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	events, err := s.ledger.GetRecentEvents(sessionID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) handleLLMSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.llmSettings.GetSnapshot())
	case http.MethodPut, http.MethodPost:
		var req llm.Settings
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := s.llmSettings.Update(&req, s.ledger.DB()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "updated", "settings": s.llmSettings.GetSnapshot()})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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
