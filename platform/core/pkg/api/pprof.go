package api

import (
	"net/http"
	"net/http/pprof"
)

// registerPprof mounts the standard library profiler behind an explicit
// opt-in (KUBONA_PPROF=1) and the operator bearer check. It is deliberately
// not part of the public route registry: the profiler is an operator
// diagnostic, not an API surface, and must never become anonymously
// reachable in zero-login local mode.
func (s *Server) registerPprof() {
	s.mux.HandleFunc("/debug/pprof/", s.pprofOperatorOnly(pprof.Index))
	s.mux.HandleFunc("/debug/pprof/cmdline", s.pprofOperatorOnly(pprof.Cmdline))
	s.mux.HandleFunc("/debug/pprof/profile", s.pprofOperatorOnly(pprof.Profile))
	s.mux.HandleFunc("/debug/pprof/symbol", s.pprofOperatorOnly(pprof.Symbol))
	s.mux.HandleFunc("/debug/pprof/trace", s.pprofOperatorOnly(pprof.Trace))
}

// pprofOperatorOnly requires the operator bearer. A lab session cookie is a
// valid identity for API reads but is not sufficient here: heap profiles can
// contain other tenants' paths and payload fragments.
func (s *Server) pprofOperatorOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, _, code := s.checkAuth(r)
		if code != http.StatusOK || tenant != "" {
			WriteAPIError(w, "unauthorized", "pprof requires the operator bearer token", nil)
			return
		}
		next(w, r)
	}
}
