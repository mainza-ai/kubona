package mcp

import (
	"fmt"
	"sync"
)

// RiskLevel defines the paperclip catalog risk classification
type RiskLevel string

const (
	RiskRead        RiskLevel = "read"
	RiskWrite       RiskLevel = "write"
	RiskDestructive RiskLevel = "destructive"
)

type CatalogEntry struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Risk        RiskLevel `json:"risk"`
	Enabled     bool      `json:"enabled"`
}

type Profile struct {
	Name         string   `json:"name"`
	AllowedTools []string `json:"allowed_tools"`
	DeniedTools  []string `json:"denied_tools"`
}

// Gateway implements Paperclip's 4-plane access governance:
// Application -> Connection -> Catalog -> Profile
type Gateway struct {
	mu       sync.RWMutex
	catalog  map[string]CatalogEntry
	profiles map[string]Profile
}

func NewGateway() *Gateway {
	gw := &Gateway{
		catalog:  make(map[string]CatalogEntry),
		profiles: make(map[string]Profile),
	}
	gw.bootstrapCatalog()
	return gw
}

func (g *Gateway) bootstrapCatalog() {
	tools := []CatalogEntry{
		{Name: "submit_job", Description: "Enqueue experiment job for QPU/simulator", Risk: RiskWrite, Enabled: true},
		{Name: "get_job_status", Description: "Query status and result of a submitted job", Risk: RiskRead, Enabled: true},
		{Name: "register_loop", Description: "Register a new autonomous research loop", Risk: RiskWrite, Enabled: true},
		{Name: "step_loop", Description: "Advance an autonomous loop step with gradient telemetry", Risk: RiskWrite, Enabled: true},
		{Name: "pause_loop", Description: "Halt an autonomous loop temporarily", Risk: RiskWrite, Enabled: true},
		{Name: "kill_loop", Description: "Terminate a runaway loop immediately", Risk: RiskDestructive, Enabled: true},
	}
	for _, t := range tools {
		g.catalog[t.Name] = t
	}

	// Standard Profile
	g.profiles["standard"] = Profile{
		Name:         "standard",
		AllowedTools: []string{"submit_job", "get_job_status", "register_loop", "step_loop", "pause_loop", "kill_loop"},
	}

	// Low Trust Preset (paperclip doc/LOW-TRUST-PRESETS.md)
	g.profiles["low_trust_review"] = Profile{
		Name:         "low_trust_review",
		AllowedTools: []string{"get_job_status"},
		DeniedTools:  []string{"submit_job", "register_loop", "kill_loop"},
	}
}

func (g *Gateway) CanCallTool(profileName, toolName string) (bool, string) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	entry, exists := g.catalog[toolName]
	if !exists {
		return false, fmt.Sprintf("tool '%s' not registered in catalog", toolName)
	}
	if !entry.Enabled {
		return false, fmt.Sprintf("tool '%s' is disabled", toolName)
	}

	prof, hasProf := g.profiles[profileName]
	if !hasProf {
		return false, fmt.Sprintf("profile '%s' not found", profileName)
	}

	// Deny beats allow invariant
	for _, d := range prof.DeniedTools {
		if d == toolName {
			return false, fmt.Sprintf("tool '%s' explicitly denied by profile '%s'", toolName, profileName)
		}
	}

	for _, a := range prof.AllowedTools {
		if a == toolName {
			return true, "allowed"
		}
	}

	return false, fmt.Sprintf("tool '%s' not in allowlist of profile '%s'", toolName, profileName)
}

func (g *Gateway) ListTools(profileName string) []CatalogEntry {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var visible []CatalogEntry
	for name, entry := range g.catalog {
		if allowed, _ := g.CanCallTool(profileName, name); allowed {
			visible = append(visible, entry)
		}
	}
	return visible
}
