package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/tenants"
)

// Session-based operator auth (A1): challenge-response login bound to a
// lab's registered Ed25519 admin key, short-lived httpOnly cookie session
// after that. This ADDS a path — the shared KUBONA_API_TOKEN bearer keeps
// working for machines and CI (no flag day). No IdP, no OAuth: identity
// is the lab admin key the tenant row already trusts (Q-13), and the
// console signs challenges with the WebCrypto keyring it already holds.
//
// Threat posture: a stolen session cookie is bounded by TTL + explicit
// revocation (vs the never-rotating shared bearer); httpOnly keeps it out
// of JS; SameSite=Lax plus an Origin allowlist check on cookie-authed
// writes blunts CSRF. Sessions never cross tenants: login names the lab,
// the key must match that lab's registered admin.

const (
	sessionCookieName  = "kubona_session"
	sessionTokenBytes  = 32
	challengeBytes     = 32
	challengeTTL       = 60 * time.Second
	defaultSessionTTL  = 12 * time.Hour
	loginMessagePrefix = "kubona-auth/v1 login "
)

// adminFingerprint is the audit identity for a lab admin key.
func adminFingerprint(pubkeyHex string) string {
	return tenants.Fingerprint(pubkeyHex)
}

// cookieSecure decides the Secure flag on the session cookie. Loopback
// HTTP stays working for local dev (r.TLS == nil → false); anything
// served over TLS gets Secure cookies. KUBONA_COOKIE_SECURE overrides
// both ways ("1" forces on — for TLS-terminating proxies where r.TLS is
// nil by the time we see the request; "0" forces off — local HTTP behind
// a proxy that sets X-Forwarded-Proto). Without Secure, a future HTTPS
// deployment would also send the cookie over accidental plain HTTP.
func cookieSecure(r *http.Request) bool {
	switch strings.TrimSpace(os.Getenv("KUBONA_COOKIE_SECURE")) {
	case "1":
		return true
	case "0":
		return false
	default:
		return r != nil && r.TLS != nil
	}
}

// AuthSessionStore holds single-use login challenges (memory-only: a
// restart invalidates outstanding challenges, which is safe — the client
// just fetches a new one) and sessions (SQLite: restarts keep operators
// logged in, revocation is durable).
type AuthSessionStore struct {
	mu         sync.Mutex
	challenges map[string]time.Time // nonce hex -> expiry
	db         *sql.DB
	sessionTTL time.Duration
}

func newAuthSessionStore(db *sql.DB) *AuthSessionStore {
	ttl := defaultSessionTTL
	if raw := getenvDuration("KUBONA_SESSION_TTL"); raw > 0 {
		ttl = raw
	}
	return &AuthSessionStore{
		challenges: make(map[string]time.Time),
		db:         db,
		sessionTTL: ttl,
	}
}

func initAuthSessionTable(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS auth_sessions (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			pubkey_fp TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP NOT NULL,
			revoked INTEGER NOT NULL DEFAULT 0
		)`)
	return err
}

// issueChallenge mints a single-use login nonce.
func (st *AuthSessionStore) issueChallenge() (nonce string, expires time.Time) {
	var b [challengeBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Crypto failure fail-closed: no challenge, no login attempts.
		return "", time.Time{}
	}
	nonce = hex.EncodeToString(b[:])
	expires = time.Now().UTC().Add(challengeTTL)
	st.mu.Lock()
	st.challenges[nonce] = expires
	st.mu.Unlock()
	return nonce, expires
}

// consumeChallenge validates and burns a nonce (single-use even on
// failure: a wrong guess never gets a second try with the same nonce).
func (st *AuthSessionStore) consumeChallenge(nonce string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	exp, ok := st.challenges[nonce]
	delete(st.challenges, nonce)
	return ok && time.Now().UTC().Before(exp)
}

// createSession mints a session token for a verified tenant+key.
// Returns the bare token (set as httpOnly cookie) — only its SHA-256 is stored.
func (st *AuthSessionStore) createSession(tenantID, fp string) (token string, expires time.Time, err error) {
	var b [sessionTokenBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", time.Time{}, fmt.Errorf("cannot mint session: %w", err)
	}
	token = hex.EncodeToString(b[:])
	sum := sha256.Sum256([]byte(token))
	expires = time.Now().UTC().Add(st.sessionTTL)
	id := hex.EncodeToString(sum[:])[:16]
	_, err = st.db.Exec(
		`INSERT INTO auth_sessions (id, tenant_id, pubkey_fp, token_hash, expires_at, revoked)
		 VALUES (?, ?, ?, ?, ?, 0)`,
		id, tenantID, fp, hex.EncodeToString(sum[:]), expires.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("cannot persist session: %w", err)
	}
	return token, expires, nil
}

// validateToken checks a presented token: known, unrevoked, unexpired.
// Returns tenant + fingerprint for audit attribution.
func (st *AuthSessionStore) validateToken(token string) (tenantID, fp string, ok bool) {
	if token == "" || st.db == nil {
		return "", "", false
	}
	sum := sha256.Sum256([]byte(token))
	var tid, f string
	var exp string
	err := st.db.QueryRow(
		`SELECT tenant_id, pubkey_fp, expires_at FROM auth_sessions WHERE token_hash = ? AND revoked = 0`,
		hex.EncodeToString(sum[:])).Scan(&tid, &f, &exp)
	if err != nil {
		return "", "", false
	}
	// SQLite timestamp formats vary; parse leniently, fail closed.
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05Z07:00"} {
		if t, perr := time.Parse(layout, exp); perr == nil {
			if time.Now().UTC().After(t) {
				return "", "", false
			}
			return tid, f, true
		}
	}
	return "", "", false
}

// revokeToken marks a session revoked. Unknown tokens still succeed —
// logout must never leak whether a token existed.
func (st *AuthSessionStore) revokeToken(token string) {
	if token == "" || st.db == nil {
		return
	}
	sum := sha256.Sum256([]byte(token))
	_, _ = st.db.Exec(`UPDATE auth_sessions SET revoked = 1 WHERE token_hash = ?`, hex.EncodeToString(sum[:]))
}

// listSessions returns a tenant's live sessions (for rotation review).
// Expired rows are pruned on read.
func (st *AuthSessionStore) listSessions(tenantID string) []map[string]any {
	if st.db == nil {
		return nil
	}
	_, _ = st.db.Exec(`DELETE FROM auth_sessions WHERE expires_at < datetime('now')`)
	rows, err := st.db.Query(
		`SELECT id, pubkey_fp, created_at, expires_at FROM auth_sessions WHERE tenant_id = ? AND revoked = 0 ORDER BY created_at DESC`,
		tenantID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, fp, created, expires string
		if err := rows.Scan(&id, &fp, &created, &expires); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"id": id, "pubkey_fp": fp, "created_at": created, "expires_at": expires,
		})
	}
	return out
}

// revokeSession revokes one session id iff it belongs to the tenant.
func (st *AuthSessionStore) revokeSession(tenantID, id string) bool {
	if st.db == nil || tenantID == "" || id == "" {
		return false
	}
	res, err := st.db.Exec(
		`UPDATE auth_sessions SET revoked = 1 WHERE id = ? AND tenant_id = ? AND revoked = 0`, id, tenantID)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n == 1
}

// getenvDuration parses KUBONA_SESSION_TTL-style durations ("12h", "30m").
// Returns <=0 when unset or unparseable (caller falls back to default).
func getenvDuration(key string) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// loginChallengeMessage is the exact bytes the operator signs. Domain
// separation prefix + nonce: signatures are useless anywhere else.
func loginChallengeMessage(nonce string) string {
	return loginMessagePrefix + nonce
}

// verifyLoginSignature checks an Ed25519 signature over the exact login
// challenge message. Shape errors name the field; content errors never
// distinguish malformed from mismatched (both are just "bad signature").
func verifyLoginSignature(pubkeyHex, nonce, sigHex string) error {
	pubBytes, err := hex.DecodeString(strings.TrimSpace(pubkeyHex))
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("bad signature")
	}
	sigBytes, err := hex.DecodeString(strings.TrimSpace(sigHex))
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("bad signature")
	}
	if !ed25519.Verify(ed25519.PublicKey(pubBytes), []byte(loginChallengeMessage(nonce)), sigBytes) {
		return fmt.Errorf("bad signature")
	}
	return nil
}

// sessionPrincipal validates the session cookie WITHOUT CSRF enforcement
// (safe-method reads + the auth endpoints themselves use this; mutating
// handlers go through checkAuth, which adds the Origin check).
func (s *Server) sessionPrincipal(r *http.Request) (tenantID, fp string, ok bool) {
	if s == nil || s.authSessions == nil {
		return "", "", false
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return "", "", false
	}
	return s.authSessions.validateToken(c.Value)
}

// csrfAllowed reports whether a cookie-authed unsafe request may proceed:
// browsers always send Origin on fetch POSTs, so a present-but-unallowlisted
// Origin is a cross-site request and refuses. Absent Origin (curl, machine
// clients, same-document navigations that omit it) passes — the cookie is
// meaningless without a browser to attach it.
func csrfAllowed(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	return corsAllowOrigin(r) != ""
}

// checkAuth is the single auth decision point: session cookie first (with
// CSRF enforcement on unsafe methods), then the shared bearer (unchanged
// semantics for machines and CI). Returns the tenant + key fingerprint for
// audit attribution; bearer callers have neither.
func (s *Server) checkAuth(r *http.Request) (tenantID, fp string, code int) {
	if s != nil && s.authSessions != nil {
		if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
			if t, f, ok := s.authSessions.validateToken(c.Value); ok {
				if r.Method != http.MethodGet && r.Method != http.MethodHead && !csrfAllowed(r) {
					return "", "", http.StatusForbidden
				}
				return t, f, http.StatusOK
			}
			// Invalid/expired cookie is not an error by itself — fall
			// through to bearer so one stale cookie never locks out a
			// valid operator token.
		}
	}
	want := strings.TrimSpace(os.Getenv("KUBONA_API_TOKEN"))
	if want == "" {
		return "", "", http.StatusServiceUnavailable
	}
	const prefix = "Bearer "
	got := r.Header.Get("Authorization")
	if !strings.HasPrefix(got, prefix) ||
		subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(got, prefix)), []byte(want)) != 1 {
		return "", "", http.StatusUnauthorized
	}
	return "", "", http.StatusOK
}

// auditAuth seals auth lifecycle events (login, logout, refusals,
// revocations) in the ledger with tenant + key fingerprint. Refusals are
// first-class audit records: a guessing campaign against /api/auth/login
// leaves a trail.
func (s *Server) auditAuth(event, tenantID, fp, detail string) {
	if s == nil || s.ledger == nil {
		return
	}
	scope := tenantID
	if scope == "" {
		scope = "global"
	}
	_ = s.ledger.AppendEvent(scope, event, map[string]any{
		"tenant_id": tenantID, "pubkey_fp": fp, "detail": detail,
	})
}

// handleAuthChallenge issues a single-use login nonce (public, rate-limited).
// No identity is established here — the nonce only binds the next login attempt.
func (s *Server) handleAuthChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "too many auth attempts: slow down", nil)
		return
	}
	nonce, exp := s.authSessions.issueChallenge()
	if nonce == "" {
		WriteAPIError(w, "internal", "cannot mint challenge: refusing login", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"nonce":      nonce,
		"expires_at": exp.UTC().Format(time.RFC3339),
		"message":    loginChallengeMessage(nonce),
	})
}

// handleAuthLogin verifies an Ed25519 signature over the challenge message
// against the named lab's registered admin key. Suspended labs may still
// log in (recovery requires access). Success mints a session cookie;
// every failure names the fix without distinguishing which half was wrong
// beyond what the tenant list already reveals (tenants are public).
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authLimiter.allow(clientKey(r)) {
		WriteAPIError(w, "rate_limited", "too many auth attempts: slow down", nil)
		return
	}
	var req struct {
		TenantID  string `json:"tenant_id"`
		PubkeyHex string `json:"pubkey_hex"`
		Nonce     string `json:"nonce"`
		Signature string `json:"signature_hex"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteAPIError(w, "bad_request", "login requires {tenant_id, pubkey_hex, nonce, signature_hex}", nil)
		return
	}
	if !s.authSessions.consumeChallenge(strings.TrimSpace(req.Nonce)) {
		s.auditAuth("auth_login_refused", req.TenantID, "", "stale or unknown challenge: fetch a fresh one")
		WriteAPIError(w, "unauthorized", "stale or unknown challenge: fetch a fresh /api/auth/challenge first", nil)
		return
	}
	t, err := s.tenantMgr.GetTenant(strings.TrimSpace(req.TenantID))
	if err != nil || t == nil {
		s.auditAuth("auth_login_refused", req.TenantID, "", "unknown tenant")
		WriteAPIError(w, "not_found", "unknown tenant: check the lab id", nil)
		return
	}
	if t.AdminPubkeyHex == "" || !strings.EqualFold(t.AdminPubkeyHex, strings.TrimSpace(req.PubkeyHex)) {
		s.auditAuth("auth_login_refused", t.ID, "", "key is not this lab's admin")
		WriteAPIError(w, "unauthorized", "key is not this lab's admin: register it first or use the lab admin key", nil)
		return
	}
	if err := verifyLoginSignature(strings.TrimSpace(req.PubkeyHex), strings.TrimSpace(req.Nonce), strings.TrimSpace(req.Signature)); err != nil {
		s.auditAuth("auth_login_refused", t.ID, adminFingerprint(req.PubkeyHex), "bad signature: "+err.Error())
		WriteAPIError(w, "unauthorized", "bad signature: sign the challenge message exactly", nil)
		return
	}
	fp := adminFingerprint(req.PubkeyHex)
	token, exp, err := s.authSessions.createSession(t.ID, fp)
	if err != nil {
		WriteAPIError(w, "internal", "cannot establish session: refusing login", nil)
		return
	}
	maxAge := int(time.Until(exp).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
	s.auditAuth("auth_login", t.ID, fp, "session established")
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "tenant_id": t.ID, "pubkey_fp": fp,
		"expires_at": exp.UTC().Format(time.RFC3339),
	})
}

// handleAuthLogout revokes the presenting session (or nothing) and clears
// the cookie. Always 200: logout must never leak whether a token existed.
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !csrfAllowed(r) {
		WriteAPIError(w, "forbidden", "cross-site session use refused: retry same-origin", nil)
		return
	}
	if c, err := r.Cookie(sessionCookieName); err == nil {
		tenant, fp, ok := s.authSessions.validateToken(c.Value)
		if ok {
			s.auditAuth("auth_logout", tenant, fp, "session revoked")
		}
		s.authSessions.revokeToken(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleAuthWhoami reports the caller's auth state (public: the console
// header needs it before any login). Never leaks validity of arbitrary tokens.
func (s *Server) handleAuthWhoami(w http.ResponseWriter, r *http.Request) {
	if tenant, fp, ok := s.sessionPrincipal(r); ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated": true, "tenant_id": tenant, "pubkey_fp": fp,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
}

// handleAuthSessions lists the caller's live sessions (session auth only:
// bearer tokens carry no tenant, so there is nothing to scope to).
func (s *Server) handleAuthSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tenant, _, ok := s.sessionPrincipal(r)
	if !ok {
		WriteAPIError(w, "unauthorized", "session required: log in first (bearer tokens carry no tenant to scope to)", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": s.authSessions.listSessions(tenant)})
}

// handleAuthRevoke revokes one of the caller's own sessions by id.
func (s *Server) handleAuthRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !csrfAllowed(r) {
		WriteAPIError(w, "forbidden", "cross-site session use refused: retry same-origin", nil)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.ID) == "" {
		WriteAPIError(w, "bad_request", "revoke requires {id}", nil)
		return
	}
	tenant, fp, ok := s.sessionPrincipal(r)
	if !ok {
		WriteAPIError(w, "unauthorized", "session required: log in first", nil)
		return
	}
	if !s.authSessions.revokeSession(tenant, strings.TrimSpace(req.ID)) {
		WriteAPIError(w, "not_found", "no such live session in your lab", nil)
		return
	}
	s.auditAuth("auth_session_revoked", tenant, fp, "session "+strings.TrimSpace(req.ID))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
