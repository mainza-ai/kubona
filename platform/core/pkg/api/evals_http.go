package api

import (
	"net/http"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/evals"
)

// handleEvalLLM serves the recorded decision-eval status for one model. It
// never runs a model and never invents a score: an unknown role or missing
// identity is refused, and an absent/stale report reads as unevaluated/stale.
func (s *Server) handleEvalLLM(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	model := strings.TrimSpace(r.URL.Query().Get("model"))
	if role == "" || provider == "" || model == "" {
		WriteAPIError(w, "bad_request", "role, provider and model are required", nil)
		return
	}
	set, err := evals.Load(role)
	if err != nil {
		WriteAPIError(w, "not_found", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, s.evalStore.Status(set, role, provider, model))
}
