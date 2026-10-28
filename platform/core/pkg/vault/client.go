package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	ErrVaultUnauthorized = errors.New("openbao unauthorized or token expired")
	ErrVaultNotFound     = errors.New("requested secret not found in openbao")
	ErrVaultRevokeFailed = errors.New("failed to revoke lease in openbao")
	ErrVaultInvalidInput = errors.New("invalid openbao client configuration")
)

// OpenBaoConfig declares connection parameters for OpenBao sidecar
type OpenBaoConfig struct {
	Addr       string        `json:"addr"`      // e.g. "http://localhost:8200"
	RoleID     string        `json:"role_id"`   // AppRole role_id
	SecretID   string        `json:"secret_id"` // AppRole secret_id
	Token      string        `json:"token"`     // Direct client token (dev/override)
	Timeout    time.Duration `json:"timeout"`
	HTTPClient *http.Client  `json:"-"`
}

// OpenBaoClient interacts directly with OpenBao / HashiCorp Vault REST APIs
type OpenBaoClient struct {
	addr       string
	roleID     string
	secretID   string
	token      string
	httpClient *http.Client
	mu         sync.RWMutex
}

// NewOpenBaoClient instantiates an authenticated client for OpenBao
func NewOpenBaoClient(cfg OpenBaoConfig) (*OpenBaoClient, error) {
	addr := strings.TrimRight(cfg.Addr, "/")
	if addr == "" {
		addr = "http://localhost:8200"
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: timeout,
		}
	}

	return &OpenBaoClient{
		addr:       addr,
		roleID:     cfg.RoleID,
		secretID:   cfg.SecretID,
		token:      cfg.Token,
		httpClient: httpClient,
	}, nil
}

// Login performs AppRole authentication if token is not already set
func (c *OpenBaoClient) Login(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.roleID == "" || c.secretID == "" {
		if c.token != "" {
			return nil // Token already supplied
		}
		return fmt.Errorf("%w: role_id and secret_id required for AppRole authentication", ErrVaultInvalidInput)
	}

	loginPayload := map[string]string{
		"role_id":   c.roleID,
		"secret_id": c.secretID,
	}
	body, err := json.Marshal(loginPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal login payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.addr+"/v1/auth/approle/login", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute approle login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: status %d, body: %s", ErrVaultUnauthorized, resp.StatusCode, string(respBody))
	}

	var result struct {
		Auth struct {
			ClientToken   string `json:"client_token"`
			LeaseDuration int    `json:"lease_duration"`
			Renewable     bool   `json:"renewable"`
		} `json:"auth"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode login response: %w", err)
	}

	if result.Auth.ClientToken == "" {
		return fmt.Errorf("%w: response missing client_token", ErrVaultUnauthorized)
	}

	c.token = result.Auth.ClientToken
	return nil
}

// Token returns the current client token
func (c *OpenBaoClient) Token() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

// SetToken manually overrides the client token
func (c *OpenBaoClient) SetToken(tok string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = tok
}

// SecretData captures dynamic secret payloads and their associated lease ID
type SecretData struct {
	Data          map[string]any `json:"data"`
	LeaseID       string         `json:"lease_id"`
	LeaseDuration int            `json:"lease_duration"`
	Renewable     bool           `json:"renewable"`
}

// GetSecret reads a secret from OpenBao KV-v2 engine
func (c *OpenBaoClient) GetSecret(ctx context.Context, path string) (*SecretData, error) {
	c.mu.RLock()
	token := c.token
	c.mu.RUnlock()

	if token == "" {
		if err := c.Login(ctx); err != nil {
			return nil, err
		}
		c.mu.RLock()
		token = c.token
		c.mu.RUnlock()
	}

	fullURL := fmt.Sprintf("%s/v1/%s", c.addr, strings.TrimLeft(path, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch secret from %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", ErrVaultNotFound, path)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("%w: access denied for %s", ErrVaultUnauthorized, path)
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openbao error (%d): %s", resp.StatusCode, string(respBody))
	}

	var rawResp struct {
		Data struct {
			Data map[string]any `json:"data"`
		} `json:"data"`
		LeaseID       string `json:"lease_id"`
		LeaseDuration int    `json:"lease_duration"`
		Renewable     bool   `json:"renewable"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawResp); err != nil {
		return nil, fmt.Errorf("failed to decode secret payload: %w", err)
	}

	data := rawResp.Data.Data
	if data == nil {
		data = make(map[string]any)
	}

	return &SecretData{
		Data:          data,
		LeaseID:       rawResp.LeaseID,
		LeaseDuration: rawResp.LeaseDuration,
		Renewable:     rawResp.Renewable,
	}, nil
}

// GetProviderCredential fetches a dynamic credential for a QPU provider
func (c *OpenBaoClient) GetProviderCredential(ctx context.Context, provider string) (apiKey string, leaseID string, err error) {
	path := fmt.Sprintf("secret/data/quantum/%s", provider)
	secret, err := c.GetSecret(ctx, path)
	if err != nil {
		return "", "", err
	}

	val, exists := secret.Data["api_key"]
	if !exists {
		val, exists = secret.Data["token"]
	}
	if !exists {
		return "", secret.LeaseID, fmt.Errorf("credential secret missing 'api_key' or 'token' field for provider '%s'", provider)
	}

	keyStr, ok := val.(string)
	if !ok {
		return "", secret.LeaseID, fmt.Errorf("credential field is not a valid string for provider '%s'", provider)
	}

	return keyStr, secret.LeaseID, nil
}

// RevokeLease immediately terminates an active credential lease.
// An empty lease id REFUSES (fail-closed): silently accepting "" used to
// report success while revoking nothing.
func (c *OpenBaoClient) RevokeLease(ctx context.Context, leaseID string) error {
	if leaseID == "" {
		return fmt.Errorf("%w: refusing to revoke an empty lease id", ErrVaultRevokeFailed)
	}

	c.mu.RLock()
	token := c.token
	c.mu.RUnlock()

	body, err := json.Marshal(map[string]string{
		"lease_id": leaseID,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.addr+"/v1/sys/leases/revoke", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-Vault-Token", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute lease revocation: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: status %d, body: %s", ErrVaultRevokeFailed, resp.StatusCode, string(respBody))
	}

	return nil
}
