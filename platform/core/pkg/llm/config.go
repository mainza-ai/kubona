package llm

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"
)

type ProviderType string

const (
	ProviderOMLX      ProviderType = "omlx"
	ProviderOllama    ProviderType = "ollama"
	ProviderLMStudio  ProviderType = "lmstudio"
	ProviderAnthropic ProviderType = "anthropic"
	ProviderOpenAI    ProviderType = "openai"
	ProviderOpenCode  ProviderType = "opencode"
	ProviderNVIDIA    ProviderType = "nvidia"
	ProviderStrata    ProviderType = "strata"
)

// KnownRoles is the fixed tribunal/copilot role set. Role assignments
// default to the active provider+model; per-role overrides come from
// KUBONA_LLM_ROLE_<ROLE>_PROVIDER / _MODEL (hyphens become underscores).
var KnownRoles = []string{
	"copilot",
	"epistemic-scribe",
	"ansatz-scout",
	"disconfirmation-officer",
	"device-auditor",
	// codesign is the bounded circuit-proposal role used by the Go-owned
	// rerun rail. It defaults to the active provider (local Strata unless
	// the operator explicitly assigns a cloud provider+model).
	"codesign",
}

// providerModelEnv maps each provider to the env var carrying its model
// ID. Model IDs live in configuration (env / DB / UI) — never in code:
// a model rotation must not require a rebuild.
func providerModelEnv(pt ProviderType) string {
	switch pt {
	case ProviderOMLX:
		return "OMLX_MODEL"
	case ProviderOllama:
		return "OLLAMA_MODEL"
	case ProviderLMStudio:
		return "LMSTUDIO_MODEL"
	case ProviderAnthropic:
		return "ANTHROPIC_MODEL"
	case ProviderOpenAI:
		return "OPENAI_MODEL"
	case ProviderOpenCode:
		return "OPENCODE_MODEL"
	case ProviderNVIDIA:
		return "NVIDIA_MODEL"
	case ProviderStrata:
		return "STRATA_MODEL"
	default:
		return ""
	}
}

// roleEnvNames maps a role to its optional per-role override env vars.
// Empty return = role not in the known set (no env consulted).
func roleEnvNames(role string) (providerEnv, modelEnv string, ok bool) {
	upper := strings.ToUpper(strings.ReplaceAll(role, "-", "_"))
	for _, known := range KnownRoles {
		if known == role {
			return "KUBONA_LLM_" + upper + "_PROVIDER", "KUBONA_LLM_" + upper + "_MODEL", true
		}
	}
	return "", "", false
}

type ProviderConfig struct {
	Type         ProviderType `json:"type"`
	Name         string       `json:"name"`
	BaseURL      string       `json:"base_url"`
	APIKey       string       `json:"api_key,omitempty"`
	DefaultModel string       `json:"default_model"`
	Enabled      bool         `json:"enabled"`
	IsLocal      bool         `json:"is_local"`
	TimeoutSec   int          `json:"timeout_sec"`
	// LastSuccessAt is process-lifetime telemetry (RFC3339, "" = never):
	// the last time this provider completed a call. Displayed in the
	// console as "last ok"; never a health claim, never persisted.
	LastSuccessAt string `json:"last_success_at,omitempty"`
}

// CurrentSettingsVersion is the settings blob schema. LoadSettings stamps
// it on first boot and migrates older blobs forward (additive only —
// values are never rewritten by a migration; stale values surface through
// validation and resolve-time errors instead).
const CurrentSettingsVersion = 1

type RoleModelAssignment struct {
	Role     string       `json:"role"`
	Provider ProviderType `json:"provider"`
	Model    string       `json:"model"`
}

type Settings struct {
	mu sync.RWMutex
	// SchemaVersion gates migrations (see LoadSettings). 0 = pre-version
	// blob: stamped current on load, values untouched.
	SchemaVersion   int                             `json:"schema_version"`
	ActiveProvider  ProviderType                    `json:"active_provider"`
	ActiveModel     string                          `json:"active_model"`
	AirGapped       bool                            `json:"air_gapped"`
	Temperature     float64                         `json:"temperature"`
	MaxTokens       int                             `json:"max_tokens"`
	Providers       map[ProviderType]ProviderConfig `json:"providers"`
	RoleAssignments map[string]RoleModelAssignment  `json:"role_assignments"`
	// DisableDSMLFallback kills the DSML-markup tool-invocation fallback
	// (W0.19): with it off, model text never becomes tool calls. Default
	// false — some gateways need the dialect; operators who don't trust it
	// switch it off and lose nothing structured.
	DisableDSMLFallback bool `json:"disable_dsml_fallback"`
}

func DefaultSettings() *Settings {
	// Model IDs come from configuration (env, DB, or UI) — never
	// from code. A model rotation is an env/DB/UI edit, not a rebuild; see
	// .env.example for the full variable list. An empty model means
	// "unconfigured": the provider stays registered but resolve-time
	// calls fail with the exact variable to set. Local oMLX stays
	// enabled as the air-gapped fallback. Production posture is the
	// reverse (air-gapped, local-only) — set air_gapped + local roles
	// explicitly there; a non-local active provider needs
	// KUBONA_LLM_CLOUD=1 anyway.
	// Base URLs are the well-known local defaults; Strata's serving port
	// comes from its Serving panel (observed http://127.0.0.1:47831/v1)
	// and is overridable without a rebuild when it changes. No-env
	// default active provider is the local Strata engine (dev-first,
	// production-safe: local needs no KUBONA_LLM_CLOUD opt-in).
	strataBase := os.Getenv("STRATA_BASE_URL")
	if strataBase == "" {
		strataBase = "http://127.0.0.1:47831/v1"
	}
	providers := map[ProviderType]ProviderConfig{
		ProviderOMLX: {
			Type:         ProviderOMLX,
			Name:         "oMLX Local LLM Engine (Apple Silicon)",
			BaseURL:      "http://127.0.0.1:8787/v1",
			DefaultModel: os.Getenv("OMLX_MODEL"),
			Enabled:      true,
			IsLocal:      true,
			TimeoutSec:   30,
		},
		ProviderOllama: {
			Type:         ProviderOllama,
			Name:         "Ollama Local Engine",
			BaseURL:      "http://127.0.0.1:11434/v1",
			DefaultModel: os.Getenv("OLLAMA_MODEL"),
			Enabled:      true,
			IsLocal:      true,
			TimeoutSec:   30,
		},
		ProviderLMStudio: {
			Type:         ProviderLMStudio,
			Name:         "LM Studio Local Engine",
			BaseURL:      "http://127.0.0.1:1234/v1",
			DefaultModel: os.Getenv("LMSTUDIO_MODEL"),
			Enabled:      true,
			IsLocal:      true,
			TimeoutSec:   30,
		},
		ProviderAnthropic: {
			Type:         ProviderAnthropic,
			Name:         "Anthropic Claude API",
			BaseURL:      "https://api.anthropic.com/v1",
			APIKey:       os.Getenv("ANTHROPIC_API_KEY"),
			DefaultModel: os.Getenv("ANTHROPIC_MODEL"),
			Enabled:      os.Getenv("ANTHROPIC_API_KEY") != "",
			IsLocal:      false,
			TimeoutSec:   60,
		},
		ProviderOpenAI: {
			Type:         ProviderOpenAI,
			Name:         "OpenAI API",
			BaseURL:      "https://api.openai.com/v1",
			APIKey:       os.Getenv("OPENAI_API_KEY"),
			DefaultModel: os.Getenv("OPENAI_MODEL"),
			Enabled:      os.Getenv("OPENAI_API_KEY") != "",
			IsLocal:      false,
			TimeoutSec:   60,
		},
		ProviderOpenCode: {
			Type:         ProviderOpenCode,
			Name:         "OpenCode Hosted Inference",
			BaseURL:      "https://opencode.ai/inference/openai/v1",
			APIKey:       os.Getenv("OPENCODE_API_KEY"),
			DefaultModel: os.Getenv("OPENCODE_MODEL"),
			Enabled:      os.Getenv("OPENCODE_API_KEY") != "",
			IsLocal:      false,
			TimeoutSec:   120,
		},
		ProviderNVIDIA: {
			Type:         ProviderNVIDIA,
			Name:         "NVIDIA NIM Cloud Inference",
			BaseURL:      "https://integrate.api.nvidia.com/v1",
			APIKey:       os.Getenv("NVIDIA_API_KEY"),
			DefaultModel: os.Getenv("NVIDIA_MODEL"),
			Enabled:      os.Getenv("NVIDIA_API_KEY") != "",
			IsLocal:      false,
			TimeoutSec:   120,
		},
		ProviderStrata: {
			Type:         ProviderStrata,
			Name:         "Strata Local Engine",
			BaseURL:      strataBase,
			APIKey:       os.Getenv("STRATA_API_KEY"),
			DefaultModel: os.Getenv("STRATA_MODEL"),
			Enabled:      true,
			IsLocal:      true,
			TimeoutSec:   30,
		},
	}

	active := ProviderType(os.Getenv("KUBONA_LLM_PROVIDER"))
	if active == "" {
		active = ProviderStrata
	}
	activeModel := os.Getenv("KUBONA_LLM_MODEL")
	if activeModel == "" {
		activeModel = providers[active].DefaultModel
	}

	roles := make(map[string]RoleModelAssignment, len(KnownRoles))
	for _, role := range KnownRoles {
		p, m := active, activeModel
		if provEnv, modelEnv, ok := roleEnvNames(role); ok {
			if v := os.Getenv(provEnv); v != "" {
				p = ProviderType(v)
			}
			if v := os.Getenv(modelEnv); v != "" {
				m = v
			} else if p != active {
				m = providers[p].DefaultModel
			}
		}
		roles[role] = RoleModelAssignment{Role: role, Provider: p, Model: m}
	}

	return &Settings{
		SchemaVersion:  CurrentSettingsVersion,
		ActiveProvider: active,
		ActiveModel:    activeModel,
		// Secure default (W0.19): fresh installs are air-gapped — no
		// non-local egress until the operator explicitly toggles it off
		// in settings. Local providers (Strata/MLX loopback) are
		// unaffected. Without keys configured nothing could egress
		// anyway; this makes the posture explicit instead of implicit.
		AirGapped:   true,
		Temperature: 0.2,
		// MaxTokens 0 = unset: the field is omitted and the model generates
		// to EOS / its own context window. Any positive value is a per-role
		// ceiling, not a default — 2048 proved too low for tribunal turns,
		// and no fixed default fits all roles (2026-10-11).
		MaxTokens:       0,
		Providers:       providers,
		RoleAssignments: roles,
	}
}

// InitSettingsTable creates the SQLite table for persisting LLM configuration
func InitSettingsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS llm_settings (
			key TEXT PRIMARY KEY,
			value_json TEXT NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// LoadSettings loads persistent settings from SQLite or defaults.
// Blobs without a schema version are pre-version: they are stamped
// current and saved (values untouched — migration never rewrites an
// operator's configuration). A blob from a NEWER binary refuses with a
// descriptive error (downgrade boots ephemeral defaults via the caller).
func LoadSettings(db *sql.DB) (*Settings, error) {
	if err := InitSettingsTable(db); err != nil {
		return nil, err
	}

	var valJSON string
	err := db.QueryRow("SELECT value_json FROM llm_settings WHERE key = 'active_settings'").Scan(&valJSON)
	if err == sql.ErrNoRows {
		s := DefaultSettings()
		_ = s.Save(db)
		return s, nil
	} else if err != nil {
		return nil, err
	}

	var s Settings
	if err := json.Unmarshal([]byte(valJSON), &s); err != nil {
		return DefaultSettings(), nil
	}
	if s.SchemaVersion == 0 {
		s.SchemaVersion = CurrentSettingsVersion
		_ = s.Save(db)
		return &s, nil
	}
	if s.SchemaVersion > CurrentSettingsVersion {
		return DefaultSettings(), fmt.Errorf("llm settings schema v%d is newer than this binary (v%d): refusing persisted settings, running ephemeral defaults", s.SchemaVersion, CurrentSettingsVersion)
	}
	return &s, nil
}

// Save persists the current settings into SQLite
func (s *Settings) Save(db *sql.DB) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.Marshal(s)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		INSERT INTO llm_settings (key, value_json, updated_at)
		VALUES ('active_settings', ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value_json = excluded.value_json,
			updated_at = CURRENT_TIMESTAMP
	`, string(data))
	return err
}

// EnsureDefaultRoles backfills role assignments missing from persisted
// settings (older seeds lacked e.g. "copilot", leaving generation on a
// silent ActiveModel fallback). Returns true when it changed anything.
func (s *Settings) EnsureDefaultRoles() bool {
	defs := DefaultSettings().RoleAssignments
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.RoleAssignments == nil {
		s.RoleAssignments = make(map[string]RoleModelAssignment)
	}
	changed := false
	for role, assign := range defs {
		if _, ok := s.RoleAssignments[role]; !ok {
			s.RoleAssignments[role] = assign
			changed = true
		}
	}
	return changed
}

// EnsureDefaultProviders backfills provider entries missing from persisted
// settings (e.g. a provider added by a newer binary). Never overwrites an
// operator-configured provider; picks up credential env vars for backfilled
// entries. Returns true when it changed anything.
func (s *Settings) EnsureDefaultProviders() bool {
	defs := DefaultSettings().Providers
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Providers == nil {
		s.Providers = make(map[ProviderType]ProviderConfig)
	}
	changed := false
	for pt, pc := range defs {
		existing, ok := s.Providers[pt]
		if !ok {
			if pc.APIKey == "" {
				pc.APIKey = os.Getenv(providerEnvKey(pt))
			}
			// Cloud providers enable on key presence (locals are always
			// on): a keyed cloud provider with no model still resolves
			// loudly at call time (missing model names its env var).
			if !pc.IsLocal {
				pc.Enabled = pc.APIKey != ""
			}
			s.Providers[pt] = pc
			changed = true
			continue
		}
		// Top-up: an empty stored secret picks up the env var (rotating or
		// newly-provisioned keys apply without a UI round-trip). A stored
		// non-empty secret is never overwritten here.
		if existing.APIKey == "" {
			if envKey := os.Getenv(providerEnvKey(pt)); envKey != "" {
				existing.APIKey = envKey
				if !existing.IsLocal {
					existing.Enabled = true
				}
				s.Providers[pt] = existing
				changed = true
			}
		}
	}
	return changed
}

// ValidateForProduction enforces the production posture (W0.19): the dev
// deployment runs cloud-hosted inference by explicit operator direction
// (OPENCODE_API_KEY present), but a PRODUCTION boot with a non-local active
// provider and no explicit KUBONA_LLM_CLOUD=1 opt-in refuses instead of
// silently exfiltrating prompts. Key presence alone is not opt-in — keys
// leak across environments; the flag is the decision.
func (s *Settings) ValidateForProduction() error {
	if strings.ToLower(strings.TrimSpace(os.Getenv("KUBONA_ENV"))) != "production" {
		return nil
	}
	snap := s.GetSnapshot()
	prov, ok := snap.Providers[snap.ActiveProvider]
	if !ok || !prov.Enabled {
		return fmt.Errorf("production: active provider %s is disabled or unconfigured", snap.ActiveProvider)
	}
	// Audit 2026-10-30: the old gate checked only the active provider, so a
	// local-active config with a cloud ROLE could still egress prompts.
	// Every enabled role is a possible egress path; refuse any cloud one
	// without the explicit opt-in.
	if !prov.IsLocal && !cloudLLMOptIn() {
		return fmt.Errorf("production: non-local LLM provider %s requires explicit KUBONA_LLM_CLOUD=1 opt-in (prompts would leave the box)", snap.ActiveProvider)
	}
	for role, assign := range snap.RoleAssignments {
		roleProv, ok := snap.Providers[assign.Provider]
		if !ok || !roleProv.Enabled || roleProv.IsLocal {
			continue
		}
		if !cloudLLMOptIn() {
			return fmt.Errorf("production: role %q resolves to non-local provider %s and requires explicit KUBONA_LLM_CLOUD=1 opt-in (prompts would leave the box)", role, assign.Provider)
		}
	}
	return nil
}

// cloudLLMOptIn is the single opt-in predicate for production cloud egress.
// Key presence alone is not consent; the explicit flag is the decision.
func cloudLLMOptIn() bool {
	return os.Getenv("KUBONA_LLM_CLOUD") == "1"
}

// productionCloudWithoutOptIn reports whether a non-local provider would be
// used in production without KUBONA_LLM_CLOUD=1. Dev/test is unaffected.
func productionCloudWithoutOptIn(isLocal bool) bool {
	return !isLocal && strings.ToLower(strings.TrimSpace(os.Getenv("KUBONA_ENV"))) == "production" && !cloudLLMOptIn()
}

func (s *Settings) GetSnapshot() Settings { return s.snapshot(false) }

// GetPublicSnapshot is the only serializer for network reads (GET and the
// PUT response envelope). API keys are server-side only: the console was
// designed for no-echo (untouched providers send "", backend keeps stored),
// so nothing over the wire needs them — and GET is a public read. Existence
// of a key remains visible via Enabled; the value never leaves the process.
// Update() merge semantics are unchanged (empty incoming keeps stored).
func (s *Settings) GetPublicSnapshot() Settings { return s.snapshot(true) }

func (s *Settings) snapshot(redact bool) Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copyProviders := make(map[ProviderType]ProviderConfig)
	for k, v := range s.Providers {
		if redact {
			v.APIKey = ""
		}
		copyProviders[k] = v
	}

	copyRoles := make(map[string]RoleModelAssignment)
	for k, v := range s.RoleAssignments {
		copyRoles[k] = v
	}

	return Settings{
		SchemaVersion:       s.SchemaVersion,
		ActiveProvider:      s.ActiveProvider,
		ActiveModel:         s.ActiveModel,
		AirGapped:           s.AirGapped,
		Temperature:         s.Temperature,
		MaxTokens:           s.MaxTokens,
		Providers:           copyProviders,
		RoleAssignments:     copyRoles,
		DisableDSMLFallback: s.DisableDSMLFallback,
	}
}

func (s *Settings) Update(update *Settings, db *sql.DB) error {
	// W0.19: values are load-bearing (a NaN temperature or negative token
	// ceiling corrupts every call silently). Refuse garbage at the edge.
	if math.IsNaN(update.Temperature) || math.IsInf(update.Temperature, 0) {
		return fmt.Errorf("temperature must be finite: refusing")
	}
	if update.MaxTokens < 0 {
		return fmt.Errorf("max_tokens must be >= 0: refusing")
	}
	// Role assignments bind calls to exact providers+models: unknown
	// providers and empty models are typos, not configuration. Refuse
	// instead of failing (or silently falling back) at call time. Only
	// changed-or-new entries are validated: pre-existing values are
	// grandfathered so a save of unrelated fields (e.g. temperature)
	// never breaks on a dormant stale entry — staleness surfaces at
	// resolve time and in validation display instead.
	s.mu.RLock()
	currentRoles := s.RoleAssignments
	currentProvs := s.Providers
	s.mu.RUnlock()
	provs := update.Providers
	if provs == nil {
		provs = currentProvs
	}
	for role, assign := range update.RoleAssignments {
		if cur, ok := currentRoles[role]; ok && cur == assign {
			continue
		}
		prov, ok := provs[assign.Provider]
		if !ok {
			return fmt.Errorf("role %q points at unknown provider %q: refusing", role, assign.Provider)
		}
		if strings.TrimSpace(assign.Model) == "" && prov.Enabled {
			return fmt.Errorf("role %q has an empty model on enabled provider %q: refusing (set one or disable the provider)", role, assign.Provider)
		}
	}
	if strings.TrimSpace(update.ActiveModel) == "" {
		s.mu.RLock()
		cur := s.ActiveModel
		s.mu.RUnlock()
		if strings.TrimSpace(cur) != "" {
			return fmt.Errorf("active_model must not be empty: refusing (clearing the fallback breaks unknown-role resolution)")
		}
		// Both empty (e.g. no model env on a fresh keyless boot): grandfathered;
		// resolve-time errors name the exact variable to set.
	}
	s.mu.Lock()
	s.ActiveProvider = update.ActiveProvider
	s.ActiveModel = update.ActiveModel
	s.AirGapped = update.AirGapped
	s.Temperature = update.Temperature
	s.MaxTokens = update.MaxTokens
	s.DisableDSMLFallback = update.DisableDSMLFallback
	if update.Providers != nil {
		// No partial provider writes: a struct without BaseURL/TimeoutSec
		// is a fragment, and storing it would silently zero Enabled,
		// BaseURL, and the model (G13). The console always sends full
		// objects (GET-modify-PUT); programmatic clients must do the same.
		// Unknown provider types are junk, never configuration.
		for pt, pc := range update.Providers {
			if _, ok := s.Providers[pt]; !ok {
				s.mu.Unlock()
				return fmt.Errorf("provider %q unknown: refusing (known: strata, ollama, olmx, lmstudio, anthropic, openai, opencode, nvidia)", pt)
			}
			if strings.TrimSpace(pc.BaseURL) == "" || pc.TimeoutSec == 0 {
				s.mu.Unlock()
				return fmt.Errorf("provider %q: partial entry refuses (send the full provider object — GET first, modify, PUT back)", pt)
			}
			// Never wipe a stored secret with an empty form field: an empty
			// incoming APIKey keeps the existing one (env-supplied keys survive
			// UI saves that don't include secrets).
			if pc.APIKey == "" {
				if existing, ok := s.Providers[pt]; ok {
					pc.APIKey = existing.APIKey
				}
				if pc.APIKey == "" {
					pc.APIKey = os.Getenv(providerEnvKey(pt))
				}
			}
			s.Providers[pt] = pc
		}
	}
	if update.RoleAssignments != nil {
		s.RoleAssignments = update.RoleAssignments
	}
	s.mu.Unlock()

	if db != nil {
		return s.Save(db)
	}
	return nil
}

// providerEnvKey maps providers to their credential env vars.
func providerEnvKey(pt ProviderType) string {
	switch pt {
	case ProviderOpenCode:
		return "OPENCODE_API_KEY"
	case ProviderNVIDIA:
		return "NVIDIA_API_KEY"
	case ProviderOpenAI:
		return "OPENAI_API_KEY"
	case ProviderAnthropic:
		return "ANTHROPIC_API_KEY"
	case ProviderStrata:
		return "STRATA_API_KEY"
	default:
		return ""
	}
}

func (s *Settings) GetModelForRole(role string) (ProviderType, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if assign, ok := s.RoleAssignments[role]; ok {
		prov, provOk := s.Providers[assign.Provider]
		if !provOk {
			return "", "", fmt.Errorf("role %q points at unknown provider %q: refusing (fix the assignment, not the call)", role, assign.Provider)
		}
		if !prov.Enabled {
			return "", "", fmt.Errorf("role %q provider %s is disabled: refusing (enable it or reassign the role)", role, assign.Provider)
		}
		if s.AirGapped && !prov.IsLocal {
			return "", "", fmt.Errorf("provider %s is blocked by Sovereign Air-Gap isolation", assign.Provider)
		}
		if productionCloudWithoutOptIn(prov.IsLocal) {
			return "", "", fmt.Errorf("provider %s is non-local and KUBONA_LLM_CLOUD=1 is not set: refusing production cloud egress", assign.Provider)
		}
		if strings.TrimSpace(assign.Model) == "" {
			env := modelEnvFor(assign.Provider, role)
			return "", "", fmt.Errorf("role %q has no model configured: set %s", role, env)
		}
		return assign.Provider, assign.Model, nil
	}

	// Fallback to active provider (only for roles with NO assignment —
	// a known role pointing at a bad provider refuses above, never falls
	// through silently).
	prov, ok := s.Providers[s.ActiveProvider]
	if !ok || !prov.Enabled {
		return "", "", fmt.Errorf("active provider %s is disabled or unconfigured", s.ActiveProvider)
	}
	if s.AirGapped && !prov.IsLocal {
		return "", "", fmt.Errorf("active provider %s is blocked by Sovereign Air-Gap isolation", s.ActiveProvider)
	}
	if productionCloudWithoutOptIn(prov.IsLocal) {
		return "", "", fmt.Errorf("active provider %s is non-local and KUBONA_LLM_CLOUD=1 is not set: refusing production cloud egress", s.ActiveProvider)
	}
	if strings.TrimSpace(s.ActiveModel) == "" {
		return "", "", fmt.Errorf("active provider %s has no model configured: set %s", s.ActiveProvider, modelEnvFor(s.ActiveProvider, ""))
	}

	return s.ActiveProvider, s.ActiveModel, nil
}

// modelEnvFor names the configuration knob for a provider/role model:
// the role-specific variable first, then the provider default.
func modelEnvFor(pt ProviderType, role string) string {
	if provEnv, modelEnv, ok := roleEnvNames(role); ok && role != "" {
		_ = provEnv
		if penv := providerModelEnv(pt); penv != "" {
			return modelEnv + " or " + penv
		}
		return modelEnv
	}
	if penv := providerModelEnv(pt); penv != "" {
		return penv + " or KUBONA_LLM_MODEL"
	}
	return "KUBONA_LLM_MODEL"
}

// RecordSuccess stamps process-lifetime proof that a provider completed a
// call (RFC3339, surfaced in the console as "last ok"). In-memory only by
// design: after a restart the signal is honestly unknown until the next
// call, never carried over as stale proof.
func (s *Settings) RecordSuccess(pt ProviderType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pc, ok := s.Providers[pt]; ok {
		pc.LastSuccessAt = time.Now().UTC().Format(time.RFC3339)
		s.Providers[pt] = pc
	}
}
