package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/skills"
)

// handleSkills serves the body-free skill catalogue (GET) and installs one
// user skill (POST). Reads are public and spend-free; bodies are never
// returned here (load_skill is the only body path, and it is tool-gated).
func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.skillsStore().Public(CopilotRole))
	case http.MethodPost:
		var req struct {
			Name    string `json:"name"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "body must be {\"name\":\"...\",\"content\":\"SKILL.md\"}", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" || strings.Contains(name, "/") {
			http.Error(w, "skill name is required and cannot contain '/'", http.StatusBadRequest)
			return
		}
		userDir := skills.DefaultUserDir()
		if userDir == "" {
			http.Error(w, "user skill directory is not configured", http.StatusInternalServerError)
			return
		}
		if err := skills.Install(userDir, name, []byte(req.Content)); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.reloadSkills(); err != nil {
			http.Error(w, fmt.Sprintf("reload skills: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "name": name})
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSkillByID toggles (PUT) or removes (DELETE) one user skill. Toggle
// persists in the ledger; removal refuses embedded seed skills.
func (s *Server) handleSkillByID(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/skills/")
	if name == "" || strings.Contains(name, "/") {
		http.Error(w, "skill path is /api/skills/{name}", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var req struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Enabled == nil {
			http.Error(w, "body must be {\"enabled\": true|false}", http.StatusBadRequest)
			return
		}
		if _, ok := s.skillsStore().SourceOf(name); !ok {
			http.Error(w, "unknown skill", http.StatusNotFound)
			return
		}
		if err := s.ledger.SetSkillEnabled(name, *req.Enabled); err != nil {
			http.Error(w, fmt.Sprintf("persist skill state: %v", err), http.StatusInternalServerError)
			return
		}
		if err := s.reloadSkills(); err != nil {
			http.Error(w, fmt.Sprintf("reload skills: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name, "enabled": *req.Enabled})
	case http.MethodDelete:
		source, ok := s.skillsStore().SourceOf(name)
		if !ok {
			http.Error(w, "unknown skill", http.StatusNotFound)
			return
		}
		if source != skills.SourceUser {
			http.Error(w, "embedded seed skills cannot be removed", http.StatusBadRequest)
			return
		}
		if err := skills.Remove(skills.DefaultUserDir(), name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := s.reloadSkills(); err != nil {
			http.Error(w, fmt.Sprintf("reload skills: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name, "removed": true})
	default:
		w.Header().Set("Allow", "PUT, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
