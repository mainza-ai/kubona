package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// handleSkills serves the body-free skill catalogue and the closed loader
// faults. Reads are public and spend-free; skill bodies are never returned
// here (load_skill is the only body path, and it is tool-gated).
func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.skillStore.Public(CopilotRole))
}

// handleSkillByID toggles one skill. The body is {enabled: bool}; the store
// validates existence and the ledger persists the decision across restarts.
func (s *Server) handleSkillByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/skills/")
	if name == "" || strings.Contains(name, "/") {
		http.Error(w, "skill path is /api/skills/{name}", http.StatusBadRequest)
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Enabled == nil {
		http.Error(w, "body must be {\"enabled\": true|false}", http.StatusBadRequest)
		return
	}
	if err := s.skillStore.SetEnabled(name, *req.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := s.ledger.SetSkillEnabled(name, *req.Enabled); err != nil {
		// Keep the in-process catalogue consistent with durable state: if
		// the write fails, undo the optimistic store flip.
		_ = s.skillStore.SetEnabled(name, !*req.Enabled)
		http.Error(w, fmt.Sprintf("persist skill state: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name, "enabled": *req.Enabled})
}
