package api

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	pb "github.com/mainza-ai/kubona/core/proto"
)

func resolveOpenAPIPath() string {
	candidates := []string{}
	if dir := strings.TrimSpace(os.Getenv("KUBONA_CONTRACTS_DIR")); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "console-openapi.json"))
	}
	candidates = append(candidates,
		"/app/contracts/console-openapi.json",
		"contracts/console-openapi.json",
		"../contracts/console-openapi.json",
		"../../contracts/console-openapi.json",
		"../../../contracts/console-openapi.json",
		"platform/contracts/console-openapi.json",
	)
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// handleOpenAPI serves the generated OpenAPI 3.1 contract so external HTTP
// agents can discover Kubona's modeled API surface without cloning the repo.
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p := resolveOpenAPIPath()
	if p == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ok":    false,
			"code":  "server_misconfigured",
			"error": "console-openapi.json contract not found on host",
		})
		return
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ok":    false,
			"code":  "server_misconfigured",
			"error": "console-openapi.json unreadable",
		})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(raw)
	}
}

// handleWellKnownAgent serves the A2A Agent Card at /.well-known/agent.json.
func (s *Server) handleWellKnownAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mcpURL := strings.TrimSpace(os.Getenv("KUBONA_MCP_PUBLIC_URL"))
	if mcpURL == "" {
		mcpURL = "http://127.0.0.1:8123/mcp"
	}
	card := map[string]any{
		"name":        "Kubona",
		"description": "Local-first quantum computing and scientific discovery platform: classical anchors, cost-governed execution, cryptographic provenance, and agentic research.",
		"version":     "2026-11-20",
		"protocols": map[string]any{
			"a2a": map[string]any{
				"endpoint":  "/api/a2a/v1",
				"challenge": "/api/a2a/v1/challenge",
				"auth":      []string{"Bearer", "X-Kubona-A2A-Token", "x402"},
			},
			"mcp": map[string]any{
				"streamable_http": mcpURL,
				"profiles":        []string{"all", "read-only", "science", "governance", "ops", "world"},
				"resources": []string{
					"kubona://skills",
					"kubona://contracts/error-catalog",
					"kubona://contracts/mcp-catalog",
					"kubona://contracts/experiment-kinds",
					"kubona://contracts/rate-cards",
				},
			},
			"http": map[string]any{
				"openapi": "/api/openapi.json",
				"events":  "/api/events/stream",
				"llms":    "/llms.txt",
			},
		},
		"invariants": []string{
			"No quantum energy or advantage claim without a classical anchor.",
			"No paid execution without a session budget ceiling and human/mandate approval.",
			"Every result and decision is recorded in a hash-chained provenance ledger.",
		},
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, card)
}

// handleLLMsTxt serves a concise plain-text entrypoint for AI agents.
func (s *Server) handleLLMsTxt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mcpURL := strings.TrimSpace(os.Getenv("KUBONA_MCP_PUBLIC_URL"))
	if mcpURL == "" {
		mcpURL = "http://127.0.0.1:8123/mcp"
	}
	body := strings.Join([]string{
		"# Kubona",
		"",
		"> Local-first quantum computing and scientific discovery platform.",
		"> Three invariants: (1) classical anchor before quantum claim, (2) budget ceiling + approval before paid execution, (3) hash-chained cryptographic receipt for every result.",
		"",
		"## Machine-Readable Discovery",
		"- OpenAPI 3.1: /api/openapi.json",
		"- A2A Agent Card: /.well-known/agent.json",
		"- A2A Endpoint: POST /api/a2a/v1 (challenge: GET /api/a2a/v1/challenge)",
		"- MCP Streamable HTTP: " + mcpURL + " (supports ?profile=read-only|science|governance|ops|world|all; Bearer auth required)",
		"- MCP Resources: kubona://skills, kubona://contracts/error-catalog, kubona://contracts/mcp-catalog",
		"- Skills Catalog (internal copilot): GET /api/skills",
		"- SSE Event Stream: GET /api/events/stream (supports Last-Event-ID replay)",
		"",
		"## External Agent Skills (Repository)",
		"- skills/kubona/SKILL.md (router)",
		"- skills/kubona-connect/SKILL.md",
		"- skills/kubona-experiments/SKILL.md",
		"- skills/kubona-governance/SKILL.md",
		"- skills/kubona-research/SKILL.md",
		"- skills/kubona-ops/SKILL.md",
		"",
	}, "\n")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write([]byte(body))
	}
}

// handleChemistryAnchor computes a real PySCF classical reference anchor
// (RHF/FCI) via the Python compute bridge. Fails closed when the bridge is
// unavailable or the input is invalid (never invents energies).
func (s *Server) handleChemistryAnchor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Molecule             string  `json:"molecule"`
		BondDistanceAngstrom float64 `json:"bond_distance_angstrom"`
		BondDistance         float64 `json:"bond_distance"`
		Basis                string  `json:"basis"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"code":  "bad_request",
			"error": "invalid JSON body",
		})
		return
	}
	mol := strings.TrimSpace(req.Molecule)
	if mol == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"code":  "bad_request",
			"error": "molecule is required",
		})
		return
	}
	dist := req.BondDistanceAngstrom
	if dist == 0 {
		dist = req.BondDistance
	}
	if dist == 0 {
		dist = defaultBondDistance(mol)
	}
	if math.IsNaN(dist) || math.IsInf(dist, 0) || dist <= 0 || dist > 20.0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"code":  "bad_request",
			"error": "bond_distance_angstrom must be finite and in (0, 20]",
		})
		return
	}
	bridge := s.bridgeClient()
	if bridge == nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"ok":    false,
			"code":  "bridge_unavailable",
			"error": "bridge unavailable: no classical anchor computation performed",
		})
		return
	}
	cctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	resp, err := bridge.ComputeClassicalAnchor(cctx, &pb.ClassicalAnchorRequest{
		Molecule:     strings.ToUpper(mol),
		BondDistance: dist,
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"ok":    false,
			"code":  "bridge_unavailable",
			"error": "classical anchor bridge call failed: " + err.Error(),
		})
		return
	}
	if !resp.Success {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"code":  "bad_request",
			"error": resp.ErrorMessage,
		})
		return
	}
	basis := strings.TrimSpace(req.Basis)
	if basis == "" {
		basis = "sto-3g"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                     true,
		"molecule":               strings.ToUpper(mol),
		"bond_distance_angstrom": dist,
		"basis":                  basis,
		"rhf_hartree":            resp.RhfEnergy,
		"fci_hartree":            resp.FciExactEnergy,
		"anchor_computed":        true,
	})
}
