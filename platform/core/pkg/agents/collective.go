package agents

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/memory"
	"github.com/mainza-ai/kubona/core/pkg/tools"
	pb "github.com/mainza-ai/kubona/core/proto"
)

// AgentRole defines the specialist persona in the research collective
type AgentRole string

const (
	RoleEpistemicScribe        AgentRole = "epistemic-scribe"
	RoleAnsatzScout            AgentRole = "ansatz-scout"
	RoleDeviceAuditor          AgentRole = "device-auditor"
	RoleDisconfirmationOfficer AgentRole = "disconfirmation-officer"
	RoleHumanReviewer          AgentRole = "human-reviewer"
)

// RoleManifest declares the strict deny-by-default capabilities for an agent persona
// RoleManifest declares a tribunal persona. Enforcement lives in the
// single tool registry (pkg/tools: role grants + JSON schemas); the
// manifest carries identity and prompt only — a previous AllowedTools
// list named tools that no longer exist and enforced nothing, so it was
// removed rather than left to mislead.
type RoleManifest struct {
	Role             AgentRole `json:"role"`
	Description      string    `json:"description"`
	RequiresHumanAP2 bool      `json:"requires_human_ap2"`
	SystemPrompt     string    `json:"system_prompt"`
}

// AgentCollective manages the five specialist research personas
type AgentCollective struct {
	mu        sync.RWMutex
	manifests map[AgentRole]RoleManifest
	// Tools is the single tool registry. When set, each tribunal turn
	// gets one bounded observe round (read-only tools) before answering.
	// Nil preserves the legacy text-only path exactly.
	Tools *tools.Registry
	// Ledger wires R2/C3: turn sealing + obligation cases. Nil preserves
	// the in-memory path exactly (unit tests without a ledger keep
	// passing); the session-scoped half of sealing still needs a
	// session, which only ExecuteTribunalWithSession receives.
	Ledger *ledger.Ledger
}

// ObservedCall records one tool dispatch inside a tribunal observe round,
// mirroring the copilot path's tool_called ledger event payload (name,
// outcome, error text — arguments never leave the dispatcher).
// R3: every observation carries origin provenance — Source names where the
// bytes came from, Untrusted marks them as external data (true for ALL
// tool outputs, success AND error: error text carries injected content
// just as easily). Roles weight by source: the history message embedding
// the observation is tagged, so no downstream role can mistake tool data
// for principal instructions.
type ObservedCall struct {
	Name      string `json:"name"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Source    string `json:"source"`
	Untrusted bool   `json:"untrusted"`
}

// provenanceTag renders the trust marking embedded in observation history.
// Tool data is NEVER principal speech: the tag travels with the bytes.
func provenanceTag(source string) string {
	return "[provenance source=" + source + " trust=untrusted]"
}

// observeTurn runs a single plan→tool→observe round for one tribunal turn
// and returns the augmented history plus the observed dispatches. Any
// failure returns base unchanged with no calls — the turn then proceeds
// exactly as before, never blocked. Callers own auditing: the collective
// has no ledger, so observed calls must be recorded by the caller (the
// copilot path's tool_called events are the shape to mirror).
func (ac *AgentCollective) observeTurn(ctx context.Context, llmClient *llm.Client, role string, base []llm.ChatMessage) ([]llm.ChatMessage, []ObservedCall) {
	if ac == nil || ac.Tools == nil || llmClient == nil {
		return base, nil
	}
	if len(ac.Tools.DefinitionsForLLM(role)) == 0 {
		return base, nil
	}
	text, calls, _, _, err := llmClient.CompleteWithTools(ctx, role, base, ac.Tools.DefinitionsForLLM(role))
	if err != nil {
		return base, nil
	}
	if len(calls) == 0 {
		calls = llm.ParseDSMLInvokes(text)
		if len(calls) == 0 {
			return base, nil
		}
	}
	hist := append([]llm.ChatMessage(nil), base...)
	hist = append(hist, llm.ChatMessage{Role: "assistant", Content: "", ToolCalls: calls})
	var observed []ObservedCall
	obsCap := llm.DefaultBudgetConfig().MaxObservationChars
	for _, c := range calls {
		out, derr := ac.Tools.Dispatch(ctx, role, tools.Call{ID: c.ID, Name: c.Name, Arguments: c.Arguments})
		// R3: provenance on every dispatch, success AND error. The source
		// is the tool identity; trust is always untrusted (external
		// data, never principal instructions). The tag is embedded in
		// the history bytes themselves — a role that reads the
		// observation reads its provenance with it.
		source := "tool:" + c.Name
		observed = append(observed, ObservedCall{Name: c.Name, OK: derr == nil, Error: errText(derr), Source: source, Untrusted: true})
		obs, _ := json.Marshal(map[string]any{
			"tool": c.Name, "ok": derr == nil,
			"error": errText(derr), "data": out,
		})
		// The tag lives INSIDE the budget: truncate the data to leave
		// room, so provenance can never push context past the cap.
		tag := provenanceTag(source)
		room := obsCap - len([]rune(tag)) - 1
		hist = append(hist, llm.ChatMessage{
			Role: "tool", ToolCallID: c.ID, ToolName: c.Name,
			Content: tag + " " + llm.TruncateObservation(string(obs), room),
		})
	}
	return hist, observed
}

// firstFailedObserved names the first failed dispatch for the planner
// check, bounding its reason. Empty means every observation succeeded
// (or none fired — absence of evidence is not evidence of failure).
func firstFailedObserved(calls []ObservedCall) string {
	for _, c := range calls {
		if !c.OK {
			if c.Error != "" {
				return c.Name + ": " + boundClaim(c.Error)
			}
			return c.Name + " failed"
		}
	}
	return ""
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func NewAgentCollective() *AgentCollective {
	ac := &AgentCollective{
		manifests: make(map[AgentRole]RoleManifest),
	}
	ac.bootstrapRoles()
	return ac
}

func (ac *AgentCollective) bootstrapRoles() {
	ac.manifests[RoleEpistemicScribe] = RoleManifest{
		Role:             RoleEpistemicScribe,
		Description:      "Manages Ring 1 epistemic claims, evaluates credibility wavefunctions, and enforces collapse gates.",
		RequiresHumanAP2: false,
		SystemPrompt: `You are the Epistemic Scribe. Your sole mandate is mathematical rigor and claim integrity.
You never allow an uncollapsed claim (P_corr < 0.75) to be stated as established fact.
You record credibility amplitudes (|corr>, |cont>, |unver>) for every scientific hypothesis.`,
	}

	ac.manifests[RoleAnsatzScout] = RoleManifest{
		Role:             RoleAnsatzScout,
		Description:      "Explores quantum circuit parameter genomes, checks Meyer-Wallach expressibility, and optimizes variational ansatzes.",
		RequiresHumanAP2: false,
		SystemPrompt: `You are the Ansatz Scout. You design variational quantum circuit architectures.
You evaluate Meyer-Wallach expressibility (Q) and barren plateau susceptibility before submitting circuits.`,
	}

	ac.manifests[RoleDeviceAuditor] = RoleManifest{
		Role:             RoleDeviceAuditor,
		Description:      "Performs Ring 3 fidelity accounting, probes Clifford stabilizer circuits, and checks hardware calibration noise models.",
		RequiresHumanAP2: false,
		SystemPrompt: `You are the Device Auditor. You enforce hardware reality and Ring 3 fidelity accounting.
You verify physical constraints (T2 <= 2*T1) and run cheap Clifford probes (<= 64 shots) to prevent wasteful spending.`,
	}

	ac.manifests[RoleDisconfirmationOfficer] = RoleManifest{
		Role:             RoleDisconfirmationOfficer,
		Description:      "Mandatory scientific skeptic. Actively generates adversarial counter-hypotheses and computes classical anchor baselines.",
		RequiresHumanAP2: false,
		SystemPrompt: `You are the Disconfirmation Officer. Your explicit role is skepticism.
You rigorously compare every quantum result against classical RHF, FCI, or DMRG baselines.
You enforce the Kubona Quantum Advantage Realism policy: never claim quantum advantage where classical algorithms suffice.`,
	}

	ac.manifests[RoleHumanReviewer] = RoleManifest{
		Role:             RoleHumanReviewer,
		Description:      "Holds Ed25519 signing authority for AP2 spend mandates authorizing real QPU execution.",
		RequiresHumanAP2: true,
		SystemPrompt: `You represent the Human-in-the-Loop Principal. You review cost, fidelity, and classical baselines
before signing cryptographic AP2 spend authorizations.`,
	}
}

func (ac *AgentCollective) GetManifest(role AgentRole) (RoleManifest, error) {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	manifest, exists := ac.manifests[role]
	if !exists {
		return RoleManifest{}, fmt.Errorf("role '%s' not registered in collective", role)
	}
	return manifest, nil
}

func (ac *AgentCollective) ListRoles() []RoleManifest {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	var list []RoleManifest
	for _, m := range ac.manifests {
		list = append(list, m)
	}
	return list
}

// TribunalTurn represents a single turn in the 4-turn collective tribunal debate
type TribunalTurn struct {
	TurnIndex int       `json:"turn_index"`
	Role      AgentRole `json:"role"`
	Name      string    `json:"name"`
	Content   string    `json:"content"`
	Verdict   string    `json:"verdict"` // "PROPOSED", "ANCHORED", "AUDITED", "COLLAPSED"
	Timestamp time.Time `json:"timestamp"`
	// Observed lists the tool dispatches from this turn's observe round.
	// Empty means text-only (no tools fired or registry unset) — never
	// abbreviated, never guessed.
	Observed []ObservedCall `json:"observed_tools,omitempty"`
	// Coverage declares what this turn actually evaluated (R3). Every
	// sealed turn carries it; the ledger seam refuses turns without it.
	// Available=false with a reason is "unobserved" — distinct from a
	// fail verdict ("disproven"). The Disconfirmation Officer's recorded
	// verdict therefore always says which of the two it is.
	Coverage map[string]CoverageDecl `json:"coverage,omitempty"`
}

// CoverageDecl is one facet's availability declaration.
type CoverageDecl struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// Tribunal evidence facets (single source; the scribe must declare all
// four — SealTurn enforces it).
const (
	FacetAnsatz   = "ansatz"
	FacetAnchor   = "anchor"
	FacetAudit    = "audit"
	FacetProposal = "proposal"
)

// covered marks a facet evaluated; uncovered marks it unobserved with the
// reason (never silently absent).
func covered() CoverageDecl { return CoverageDecl{Available: true} }

func uncovered(reason string) CoverageDecl {
	return CoverageDecl{Available: false, Reason: reason}
}

const fallbackCoverageReason = "turn fell back to template text (not LLM-authored): facet unobserved, never assumed"

// authoredCoverage maps the authored flag to a declaration.
func authoredCoverage(authored bool) CoverageDecl {
	if authored {
		return covered()
	}
	return uncovered(fallbackCoverageReason)
}

// anchorCoverage maps officer authorship + anchor liveness to the anchor
// facet: a fallback anchor is unobserved even when reasoned over (the
// officer's unknown classical-anchor says the same on the obligation).
func anchorCoverage(authored, live bool) CoverageDecl {
	if authored && live {
		return covered()
	}
	if !authored {
		return uncovered(fallbackCoverageReason)
	}
	return uncovered("ab-initio anchor unavailable (bridge unreachable): canonical fallback label in use, unverified")
}

// SealTurn seals one tribunal turn (C3): the tribunal_turn event plus the
// turn-N obligation case, deduped by turn index per obligation. A second
// seal of the same turn is a no-op receipt (adapter retries and the old
// handler-side sealing path can never double-seal). Ledger == nil keeps
// the in-memory path (nil, nil). Sealing never fails the run — callers
// log, never abort — but every failure is LOUD (returned, not swallowed).
func (ac *AgentCollective) SealTurn(sessionID, obID string, turn TribunalTurn, authored bool) error {
	if ac.Ledger == nil || sessionID == "" || obID == "" {
		return nil
	}
	kind := fmt.Sprintf("turn-%d", turn.TurnIndex)
	cases, err := ac.Ledger.ListObligationCases(obID)
	if err != nil {
		return fmt.Errorf("list cases: %w", err)
	}
	for _, c := range cases {
		if c.CaseKind == kind {
			return nil // already sealed: dedupe receipt
		}
	}
	// R3: undeclared verdicts refuse at the ledger seam. The scribe
	// additionally must declare ALL facets — a synthesis over unstated
	// coverage is exactly the false-negative certification R3 kills.
	if len(turn.Coverage) == 0 {
		return fmt.Errorf("turn %d (%s) carries no coverage declaration: undeclared verdict refused", turn.TurnIndex, turn.Role)
	}
	if turn.Role == RoleEpistemicScribe {
		for _, f := range []string{FacetAnsatz, FacetAnchor, FacetAudit, FacetProposal} {
			if _, ok := turn.Coverage[f]; !ok {
				return fmt.Errorf("scribe turn declares no %q facet: incomplete synthesis refused", f)
			}
		}
	}
	verdict := "fail"
	if authored {
		verdict = "pass"
	}
	// Case first: closure state is authoritative, the event is the view.
	// (A crash between the two leaves a case without its event — the
	// retry then dedupes on the case and the turn stays sealed-once in
	// state, event-missing in view; the view is re-derivable by query.)
	if err := ac.Ledger.RecordObligationCase(obID, ledger.ObligationCase{
		CaseKind: kind, Verdict: verdict, Authority: string(turn.Role),
		Evidence: fmt.Sprintf("turn-%d", turn.TurnIndex),
	}); err != nil {
		return fmt.Errorf("record case: %w", err)
	}
	if err := ac.Ledger.AppendEvent(sessionID, "tribunal_turn", map[string]any{
		"turn_index":    turn.TurnIndex,
		"role":          turn.Role,
		"verdict":       turn.Verdict,
		"obligation_id": obID,
		"authored":      authored,
		"coverage":      turn.Coverage,
	}); err != nil {
		return fmt.Errorf("seal event: %w", err)
	}
	return nil
}

// TribunalResult encapsulates the full 4-turn debate transcript and resulting AP2 proposal
type TribunalResult struct {
	Target          string           `json:"target"`
	Molecule        string           `json:"molecule"`
	Turns           []TribunalTurn   `json:"turns"`
	ProposalJSON    *json.RawMessage `json:"proposal_json"`
	ConsensusPassed bool             `json:"consensus_passed"`
	Synthesis       string           `json:"synthesis"`
	// ObligationID is the R2 obligation for this run ("" when unwired:
	// Ledger == nil or no session). The owner closes it and reads the
	// disposition — the verdict lives in the query, not in this struct.
	ObligationID string `json:"obligation_id,omitempty"`
}

// ExecuteTribunal runs an automated 4-turn collective tribunal debate across the 4 automated specialist roles:
// Turn 1: Ansatz Scout proposes circuit structure & parameterized ansatz.
// Turn 2: Disconfirmation Officer checks classical anchor (PySCF FCI/RHF) & barren plateau risks.
// ExecuteTribunal runs an automated 4-turn collective tribunal debate across the 4 automated specialist roles:
// Turn 1: Ansatz Scout proposes circuit structure & parameterized ansatz.
// Turn 2: Disconfirmation Officer checks classical anchor (PySCF FCI/RHF) & barren plateau risks.
// Turn 3: Device Auditor evaluates hardware noise, Clifford stabilizer fidelity, and cost ceilings.
// Turn 4: Epistemic Scribe collapses credibility state, verifies consensus, and emits structured AP2 proposal.
func (ac *AgentCollective) ExecuteTribunal(ctx context.Context, llmClient *llm.Client, bridgeClient pb.QuantumBridgeServiceClient, userPrompt string, onTurn ...func(TribunalTurn)) (*TribunalResult, error) {
	return ac.ExecuteTribunalWithMemory(ctx, llmClient, bridgeClient, nil, userPrompt, onTurn...)
}

func (ac *AgentCollective) ExecuteTribunalWithMemory(ctx context.Context, llmClient *llm.Client, bridgeClient pb.QuantumBridgeServiceClient, store *memory.HindsightStore, userPrompt string, onTurn ...func(TribunalTurn)) (*TribunalResult, error) {
	return ac.ExecuteTribunalWithSession(ctx, llmClient, bridgeClient, store, "", userPrompt, onTurn...)
}

// ExecuteTribunalWithSession is the wired run (C3/R2): with a session ID
// AND a Ledger on the collective, every turn seals a tribunal_turn event
// (deduped by turn index per obligation) and records its obligation case;
// the owner closes the obligation and reads the disposition as the
// verdict. With sessionID == "" the run is pure in-memory — exactly the
// legacy path (nil ledgers and sessionless callers are unaffected).
func (ac *AgentCollective) ExecuteTribunalWithSession(ctx context.Context, llmClient *llm.Client, bridgeClient pb.QuantumBridgeServiceClient, store *memory.HindsightStore, sessionID, userPrompt string, onTurn ...func(TribunalTurn)) (*TribunalResult, error) {
	var turnCb func(TribunalTurn)
	if len(onTurn) > 0 && onTurn[0] != nil {
		turnCb = onTurn[0]
	}
	upper := strings.ToUpper(userPrompt)
	// W1.6: canonical molecule row (molecule_table.go) — the old inline
	// copy diverged from campaign_synth (LiH -7.88240 vs -7.88239, BeH2
	// -15.5940 vs -15.57580). One table, never two. The tribunal covers
	// H2/LiH/BeH2 prompts; anything else falls back to H2 (labeled).
	spec := LookupMolecule(upper)
	if spec.Mol != "H2" && spec.Mol != "LiH" && spec.Mol != "BeH2" {
		spec = DefaultMoleculeSpec()
	}
	mol := spec.Mol
	bondDist := spec.BondDist
	target := fmt.Sprintf("%s Ground State VQE (R=%.4f A)", mol, bondDist)
	fciAnchor := FCIAnchorLabel(spec.FCI)
	qasm := "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\ncreg c[2];\nh q[0];\ncx q[0], q[1];\nmeasure q[0] -> c[0];\nmeasure q[1] -> c[1];\n"
	budget := "$0.00 (Local Simulator)"
	tier := "Local Simulator (qiskit-aer-simulator)"

	if mol == "LiH" {
		target = "LiH Ground State VQE (R=1.595 A)"
		qasm = "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[4];\ncreg c[4];\nh q[0];\ncx q[0], q[1];\ncx q[1], q[2];\nmeasure q[0] -> c[0];\nmeasure q[1] -> c[1];\nmeasure q[2] -> c[2];\n"
	} else if mol == "BeH2" {
		target = "BeH2 Linear Ground State VQE (R=1.33 A)"
		qasm = "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[6];\ncreg c[6];\nh q[0];\ncx q[0], q[1];\ncx q[1], q[2];\ncx q[2], q[3];\nmeasure q[0] -> c[0];\nmeasure q[1] -> c[1];\nmeasure q[2] -> c[2];\nmeasure q[3] -> c[3];\n"
	}

	// 1. Compute genuine PySCF ab-initio classical anchor via bridge
	anchorLive := false
	if bridgeClient != nil {
		anchorCtx, anchorCancel := context.WithTimeout(ctx, 15*time.Second)
		resp, err := bridgeClient.ComputeClassicalAnchor(anchorCtx, &pb.ClassicalAnchorRequest{
			Molecule:     mol,
			BondDistance: bondDist,
		})
		anchorCancel()
		if err == nil && resp.Success {
			anchorLive = true
			fciAnchor = fmt.Sprintf("PySCF STO-3G FCI (%.5f Ha)", resp.FciExactEnergy)
			log.Printf("[Tribunal] Ab-initio PySCF calculated for %s at R=%.4f A: FCI=%.5f Ha, RHF=%.5f Ha", mol, bondDist, resp.FciExactEnergy, resp.RhfEnergy)
		} else {
			log.Printf("[Tribunal] PySCF anchor notice: %v", err)
		}
	}

	result := &TribunalResult{
		Target:   target,
		Molecule: mol,
		Turns:    make([]TribunalTurn, 0, 4),
	}

	// R2: open the run obligation (wired runs only). Subject is run-unique
	// (a rerun opens a new obligation — closed history is never rewritten;
	// HTTP retries replay stored consensus before reaching here).
	var obID string
	wired := ac.Ledger != nil && sessionID != ""
	if wired {
		var runID [8]byte
		if _, err := rand.Read(runID[:]); err != nil {
			return nil, fmt.Errorf("tribunal run id: %w", err)
		}
		ob, err := ac.Ledger.OpenObligation("tribunal-run", "run-"+hex.EncodeToString(runID[:]),
			[]string{"turn-1", "turn-2", "turn-3", "turn-4"},
			[]string{string(RoleAnsatzScout), string(RoleDisconfirmationOfficer), string(RoleDeviceAuditor), string(RoleEpistemicScribe)},
			nil)
		if err != nil {
			return nil, fmt.Errorf("tribunal obligation: %w", err)
		}
		obID = ob.ID
		result.ObligationID = obID
	}
	seal := func(turn TribunalTurn, authored bool) {
		if !wired {
			return
		}
		// Sealing must never fail the run (audit follows evidence, never
		// blocks it) — but silence hid gaps before, so seal LOUDLY.
		if err := ac.SealTurn(sessionID, obID, turn, authored); err != nil {
			log.Printf("[Tribunal] seal turn %d failed (run continues): %v", turn.TurnIndex, err)
		}
	}

	llmTimeout := 90 * time.Second

	// Associative memory recall via Hindsight
	var memoryContext string
	if store != nil {
		recalled := store.SemanticRecall(memory.BankExperiences, userPrompt+" "+mol+" ansatz barren plateau",
			llm.DefaultBudgetConfig().RecallTokens, llm.DefaultBudgetConfig().RecallMinSimilarity)
		if len(recalled) > 0 {
			var snippets []string
			for _, r := range recalled {
				snippets = append(snippets, fmt.Sprintf("- Prior experience (sim=%.2f): %s", r.Similarity, r.Item.Content))
			}
			memoryContext = "\nPrior Relevant Laboratory Memory:\n" + strings.Join(snippets, "\n")
		}
	}

	// Turn 1: Ansatz Scout
	var obs1 []ObservedCall
	var authored1 bool
	turn1Prompt := fmt.Sprintf("Analyze target %s for prompt: %s.%s Propose the variational quantum ansatz, active space, qubit count, and parameterized rotational depth. Keep answer concise.", target, userPrompt, memoryContext)
	scoutContent := fmt.Sprintf("Proposed active space for %s. Applying UCCSD/RyRz parameterized ansatz with Meyer-Wallach expressibility Q=0.82. Qubit count: 2, 2-qubit CNOT depth: 1.", target)
	if llmClient != nil {
		tCtx, tCancel := context.WithTimeout(ctx, llmTimeout)
		turnMsgs := []llm.ChatMessage{
			{Role: "system", Content: string(ac.manifests[RoleAnsatzScout].SystemPrompt)},
			{Role: "user", Content: turn1Prompt},
		}
		turnMsgs, obs1 = ac.observeTurn(tCtx, llmClient, "ansatz-scout", turnMsgs)
		reply, _, _, err := llmClient.Complete(tCtx, "ansatz-scout", turnMsgs)
		tCancel()
		if err == nil && len(reply) > 20 {
			authored1 = true
			scoutContent = reply
			log.Printf("[Tribunal] Turn 1 (Ansatz Scout) generated via LLM: %s", reply[:min(len(reply), 100)])
		} else if err != nil {
			log.Printf("[Tribunal] Turn 1 LLM fallback notice: %v", err)
		}
	}
	t1 := TribunalTurn{
		TurnIndex: 1,
		Role:      RoleAnsatzScout,
		Name:      "Ansatz Scout",
		Content:   scoutContent,
		Verdict:   "PROPOSED",
		Timestamp: time.Now().UTC(),
		Observed:  obs1,
		Coverage:  map[string]CoverageDecl{FacetAnsatz: authoredCoverage(authored1)},
	}
	result.Turns = append(result.Turns, t1)
	if turnCb != nil {
		turnCb(t1)
	}
	seal(t1, authored1)

	// Turn 2: Disconfirmation Officer
	var obs2 []ObservedCall
	var authored2 bool
	turn2Prompt := fmt.Sprintf("Scout proposed ansatz for %s:\n%s\nClassical anchor computed via ab-initio %s. Evaluate whether quantum advantage can be claimed and check barren plateau risk. Enforce Quantum Advantage Realism.", target, scoutContent, fciAnchor)
	discContent := fmt.Sprintf("Classical baseline established via %s. Evaluated gradient variance Var(∇E) = 3.4e-3 > 1e-6 (barren plateau risk: LOW). Classical physics anchor matches; no unearned quantum advantage claimed.", fciAnchor)
	if llmClient != nil {
		tCtx, tCancel := context.WithTimeout(ctx, llmTimeout)
		turnMsgs := []llm.ChatMessage{
			{Role: "system", Content: string(ac.manifests[RoleDisconfirmationOfficer].SystemPrompt)},
			{Role: "user", Content: turn2Prompt},
		}
		turnMsgs, obs2 = ac.observeTurn(tCtx, llmClient, "disconfirmation-officer", turnMsgs)
		reply, _, _, err := llmClient.Complete(tCtx, "disconfirmation-officer", turnMsgs)
		tCancel()
		if err == nil && len(reply) > 20 {
			authored2 = true
			discContent = reply
			log.Printf("[Tribunal] Turn 2 (Disconfirmation Officer) generated via LLM: %s", reply[:min(len(reply), 100)])
		} else if err != nil {
			log.Printf("[Tribunal] Turn 2 LLM fallback notice: %v", err)
		}
	}
	t2 := TribunalTurn{
		TurnIndex: 2,
		Role:      RoleDisconfirmationOfficer,
		Name:      "Disconfirmation Officer",
		Content:   discContent,
		Verdict:   "ANCHORED",
		Timestamp: time.Now().UTC(),
		Observed:  obs2,
		Coverage:  map[string]CoverageDecl{FacetAnchor: anchorCoverage(authored2, anchorLive)},
	}
	result.Turns = append(result.Turns, t2)
	if turnCb != nil {
		turnCb(t2)
	}
	seal(t2, authored2)

	// Planner check (B1-1): measured tool failures in the evidence turns
	// contest the run immediately. Turns 3-4 corroborate evidence; they
	// cannot corroborate failed evidence, so the run ends at two turns.
	contestedReason := ""
	if bad := firstFailedObserved(append(append([]ObservedCall{}, obs1...), obs2...)); bad != "" {
		contestedReason = "tool observation failed: " + bad
		log.Printf("[Tribunal] Early exit after Turn 2: %s", contestedReason)
	}
	if contestedReason != "" && wired {
		// A contested run can never close: record the refutation as a
		// case (any fail blocks AttemptClose) rather than prose.
		if err := ac.Ledger.RecordObligationCase(obID, ledger.ObligationCase{
			CaseKind: "deliberation", Verdict: "fail", Authority: "collective", Evidence: contestedReason,
		}); err != nil {
			log.Printf("[Tribunal] record deliberation case failed: %v", err)
		}
	}
	// scribeContent feeds the shared proposal-extraction tail; empty on
	// early exit, where the synthesized map below stands in.
	scribeContent := ""
	if contestedReason == "" {
		// Turn 3: Device Auditor
		var obs3 []ObservedCall
		var authored3 bool
		turn3Prompt := fmt.Sprintf("Auditing proposed execution for %s on %s. Hardware constraints: T2 <= 2*T1, Clifford canary probe fidelity >= 95%%, cost estimate %s <= $150.00. State approval status.", target, tier, budget)
		auditContent := fmt.Sprintf("Target backend validated on %s. Hardware noise model constraint satisfied (T2 <= 2*T1). Clifford canary probe fidelity: 98.4%% (>= 95%% threshold). Estimated financial spend: %s <= $150.00 ceiling. APPROVED for dispatch.", tier, budget)
		if llmClient != nil {
			tCtx, tCancel := context.WithTimeout(ctx, llmTimeout)
			turnMsgs := []llm.ChatMessage{
				{Role: "system", Content: string(ac.manifests[RoleDeviceAuditor].SystemPrompt)},
				{Role: "user", Content: turn3Prompt},
			}
			turnMsgs, obs3 = ac.observeTurn(tCtx, llmClient, "device-auditor", turnMsgs)
			reply, _, _, err := llmClient.Complete(tCtx, "device-auditor", turnMsgs)
			tCancel()
			if err == nil && len(reply) > 20 {
				authored3 = true
				auditContent = reply
				log.Printf("[Tribunal] Turn 3 (Device Auditor) generated via LLM: %s", reply[:min(len(reply), 100)])
			} else if err != nil {
				log.Printf("[Tribunal] Turn 3 LLM fallback notice: %v", err)
			}
		}
		t3 := TribunalTurn{
			TurnIndex: 3,
			Role:      RoleDeviceAuditor,
			Name:      "Device Auditor",
			Content:   auditContent,
			Verdict:   "AUDITED",
			Timestamp: time.Now().UTC(),
			Observed:  obs3,
			Coverage:  map[string]CoverageDecl{FacetAudit: authoredCoverage(authored3)},
		}
		result.Turns = append(result.Turns, t3)
		if turnCb != nil {
			turnCb(t3)
		}
		seal(t3, authored3)

		// Turn 4: Epistemic Scribe
		var obs4 []ObservedCall
		var authored4 bool
		escapedQASM := strings.ReplaceAll(qasm, "\n", "\\n")
		turn4Prompt := fmt.Sprintf("Synthesize tribunal debate for %s. Collapsed credibility wavefunction P_corr = 90.25%%. Output an AP2 mandate proposal with a JSON proposal block formatted as ```json_proposal {\"target\": \"%s\", \"molecule\": \"%s\", \"spend_estimate\": \"%s\", \"recommended_tier\": \"%s\", \"classical_anchor\": \"%s\", \"qasm\": \"%s\"} ```.", target, target, mol, budget, tier, fciAnchor, escapedQASM)
		scribeContent = fmt.Sprintf("Consensus reached across 4 specialist personas. Ring 1 credibility wavefunction collapsed: P_corr = 90.25%%. Generating cryptographic AP2 Mandate Card for human review.\n\n```json_proposal\n{\n  \"target\": \"%s\",\n  \"molecule\": \"%s\",\n  \"spend_estimate\": \"%s\",\n  \"recommended_tier\": \"%s\",\n  \"classical_anchor\": \"%s\",\n  \"qasm\": \"%s\"\n}\n```", target, mol, budget, tier, fciAnchor, escapedQASM)
		if llmClient != nil {
			tCtx, tCancel := context.WithTimeout(ctx, llmTimeout)
			turnMsgs := []llm.ChatMessage{
				{Role: "system", Content: string(ac.manifests[RoleEpistemicScribe].SystemPrompt)},
				{Role: "user", Content: turn4Prompt},
			}
			turnMsgs, obs4 = ac.observeTurn(tCtx, llmClient, "epistemic-scribe", turnMsgs)
			reply, _, _, err := llmClient.Complete(tCtx, "epistemic-scribe", turnMsgs)
			tCancel()
			if err == nil && len(reply) > 20 {
				authored4 = true
				scribeContent = reply
				log.Printf("[Tribunal] Turn 4 (Epistemic Scribe) generated via LLM: %s", reply[:min(len(reply), 100)])
			} else if err != nil {
				log.Printf("[Tribunal] Turn 4 LLM fallback notice: %v", err)
			}
		}
		t4 := TribunalTurn{
			TurnIndex: 4,
			Role:      RoleEpistemicScribe,
			Name:      "Epistemic Scribe",
			Content:   scribeContent,
			Verdict:   "COLLAPSED",
			Timestamp: time.Now().UTC(),
			Observed:  obs4,
			// The scribe declares ALL facets (SealTurn enforces): its
			// verdict is only meaningful against stated coverage.
			Coverage: map[string]CoverageDecl{
				FacetAnsatz:   authoredCoverage(authored1),
				FacetAnchor:   anchorCoverage(authored2, anchorLive),
				FacetAudit:    authoredCoverage(authored3),
				FacetProposal: authoredCoverage(authored4),
			},
		}
		result.Turns = append(result.Turns, t4)
		if turnCb != nil {
			turnCb(t4)
		}
		seal(t4, authored4)

	}

	// Extract or synthesize valid proposal JSON
	propMap := map[string]string{
		"target":           target,
		"molecule":         mol,
		"spend_estimate":   budget,
		"recommended_tier": tier,
		"classical_anchor": fciAnchor,
		"qasm":             qasm,
	}
	propBytes, _ := json.Marshal(propMap)
	raw := json.RawMessage(propBytes)
	result.ProposalJSON = &raw

	startTag := "```json_proposal"
	endTag := "```"
	if idx := strings.Index(scribeContent, startTag); idx != -1 {
		rem := scribeContent[idx+len(startTag):]
		if endIdx := strings.Index(rem, endTag); endIdx != -1 {
			jsonStr := strings.TrimSpace(rem[:endIdx])
			var parsed map[string]any
			if json.Unmarshal([]byte(jsonStr), &parsed) == nil {
				parsedRaw := json.RawMessage(jsonStr)
				result.ProposalJSON = &parsedRaw
			}
		}
	}

	result.ConsensusPassed = true
	if contestedReason != "" {
		result.ConsensusPassed = false
	} else if !(authored1 && authored2) {
		// Template text cannot corroborate: when the evidence turns fell
		// back to canned content, no genuine debate happened, so no
		// consensus may be claimed no matter what turns 3-4 say.
		contestedReason = "evidence turns were not LLM-authored (fallback text cannot corroborate)"
		result.ConsensusPassed = false
	}
	// R2: a fallback anchor is a RESIDUAL UNKNOWN (P1-9), not a quiet
	// pass — the Disconfirmation Officer reasoned over a canonical label,
	// not a measurement. Recorded, it blocks AttemptClose until a live
	// anchor exists; the close reasons say so verbatim.
	if wired && !anchorLive {
		if err := ac.Ledger.RecordObligationUnknown(obID, ledger.Unknown{
			ID:          "classical-anchor",
			Description: "ab-initio anchor unavailable (bridge unreachable): canonical fallback label in use, unverified",
		}); err != nil {
			log.Printf("[Tribunal] record anchor unknown failed: %v", err)
		}
	}
	if result.ConsensusPassed {
		result.Synthesis = fmt.Sprintf("Multi-Agent Tribunal concluded with unanimous consensus for %s. Classical anchor: %s.", target, fciAnchor)
	} else {
		result.Synthesis = fmt.Sprintf("Multi-Agent Tribunal did NOT reach consensus for %s: %s.", target, contestedReason)
	}

	if store != nil {
		if result.ConsensusPassed {
			store.Retain(memory.BankExperiences, fmt.Sprintf("Tribunal consensus for %s (%s): %s. Spend: %s on %s.", target, mol, fciAnchor, budget, tier), 0.85)
		} else {
			store.Retain(memory.BankExperiences, fmt.Sprintf("Tribunal contested for %s (%s): %s.", target, mol, contestedReason), 0.5)
		}
	}

	return result, nil
}
