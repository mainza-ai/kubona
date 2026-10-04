package llm

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
)

type ProviderType string

const (
	ProviderOMLX      ProviderType = "omlx"
	ProviderOllama    ProviderType = "ollama"
	ProviderLMStudio  ProviderType = "lmstudio"
	ProviderAnthropic ProviderType = "anthropic"
	ProviderOpenAI    ProviderType = "openai"
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
	ActiveProvider  ProviderType                   `json:"active_provider"`
	ActiveModel     string                         `json:"active_model"`
	AirGapped       bool                           `json:"air_gapped"`
	Temperature     float64                        `json:"temperature"`
	MaxTokens       int                            `json:"max_tokens"`
	Providers       map[ProviderType]ProviderConfig `json:"providers"`
	RoleAssignments map[string]RoleModelAssignment `json:"role_assignments"`
}

func DefaultSettings() *Settings {
	return &Settings{
		ActiveProvider: ProviderOMLX,
		ActiveModel:    "Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit",
		AirGapped:      true,
		Temperature:    0.2,
		MaxTokens:      2048,
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
		},
		RoleAssignments: map[string]RoleModelAssignment{
			"copilot": {
				Role:     "copilot",
				Provider: ProviderOMLX,
				Model:    "Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit",
			},
			"epistemic-scribe": {
				Role:     "epistemic-scribe",
				Provider: ProviderOMLX,
				Model:    "Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit",
			},
			"ansatz-scout": {
				Role:     "ansatz-scout",
				Provider: ProviderOMLX,
				Model:    "Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit",
			},
			"disconfirmation-officer": {
				Role:     "disconfirmation-officer",
				Provider: ProviderOMLX,
				Model:    "Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit",
			},
			"device-auditor": {
				Role:     "device-auditor",
				Provider: ProviderOMLX,
				Model:    "Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit",
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
	s.mu.Lock()
	s.ActiveProvider = update.ActiveProvider
	s.ActiveModel = update.ActiveModel
	s.AirGapped = update.AirGapped
	s.Temperature = update.Temperature
	s.MaxTokens = update.MaxTokens
	if update.Providers != nil {
		s.Providers = update.Providers
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
