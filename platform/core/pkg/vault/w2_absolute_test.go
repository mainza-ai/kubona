package vault_test

import (
	"path/filepath"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/vault"
)

// W2.20: revocation durability paths must be absolute — a relative file
// would fork tombstones across working directories.
func TestW220PersistenceRequiresAbsolute(t *testing.T) {
	if _, err := vault.NewVaultClientWithPersistence("relative/revocations.log"); err == nil {
		t.Error("relative persistence path must refuse")
	}
	abs := filepath.Join(t.TempDir(), "revocations.log")
	if _, err := vault.NewVaultClientWithPersistence(abs); err != nil {
		t.Errorf("absolute path must validate: %v", err)
	}
}
