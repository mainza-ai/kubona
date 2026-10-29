package tenants

// Cross-tenant read grants (Phase-2 production feature).
//
// Phase-1 made every cross-tenant request refuse fail-closed with no grant
// mechanism at all. This file implements the audited grant mechanism that
// follow-up specified: explicit, admin-signed, scoped, expiring, revocable.
//
// A grant authorizes ONE direction of access: the SOURCE tenant's admin
// grants the TARGET tenant (a lab slug) or principal ("sub:<id>") a set of
// explicit scopes. Grants are stored server-side (SQL row) and every
// lifecycle transition is sealed as a ledger event. No grant = no access:
// default deny is preserved everywhere.
//
// Lifecycle:
//   - Issue: the source tenant's admin signs the grant canonical payload
//     with Ed25519. The server verifies the signature against the source
//     tenant's CURRENT registered admin key (same rule as the Phase-0 A2A
//     binding) plus scope/expiry hygiene. Wildcard scopes refuse at issue.
//   - Verify (CheckGrant): signature + scope match + unexpired + unrevoked,
//     re-checked live on EVERY use. Nothing is cached past (or up to) TTL:
//     every check re-reads the row and the current admin key.
//   - Revoke: the source admin signs the revoke payload; the revoked flag
//     flips in the store and takes effect on the very next check.
//   - Expiry: enforced strictly (now >= expiry refuses).
//
// Canonical payloads (byte-identical construction in every language, so
// signatures verify cross-language):
//
//	grant:        "grant|<source>|<target>|<scopes csv, sorted>|<expiry UTC seconds>"
//	revoke:       "grant-revoke|<grant_id>|<reason>"
//
// Grant IDs are content-addressed: hex(sha256(canonical || raw_signature)).
// Consequence (fail-closed, deliberate): revocation is sticky. Re-presenting
// the identical grant blob after revocation does NOT revive it (the stored
// revoked flag wins over the replay — otherwise anyone holding the public
// grant blob could undo the owner's revocation). A fresh lifecycle needs
// fresh bytes, e.g. a new expiry.

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrGrantNotFound      = errors.New("grant not found")
	ErrGrantRevoked       = errors.New("grant revoked")
	ErrGrantExpired       = errors.New("grant expired")
	ErrGrantScopeMismatch = errors.New("grant does not cover the requested scope")
	ErrGrantSignature     = errors.New("grant signature verification failed")
	ErrWildcardScope      = errors.New("wildcard scope refused: grants carry an explicit allowlist, never *")
	ErrGrantTargetUnknown = errors.New("grant target tenant unknown")
)

// ReadScope is the scope covering tenant-record reads (the Go read paths
// this package enforces: GET /api/tenants/:id and the lab-scoped list).
const ReadScope = "read"

// GrantPrincipalPrefix marks a principal-bound grantee: "sub:<id>" targets
// one authenticated principal instead of a whole lab tenant.
const GrantPrincipalPrefix = "sub:"

// Grant is one audited cross-tenant authorization.
type Grant struct {
	GrantID          string    `json:"grant_id"`
	SourceTenant     string    `json:"source_tenant"`
	TargetTenant     string    `json:"target_tenant"`
	Scopes           []string  `json:"scopes"`
	Expiry           time.Time `json:"expiry"`
	GrantorPubkeyHex string    `json:"grantor_pubkey_hex"`
	SignatureHex     string    `json:"signature_hex"`
	Revoked          bool      `json:"revoked"`
}

// GrantCanonicalPayload returns the exact bytes the source admin signs.
// Scopes are sorted so independently-ordered but equal sets sign equally;
// expiry is truncated to whole UTC seconds.
func GrantCanonicalPayload(sourceTenant, targetTenant string, scopes []string, expiry time.Time) []byte {
	cp := append([]string(nil), scopes...)
	sort.Strings(cp)
	exp := expiry.UTC().Truncate(time.Second).Format("2006-01-02T15:04:05Z")
	return []byte(fmt.Sprintf("grant|%s|%s|%s|%s",
		sourceTenant, targetTenant, strings.Join(cp, ","), exp))
}

// RevokeCanonicalPayload returns the exact bytes the source admin signs to
// revoke a grant. Revocation is idempotent and replay-harmless (flipping an
// already-set flag), so no expiry is bound into it.
func RevokeCanonicalPayload(grantID, reason string) []byte {
	return []byte(fmt.Sprintf("grant-revoke|%s|%s", grantID, reason))
}

// GrantIDFor derives the content-addressed grant id from the canonical
// payload and the raw grantor signature.
func GrantIDFor(canonical []byte, sig []byte) string {
	sum := sha256.Sum256(append(append([]byte(nil), canonical...), sig...))
	return hex.EncodeToString(sum[:])
}

// validGrantScope enforces the explicit-allowlist rule: non-empty, charset
// [a-z0-9:._-], and never a wildcard. Any "*" anywhere refuses at issue.
func validGrantScope(s string) error {
	if s == "" {
		return fmt.Errorf("empty grant scope: refused")
	}
	if strings.Contains(s, "*") {
		return ErrWildcardScope
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == ':' || r == '.' || r == '_' || r == '-') {
			return fmt.Errorf("grant scope %q is off-vocabulary (want [a-z0-9:._-]): refused", s)
		}
	}
	return nil
}

// validGrantTarget accepts a lab tenant slug or a principal binding
// ("sub:<id>"). Both must be non-empty and free of envelope metacharacters.
func validGrantTarget(t string) error {
	if t == "" {
		return fmt.Errorf("empty grant target: refused")
	}
	if strings.ContainsAny(t, "|*") {
		return fmt.Errorf("grant target %q carries metacharacters: refused", t)
	}
	if strings.HasPrefix(t, GrantPrincipalPrefix) {
		sub := strings.TrimPrefix(t, GrantPrincipalPrefix)
		if sub == "" || strings.ContainsAny(sub, "| \t\n") {
			return fmt.Errorf("grant principal target %q is malformed: refused", t)
		}
		if len(t) > 128 {
			return fmt.Errorf("grant principal target too long: refused")
		}
		return nil
	}
	if !validTenantID(t) {
		return fmt.Errorf("grant target %q is not a valid tenant slug: refused", t)
	}
	return nil
}

// grantTargetMatches reports whether a grant naming target covers a caller
// identified by (granteeTenant, granteeSub). Tenant grants cover the whole
// lab; principal grants ("sub:<id>") cover exactly one principal.
func grantTargetMatches(target, granteeTenant, granteeSub string) bool {
	if target == granteeTenant {
		return true
	}
	if granteeSub != "" && target == GrantPrincipalPrefix+granteeSub {
		return true
	}
	return false
}

func (m *Manager) ensureGrantsTable() error {
	_, err := m.ledger.DB().Exec(`CREATE TABLE IF NOT EXISTS tenant_grants (
		grant_id TEXT PRIMARY KEY,
		source_tenant TEXT NOT NULL,
		target_tenant TEXT NOT NULL,
		scopes_csv TEXT NOT NULL,
		expiry DATETIME NOT NULL,
		grantor_pubkey_hex TEXT NOT NULL,
		signature_hex TEXT NOT NULL,
		revoked INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME
	)`)
	if err != nil {
		return fmt.Errorf("grant store unavailable: %w", err)
	}
	_, err = m.ledger.DB().Exec(`CREATE INDEX IF NOT EXISTS idx_tenant_grants_source_target ON tenant_grants(source_tenant, target_tenant)`)
	if err != nil {
		return fmt.Errorf("grant store unavailable: %w", err)
	}
	return nil
}

// verifyGrantSignature checks the grant payload signature against the source
// tenant's CURRENT admin key (rotation-safe: rotation invalidates
// outstanding grants fail-closed, same rule as the UCAN trust roots).
func (m *Manager) verifyGrantSignature(sourceTenant string, canonical []byte, grantorPubkeyHex, sigHex string) error {
	src, err := m.GetTenant(sourceTenant)
	if err != nil {
		return err
	}
	if src.Status != "active" {
		return ErrTenantSuspended
	}
	if src.AdminPubkeyHex == "" || !strings.EqualFold(src.AdminPubkeyHex, grantorPubkeyHex) {
		return ErrNotAdmin
	}
	pubBytes, err := hex.DecodeString(grantorPubkeyHex)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid grantor public key")
	}
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("invalid grant signature format")
	}
	if !ed25519.Verify(ed25519.PublicKey(pubBytes), canonical, sigBytes) {
		return ErrGrantSignature
	}
	return nil
}

// IssueGrant records a cross-tenant grant. Authorization is the grantor
// signature itself: it must verify against the source tenant's current admin
// key (tenant-admin-signed only — a self-signed foreign grantor refuses with
// ErrNotAdmin/ErrGrantSignature). Issue seals a grant_issued ledger event
// (kind "tenant_read" — the registered taxonomy type, shared with the UCAN
// spend-grant flow whose payloads carry root_cid/leaf_cid instead).
func (m *Manager) IssueGrant(sourceTenant, targetTenant string, scopes []string, expiry time.Time, grantorPubkeyHex, sigHex string) (*Grant, error) {
	if err := m.ensureGrantsTable(); err != nil {
		return nil, err
	}
	if !validTenantID(sourceTenant) {
		return nil, fmt.Errorf("invalid source tenant %q", sourceTenant)
	}
	if err := validGrantTarget(targetTenant); err != nil {
		return nil, err
	}
	if len(scopes) == 0 {
		return nil, fmt.Errorf("at least one scope is required: refused")
	}
	for _, s := range scopes {
		if err := validGrantScope(s); err != nil {
			return nil, err
		}
	}
	exp := expiry.UTC().Truncate(time.Second)
	if !time.Now().UTC().Before(exp) {
		return nil, fmt.Errorf("grant expiry must be in the future: refusing already-expired grant")
	}
	// The target lab must exist (no grants to ghosts — and no existence
	// oracle either way, since issue is admin-only on the source side).
	if !strings.HasPrefix(targetTenant, GrantPrincipalPrefix) {
		if _, err := m.GetTenant(targetTenant); err != nil {
			return nil, ErrGrantTargetUnknown
		}
	}
	canonical := GrantCanonicalPayload(sourceTenant, targetTenant, scopes, exp)
	if err := m.verifyGrantSignature(sourceTenant, canonical, grantorPubkeyHex, sigHex); err != nil {
		return nil, err
	}
	cp := append([]string(nil), scopes...)
	sort.Strings(cp)
	sigBytes, _ := hex.DecodeString(sigHex)
	g := &Grant{
		GrantID:          GrantIDFor(canonical, sigBytes),
		SourceTenant:     sourceTenant,
		TargetTenant:     targetTenant,
		Scopes:           cp,
		Expiry:           exp,
		GrantorPubkeyHex: strings.ToLower(strings.TrimSpace(grantorPubkeyHex)),
		SignatureHex:     strings.ToLower(strings.TrimSpace(sigHex)),
		Revoked:          false,
	}
	if _, err := m.ledger.DB().Exec(
		`INSERT OR IGNORE INTO tenant_grants (grant_id, source_tenant, target_tenant, scopes_csv, expiry, grantor_pubkey_hex, signature_hex, revoked, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?)`,
		g.GrantID, g.SourceTenant, g.TargetTenant, strings.Join(cp, ","),
		exp.UTC().Format("2006-01-02T15:04:05Z"),
		g.GrantorPubkeyHex, g.SignatureHex, time.Now().UTC(),
	); err != nil {
		return nil, fmt.Errorf("failed to store grant: %w", err)
	}
	m.ledger.AppendEvent(sourceTenant, "grant_issued", map[string]any{
		"kind": "tenant_read", "grant_id": g.GrantID,
		"source_tenant": g.SourceTenant,
		"target_tenant": g.TargetTenant, "scopes": cp,
		"expiry":     exp.UTC().Format(time.RFC3339),
		"grantor_fp": Fingerprint(grantorPubkeyHex),
	})
	return m.GetGrant(g.GrantID)
}

// RevokeGrant flips the revoked flag. Authorization: the caller presents a
// signature over the revoke payload by the grant's SOURCE tenant current
// admin key — only the lab that issued a grant can revoke it. Revocation is
// effective on the next check (checks re-read the row live; nothing cached).
func (m *Manager) RevokeGrant(sourceTenant, grantID, reason, pubkeyHex, sigHex string) error {
	if err := m.ensureGrantsTable(); err != nil {
		return err
	}
	g, err := m.GetGrant(grantID)
	if err != nil {
		return err
	}
	if g.SourceTenant != sourceTenant {
		return fmt.Errorf("grant %s belongs to %q, not %q: only the issuing lab revokes", grantID, g.SourceTenant, sourceTenant)
	}
	if err := m.verifyGrantSignature(sourceTenant, RevokeCanonicalPayload(grantID, reason), pubkeyHex, sigHex); err != nil {
		return err
	}
	if _, err := m.ledger.DB().Exec(`UPDATE tenant_grants SET revoked = 1 WHERE grant_id = ?`, grantID); err != nil {
		return fmt.Errorf("grant revocation failed: %w", err)
	}
	m.ledger.AppendEvent(sourceTenant, "grant_revoked", map[string]any{
		"kind": "tenant_read", "grant_id": grantID,
		"reason": reason, "revoked_by_fp": Fingerprint(pubkeyHex),
	})
	return nil
}

// GetGrant loads a grant by id (revoked ones included — revocation is
// visible, not hidden).
func (m *Manager) GetGrant(grantID string) (*Grant, error) {
	if err := m.ensureGrantsTable(); err != nil {
		return nil, err
	}
	row := m.ledger.DB().QueryRow(
		`SELECT grant_id, source_tenant, target_tenant, scopes_csv, expiry, grantor_pubkey_hex, signature_hex, revoked FROM tenant_grants WHERE grant_id = ?`, grantID)
	var g Grant
	var scopesCSV, expiryStr string
	var revoked int
	if err := row.Scan(&g.GrantID, &g.SourceTenant, &g.TargetTenant, &scopesCSV, &expiryStr, &g.GrantorPubkeyHex, &g.SignatureHex, &revoked); err != nil {
		return nil, ErrGrantNotFound
	}
	if scopesCSV != "" {
		g.Scopes = strings.Split(scopesCSV, ",")
	}
	for _, layout := range []string{"2006-01-02T15:04:05Z", time.RFC3339, "2006-01-02 15:04:05Z07:00", "2006-01-02 15:04:05+00:00"} {
		if exp, perr := time.Parse(layout, expiryStr); perr == nil {
			g.Expiry = exp.UTC()
			break
		}
	}
	g.Revoked = revoked != 0
	return &g, nil
}

// CheckGrant is the live enforcement decision: does a grant from
// sourceTenant authorize (granteeTenant, granteeSub) for scope right now?
// Every check re-reads the grant row, the expiry, the revocation flag, and
// re-verifies the signature against the source tenant's CURRENT admin key —
// never cached, so issue/revoke/rotation/expiry take effect immediately.
// Unknown grants, revoked, expired, out-of-scope, and forged grants all
// refuse (default deny); nil means allow.
func (m *Manager) CheckGrant(sourceTenant, granteeTenant, granteeSub, scope string) error {
	if err := m.ensureGrantsTable(); err != nil {
		return err
	}
	rows, err := m.ledger.DB().Query(
		`SELECT grant_id, target_tenant, scopes_csv, expiry, grantor_pubkey_hex, signature_hex FROM tenant_grants WHERE source_tenant = ? AND revoked = 0`, sourceTenant)
	if err != nil {
		return fmt.Errorf("grant check unavailable: %w", err)
	}
	defer rows.Close()
	type candidate struct {
		grantID, target, scopesCSV, expiryStr, pubkey, sig string
	}
	var cands []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.grantID, &c.target, &c.scopesCSV, &c.expiryStr, &c.pubkey, &c.sig); err != nil {
			continue
		}
		cands = append(cands, c)
	}
	now := time.Now().UTC()
	for _, c := range cands {
		if !grantTargetMatches(c.target, granteeTenant, granteeSub) {
			continue
		}
		covered := false
		for _, s := range strings.Split(c.scopesCSV, ",") {
			if s == scope {
				covered = true
				break
			}
		}
		if !covered {
			continue
		}
		var exp time.Time
		for _, layout := range []string{"2006-01-02T15:04:05Z", time.RFC3339, "2006-01-02 15:04:05Z07:00", "2006-01-02 15:04:05+00:00"} {
			if parsed, perr := time.Parse(layout, c.expiryStr); perr == nil {
				exp = parsed.UTC()
				break
			}
		}
		if exp.IsZero() || !now.Before(exp) {
			continue
		}
		canonical := GrantCanonicalPayload(sourceTenant, c.target, strings.Split(c.scopesCSV, ","), exp)
		if err := m.verifyGrantSignature(sourceTenant, canonical, c.pubkey, c.sig); err != nil {
			continue
		}
		return nil
	}
	return fmt.Errorf("no live grant from %q covers %q for scope %q: refusing", sourceTenant, granteeTenant, scope)
}
