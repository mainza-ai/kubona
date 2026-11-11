package llm

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

type ProviderType string

const (
	ProviderOMLX       ProviderType = "omlx"
	ProviderOllama     ProviderType = "ollama"
	ProviderLMStudio   ProviderType = "lmstudio"
	ProviderAnthropic  ProviderType = "anthropic"
	ProviderOpenAI     ProviderType = "openai"
	ProviderOpenCode   ProviderType = "opencode"
	ProviderNVIDIA     ProviderType = "nvidia"
	ProviderStrata     ProviderType = "strata"
	ProviderGoogle     ProviderType = "google"
	ProviderOpenRouter ProviderType = "openrouter"
	ProviderXAI        ProviderType = "xai"
	ProviderMistral    ProviderType = "mistral"
	ProviderGroq       ProviderType = "groq"
	ProviderCustom     ProviderType = "custom"
)

// WireType identifies the native wire protocol spoken by a provider endpoint.
type WireType string

const (
	WireChatCompletions   WireType = "chat_completions"
	WireAnthropicMessages WireType = "anthropic_messages"
	WireOpenAIResponses   WireType = "openai_responses"
	WireGemini            WireType = "gemini"
	WireOllamaNative      WireType = "ollama_native"
)

// DefaultWireFor returns the canonical wire dialect for a provider slot.
func DefaultWireFor(pt ProviderType) WireType {
	switch pt {
	case ProviderAnthropic:
		return WireAnthropicMessages
	case ProviderOpenAI:
		return WireOpenAIResponses
	case ProviderGoogle:
		return WireGemini
	case ProviderOllama:
		return WireOllamaNative
	default:
		return WireChatCompletions
	}
}

func validWireType(w WireType) bool {
	switch w {
	case "", WireChatCompletions, WireAnthropicMessages, WireOpenAIResponses, WireGemini, WireOllamaNative:
		return true
	default:
		return false
	}
}

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
	// Autonomous loop & evolution roles used by pkg/autoresearch and pkg/metasearch.
	"coder",
	"critic",
	"supervisor",
}

// RoleMeta describes a registered LLM execution role for UI grouping and documentation.
type RoleMeta struct {
	Role        string `json:"role"`
	Group       string `json:"group"` // "interactive" | "tribunal" | "autonomous"
	Label       string `json:"label"`
	Description string `json:"description"`
}

// KnownRoleCatalog returns ordered metadata for every role in KnownRoles.
func KnownRoleCatalog() []RoleMeta {
	return []RoleMeta{
		{Role: "copilot", Group: "interactive", Label: "Copilot", Description: "Primary interactive research assistant and command-dock chat"},
		{Role: "codesign", Group: "interactive", Label: "Co-Design", Description: "Bounded quantum-circuit co-design proposal and hardware tradeoff synthesis"},
		{Role: "ansatz-scout", Group: "tribunal", Label: "Ansatz Scout", Description: "Proposes candidate Hamiltonians, variational circuits, and simulation experiments"},
		{Role: "disconfirmation-officer", Group: "tribunal", Label: "Disconfirmation Officer", Description: "Adversarially critiques proposals, hunts counter-evidence, and enforces falsification"},
		{Role: "device-auditor", Group: "tribunal", Label: "Device Auditor", Description: "Verifies hardware topology, noise constraints, and calibration telemetry limits"},
		{Role: "epistemic-scribe", Group: "tribunal", Label: "Epistemic Scribe", Description: "Synthesizes tribunal evidence into structured consensus and claim records"},
		{Role: "coder", Group: "autonomous", Label: "Autoresearch Coder", Description: "Generates bounded code and circuit mutations inside autoresearch loops"},
		{Role: "critic", Group: "autonomous", Label: "Autoresearch Critic", Description: "Evaluates candidate diffs and rejects speculative or unverified regressions"},
		{Role: "supervisor", Group: "autonomous", Label: "Meta-Search Supervisor", Description: "Guides outer meta-search evolution strategies across stalled frontiers"},
	}
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
	case ProviderGoogle:
		return "GEMINI_MODEL"
	case ProviderOpenRouter:
		return "OPENROUTER_MODEL"
	case ProviderXAI:
		return "XAI_MODEL"
	case ProviderMistral:
		return "MISTRAL_MODEL"
	case ProviderGroq:
		return "GROQ_MODEL"
	case ProviderCustom:
		return "CUSTOM_LLM_MODEL"
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

const (
	KeyStatusMissing = "missing"
	KeyStatusSet     = "set"
	KeyStatusValid   = "valid"
	KeyStatusInvalid = "invalid"
)

const (
	SecretSourceNone        = "none"
	SecretSourceEnv         = "env"
	SecretSourceSealedDB    = "sealed_db"
	SecretSourcePlaintextDB = "plaintext_db"
)

type ProviderConfig struct {
	Type         ProviderType `json:"type"`
	Name         string       `json:"name"`
	BaseURL      string       `json:"base_url"`
	APIKey       string       `json:"api_key,omitempty"`
	DefaultModel string       `json:"default_model"`
	Wire         WireType     `json:"wire,omitempty"`
	Enabled      bool         `json:"enabled"`
	IsLocal      bool         `json:"is_local"`
	TimeoutSec   int          `json:"timeout_sec"`
	// LastSuccessAt is process-lifetime telemetry (RFC3339, "" = never):
	// the last time this provider completed a call. Displayed in the
	// console as "last ok"; never a health claim, never persisted.
	LastSuccessAt string `json:"last_success_at,omitempty"`
	// HasKey reports whether a credential is currently configured server-side
	// (populated on GetPublicSnapshot). Never carries secret material.
	HasKey bool `json:"has_key"`
	// SecretSource reports where the server-side credential originates:
	// "env" | "sealed_db" | "plaintext_db" | "none" (populated on GetPublicSnapshot).
	SecretSource string `json:"secret_source,omitempty"`
	// KeyStatus tracks credential standing: "missing" | "set" | "valid" | "invalid".
	KeyStatus string `json:"key_status,omitempty"`
	// LastFault holds the last categorized transport/credential failure summary.
	LastFault string `json:"last_fault,omitempty"`
	// LastCheckedAt is RFC3339 timestamp of the last /verify probe.
	LastCheckedAt string `json:"last_checked_at,omitempty"`
	// ClearKey is an input-only flag in PUT /api/llm/settings requesting explicit
	// removal of any stored credential instead of merging existing keys.
	ClearKey bool `json:"clear_key,omitempty"`
}

// CurrentSettingsVersion is the settings blob schema. LoadSettings stamps
// it on first boot and migrates older blobs forward (additive only —
// values are never rewritten by a migration; stale values surface through
// validation and resolve-time errors instead).
const CurrentSettingsVersion = 1

type ModelTarget struct {
	Provider ProviderType `json:"provider"`
	Model    string       `json:"model"`
}

type RoleModelAssignment struct {
	Role            string        `json:"role"`
	Provider        ProviderType  `json:"provider"`
	Model           string        `json:"model"`
	Fallback        []ModelTarget `json:"fallback,omitempty"`
	Temperature     *float64      `json:"temperature,omitempty"`
	TopP            *float64      `json:"top_p,omitempty"`
	Seed            *int64        `json:"seed,omitempty"`
	MaxTokens       *int          `json:"max_tokens,omitempty"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
}

// ResolvedRolePlan captures an ordered failover chain plus effective per-role
// sampling/budget parameters merged over global settings defaults.
type ResolvedRolePlan struct {
	Role            string
	Hops            []ModelTarget
	Temperature     float64
	TopP            *float64
	Seed            *int64
	MaxTokens       int
	HasMaxTokens    bool
	ReasoningEffort string
}

func cloneRoleAssignment(a RoleModelAssignment) RoleModelAssignment {
	out := a
	out.ReasoningEffort = strings.ToLower(strings.TrimSpace(a.ReasoningEffort))
	if len(a.Fallback) > 0 {
		out.Fallback = append([]ModelTarget(nil), a.Fallback...)
	} else {
		out.Fallback = nil
	}
	if a.Temperature != nil {
		v := *a.Temperature
		out.Temperature = &v
	}
	if a.TopP != nil {
		v := *a.TopP
		out.TopP = &v
	}
	if a.Seed != nil {
		v := *a.Seed
		out.Seed = &v
	}
	if a.MaxTokens != nil {
		v := *a.MaxTokens
		out.MaxTokens = &v
	}
	return out
}

func roleAssignmentsEqual(a, b RoleModelAssignment) bool {
	return reflect.DeepEqual(cloneRoleAssignment(a), cloneRoleAssignment(b))
}

func validReasoningEffort(effort string) bool {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "", "none", "low", "medium", "high":
		return true
	default:
		return false
	}
}

type Settings struct {
	mu            sync.RWMutex
	secretSources map[ProviderType]string
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
	RoleCatalog     []RoleMeta                      `json:"role_catalog,omitempty"`
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
	geminiKey, _ := providerEnvValue(ProviderGoogle)
	customBase := strings.TrimSpace(os.Getenv("CUSTOM_LLM_BASE_URL"))
	if customBase == "" {
		customBase = "http://127.0.0.1:8080/v1"
	} else if cleaned, _, err := ValidateEndpointURL(ProviderCustom, customBase); err == nil && cleaned != "" {
		customBase = cleaned
	} else {
		customBase = "http://127.0.0.1:8080/v1"
	}
	customKey := os.Getenv("CUSTOM_LLM_API_KEY")

	providers := map[ProviderType]ProviderConfig{
		ProviderOMLX: {
			Type:         ProviderOMLX,
			Name:         "oMLX Local LLM Engine (Apple Silicon)",
			BaseURL:      "http://127.0.0.1:8787/v1",
			DefaultModel: os.Getenv("OMLX_MODEL"),
			Wire:         WireChatCompletions,
			Enabled:      true,
			IsLocal:      true,
			TimeoutSec:   30,
		},
		ProviderOllama: {
			Type:         ProviderOllama,
			Name:         "Ollama Local Engine",
			BaseURL:      "http://127.0.0.1:11434/v1",
			DefaultModel: os.Getenv("OLLAMA_MODEL"),
			Wire:         WireOllamaNative,
			Enabled:      true,
			IsLocal:      true,
			TimeoutSec:   30,
		},
		ProviderLMStudio: {
			Type:         ProviderLMStudio,
			Name:         "LM Studio Local Engine",
			BaseURL:      "http://127.0.0.1:1234/v1",
			DefaultModel: os.Getenv("LMSTUDIO_MODEL"),
			Wire:         WireChatCompletions,
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
			Wire:         WireAnthropicMessages,
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
			Wire:         WireOpenAIResponses,
			Enabled:      os.Getenv("OPENAI_API_KEY") != "",
			IsLocal:      false,
			TimeoutSec:   60,
		},
		ProviderGoogle: {
			Type:         ProviderGoogle,
			Name:         "Google Gemini API",
			BaseURL:      "https://generativelanguage.googleapis.com/v1beta",
			APIKey:       geminiKey,
			DefaultModel: os.Getenv("GEMINI_MODEL"),
			Wire:         WireGemini,
			Enabled:      geminiKey != "",
			IsLocal:      false,
			TimeoutSec:   60,
		},
		ProviderOpenRouter: {
			Type:         ProviderOpenRouter,
			Name:         "OpenRouter Inference",
			BaseURL:      "https://openrouter.ai/api/v1",
			APIKey:       os.Getenv("OPENROUTER_API_KEY"),
			DefaultModel: os.Getenv("OPENROUTER_MODEL"),
			Wire:         WireChatCompletions,
			Enabled:      os.Getenv("OPENROUTER_API_KEY") != "",
			IsLocal:      false,
			TimeoutSec:   120,
		},
		ProviderXAI: {
			Type:         ProviderXAI,
			Name:         "xAI Grok API",
			BaseURL:      "https://api.x.ai/v1",
			APIKey:       os.Getenv("XAI_API_KEY"),
			DefaultModel: os.Getenv("XAI_MODEL"),
			Wire:         WireChatCompletions,
			Enabled:      os.Getenv("XAI_API_KEY") != "",
			IsLocal:      false,
			TimeoutSec:   120,
		},
		ProviderMistral: {
			Type:         ProviderMistral,
			Name:         "Mistral AI API",
			BaseURL:      "https://api.mistral.ai/v1",
			APIKey:       os.Getenv("MISTRAL_API_KEY"),
			DefaultModel: os.Getenv("MISTRAL_MODEL"),
			Wire:         WireChatCompletions,
			Enabled:      os.Getenv("MISTRAL_API_KEY") != "",
			IsLocal:      false,
			TimeoutSec:   60,
		},
		ProviderGroq: {
			Type:         ProviderGroq,
			Name:         "Groq Cloud API",
			BaseURL:      "https://api.groq.com/openai/v1",
			APIKey:       os.Getenv("GROQ_API_KEY"),
			DefaultModel: os.Getenv("GROQ_MODEL"),
			Wire:         WireChatCompletions,
			Enabled:      os.Getenv("GROQ_API_KEY") != "",
			IsLocal:      false,
			TimeoutSec:   60,
		},
		ProviderOpenCode: {
			Type:         ProviderOpenCode,
			Name:         "OpenCode Hosted Inference",
			BaseURL:      "https://opencode.ai/inference/openai/v1",
			APIKey:       os.Getenv("OPENCODE_API_KEY"),
			DefaultModel: os.Getenv("OPENCODE_MODEL"),
			Wire:         WireChatCompletions,
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
			Wire:         WireChatCompletions,
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
			Wire:         WireChatCompletions,
			Enabled:      true,
			IsLocal:      true,
			TimeoutSec:   30,
		},
		ProviderCustom: {
			Type:         ProviderCustom,
			Name:         "Custom OpenAI-Compatible Endpoint",
			BaseURL:      customBase,
			APIKey:       customKey,
			DefaultModel: os.Getenv("CUSTOM_LLM_MODEL"),
			Wire:         WireChatCompletions,
			Enabled:      os.Getenv("CUSTOM_LLM_BASE_URL") != "",
			IsLocal:      IsLocalEndpoint(customBase),
			TimeoutSec:   60,
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

	sources := make(map[ProviderType]string, len(providers))
	for pt, pc := range providers {
		if pc.APIKey != "" {
			sources[pt] = SecretSourceEnv
		} else {
			sources[pt] = SecretSourceNone
		}
	}

	return &Settings{
		secretSources:  sources,
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
	if err := hydrateLoadedProviders(&s); err != nil {
		return nil, err
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

// hydrateLoadedProviders unseals persisted credentials, records their
// storage posture (sealed_db vs plaintext_db vs none), and zeroes transient fields.
func hydrateLoadedProviders(s *Settings) error {
	s.secretSources = make(map[ProviderType]string, len(s.Providers))
	for pt, pc := range s.Providers {
		rawKey := pc.APIKey
		switch {
		case rawKey == "":
			s.secretSources[pt] = SecretSourceNone
		case strings.HasPrefix(rawKey, sealedPrefix):
			s.secretSources[pt] = SecretSourceSealedDB
		default:
			s.secretSources[pt] = SecretSourcePlaintextDB
		}
		plain, derr := openSecret(rawKey)
		if derr != nil {
			return fmt.Errorf("provider %s: %w", pt, derr)
		}
		pc.APIKey = plain
		pc.HasKey = false
		pc.SecretSource = ""
		pc.ClearKey = false
		s.Providers[pt] = pc
	}
	return nil
}

// LoadSettingsReadOnly loads persisted settings without creating/migrating
// the table. CLI audit paths (kubonad rates-audit) use it against a read-only
// ledger; a missing table is a fresh state, not an error.
func LoadSettingsReadOnly(db *sql.DB) (*Settings, error) {
	var valJSON string
	err := db.QueryRow("SELECT value_json FROM llm_settings WHERE key = 'active_settings'").Scan(&valJSON)
	if err == sql.ErrNoRows {
		return DefaultSettings(), nil
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return DefaultSettings(), nil
		}
		return nil, err
	}
	var s Settings
	if err := json.Unmarshal([]byte(valJSON), &s); err != nil {
		return nil, fmt.Errorf("stored LLM settings are unparseable: %w", err)
	}
	if err := hydrateLoadedProviders(&s); err != nil {
		return nil, err
	}
	if s.SchemaVersion > CurrentSettingsVersion {
		return nil, fmt.Errorf("llm settings schema v%d is newer than this binary (v%d)", s.SchemaVersion, CurrentSettingsVersion)
	}
	return &s, nil
}

// Save persists the current settings into SQLite
func (s *Settings) Save(db *sql.DB) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	storage := s.snapshot(false)
	for pt, pc := range storage.Providers {
		sealed, serr := sealSecret(pc.APIKey)
		if serr != nil {
			return serr
		}
		pc.APIKey = sealed
		storage.Providers[pt] = pc
	}
	data, err := json.Marshal(&storage)
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
	if s.secretSources == nil {
		s.secretSources = make(map[ProviderType]string)
	}
	changed := false
	for pt, pc := range defs {
		existing, ok := s.Providers[pt]
		if !ok {
			if pc.APIKey == "" {
				pc.APIKey, _ = providerEnvValue(pt)
			}
			if pc.APIKey != "" {
				s.secretSources[pt] = SecretSourceEnv
			} else {
				s.secretSources[pt] = SecretSourceNone
			}
			// Cloud providers enable on key presence (locals are always
			// on): a keyed cloud provider with no model still resolves
			// loudly at call time (missing model names its env var).
			if !pc.IsLocal && pt != ProviderCustom {
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
			if envKey, okEnv := providerEnvValue(pt); okEnv && envKey != "" {
				existing.APIKey = envKey
				s.secretSources[pt] = SecretSourceEnv
				if !existing.IsLocal && pt != ProviderCustom {
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
		if ok && roleProv.Enabled && !roleProv.IsLocal && !cloudLLMOptIn() {
			return fmt.Errorf("production: role %q resolves to non-local provider %s and requires explicit KUBONA_LLM_CLOUD=1 opt-in (prompts would leave the box)", role, assign.Provider)
		}
		for _, fb := range assign.Fallback {
			fbProv, fbOk := snap.Providers[fb.Provider]
			if fbOk && fbProv.Enabled && !fbProv.IsLocal && !cloudLLMOptIn() {
				return fmt.Errorf("production: role %q fallback resolves to non-local provider %s and requires explicit KUBONA_LLM_CLOUD=1 opt-in (prompts would leave the box)", role, fb.Provider)
			}
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

func (s *Settings) computeSecretSourceLocked(pt ProviderType, apiKey string) string {
	if apiKey == "" {
		return SecretSourceNone
	}
	if s.secretSources != nil {
		if src := s.secretSources[pt]; src != "" && src != SecretSourceNone {
			return src
		}
	}
	if envKey, ok := providerEnvValue(pt); ok && envKey == apiKey {
		return SecretSourceEnv
	}
	if HasSecretKey() {
		return SecretSourceSealedDB
	}
	return SecretSourcePlaintextDB
}

func baselineKeyStatus(pc ProviderConfig) string {
	if pc.KeyStatus == KeyStatusValid || pc.KeyStatus == KeyStatusInvalid {
		return pc.KeyStatus
	}
	if pc.APIKey != "" {
		return KeyStatusSet
	}
	if !pc.IsLocal {
		return KeyStatusMissing
	}
	return ""
}

func knownProviderList(provs map[ProviderType]ProviderConfig) string {
	names := make([]string, 0, len(provs))
	for pt := range provs {
		names = append(names, string(pt))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func (s *Settings) snapshot(redact bool) Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copyProviders := make(map[ProviderType]ProviderConfig, len(s.Providers))
	for k, v := range s.Providers {
		if redact {
			v.HasKey = v.APIKey != ""
			v.SecretSource = s.computeSecretSourceLocked(k, v.APIKey)
			v.KeyStatus = baselineKeyStatus(v)
			v.APIKey = ""
		} else {
			v.HasKey = false
			v.SecretSource = ""
		}
		v.ClearKey = false
		copyProviders[k] = v
	}

	copyRoles := make(map[string]RoleModelAssignment, len(s.RoleAssignments))
	for k, v := range s.RoleAssignments {
		copyRoles[k] = cloneRoleAssignment(v)
	}

	var roleCatalog []RoleMeta
	if redact {
		roleCatalog = KnownRoleCatalog()
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
		RoleCatalog:         roleCatalog,
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
	s.mu.RLock()
	currentRoles := s.RoleAssignments
	currentProvs := s.Providers
	curActiveModel := s.ActiveModel
	// Snapshot current secret sources under lock.
	currentSources := make(map[ProviderType]string, len(s.secretSources))
	for k, v := range s.secretSources {
		currentSources[k] = v
	}
	s.mu.RUnlock()

	if strings.TrimSpace(update.ActiveModel) == "" && strings.TrimSpace(curActiveModel) != "" {
		return fmt.Errorf("active_model must not be empty: refusing (clearing the fallback breaks unknown-role resolution)")
	}

	var preparedProvs map[ProviderType]ProviderConfig
	var preparedSources map[ProviderType]string
	provsForRoleCheck := currentProvs
	if update.Providers != nil {
		// No partial provider writes: a struct without BaseURL/TimeoutSec
		// is a fragment, and storing it would silently zero Enabled,
		// BaseURL, and the model (G13). Validate everything before
		// mutating any field on Settings.
		preparedProvs = make(map[ProviderType]ProviderConfig, len(currentProvs))
		preparedSources = make(map[ProviderType]string, len(currentProvs))
		for k, v := range currentProvs {
			preparedProvs[k] = v
			if src, ok := currentSources[k]; ok {
				preparedSources[k] = src
			}
		}
		knownList := knownProviderList(currentProvs)
		for pt, pc := range update.Providers {
			existing, ok := currentProvs[pt]
			if !ok {
				return fmt.Errorf("provider %q unknown: refusing (known: %s)", pt, knownList)
			}
			if strings.TrimSpace(pc.BaseURL) == "" || pc.TimeoutSec == 0 {
				return fmt.Errorf("provider %q: partial entry refuses (send the full provider object — GET first, modify, PUT back)", pt)
			}
			if !validWireType(pc.Wire) {
				return fmt.Errorf("provider %q: unsupported wire protocol %q: refusing", pt, pc.Wire)
			}
			cleanedURL, isLocalURL, uerr := ValidateEndpointURL(pt, pc.BaseURL)
			if uerr != nil {
				return uerr
			}
			if cleanedURL != "" {
				pc.BaseURL = cleanedURL
			}
			switch {
			case pt == ProviderCustom:
				pc.IsLocal = isLocalURL
			case isLocalProvider(pt):
				pc.IsLocal = isLocalURL
			default:
				pc.IsLocal = false
			}
			urlChanged := pc.BaseURL != existing.BaseURL
			if pc.Wire == "" {
				if urlChanged {
					pc.Wire = ResolveWire(pt, pc)
				} else {
					pc.Wire = existing.Wire
				}
			}
			clearRequested := pc.ClearKey
			pc.HasKey = false
			pc.SecretSource = ""
			pc.ClearKey = false
			if pc.LastSuccessAt == "" {
				pc.LastSuccessAt = existing.LastSuccessAt
			}
			if clearRequested {
				pc.APIKey = ""
				preparedSources[pt] = SecretSourceNone
				pc.KeyStatus = KeyStatusMissing
				pc.LastCheckedAt = ""
				pc.LastFault = ""
				if !pc.IsLocal {
					pc.Enabled = false
				}
			} else if pc.APIKey != "" {
				keyChanged := pc.APIKey != existing.APIKey
				if HasSecretKey() {
					preparedSources[pt] = SecretSourceSealedDB
				} else {
					preparedSources[pt] = SecretSourcePlaintextDB
				}
				if keyChanged || urlChanged {
					pc.KeyStatus = KeyStatusSet
					pc.LastCheckedAt = ""
					pc.LastFault = ""
				} else if pc.KeyStatus == "" {
					pc.KeyStatus = existing.KeyStatus
					pc.LastCheckedAt = existing.LastCheckedAt
					pc.LastFault = existing.LastFault
				}
			} else {
				// Empty incoming APIKey preserves existing key (or env key) when clear_key is false.
				if existing.APIKey != "" {
					pc.APIKey = existing.APIKey
					if src := currentSources[pt]; src != "" {
						preparedSources[pt] = src
					}
				} else if envKey, okEnv := providerEnvValue(pt); okEnv && envKey != "" {
					pc.APIKey = envKey
					preparedSources[pt] = SecretSourceEnv
				} else {
					preparedSources[pt] = SecretSourceNone
				}
				if urlChanged {
					pc.KeyStatus = ""
					pc.LastCheckedAt = ""
					pc.LastFault = ""
				} else if pc.KeyStatus == "" {
					pc.KeyStatus = existing.KeyStatus
					if pc.LastCheckedAt == "" {
						pc.LastCheckedAt = existing.LastCheckedAt
					}
					if pc.LastFault == "" {
						pc.LastFault = existing.LastFault
					}
				}
			}
			preparedProvs[pt] = pc
		}
		provsForRoleCheck = preparedProvs
	}

	// Role assignments bind calls to exact providers+models: unknown
	// providers and empty models are typos, not configuration. Refuse
	// instead of failing (or silently falling back) at call time. Only
	// changed-or-new entries are validated.
	var preparedRoles map[string]RoleModelAssignment
	if update.RoleAssignments != nil {
		preparedRoles = make(map[string]RoleModelAssignment, len(update.RoleAssignments))
		for role, rawAssign := range update.RoleAssignments {
			assign := cloneRoleAssignment(rawAssign)
			if cur, ok := currentRoles[role]; ok && roleAssignmentsEqual(cur, assign) {
				preparedRoles[role] = assign
				continue
			}
			prov, ok := provsForRoleCheck[assign.Provider]
			if !ok {
				return fmt.Errorf("role %q points at unknown provider %q: refusing", role, assign.Provider)
			}
			if strings.TrimSpace(assign.Model) == "" && prov.Enabled {
				return fmt.Errorf("role %q has an empty model on enabled provider %q: refusing (set one or disable the provider)", role, assign.Provider)
			}
			for i, fb := range assign.Fallback {
				if _, ok := provsForRoleCheck[fb.Provider]; !ok {
					return fmt.Errorf("role %q fallback[%d] points at unknown provider %q: refusing", role, i, fb.Provider)
				}
				if strings.TrimSpace(fb.Model) == "" {
					return fmt.Errorf("role %q fallback[%d] on provider %q has an empty model: refusing", role, i, fb.Provider)
				}
			}
			if assign.Temperature != nil {
				tVal := *assign.Temperature
				if math.IsNaN(tVal) || math.IsInf(tVal, 0) || tVal < 0 || tVal > 2 {
					return fmt.Errorf("role %q temperature must be finite within [0, 2]: refusing", role)
				}
			}
			if assign.TopP != nil {
				pVal := *assign.TopP
				if math.IsNaN(pVal) || math.IsInf(pVal, 0) || pVal <= 0 || pVal > 1 {
					return fmt.Errorf("role %q top_p must be finite within (0, 1]: refusing", role)
				}
			}
			if assign.MaxTokens != nil && *assign.MaxTokens < 0 {
				return fmt.Errorf("role %q max_tokens must be >= 0: refusing", role)
			}
			if !validReasoningEffort(assign.ReasoningEffort) {
				return fmt.Errorf("role %q reasoning_effort %q invalid (allowed: none, low, medium, high): refusing", role, assign.ReasoningEffort)
			}
			preparedRoles[role] = assign
		}
	}

	s.mu.Lock()
	s.ActiveProvider = update.ActiveProvider
	s.ActiveModel = update.ActiveModel
	s.AirGapped = update.AirGapped
	s.Temperature = update.Temperature
	s.MaxTokens = update.MaxTokens
	s.DisableDSMLFallback = update.DisableDSMLFallback
	if preparedProvs != nil {
		s.Providers = preparedProvs
		if s.secretSources == nil {
			s.secretSources = make(map[ProviderType]string, len(preparedSources))
		}
		for k, v := range preparedSources {
			s.secretSources[k] = v
		}
	}
	if preparedRoles != nil {
		s.RoleAssignments = preparedRoles
	}
	s.mu.Unlock()

	if db != nil {
		return s.Save(db)
	}
	return nil
}

// ClearProviderKey explicitly removes a provider's stored API key and updates
// key standing and enablement safely without relying on empty-string heuristics.
func (s *Settings) ClearProviderKey(pt ProviderType, db *sql.DB) error {
	s.mu.Lock()
	pc, ok := s.Providers[pt]
	if !ok {
		known := knownProviderList(s.Providers)
		s.mu.Unlock()
		return fmt.Errorf("provider %q unknown: refusing (known: %s)", pt, known)
	}
	pc.APIKey = ""
	pc.HasKey = false
	pc.SecretSource = ""
	pc.ClearKey = false
	pc.KeyStatus = KeyStatusMissing
	pc.LastCheckedAt = ""
	pc.LastFault = ""
	if !pc.IsLocal {
		pc.Enabled = false
	}
	s.Providers[pt] = pc
	if s.secretSources == nil {
		s.secretSources = make(map[ProviderType]string)
	}
	s.secretSources[pt] = SecretSourceNone
	s.mu.Unlock()

	if db != nil {
		return s.Save(db)
	}
	return nil
}

// providerEnvKey maps providers to their primary credential env vars.
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
	case ProviderGoogle:
		return "GEMINI_API_KEY"
	case ProviderOpenRouter:
		return "OPENROUTER_API_KEY"
	case ProviderXAI:
		return "XAI_API_KEY"
	case ProviderMistral:
		return "MISTRAL_API_KEY"
	case ProviderGroq:
		return "GROQ_API_KEY"
	case ProviderStrata:
		return "STRATA_API_KEY"
	case ProviderCustom:
		return "CUSTOM_LLM_API_KEY"
	default:
		return ""
	}
}

func providerEnvValue(pt ProviderType) (string, bool) {
	if pt == ProviderGoogle {
		if v := os.Getenv("GEMINI_API_KEY"); v != "" {
			return v, true
		}
		if v := os.Getenv("GOOGLE_API_KEY"); v != "" {
			return v, true
		}
		return "", false
	}
	if name := providerEnvKey(pt); name != "" {
		if v := os.Getenv(name); v != "" {
			return v, true
		}
	}
	return "", false
}

// ResolveWire chooses the effective wire protocol for a configured provider.
func ResolveWire(pt ProviderType, pc ProviderConfig) WireType {
	if pc.Wire != "" {
		return pc.Wire
	}
	if pt == ProviderOpenAI {
		if strings.Contains(strings.ToLower(pc.BaseURL), "api.openai.com") {
			return WireOpenAIResponses
		}
		return WireChatCompletions
	}
	return DefaultWireFor(pt)
}

// IsProviderLocal checks both the provider type and configured endpoint URL
// so a custom or locally-labeled slot pointing off-box is never treated as $0 local.
func (s *Settings) IsProviderLocal(pt ProviderType) bool {
	if s == nil {
		return isLocalProvider(pt)
	}
	s.mu.RLock()
	pc, ok := s.Providers[pt]
	s.mu.RUnlock()
	if !ok {
		return isLocalProvider(pt)
	}
	if pt == ProviderCustom {
		return IsLocalEndpoint(pc.BaseURL)
	}
	if !isLocalProvider(pt) {
		return false
	}
	if strings.TrimSpace(pc.BaseURL) == "" {
		return true
	}
	return IsLocalEndpoint(pc.BaseURL)
}

// RecordProviderCheck records the outcome of a verification probe or an
// authoritative runtime standing change (such as a 401 credential failure).
func (s *Settings) RecordProviderCheck(pt ProviderType, status, fault, checkedAt string, db *sql.DB) {
	if s == nil {
		return
	}
	s.mu.Lock()
	pc, ok := s.Providers[pt]
	if ok {
		pc.KeyStatus = status
		pc.LastFault = fault
		if checkedAt != "" {
			pc.LastCheckedAt = checkedAt
		}
		s.Providers[pt] = pc
	}
	s.mu.Unlock()
	if db != nil && ok {
		_ = s.Save(db)
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

// ResolveRolePlan returns the ordered [primary, ...fallback] hops and merged
// sampling/budget parameters for a role. Unknown or unassigned roles fall back
// to the active provider+model and global settings defaults.
func (s *Settings) ResolveRolePlan(role string) ResolvedRolePlan {
	s.mu.RLock()
	defer s.mu.RUnlock()

	plan := ResolvedRolePlan{
		Role:        role,
		Temperature: s.Temperature,
		MaxTokens:   s.MaxTokens,
	}
	if assign, ok := s.RoleAssignments[role]; ok {
		hops := make([]ModelTarget, 0, 1+len(assign.Fallback))
		hops = append(hops, ModelTarget{Provider: assign.Provider, Model: assign.Model})
		hops = append(hops, assign.Fallback...)
		plan.Hops = hops
		if assign.Temperature != nil {
			plan.Temperature = *assign.Temperature
		}
		if assign.TopP != nil {
			v := *assign.TopP
			plan.TopP = &v
		}
		if assign.Seed != nil {
			v := *assign.Seed
			plan.Seed = &v
		}
		if assign.MaxTokens != nil {
			plan.MaxTokens = *assign.MaxTokens
			plan.HasMaxTokens = true
		}
		plan.ReasoningEffort = strings.ToLower(strings.TrimSpace(assign.ReasoningEffort))
		return plan
	}
	plan.Hops = []ModelTarget{{Provider: s.ActiveProvider, Model: s.ActiveModel}}
	return plan
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
		if !pc.IsLocal || pc.APIKey != "" {
			pc.KeyStatus = KeyStatusValid
		}
		pc.LastFault = ""
		s.Providers[pt] = pc
	}
}
