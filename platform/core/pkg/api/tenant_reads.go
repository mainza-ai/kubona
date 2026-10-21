package api

// T-09: caller-vs-session tenant binding for session-scoped reads
// (D-09: single-lab open mode).
//
// Threat: session IDs are guessable UUIDs across labs, and every
// session-keyed read resolved only the *session's* tenant — never the
// *caller's* — so any network-adjacent party could enumerate the list
// and replay IDs across 13 endpoints into another lab's transcripts,
// campaign state, claims, events, and ledger exports.
//
// Rule:
//   - cookie caller: sees only sessions of its own lab (403 otherwise);
//   - bearer caller: operator token, carries no tenant — full access;
//   - anonymous (no cookie, no bearer header): allowed only while the
//     store holds at most one distinct tenant; the moment a second lab
//     exists, anonymous reads 401 with a named reason. This preserves
//     the zero-login local flow (the primary topology) and closes the
//     hole exactly where it can occur.
// Fail-closed notes: DB errors refuse (never default-allow); a stale
// cookie falls through to bearer and 401s (no downgrade to anonymous).

import (
	"net/http"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/session"
)

// hasNoCredentials reports a truly anonymous request: no session cookie
// and no Authorization header. A stale cookie is NOT anonymous —
// checkAuth falls it through to bearer, which then 401s.
func hasNoCredentials(r *http.Request) bool {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		return false
	}
	return strings.TrimSpace(r.Header.Get("Authorization")) == ""
}

// singleTenantStore reports whether the sessions table holds at most one
// distinct tenant. Errors fail closed (false = treat as multi-tenant).
func (s *Server) singleTenantStore() bool {
	n, err := s.sessionMgr.CountDistinctTenants()
	if err != nil {
		return false
	}
	return n <= 1
}

// checkCallerAuth maps checkAuth onto the D-09 caller model, writing the
// refusal for anything but an authenticated caller. Returns the lab tenant
// ("" for the bearer operator) and whether to proceed. CSRF 403s keep
// their status — they are a distinct protection, never masked as 401.
func (s *Server) checkCallerAuth(w http.ResponseWriter, r *http.Request) (tenant string, ok bool) {
	tenant, _, code := s.checkAuth(r)
	if code != http.StatusOK {
		switch code {
		case http.StatusForbidden:
			WriteAPIError(w, "forbidden", "cross-site session use refused: retry same-origin", nil)
		case http.StatusServiceUnavailable:
			http.Error(w, "operator token not configured: cannot authenticate reads", http.StatusServiceUnavailable)
		default:
			WriteAPIError(w, "unauthorized", "session required: log in or present the operator token", nil)
		}
		return "", false
	}
	return tenant, true
}

// authorizeSessionRead enforces D-09 on one resolved session row.
// Callers resolve the session first (ghosts keep their legacy behavior —
// 404 or empty list — via authorizeKnownSession); this function only
// compares caller vs session tenant.
func (s *Server) authorizeSessionRead(w http.ResponseWriter, r *http.Request, sess *session.Session) bool {
	if hasNoCredentials(r) {
		if s.singleTenantStore() {
			return true
		}
		WriteAPIError(w, "unauthorized", "multi-lab store: log in or present the operator token to read sessions", nil)
		return false
	}
	tenant, ok := s.checkCallerAuth(w, r)
	if !ok {
		return false
	}
	if tenant == "" {
		return true // bearer operator: no tenant to scope to, full access
	}
	if sess != nil && tenant != sess.TenantID {
		WriteAPIError(w, "forbidden", "session belongs to another lab: refused", nil)
		return false
	}
	return true
}

// authorizeKnownSession resolves the session and, when it exists,
// enforces the caller-vs-session binding. Ghosts return (nil, true) so
// each endpoint's legacy ghost behavior (404 or empty list) continues
// untouched — the gate only narrows what was previously cross-tenant.
func (s *Server) authorizeKnownSession(w http.ResponseWriter, r *http.Request, sessionID string) (*session.Session, bool) {
	sess, err := s.sessionMgr.GetSession(sessionID)
	if err != nil {
		return nil, true
	}
	if !s.authorizeSessionRead(w, r, sess) {
		return nil, false
	}
	return sess, true
}
