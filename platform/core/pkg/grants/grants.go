package grants

import (
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	did "github.com/ucan-wg/go-did-it"
	didkeyctl "github.com/ucan-wg/go-did-it/controller/did-key"
	_ "github.com/ucan-wg/go-did-it/crypto/all"
	dided25519 "github.com/ucan-wg/go-did-it/crypto/ed25519"
	"github.com/ucan-wg/go-ucan/pkg/command"
	"github.com/ucan-wg/go-ucan/pkg/policy"
	"github.com/ucan-wg/go-ucan/pkg/policy/literal"
	"github.com/ucan-wg/go-ucan/token/delegation"
)

// Cross-lab capability grants as UCAN 1.0 delegation chains (DAG-CBOR sealed
// envelopes). A lab admin mints a root grant to another lab (or agent DID);
// holders attenuate offline (narrower command, lower budget, shorter expiry).
// The orchestrator verifies chains at dispatch: signatures (per envelope),
// linkage, expiry, command narrowing, meta narrowing, trusted root, and
// ledger revocation. Budgets stay orchestrator-metered: the grant carries a
// ceiling, spending is enforced against it plus the granting lab's ceiling.
//
// Command vocabulary (hierarchical; parent Covers child):
//
//	/kubona/tenant/<granting-lab>/qpu/submit   metered hardware dispatch
//	/kubona/tenant/<granting-lab>/read         read foreign-tenant records
//
// Meta terms (verified at every level, narrowing only):
//
//	kubona.tenant          target lab the grant authorizes spend/read for
//	kubona.max_budget_usd  ceiling (non-increasing down the chain)
//	kubona.scopes          CSV capability scopes (subset down the chain)

const (
	MetaTenant = "kubona.tenant"
	MetaBudget = "kubona.max_budget_usd"
	MetaScopes = "kubona.scopes"

	ScopeQPUSubmit = "qpu:submit"
	ScopeRead      = "read"
)

// MaxChainDepth bounds presented chains (DoS + sanity; delegation depth for
// agent-created agents is separately capped at 2 in the A2A layer).
const MaxChainDepth = 8

// DIDForPrivkey derives the did:key DID for a private key (hex seed or key).
// Operator tooling only: prints identities for wiring audiences.
func DIDForPrivkey(privHex string) (string, error) {
	priv, err := parsePrivKey(privHex)
	if err != nil {
		return "", err
	}
	return didkeyctl.FromPrivateKey(priv).String(), nil
}

// DIDForAdminPubkey maps a lab admin's ed25519 public key (hex) to its
// did:key DID. No new PKI: mandate keys ARE grant keys.
func DIDForAdminPubkey(pubHex string) (string, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(pubHex))
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return "", fmt.Errorf("invalid admin public key")
	}
	pub, err := dided25519.PublicKeyFromBytes(raw)
	if err != nil {
		return "", fmt.Errorf("invalid admin public key: %w", err)
	}
	return didkeyctl.FromPublicKey(pub).String(), nil
}

func parsePrivKey(privHex string) (dided25519.PrivateKey, error) {
	var zero dided25519.PrivateKey
	raw, err := hex.DecodeString(strings.TrimSpace(privHex))
	if err != nil {
		return zero, fmt.Errorf("invalid private key hex: %w", err)
	}
	switch len(raw) {
	case 64:
		return dided25519.PrivateKeyFromBytes(raw)
	case 32:
		return dided25519.PrivateKeyFromSeed(raw)
	default:
		return zero, fmt.Errorf("private key must be 32-byte seed or 64-byte hex")
	}
}

func parseDID(s string) (did.DID, error) {
	d, err := did.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("invalid DID %q: %w", s, err)
	}
	return d, nil
}

func grantCommand(grantingTenant, scope string) (command.Command, error) {
	segments := append([]string{"kubona", "tenant", grantingTenant}, strings.Split(scope, "/")...)
	return command.New(segments...), nil
}

func tenantPolicy(targetTenant string) (policy.Policy, error) {
	return policy.Construct(policy.Equal(".kubona.tenant", literal.String(targetTenant)))
}

// validScope enforces the canonical scope vocabulary (W0.8): scopes are
// colon-separated ("qpu:submit"), non-empty, and never contain "/" —
// otherwise "a:b" and "a/b" collide in scopePath and a minted scope means
// something the verifier maps elsewhere.
func validScope(s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("empty scope: refused")
	}
	if strings.Contains(s, "/") {
		return fmt.Errorf("scope %q contains '/': use ':' separators", s)
	}
	return nil
}

// checkScopes requires at least one valid scope: an empty list is a subset
// of everything, so without this an "attenuation" to no-scopes would verify
// as narrowing while authorizing nothing-checkable.
func checkScopes(scopes []string) error {
	if len(scopes) == 0 {
		return fmt.Errorf("at least one scope is required: refused")
	}
	for _, s := range scopes {
		if err := validScope(s); err != nil {
			return err
		}
	}
	return nil
}

// checkTTL mirrors MintGrant's bound: positive and <= 90 days.
func checkTTL(ttl time.Duration) error {
	if ttl <= 0 || ttl > 90*24*time.Hour {
		return fmt.Errorf("grant TTL must be positive and <= 90 days: refused")
	}
	return nil
}

// MintGrant creates a root cross-lab grant. The issuer holds the granting
// lab's admin private key (operator CLI / key file — private keys never touch
// the server). audDID is the grantee lab or agent DID.
func MintGrant(issuerPrivHex, audDID, grantingTenant, targetTenant, scope string, scopes []string, maxBudgetUSD float64, ttl time.Duration) (sealed []byte, cidStr string, err error) {
	if grantingTenant == "" || targetTenant == "" || scope == "" {
		return nil, "", fmt.Errorf("granting/target tenant and scope are required")
	}
	if err := checkScopes(scopes); err != nil {
		return nil, "", err
	}
	if maxBudgetUSD <= 0 {
		return nil, "", fmt.Errorf("grant budget must be positive")
	}
	if err := checkTTL(ttl); err != nil {
		return nil, "", err
	}
	priv, err := parsePrivKey(issuerPrivHex)
	if err != nil {
		return nil, "", err
	}
	iss := didkeyctl.FromPrivateKey(priv)
	aud, err := parseDID(audDID)
	if err != nil {
		return nil, "", err
	}
	cmd, err := grantCommand(grantingTenant, scope)
	if err != nil {
		return nil, "", err
	}
	pol, err := tenantPolicy(targetTenant)
	if err != nil {
		return nil, "", err
	}
	tok, err := delegation.Root(
		iss, aud, cmd, pol,
		delegation.WithExpirationIn(ttl),
		delegation.WithMeta(MetaTenant, targetTenant),
		delegation.WithMeta(MetaBudget, maxBudgetUSD),
		delegation.WithMeta(MetaScopes, strings.Join(scopes, ",")),
	)
	if err != nil {
		return nil, "", fmt.Errorf("mint grant: %w", err)
	}
	sealed, cid, err := tok.ToSealed(priv)
	if err != nil {
		return nil, "", fmt.Errorf("seal grant: %w", err)
	}
	return sealed, cid.String(), nil
}

// AttenuateGrant narrows a parent grant: strictly smaller-or-equal command
// (parent Covers child), budget, scopes, and expiry. Tenant is immutable.
// The issuer must be the parent grant's audience (W0.8): anyone else minting
// "down" the chain produces a chain VerifyChain refuses at level i —
// fail-early here instead of minting known-dead envelopes.
func AttenuateGrant(parentSealed []byte, issuerPrivHex, audDID, scope string, scopes []string, maxBudgetUSD float64, ttl time.Duration) (sealed []byte, cidStr string, err error) {
	parent, parentCID, err := delegation.FromSealed(parentSealed)
	if err != nil {
		return nil, "", fmt.Errorf("parent grant undecodable: %w", err)
	}
	parentMeta, err := readGrantMeta(parent)
	if err != nil {
		return nil, "", err
	}
	priv, err := parsePrivKey(issuerPrivHex)
	if err != nil {
		return nil, "", err
	}
	iss := didkeyctl.FromPrivateKey(priv)
	if iss.String() != parent.Audience().String() {
		return nil, "", fmt.Errorf("attenuator %s is not the parent audience %s: only the grantee narrows its own grant", iss.String(), parent.Audience().String())
	}
	if err := checkTTL(ttl); err != nil {
		return nil, "", err
	}
	if err := checkScopes(scopes); err != nil {
		return nil, "", err
	}
	aud, err := parseDID(audDID)
	if err != nil {
		return nil, "", err
	}
	granting, err := grantingTenantOf(parent.Command())
	if err != nil {
		return nil, "", err
	}
	childCmd, err := grantCommand(granting, scope)
	if err != nil {
		return nil, "", err
	}
	if !parent.Command().Covers(childCmd) {
		return nil, "", fmt.Errorf("attenuation widens command %s beyond %s: refused", childCmd, parent.Command())
	}
	if maxBudgetUSD <= 0 || maxBudgetUSD > parentMeta.budget {
		return nil, "", fmt.Errorf("attenuated budget %.4f exceeds parent %.4f: refused", maxBudgetUSD, parentMeta.budget)
	}
	if !scopesSubset(scopes, parentMeta.scopes) {
		return nil, "", fmt.Errorf("attenuated scopes %v exceed parent %v: refused", scopes, parentMeta.scopes)
	}
	childExp := time.Now().UTC().Add(ttl)
	if parentExp := parent.Expiration(); parentExp != nil && childExp.After(*parentExp) {
		childExp = *parentExp
	}
	pol, err := tenantPolicy(parentMeta.tenant)
	if err != nil {
		return nil, "", err
	}
	_ = parentCID
	tok, err := delegation.New(
		iss, aud, childCmd, pol, parent.Subject(),
		delegation.WithExpiration(childExp),
		delegation.WithMeta(MetaTenant, parentMeta.tenant),
		delegation.WithMeta(MetaBudget, maxBudgetUSD),
		delegation.WithMeta(MetaScopes, strings.Join(scopes, ",")),
	)
	if err != nil {
		return nil, "", fmt.Errorf("attenuate grant: %w", err)
	}
	sealed, cid, err := tok.ToSealed(priv)
	if err != nil {
		return nil, "", fmt.Errorf("seal grant: %w", err)
	}
	return sealed, cid.String(), nil
}

type grantMeta struct {
	tenant string
	budget float64
	scopes []string
}

func readGrantMeta(tok *delegation.Token) (grantMeta, error) {
	var m grantMeta
	tenant, err := tok.Meta().GetString(MetaTenant)
	if err != nil || tenant == "" {
		return m, fmt.Errorf("grant lacks %s meta: refused", MetaTenant)
	}
	budget, err := tok.Meta().GetFloat64(MetaBudget)
	if err != nil || budget <= 0 {
		return m, fmt.Errorf("grant lacks positive %s meta: refused", MetaBudget)
	}
	scopesStr, _ := tok.Meta().GetString(MetaScopes)
	var scopes []string
	for _, s := range strings.Split(scopesStr, ",") {
		if s = strings.TrimSpace(s); s != "" {
			scopes = append(scopes, s)
		}
	}
	return grantMeta{tenant: tenant, budget: budget, scopes: scopes}, nil
}

func scopesSubset(child, parent []string) bool {
	allowed := make(map[string]bool, len(parent))
	for _, s := range parent {
		allowed[s] = true
	}
	for _, s := range child {
		if !allowed[s] {
			return false
		}
	}
	return true
}

func grantingTenantOf(cmd command.Command) (string, error) {
	segs := cmd.Segments()
	if len(segs) < 3 || segs[0] != "kubona" || segs[1] != "tenant" || segs[2] == "" {
		return "", fmt.Errorf("command %s is not a grant command (/kubona/tenant/<lab>/...)", cmd)
	}
	return segs[2], nil
}

// VerifiedGrant is the outcome of chain verification.
type VerifiedGrant struct {
	RootCID        string
	LeafCID        string
	GrantingTenant string
	TargetTenant   string
	MaxBudgetUSD   float64
	Scopes         []string
	Command        string
}

// VerifyOptions configures chain verification.
type VerifyOptions struct {
	// TrustedRoots maps root-issuer did:key strings to lab tenant ids.
	TrustedRoots map[string]string
	// Revoked maps delegation CIDs to true (from ledger grant_revoked events).
	Revoked map[string]bool
	// Now, if non-zero, overrides time.Now (tests).
	Now time.Time
	// RequiredScope, if non-empty, must be covered by the leaf command and
	// present in the leaf scopes.
	RequiredScope string
}

// VerifyChain verifies a root-first chain of sealed delegation envelopes:
// per-envelope signatures, linkage, expiry, command narrowing, meta
// narrowing, trusted root, and revocation. Short TTLs bound revocation
// latency; the ledger is the revocation registry (upstream go-ucan does not
// implement revocation — our ledger is strictly better here).
func VerifyChain(chain [][]byte, opts VerifyOptions) (*VerifiedGrant, error) {
	if len(chain) == 0 {
		return nil, fmt.Errorf("empty grant chain: refused")
	}
	if len(chain) > MaxChainDepth {
		return nil, fmt.Errorf("grant chain depth %d exceeds %d: refused", len(chain), MaxChainDepth)
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	toks := make([]*delegation.Token, len(chain))
	cids := make([]string, len(chain))
	for i, sealed := range chain {
		tok, cid, err := delegation.FromSealed(sealed)
		if err != nil {
			return nil, fmt.Errorf("grant envelope %d undecodable/unsigned: %w", i, err)
		}
		toks[i] = tok
		cids[i] = cid.String()
		if opts.Revoked[cid.String()] {
			return nil, fmt.Errorf("grant %s revoked: refused", cid.String())
		}
		if exp := tok.Expiration(); exp != nil && !now.Before(*exp) {
			return nil, fmt.Errorf("grant envelope %d expired: refused", i)
		}
		if nbf := tok.NotBefore(); nbf != nil && now.Before(*nbf) {
			return nil, fmt.Errorf("grant envelope %d not yet valid: refused", i)
		}
	}

	root := toks[0]
	if !root.IsRoot() {
		return nil, fmt.Errorf("chain root is not a root delegation (iss != sub): refused")
	}
	rootDID := root.Issuer().String()
	grantingTenant, trusted := opts.TrustedRoots[rootDID]
	if !trusted {
		return nil, fmt.Errorf("root issuer %s is not a trusted lab admin: refused", rootDID)
	}

	prevCmd := root.Command()
	prevMeta, err := readGrantMeta(root)
	if err != nil {
		return nil, err
	}
	for i := 1; i < len(toks); i++ {
		child := toks[i]
		if child.Issuer().String() != toks[i-1].Audience().String() {
			return nil, fmt.Errorf("chain broken at level %d (iss != parent aud): refused", i)
		}
		if !prevCmd.Covers(child.Command()) {
			return nil, fmt.Errorf("level %d widens command %s beyond %s: refused", i, child.Command(), prevCmd)
		}
		m, err := readGrantMeta(child)
		if err != nil {
			return nil, err
		}
		if m.tenant != prevMeta.tenant {
			return nil, fmt.Errorf("level %d retargets tenant %s -> %s: refused", i, prevMeta.tenant, m.tenant)
		}
		if m.budget > prevMeta.budget {
			return nil, fmt.Errorf("level %d raises budget %.4f > %.4f: refused", i, m.budget, prevMeta.budget)
		}
		if !scopesSubset(m.scopes, prevMeta.scopes) {
			return nil, fmt.Errorf("level %d widens scopes: refused", i)
		}
		prevCmd = child.Command()
		prevMeta = m
	}

	leaf := toks[len(toks)-1]
	if opts.RequiredScope != "" {
		need, err := grantCommand(grantingTenant, scopePath(opts.RequiredScope))
		if err != nil {
			return nil, err
		}
		if !leaf.Command().Covers(need) {
			return nil, fmt.Errorf("grant command %s does not cover required %s: refused", leaf.Command(), need)
		}
		if !scopesSubset([]string{opts.RequiredScope}, prevMeta.scopes) {
			return nil, fmt.Errorf("grant scopes %v lack required %s: refused", prevMeta.scopes, opts.RequiredScope)
		}
	}

	return &VerifiedGrant{
		RootCID:        cids[0],
		LeafCID:        cids[len(cids)-1],
		GrantingTenant: grantingTenant,
		TargetTenant:   prevMeta.tenant,
		MaxBudgetUSD:   prevMeta.budget,
		Scopes:         prevMeta.scopes,
		Command:        leaf.Command().String(),
	}, nil
}

// scopePath maps a scope name to its command path under a granting tenant.
func scopePath(scope string) string {
	switch scope {
	case ScopeQPUSubmit:
		return "qpu/submit"
	case ScopeRead:
		return "read"
	default:
		return strings.ReplaceAll(scope, ":", "/")
	}
}

// EncodeChainB64 / DecodeChainB64 transport chains as base64 strings.
func EncodeChainB64(chain [][]byte) []string {
	out := make([]string, len(chain))
	for i, sealed := range chain {
		out[i] = base64.StdEncoding.EncodeToString(sealed)
	}
	return out
}

func DecodeChainB64(strs []string) ([][]byte, error) {
	chain := make([][]byte, len(strs))
	for i, s := range strs {
		raw, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("grant envelope %d not base64: %w", i, err)
		}
		chain[i] = raw
	}
	return chain, nil
}

// RevokedCIDs reads the ledger revocation registry (W0.8): grant_revoked
// events name a root CID (whole chain refuses) and/or a leaf CID (chains
// containing that exact envelope refuse — VerifyChain checks EVERY
// envelope's CID, so leaf revocation propagates without killing siblings).
// Previously only roots were recorded, so leaf revocation was unexpressible
// although the verifier already checked every level.
func RevokedCIDs(db *sql.DB) map[string]bool {
	out := map[string]bool{}
	rows, err := db.Query(`SELECT payload FROM ledger_events WHERE event_type = 'grant_revoked'`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var payloadStr string
		var rec struct {
			RootCID string `json:"root_cid"`
			LeafCID string `json:"leaf_cid"`
		}
		if err := rows.Scan(&payloadStr); err != nil {
			continue
		}
		if err := json.Unmarshal([]byte(payloadStr), &rec); err != nil {
			continue
		}
		if rec.RootCID != "" {
			out[rec.RootCID] = true
		}
		if rec.LeafCID != "" {
			out[rec.LeafCID] = true
		}
	}
	return out
}

// AdminDID is the minimal admin-key view TrustRootsForTenants needs, so
// callers are not forced to import tenant model packages.
type AdminDID struct {
	TenantID       string
	AdminPubkeyHex string
	Status         string
}

// TrustRootsForTenants builds the did:key -> tenant trust map from lab admin
// keys. Tenants without an admin key or not active contribute no root: grants
// anchored at unknown keys refuse at verification.
func TrustRootsForTenants(all []AdminDID) map[string]string {
	roots := make(map[string]string)
	for _, t := range all {
		if t.AdminPubkeyHex == "" || t.Status != "active" {
			continue
		}
		didStr, err := DIDForAdminPubkey(t.AdminPubkeyHex)
		if err != nil {
			continue
		}
		roots[didStr] = t.TenantID
	}
	return roots
}
