// keygen prints operator-owned Ed25519 keys for a local private install.
//
// The reviewer key signs human mandate approvals; production refuses an
// ephemeral signer. The operator key is used by the bridge-side spend
// approval rail. Both are 32-byte seeds (64 hex chars), the same shape
// KUBONA_REVIEWER_PRIVKEY_HEX / KUBONA_OPERATOR_PRIVKEY_HEX accept.
//
//   kubonad keygen                 # reviewer key
//   kubonad keygen --kind operator # operator key
//   kubonad keygen --kind both     # both, labelled
//
// Secrets are printed to stdout by design. Write them to .env (gitignored)
// or a secret manager; never commit them.

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
)

func generateKeySeed() ([]byte, error) {
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		return nil, err
	}
	return seed, nil
}

func runKeygen(args []string) int {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	kind := fs.String("kind", "reviewer", "key kind: reviewer, operator, or both")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "keygen takes no positional arguments")
		return 2
	}
	var kinds []string
	switch *kind {
	case "reviewer", "operator":
		kinds = []string{*kind}
	case "both":
		kinds = []string{"reviewer", "operator"}
	default:
		fmt.Fprintf(os.Stderr, "unknown --kind %q: use reviewer, operator, or both\n", *kind)
		return 2
	}
	for _, k := range kinds {
		seed, err := generateKeySeed()
		if err != nil {
			fmt.Fprintf(os.Stderr, "key generation failed: %v\n", err)
			return 1
		}
		priv := ed25519.NewKeyFromSeed(seed)
		pub := priv.Public().(ed25519.PublicKey)
		env := "KUBONA_REVIEWER_PRIVKEY_HEX"
		if k == "operator" {
			env = "KUBONA_OPERATOR_PRIVKEY_HEX"
		}
		fmt.Printf("# %s key (secret; store in .env or a secret manager)\n", k)
		fmt.Printf("%s=%s\n", env, hex.EncodeToString(seed))
		fmt.Printf("# public key: %s\n", hex.EncodeToString(pub))
	}
	return 0
}
