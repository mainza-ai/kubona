package llm

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
)

type ProviderType string

const (
	ProviderOMLX      ProviderType = "omlx"
	ProviderOllama    ProviderType = "ollama"
	ProviderLMStudio  ProviderType = "lmstudio"
	ProviderAnthropic ProviderType = "anthropic"
	ProviderOpenAI    ProviderType = "openai"
	ProviderOpenCode  ProviderType = "opencode"
)

type ProviderConfig struct {
	Type         ProviderType `json:"type"`
	Name         string       `json:"name"`
	BaseURL      string       `json:"base_url"`
	APIKey       string       `json:"api_key,omitempty"`
	DefaultModel string       `json:"default_model"`
	Enabled      bool         `json:"enabled"`
	IsLocal      bool         `json:"is_local"`
	TimeoutSec   int          `json:"timeout_sec"`
}

type RoleModelAssignment struct {
	Role     string       `json:"role"`
	Provider ProviderType `json:"provider"`
	Model    string       `json:"model"`
}

type Settings struct {
	mu              sync.RWMutex
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
	// Development default (2026-10-11, operator-directed): OpenCode-hosted
	// deepseek-v4.1-flash for every role. Local oMLX stays enabled as the
	// air-gapped fallback. Production posture is the reverse (air-gapped,
	// local-only) — set air_gapped + local roles explicitly there.
	opencodeKey := os.Getenv("OPENCODE_API_KEY")
	return &Settings{
		ActiveProvider: ProviderOpenCode,
		ActiveModel:    "deepseek-v4.1-flash",
		AirGapped:      false,
		Temperature:    0.2,
		// MaxTokens 0 = unset: the field is omitted and the model generates
		// to EOS / its own context window. Any positive value is a per-role
		// ceiling, not a default — 2048 proved too low for tribunal turns,
		// and no fixed default fits all roles (2026-10-11).
		MaxTokens: 0,
		Providers: map[ProviderType]ProviderConfig{
			ProviderOMLX: {
				Type:         ProviderOMLX,
				Name:         "oMLX Local LLM Engine (Apple Silicon)",
				BaseURL:      "http://127.0.0.1:8787/v1",
				DefaultModel: "Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit",
				Enabled:      true,
				IsLocal:      true,
				TimeoutSec:   30,
			},
			ProviderOllama: {
				Type:         ProviderOllama,
				Name:         "Ollama Local Engine",
				BaseURL:      "http://127.0.0.1:11434/v1",
				DefaultModel: "qwen2.5:32b",
				Enabled:      true,
				IsLocal:      true,
				TimeoutSec:   30,
			},
			ProviderLMStudio: {
				Type:         ProviderLMStudio,
				Name:         "LM Studio Local Engine",
				BaseURL:      "http://127.0.0.1:1234/v1",
				DefaultModel: "local-model",
				Enabled:      true,
				IsLocal:      true,
				TimeoutSec:   30,
			},
			ProviderAnthropic: {
				Type:         ProviderAnthropic,
				Name:         "Anthropic Claude API",
				BaseURL:      "https://api.anthropic.com/v1",
				DefaultModel: "claude-3-7-sonnet-20250219",
				Enabled:      false,
				IsLocal:      false,
				TimeoutSec:   60,
			},
			ProviderOpenAI: {
				Type:         ProviderOpenAI,
				Name:         "OpenAI API",
				BaseURL:      "https://api.openai.com/v1",
				DefaultModel: "gpt-4o",
				Enabled:      false,
				IsLocal:      false,
				TimeoutSec:   60,
			},
			ProviderOpenCode: {
				Type:         ProviderOpenCode,
				Name:         "OpenCode Hosted Inference",
				BaseURL:      "https://opencode.ai/inference/openai/v1",
				APIKey:       opencodeKey,
				DefaultModel: "deepseek-v4.1-flash",
				Enabled:      opencodeKey != "",
				IsLocal:      false,
				TimeoutSec:   120,
			},
		},
		RoleAssignments: map[string]RoleModelAssignment{
			"copilot": {
				Role:     "copilot",
				Provider: ProviderOpenCode,
				Model:    "deepseek-v4.1-flash",
			},
			"epistemic-scribe": {
				Role:     "epistemic-scribe",
				Provider: ProviderOpenCode,
				Model:    "deepseek-v4.1-flash",
			},
			"ansatz-scout": {
				Role:     "ansatz-scout",
				Provider: ProviderOpenCode,
				Model:    "deepseek-v4.1-flash",
			},
			"disconfirmation-officer": {
				Role:     "disconfirmation-officer",
				Provider: ProviderOpenCode,
				Model:    "deepseek-v4.1-flash",
			},
			"device-auditor": {
				Role:     "device-auditor",
				Provider: ProviderOpenCode,
				Model:    "deepseek-v4.1-flash",
			},
		},
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

// LoadSettings loads persistent settings from SQLite or defaults
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
			if pc.Type == ProviderOpenCode {
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
				if pt == ProviderOpenCode {
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
	if prov.IsLocal {
		return nil
	}
	if os.Getenv("KUBONA_LLM_CLOUD") != "1" {
		return fmt.Errorf("production: non-local LLM provider %s requires explicit KUBONA_LLM_CLOUD=1 opt-in (prompts would leave the box)", snap.ActiveProvider)
	}
	return nil
}

func (s *Settings) GetSnapshot() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copyProviders := make(map[ProviderType]ProviderConfig)
	for k, v := range s.Providers {
		copyProviders[k] = v
	}

	copyRoles := make(map[string]RoleModelAssignment)
	for k, v := range s.RoleAssignments {
		copyRoles[k] = v
	}

	return Settings{
		ActiveProvider:  s.ActiveProvider,
		ActiveModel:     s.ActiveModel,
		AirGapped:       s.AirGapped,
		Temperature:     s.Temperature,
		MaxTokens:       s.MaxTokens,
		Providers:       copyProviders,
		RoleAssignments: copyRoles,
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
	s.mu.Lock()
	s.ActiveProvider = update.ActiveProvider
	s.ActiveModel = update.ActiveModel
	s.AirGapped = update.AirGapped
	s.Temperature = update.Temperature
	s.MaxTokens = update.MaxTokens
	if update.Providers != nil {
		// Never wipe a stored secret with an empty form field: an empty
		// incoming APIKey keeps the existing one (env-supplied keys survive
		// UI saves that don't include secrets).
		for pt, pc := range update.Providers {
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
	case ProviderOpenAI:
		return "OPENAI_API_KEY"
	case ProviderAnthropic:
		return "ANTHROPIC_API_KEY"
	default:
		return ""
	}
}

func (s *Settings) GetModelForRole(role string) (ProviderType, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if assign, ok := s.RoleAssignments[role]; ok {
		prov, provOk := s.Providers[assign.Provider]
		if provOk && prov.Enabled {
			if s.AirGapped && !prov.IsLocal {
				return "", "", fmt.Errorf("provider %s is blocked by Sovereign Air-Gap isolation", assign.Provider)
			}
			return assign.Provider, assign.Model, nil
		}
	}

	// Fallback to active provider
	prov, ok := s.Providers[s.ActiveProvider]
	if !ok || !prov.Enabled {
		return "", "", fmt.Errorf("active provider %s is disabled or unconfigured", s.ActiveProvider)
	}
	if s.AirGapped && !prov.IsLocal {
		return "", "", fmt.Errorf("active provider %s is blocked by Sovereign Air-Gap isolation", s.ActiveProvider)
	}

	return s.ActiveProvider, s.ActiveModel, nil
}
