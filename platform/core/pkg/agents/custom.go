package agents

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// CustomAgentManifest defines a user-created agent persona
type CustomAgentManifest struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Role           string   `json:"role"`
	Description    string   `json:"description"`
	SystemPrompt   string   `json:"system_prompt"`
	AvatarEmoji    string   `json:"avatar_emoji"`
	BadgeColor     string   `json:"badge_color"`
	Capabilities   []string `json:"capabilities"`
	Modalities     []string `json:"modalities"`
	PreferredModel string   `json:"preferred_model"`
	MCPTools       []string `json:"mcp_tools"`
	SpendBudgetUSD float64  `json:"spend_budget_usd"`
	MemoryScopes   []string `json:"memory_scopes"`
	IsSystemAgent  bool     `json:"is_system_agent"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
}

// CustomAgentRegistry manages user-defined custom agents
type CustomAgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]CustomAgentManifest
	seeds  map[string]bool
	db     *sql.DB
}

// NewCustomAgentRegistry creates and seeds a custom agent registry
func NewCustomAgentRegistry() *CustomAgentRegistry {
	reg := &CustomAgentRegistry{
		agents: make(map[string]CustomAgentManifest),
		seeds:  make(map[string]bool),
	}
	reg.seedDefaults()
	return reg
}

// NewCustomAgentRegistryWithDB creates the registry and loads persisted
// user agents on top of the seeds. K3: user agents previously vanished
// on every restart (in-memory only) — seeds stay code-owned, user rows
// live in custom_agents.
func NewCustomAgentRegistryWithDB(db *sql.DB) *CustomAgentRegistry {
	reg := NewCustomAgentRegistry()
	if db == nil {
		return reg
	}
	reg.db = db
	rows, err := db.Query(`SELECT id, manifest_json FROM custom_agents`)
	if err != nil {
		return reg
	}
	defer rows.Close()
	for rows.Next() {
		var id, raw string
		var m CustomAgentManifest
		if err := rows.Scan(&id, &raw); err != nil {
			continue
		}
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			continue
		}
		m.ID = id
		m.IsSystemAgent = false
		reg.agents[id] = m
	}
	return reg
}

// seedIDs are immutable: user registration can never overwrite or delete
// them (otherwise an attacker re-stamps a seed non-system, then deletes
// it — the Delete guard below would be theater).
func (r *CustomAgentRegistry) isSeedID(id string) bool {
	// Lock-free by design (caller holds mu): seeds are written once at
	// construction, never mutated after.
	return r.seeds[id]
}

func (r *CustomAgentRegistry) seedDefaults() {
	now := time.Now().UTC().Format(time.RFC3339)
	defaults := []CustomAgentManifest{
		{
			ID:             "agent_qksd_theorist",
			Name:           "Quantum Krylov Theorist",
			Role:           "lead_theorist",
			Description:    "Specialist in real-time Krylov subspace expansion and multi-reference active space Hamiltonian reduction.",
			SystemPrompt:   "You are the Quantum Krylov Theorist. Your goal is to maximize the overlap matrix condition number while minimizing non-orthogonal basis states.",
			AvatarEmoji:    "⚛️",
			BadgeColor:     "cyan",
			Capabilities:   []string{"qksd", "electronic_structure", "generalized_eigenvalues"},
			Modalities:     []string{"text", "molecule_3d"},
			PreferredModel: "local/qwen2.5-coder:32b",
			MCPTools:       []string{"bridge_solve_krylov", "bridge_compute_anchor"},
			SpendBudgetUSD: 5.0,
			MemoryScopes:   []string{"session", "hindsight_pareto"},
			IsSystemAgent:  true,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		{
			ID:             "agent_mitigation_auditor",
			Name:           "Error Mitigation Auditor",
			Role:           "device_auditor",
			Description:    "Audits pulse-level zero-noise extrapolation, depolarizing quasi-probability representations, and Clifford data regression.",
			SystemPrompt:   "You are the Error Mitigation Auditor. You enforce rigorous Richardson extrapolation bounds and reject under-folded circuits.",
			AvatarEmoji:    "🛡️",
			BadgeColor:     "purple",
			Capabilities:   []string{"zne", "pec", "cdr", "noise_budget"},
			Modalities:     []string{"text", "circuit_qir"},
			PreferredModel: "local/qwen2.5-coder:32b",
			MCPTools:       []string{"bridge_mitigate_circuit"},
			SpendBudgetUSD: 2.5,
			MemoryScopes:   []string{"session"},
			IsSystemAgent:  true,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}

	for _, a := range defaults {
		r.agents[a.ID] = a
		r.seeds[a.ID] = true
	}
}

// List returns all registered custom and system agents
func (r *CustomAgentRegistry) List() []CustomAgentManifest {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]CustomAgentManifest, 0, len(r.agents))
	for _, a := range r.agents {
		list = append(list, a)
	}
	return list
}

// Register adds or updates a user-defined custom agent manifest, validated
// and persisted. K3: previously anything went — empty names, negative
// budgets, IsSystemAgent forgery, seed-ID overwrite (then deletable).
// Manifest Capabilities/MCPTools stay descriptive: execution is gated
// separately by tool-registry role grants at dispatch, never by this
// manifest — so registration validates shape, not authority.
func (r *CustomAgentRegistry) Register(m CustomAgentManifest) (CustomAgentManifest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if strings.TrimSpace(m.Name) == "" {
		return CustomAgentManifest{}, fmt.Errorf("agent name is required")
	}
	if m.SpendBudgetUSD < 0 {
		return CustomAgentManifest{}, fmt.Errorf("spend budget cannot be negative")
	}
	if m.IsSystemAgent {
		return CustomAgentManifest{}, fmt.Errorf("IsSystemAgent is reserved for built-in seeds")
	}
	if m.ID == "" {
		m.ID = "agent_" + uuid.New().String()[:8]
	}
	if r.isSeedID(m.ID) {
		return CustomAgentManifest{}, fmt.Errorf("agent id %q is a reserved system seed", m.ID)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if m.CreatedAt == "" {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	m.IsSystemAgent = false

	r.agents[m.ID] = m
	if r.db != nil {
		raw, _ := json.Marshal(m)
		if _, err := r.db.Exec(
			`INSERT INTO custom_agents (id, manifest_json, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
			 ON CONFLICT(id) DO UPDATE SET manifest_json = excluded.manifest_json, updated_at = CURRENT_TIMESTAMP`,
			m.ID, string(raw)); err != nil {
			delete(r.agents, m.ID)
			return CustomAgentManifest{}, fmt.Errorf("failed to persist agent: %w", err)
		}
	}
	return m, nil
}

// Delete removes a user-defined custom agent
func (r *CustomAgentRegistry) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, exists := r.agents[id]
	if !exists {
		return fmt.Errorf("agent %s not found", id)
	}
	// Seed IDs are immutable even if a stored row disagrees (defense in
	// depth: registration can no longer plant such a row, but old ones
	// must still refuse).
	if agent.IsSystemAgent || r.isSeedID(id) {
		return fmt.Errorf("cannot delete system agent %s", id)
	}

	delete(r.agents, id)
	if r.db != nil {
		if _, err := r.db.Exec(`DELETE FROM custom_agents WHERE id = ?`, id); err != nil {
			return fmt.Errorf("failed to delete persisted agent: %w", err)
		}
	}
	return nil
}

// MultimodalItem represents an individual multimodal component
type MultimodalItem struct {
	Modality string                 `json:"modality"`
	Data     string                 `json:"data"`
	MimeType string                 `json:"mime_type"`
	Metadata map[string]interface{} `json:"metadata"`
}

// MultimodalPayload is the incoming request to the mesh router
type MultimodalPayload struct {
	Prompt            string           `json:"prompt"`
	SystemPrompt      string           `json:"system_prompt"`
	Items             []MultimodalItem `json:"items"`
	RoleHint          string           `json:"role_hint"`
	PreferredModel    string           `json:"preferred_model"`
	MaxTokens         int              `json:"max_tokens"`
	Temperature       float64          `json:"temperature"`
	SpendBudgetUSD    float64          `json:"spend_budget_usd"`
	HasEd25519Mandate bool             `json:"has_ed25519_mandate"`
}

// RoutingDecision is the synthesized router outcome
type RoutingDecision struct {
	Provider            string   `json:"provider"`
	Model               string   `json:"model"`
	IsLocal             bool     `json:"is_local"`
	AirGapEnforced      bool     `json:"air_gap_enforced"`
	EstimatedCostUSD    float64  `json:"estimated_cost_usd"`
	Explanation         string   `json:"explanation"`
	ModalitiesSupported []string `json:"modalities_supported"`
}

// RouteMultimodal evaluates task complexity, air-gap policy, and budget constraints
func RouteMultimodal(p MultimodalPayload, airGapped bool) RoutingDecision {
	has3D := false
	for _, it := range p.Items {
		if strings.Contains(it.Modality, "molecule") || strings.Contains(it.Modality, "3d") {
			has3D = true
			break
		}
	}

	// Sovereign air-gap: local only, and only this branch may claim
	// enforcement. (A previous revision stamped AirGapEnforced on every
	// local branch, including the plain default below.)
	if airGapped {
		return RoutingDecision{
			Provider:            "omlx",
			Model:               "local/qwen2.5-coder:32b",
			IsLocal:             true,
			AirGapEnforced:      true,
			EstimatedCostUSD:    0.0,
			Explanation:         "Sovereign air-gap enforced: routed to local on-device oMLX engine with zero external network egress.",
			ModalitiesSupported: []string{"text", "code", "ast", "molecule_3d"},
		}
	}

	// J2: cloud escalation is spend-gated, mirroring the Python router.
	// A complex prompt with no budget and no mandate routes local —
	// Previously this branch fired on keywords alone ("femoco") and
	// claimed "AP2 mandate backing" it never checked.
	wantsCloud := has3D || strings.Contains(strings.ToLower(p.Prompt), "femoco") || strings.Contains(strings.ToLower(p.Prompt), "kohn-sham")
	funded := p.HasEd25519Mandate || p.SpendBudgetUSD > 0.05
	if wantsCloud && funded {
		backing := "spend budget declared"
		if p.HasEd25519Mandate {
			backing = "AP2 mandate backing"
		}
		return RoutingDecision{
			Provider:            "anthropic",
			Model:               "claude-3-5-sonnet",
			IsLocal:             false,
			AirGapEnforced:      false,
			EstimatedCostUSD:    0.045,
			Explanation:         "Complex electronic structure multi-reference character detected. Routed to high-reasoning frontier model with " + backing + ".",
			ModalitiesSupported: []string{"text", "code", "vision", "molecule_3d"},
		}
	}

	// Local default: cheap, unblocked, and honestly labeled — not enforced.
	return RoutingDecision{
		Provider:            "omlx",
		Model:               "local/qwen2.5-coder:32b",
		IsLocal:             true,
		AirGapEnforced:      false,
		EstimatedCostUSD:    0.0,
		Explanation:         "Standard quantum compilation query. Routed to sovereign local engine.",
		ModalitiesSupported: []string{"text", "code"},
	}
}
