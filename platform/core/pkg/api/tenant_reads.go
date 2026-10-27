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
	if !singleLabDeclared() {
		return false
	}
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

// checkSessionTenant is the pure D-09 decision (no writes): whether this
// request may touch sess, and if not, the refusal to report. HTTP
// handlers report via authorizeSessionRead (status codes); SSE reports
// via error events (headers already sent) — one rule, two reporters.
func (s *Server) checkSessionTenant(r *http.Request, sess *session.Session) (allow bool, code int, msg string) {
	if hasNoCredentials(r) {
		if s.singleTenantStore() {
			return true, http.StatusOK, ""
		}
		return false, http.StatusUnauthorized, "multi-lab store: log in or present the operator token to touch sessions"
	}
	tenant, _, code := s.checkAuth(r)
	if code != http.StatusOK {
		switch code {
		case http.StatusForbidden:
			return false, code, "cross-site session use refused: retry same-origin"
		case http.StatusServiceUnavailable:
			return false, code, "operator token not configured: cannot authenticate"
		default:
			return false, code, "session required: log in or present the operator token"
		}
	}
	if tenant == "" {
		return true, http.StatusOK, "" // bearer operator: no tenant to scope to, full access
	}
	if sess != nil && tenant != sess.TenantID {
		return false, http.StatusForbidden, "session belongs to another lab: refused"
	}
	return true, http.StatusOK, ""
}

// authorizeSessionRead enforces D-09 on one resolved session row.
// Callers resolve the session first (ghosts keep their legacy behavior —
// 404 or empty list — via authorizeKnownSession); this function only
// compares caller vs session tenant.
func (s *Server) authorizeSessionRead(w http.ResponseWriter, r *http.Request, sess *session.Session) bool {
	allow, code, msg := s.checkSessionTenant(r, sess)
	if allow {
		return true
	}
	if code == http.StatusServiceUnavailable {
		http.Error(w, msg, code)
		return false
	}
	if code == http.StatusForbidden {
		WriteAPIError(w, "forbidden", msg, nil)
		return false
	}
	WriteAPIError(w, "unauthorized", msg, nil)
	return false
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
