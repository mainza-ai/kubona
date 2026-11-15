package api

import "net/http"

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
