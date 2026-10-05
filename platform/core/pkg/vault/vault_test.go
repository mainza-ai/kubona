package vault

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVaultLeaseLifecycle(t *testing.T) {
	client := NewVaultClient()

	// Issue lease
	lease, err := client.IssueLease("ibm_quantum", "tenant-alpha", "super-secret-api-token", 10*time.Second)
	if err != nil {
		t.Fatalf("failed to issue lease: %v", err)
	}

	if lease.Scope != "ibm_quantum" || lease.TenantID != "tenant-alpha" {
		t.Fatalf("unexpected lease metadata: %+v", lease)
	}

	// Validate before consumption
	valid, reason := client.ValidateLease(lease.LeaseID)
	if !valid {
		t.Fatalf("expected lease to be valid, got false: %s", reason)
	}

	// Consume lease
	secret, err := client.ConsumeLease(lease.LeaseID)
	if err != nil {
		t.Fatalf("failed to consume lease: %v", err)
	}
	if secret != "super-secret-api-token" {
		t.Fatalf("unexpected secret returned: %s", secret)
	}

	// Re-consuming same lease must fail (single-use invariant)
	_, err = client.ConsumeLease(lease.LeaseID)
	if err == nil {
		t.Fatalf("expected error re-consuming single-use lease, got nil")
	}

	// Issue another lease and revoke it
	lease2, _ := client.IssueLease("rigetti", "tenant-beta", "rigetti-secret", 5*time.Second)
	if err := client.RevokeLease(lease2.LeaseID); err != nil {
		t.Fatalf("failed to revoke lease: %v", err)
	}

	valid, _ = client.ValidateLease(lease2.LeaseID)
	if valid {
		t.Fatalf("expected revoked lease to be invalid")
	}
}

func TestOpenBaoClientAppRoleLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/approle/login" && r.Method == http.MethodPost {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if body["role_id"] == "kubona-role" && body["secret_id"] == "kubona-secret" {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"auth": map[string]any{
						"client_token":   "s.kubona-test-token-12345",
						"lease_duration": 3600,
						"renewable":      true,
					},
				})
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewOpenBaoClient(OpenBaoConfig{
		Addr:     server.URL,
		RoleID:   "kubona-role",
		SecretID: "kubona-secret",
	})
	if err != nil {
		t.Fatalf("failed to create openbao client: %v", err)
	}

	ctx := context.Background()
	if err := client.Login(ctx); err != nil {
		t.Fatalf("failed to login via approle: %v", err)
	}

	if client.Token() != "s.kubona-test-token-12345" {
		t.Fatalf("unexpected token: %s", client.Token())
	}
}

func TestOpenBaoClientGetSecretAndRevoke(t *testing.T) {
	revokedLeases := make(map[string]bool)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Vault-Token")
		if token != "s.valid-token" {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		if r.URL.Path == "/v1/secret/data/quantum/ibm_quantum" && r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"data": map[string]any{
						"api_key":  "ibm-quantum-secret-key-xyz",
						"instance": "ibm-q/open/main",
					},
				},
				"lease_id":       "lease_ibm_quantum_789",
				"lease_duration": 60,
				"renewable":      false,
			})
			return
		}

		if r.URL.Path == "/v1/sys/leases/revoke" && r.Method == http.MethodPut {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			revokedLeases[body["lease_id"]] = true
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewOpenBaoClient(OpenBaoConfig{
		Addr:  server.URL,
		Token: "s.valid-token",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()

	// 1. Fetch provider credentials
	apiKey, leaseID, err := client.GetProviderCredential(ctx, "ibm_quantum")
	if err != nil {
		t.Fatalf("failed to get provider credential: %v", err)
	}

	if apiKey != "ibm-quantum-secret-key-xyz" {
		t.Fatalf("unexpected api_key: %s", apiKey)
	}
	if leaseID != "lease_ibm_quantum_789" {
		t.Fatalf("unexpected lease_id: %s", leaseID)
	}

	// 2. Revoke the lease
	if err := client.RevokeLease(ctx, leaseID); err != nil {
		t.Fatalf("failed to revoke lease: %v", err)
	}

	if !revokedLeases[leaseID] {
		t.Fatalf("expected lease '%s' to be recorded as revoked", leaseID)
	}

	// 3. Error case: Not found
	_, _, err = client.GetProviderCredential(ctx, "non_existent_provider")
	if !errors.Is(err, ErrVaultNotFound) {
		t.Fatalf("expected ErrVaultNotFound, got: %v", err)
	}
}

func TestOpenBaoClientForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client, _ := NewOpenBaoClient(OpenBaoConfig{
		Addr:  server.URL,
		Token: "s.invalid-token",
	})

	ctx := context.Background()
	_, err := client.GetSecret(ctx, "secret/data/quantum/dwave")
	if !errors.Is(err, ErrVaultUnauthorized) {
		t.Fatalf("expected ErrVaultUnauthorized, got: %v", err)
	}
}
