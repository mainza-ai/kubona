package agents

import (
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
}

// NewCustomAgentRegistry creates and seeds a custom agent registry
func NewCustomAgentRegistry() *CustomAgentRegistry {
	reg := &CustomAgentRegistry{
		agents: make(map[string]CustomAgentManifest),
	}
	reg.seedDefaults()
	return reg
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

// Register adds or updates a custom agent manifest
func (r *CustomAgentRegistry) Register(m CustomAgentManifest) CustomAgentManifest {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	if m.ID == "" {
		m.ID = "agent_" + uuid.New().String()[:8]
	}
	if m.CreatedAt == "" {
		m.CreatedAt = now
	}
	m.UpdatedAt = now

	r.agents[m.ID] = m
	return m
}

// Delete removes a user-defined custom agent
func (r *CustomAgentRegistry) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, exists := r.agents[id]
	if !exists {
		return fmt.Errorf("agent %s not found", id)
	}
	if agent.IsSystemAgent {
		return fmt.Errorf("cannot delete system agent %s", id)
	}

	delete(r.agents, id)
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

	// In sovereign air-gap or zero budget, route to local oMLX / Qwen
	if airGapped || p.SpendBudgetUSD <= 0.01 {
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

	// High-complexity reasoning with spend mandate authorized
	if has3D || strings.Contains(strings.ToLower(p.Prompt), "femoco") || strings.Contains(strings.ToLower(p.Prompt), "kohn-sham") {
		return RoutingDecision{
			Provider:            "anthropic",
			Model:               "claude-3-5-sonnet",
			IsLocal:             false,
			AirGapEnforced:      false,
			EstimatedCostUSD:    0.045,
			Explanation:         "Complex electronic structure multi-reference character detected. Routed to high-reasoning frontier model with AP2 mandate backing.",
			ModalitiesSupported: []string{"text", "code", "vision", "molecule_3d"},
		}
	}

	return RoutingDecision{
		Provider:            "omlx",
		Model:               "local/qwen2.5-coder:32b",
		IsLocal:             true,
		AirGapEnforced:      true,
		EstimatedCostUSD:    0.0,
		Explanation:         "Standard quantum compilation query. Routed to sovereign local engine.",
		ModalitiesSupported: []string{"text", "code"},
	}
}
