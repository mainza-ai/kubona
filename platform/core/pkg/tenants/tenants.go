package tenants

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

var (
	ErrTenantNotFound  = errors.New("tenant not found")
	ErrTenantSuspended = errors.New("tenant is suspended")
	ErrNotAdmin        = errors.New("not the tenant admin key")
	ErrActionExpired   = errors.New("admin action expired")
	ErrInvalidLevel    = errors.New("unknown autonomy level")
	ErrCeilingExceeded = errors.New("amount exceeds tenant spend ceiling")
	ErrAdminKeySet     = errors.New("admin key already set: rotation requires current admin signature")
)

// Autonomy levels. L0 observe/suggest only; L1 simulator-only auto-approved;
// L2 capped QPU with a signed mandate per job; L3 pre-approved program
// envelope with burn-rate kill-switch. New tenants default to L1.
const (
	LevelL0 = "L0"
	LevelL1 = "L1"
	LevelL2 = "L2"
	LevelL3 = "L3"
)

// DefaultCeilingUSD is the hard ceiling for a new lab tenant (Q-15 rail).
const DefaultCeilingUSD = 150.0

// DefaultTenantID is the single-operator lab present before multi-tenant use.
const DefaultTenantID = "default"

// Tenant is a lab: the billing, credential, and ceiling boundary. Members
// (Phase 4+) join tenants; agents never are tenants.
type Tenant struct {
	ID              string    `json:"id"`
	DisplayName     string    `json:"display_name"`
	AdminPubkeyHex  string    `json:"admin_pubkey_hex"`
	AutonomyLevel   string    `json:"autonomy_level"`
	SpendCeilingUSD float64   `json:"spend_ceiling_usd"`
	SpentUSD        float64   `json:"spent_usd"`
	Status          string    `json:"status"` // "active" | "suspended"
	CreatedAt       time.Time `json:"created_at"`
}

// AdminAction is a tenant-admin mutation envelope. The admin signs
// TenantID|Action|ParamsHash|Expiry over ed25519; the server verifies the
// admin key matches the tenant row and the action is unexpired. The 5-minute
// default TTL bounds replay.
type AdminAction struct {
	TenantID     string    `json:"tenant_id"`
	Action       string    `json:"action"`
	ParamsHash   string    `json:"params_hash"`
	Expiry       time.Time `json:"expiry"`
	PubkeyHex    string    `json:"pubkey_hex"`
	SignatureHex string    `json:"signature_hex"`
}

// CanonicalPayload returns the deterministic bytes the admin signs.
func (a *AdminAction) CanonicalPayload() []byte {
	return []byte(fmt.Sprintf("%s|%s|%s|%s",
		a.TenantID, a.Action, a.ParamsHash, a.Expiry.UTC().Format("2006-01-02T15:04:05Z")))
}

// ParamsHashOf hashes action parameters so the signature binds them.
func ParamsHashOf(params any) (string, error) {
	data, err := json.Marshal(params)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

type Manager struct {
	ledger *ledger.Ledger
}

func NewManager(l *ledger.Ledger) *Manager {
	return &Manager{ledger: l}
}

func validLevel(level string) bool {
	return level == LevelL0 || level == LevelL1 || level == LevelL2 || level == LevelL3
}

func validTenantID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// CreateTenant registers a lab tenant at L1 with the default ceiling.
func (m *Manager) CreateTenant(id, displayName string) (*Tenant, error) {
	if !validTenantID(id) {
		return nil, fmt.Errorf("invalid tenant id %q", id)
	}
	t := &Tenant{
		ID:              id,
		DisplayName:     displayName,
		AdminPubkeyHex:  "",
		AutonomyLevel:   LevelL1,
		SpendCeilingUSD: DefaultCeilingUSD,
		Status:          "active",
		CreatedAt:       time.Now().UTC(),
	}
	_, err := m.ledger.DB().Exec(
		`INSERT INTO tenants (id, display_name, admin_pubkey_hex, autonomy_level, spend_ceiling_usd, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.DisplayName, t.AdminPubkeyHex, t.AutonomyLevel, t.SpendCeilingUSD, t.Status, t.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}
	m.ledger.AppendEvent(id, "tenant_created", t)
	return t, nil
}

// GetTenant loads a tenant by id.
func (m *Manager) GetTenant(id string) (*Tenant, error) {
	row := m.ledger.DB().QueryRow(
		`SELECT id, display_name, admin_pubkey_hex, autonomy_level, spend_ceiling_usd, spent_usd, status, created_at FROM tenants WHERE id = ?`, id)
	var t Tenant
	if err := row.Scan(&t.ID, &t.DisplayName, &t.AdminPubkeyHex, &t.AutonomyLevel, &t.SpendCeilingUSD, &t.SpentUSD, &t.Status, &t.CreatedAt); err != nil {
		return nil, ErrTenantNotFound
	}
	return &t, nil
}

// verifyAdmin checks the action envelope against the tenant's admin key.
func (m *Manager) verifyAdmin(action *AdminAction) (*Tenant, error) {
	t, err := m.GetTenant(action.TenantID)
	if err != nil {
		return nil, err
	}
	if t.Status != "active" {
		return nil, ErrTenantSuspended
	}
	if t.AdminPubkeyHex == "" || !strings.EqualFold(t.AdminPubkeyHex, action.PubkeyHex) {
		return nil, ErrNotAdmin
	}
	if time.Now().UTC().After(action.Expiry) {
		return nil, ErrActionExpired
	}
	pubBytes, err := hex.DecodeString(action.PubkeyHex)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid admin public key")
	}
	sigBytes, err := hex.DecodeString(action.SignatureHex)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return nil, fmt.Errorf("invalid admin signature format")
	}
	if !ed25519.Verify(ed25519.PublicKey(pubBytes), action.CanonicalPayload(), sigBytes) {
		return nil, fmt.Errorf("admin signature verification failed")
	}
	return t, nil
}

// VerifyAdminAction fully verifies an admin envelope (tenant match, active
// status, admin-key match, expiry, Ed25519 signature). Use it for any
// admin-gated action that does not fit the built-in mutators (e.g. grant
// revocation), after binding the envelope to the mutation parameters.
func (m *Manager) VerifyAdminAction(action *AdminAction) (*Tenant, error) {
	return m.verifyAdmin(action)
}

// SetAdminKey bootstraps (first write when empty) or rotates (current-admin
// signed) the tenant admin key.
func (m *Manager) SetAdminKey(tenantID, newPubkeyHex string, action *AdminAction) (*Tenant, error) {
	t, err := m.GetTenant(tenantID)
	if err != nil {
		return nil, err
	}
	if _, err := hex.DecodeString(newPubkeyHex); err != nil {
		return nil, fmt.Errorf("invalid pubkey hex")
	}
	if t.AdminPubkeyHex == "" {
		// Bootstrap: first write wins, sealed in the ledger.
		if _, err := m.ledger.DB().Exec(`UPDATE tenants SET admin_pubkey_hex = ? WHERE id = ?`, newPubkeyHex, tenantID); err != nil {
			return nil, err
		}
		m.ledger.AppendEvent(tenantID, "tenant_admin_bootstrapped", map[string]string{"tenant_id": tenantID, "admin_pubkey_hex": newPubkeyHex})
		return m.GetTenant(tenantID)
	}
	if action == nil {
		return nil, ErrAdminKeySet
	}
	if _, err := m.verifyAdmin(action); err != nil {
		return nil, err
	}
	if _, err := m.ledger.DB().Exec(`UPDATE tenants SET admin_pubkey_hex = ? WHERE id = ?`, newPubkeyHex, tenantID); err != nil {
		return nil, err
	}
	m.ledger.AppendEvent(tenantID, "tenant_admin_rotated", map[string]string{"tenant_id": tenantID})
	return m.GetTenant(tenantID)
}

// SetCeiling changes the tenant spend ceiling (admin-signed).
func (m *Manager) SetCeiling(action *AdminAction, ceilingUSD float64) (*Tenant, error) {
	t, err := m.verifyAdmin(action)
	if err != nil {
		return nil, err
	}
	if ceilingUSD <= 0 {
		return nil, fmt.Errorf("ceiling must be positive")
	}
	if _, err := m.ledger.DB().Exec(`UPDATE tenants SET spend_ceiling_usd = ? WHERE id = ?`, ceilingUSD, t.ID); err != nil {
		return nil, err
	}
	m.ledger.AppendEvent(t.ID, "tenant_ceiling_changed", map[string]any{"tenant_id": t.ID, "spend_ceiling_usd": ceilingUSD})
	return m.GetTenant(t.ID)
}

// GrantAutonomy moves a tenant to a new level (admin-signed, ledger-sealed).
func (m *Manager) GrantAutonomy(action *AdminAction, level string) (*Tenant, error) {
	t, err := m.verifyAdmin(action)
	if err != nil {
		return nil, err
	}
	if !validLevel(level) {
		return nil, ErrInvalidLevel
	}
	from := t.AutonomyLevel
	if _, err := m.ledger.DB().Exec(`UPDATE tenants SET autonomy_level = ? WHERE id = ?`, level, t.ID); err != nil {
		return nil, err
	}
	m.ledger.AppendEvent(t.ID, "tenant_autonomy_granted", map[string]string{"tenant_id": t.ID, "from": from, "to": level})
	return m.GetTenant(t.ID)
}

// RevokeAutonomy drops a tenant to L0 (observe/suggest only). Revocation is
// effective on next dispatch check (live, no cached grants).
func (m *Manager) RevokeAutonomy(action *AdminAction, reason string) (*Tenant, error) {
	t, err := m.verifyAdmin(action)
	if err != nil {
		return nil, err
	}
	from := t.AutonomyLevel
	if _, err := m.ledger.DB().Exec(`UPDATE tenants SET autonomy_level = ? WHERE id = ?`, LevelL0, t.ID); err != nil {
		return nil, err
	}
	m.ledger.AppendEvent(t.ID, "tenant_autonomy_revoked", map[string]string{"tenant_id": t.ID, "from": from, "reason": reason})
	return m.GetTenant(t.ID)
}

// ListTenants returns every registered lab tenant (used to build grant
// trust roots from admin keys).
func (m *Manager) ListTenants() ([]Tenant, error) {
	rows, err := m.ledger.DB().Query(
		`SELECT id, display_name, admin_pubkey_hex, autonomy_level, spend_ceiling_usd, spent_usd, status, created_at FROM tenants ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.DisplayName, &t.AdminPubkeyHex, &t.AutonomyLevel, &t.SpendCeilingUSD, &t.SpentUSD, &t.Status, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CheckSpend refuses amounts above the tenant ceiling (live check at dispatch).
// Cumulative: already-reserved spend counts, so ten $14 jobs exhaust a $150
// ceiling — the ceiling is a billing boundary, not a per-job suggestion.
func (m *Manager) CheckSpend(tenantID string, amountUSD float64) error {
	t, err := m.GetTenant(tenantID)
	if err != nil {
		return err
	}
	if t.Status != "active" {
		return ErrTenantSuspended
	}
	if amountUSD > t.SpendCeilingUSD || t.SpentUSD+amountUSD > t.SpendCeilingUSD {
		return ErrCeilingExceeded
	}
	return nil
}

// ReserveSpend atomically reserves amountUSD against the tenant ceiling.
// The compare-and-set runs inside one UPDATE so concurrent dispatches cannot
// jointly overspend (no read-check-write race across workers). On success the
// reservation is sealed as a ledger event; the caller releases it when the
// job settles terminal-failed, and keeps it when the job completes (the
// authorized amount is the spend — provider true-ups are out of scope).
func (m *Manager) ReserveSpend(tenantID string, amountUSD float64) error {
	if amountUSD <= 0 {
		return m.CheckSpend(tenantID, 0)
	}
	res, err := m.ledger.DB().Exec(
		`UPDATE tenants SET spent_usd = spent_usd + ? WHERE id = ? AND status = 'active' AND spent_usd + ? <= spend_ceiling_usd AND ? <= spend_ceiling_usd`,
		amountUSD, tenantID, amountUSD, amountUSD)
	if err != nil {
		return fmt.Errorf("spend reservation failed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("spend reservation failed: %w", err)
	}
	if n == 0 {
		// Name the reason without leaking more than the refusal already does.
		if _, gerr := m.GetTenant(tenantID); gerr != nil {
			return gerr
		}
		t, _ := m.GetTenant(tenantID)
		if t.Status != "active" {
			return ErrTenantSuspended
		}
		return ErrCeilingExceeded
	}
	m.ledger.AppendEvent(tenantID, "tenant_spend_reserved", map[string]any{
		"tenant_id": tenantID, "amount_usd": amountUSD})
	return nil
}

// ReleaseSpend returns a reservation (terminal-failed jobs only). Floored at
// zero so double-release can never mint negative spend; callers additionally
// zero the job's reserved_amount for idempotence.
func (m *Manager) ReleaseSpend(tenantID string, amountUSD float64) error {
	if amountUSD <= 0 {
		return nil
	}
	if _, err := m.ledger.DB().Exec(
		`UPDATE tenants SET spent_usd = MAX(0, spent_usd - ?) WHERE id = ?`,
		amountUSD, tenantID); err != nil {
		return fmt.Errorf("spend release failed: %w", err)
	}
	m.ledger.AppendEvent(tenantID, "tenant_spend_released", map[string]any{
		"tenant_id": tenantID, "amount_usd": amountUSD})
	return nil
}

// VaultScope returns the per-tenant vault scope convention: provider
// credential leases for a tenant live under tenant/<id> and must never be
// issued outside it.
func VaultScope(tenantID string) string {
	return "tenant/" + tenantID
}
