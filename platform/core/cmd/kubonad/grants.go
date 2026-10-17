package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/grants"
)

// Cross-lab grant minting lives in the operator CLI — never in the browser
// or on the server. Rationale: minting requires the granting lab's admin
// PRIVATE key; browsers are the wrong vault for rare, high-value signatures,
// and the server must never see private keys at all (it verifies with public
// keys from the tenants table). TS UCAN 0.x (@ucans/ucans, JWT envelopes) is
// additionally wire-incompatible with the UCAN 1.0 DAG-CBOR envelopes the Go
// verifier enforces, so there is no browser mint path by design — not by gap.
//
// Usage:
//
//	kubonad grants mint --key-file admin.hex --aud did:key:z... \
//	  --granting lab-a --target lab-b --scope qpu/submit \
//	  --scopes qpu:submit,read --budget 100 --ttl 24h
//	kubonad grants attenuate --key-file blab.hex --aud did:key:z... \
//	  --parent parent.b64 --scope qpu/submit --scopes qpu:submit \
//	  --budget 25 --ttl 30m
//	kubonad grants verify --chain a.b64 [--chain b.b64 ...]
//	[--trust did:key:z...=lab-a ...]
//
// Key files hold one hex-encoded ed25519 private key (64-byte) or seed
// (32-byte) and should be mode 0600. Envelopes print as base64 (one per line
// in file mode, JSON on stdout otherwise).

func runGrants(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: kubonad grants <mint|attenuate|verify> [flags]")
		return 2
	}
	switch args[0] {
	case "mint":
		return grantsMint(args[1:])
	case "attenuate":
		return grantsAttenuate(args[1:])
	case "verify":
		return grantsVerify(args[1:])
	case "did":
		return grantsDID(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown grants subcommand %q\n", args[0])
		return 2
	}
}

func readKeyFile(path string) (string, error) {
	// W0.20: refuse world/group-readable key files. A private key readable
	// by other users is already compromised — loading it silently blesses
	// the leak. 0600 (or stricter 0400) only.
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if perm := fi.Mode().Perm(); perm&0077 != 0 {
		return "", fmt.Errorf("key file %s has permissions %04o: refusing (must be 0600 or stricter — a shared key file is a compromised key)", path, perm)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hexKey := strings.TrimSpace(string(raw))
	if _, err := hex.DecodeString(hexKey); err != nil {
		return "", fmt.Errorf("key file %s is not hex: %w", path, err)
	}
	return hexKey, nil
}

type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func grantsMint(args []string) int {
	fs := flag.NewFlagSet("mint", flag.ContinueOnError)
	keyFile := fs.String("key-file", "", "path to hex admin private key file (0600)")
	aud := fs.String("aud", "", "grantee DID (did:key:...)")
	granting := fs.String("granting", "", "granting lab tenant id")
	target := fs.String("target", "", "target lab tenant id")
	scope := fs.String("scope", "", "command scope path, e.g. qpu/submit")
	scopes := fs.String("scopes", "", "CSV capability scopes, e.g. qpu:submit,read")
	budget := fs.Float64("budget", 0, "max budget USD")
	ttl := fs.Duration("ttl", 0, "grant TTL (e.g. 24h, max 90d)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *keyFile == "" || *aud == "" || *granting == "" || *target == "" || *scope == "" || *scopes == "" || *budget <= 0 || *ttl <= 0 {
		fmt.Fprintln(os.Stderr, "mint requires --key-file --aud --granting --target --scope --scopes --budget --ttl")
		return 2
	}
	privHex, err := readKeyFile(*keyFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	sealed, cidStr, err := grants.MintGrant(privHex, *aud, *granting, *target, *scope,
		strings.Split(*scopes, ","), *budget, *ttl)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mint error:", err)
		return 1
	}
	out, _ := json.Marshal(map[string]string{
		"root_cid": cidStr,
		"chain_0":  grants.EncodeChainB64([][]byte{sealed})[0],
	})
	fmt.Println(string(out))
	return 0
}

func grantsAttenuate(args []string) int {
	fs := flag.NewFlagSet("attenuate", flag.ContinueOnError)
	keyFile := fs.String("key-file", "", "path to hex grantee private key file (0600)")
	aud := fs.String("aud", "", "audience DID (did:key:...)")
	parentFile := fs.String("parent", "", "path to parent sealed envelope (base64)")
	scope := fs.String("scope", "", "narrowed command scope path")
	scopes := fs.String("scopes", "", "CSV capability scopes subset")
	budget := fs.Float64("budget", 0, "max budget USD (<= parent)")
	ttl := fs.Duration("ttl", 0, "grant TTL")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *keyFile == "" || *aud == "" || *parentFile == "" || *scope == "" || *scopes == "" || *budget <= 0 || *ttl <= 0 {
		fmt.Fprintln(os.Stderr, "attenuate requires --key-file --aud --parent --scope --scopes --budget --ttl")
		return 2
	}
	privHex, err := readKeyFile(*keyFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	parentRaw, err := os.ReadFile(*parentFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	// W0.20: parent files hold one base64 envelope per line (a 2+-chain
	// file attenuates from its leaf). Single-envelope files keep working.
	parentChain, err := grants.DecodeChainB64(strings.Fields(string(parentRaw)))
	if err != nil || len(parentChain) == 0 {
		fmt.Fprintln(os.Stderr, "parent envelope unreadable")
		return 1
	}
	// W0.20: attenuate the leaf you hand over — a multi-envelope chain
	// attenuates from its last envelope (depth>1 chains were previously
	// unattentuatable). Chain validity itself is checked at register and
	// dispatch, not here.
	sealed, cidStr, err := grants.AttenuateGrant(parentChain[len(parentChain)-1], privHex, *aud, *scope,
		strings.Split(*scopes, ","), *budget, *ttl)
	if err != nil {
		fmt.Fprintln(os.Stderr, "attenuate error:", err)
		return 1
	}
	out, _ := json.Marshal(map[string]string{
		"leaf_cid": cidStr,
		"chain_1":  grants.EncodeChainB64([][]byte{sealed})[0],
	})
	fmt.Println(string(out))
	return 0
}

func grantsVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	var chains stringSlice
	fs.Var(&chains, "chain", "base64 sealed envelope, root-first (repeatable)")
	var trusts stringSlice
	fs.Var(&trusts, "trust", "trusted root mapping did:key:...=tenant (repeatable)")
	scope := fs.String("scope", "", "required scope to check coverage for")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(chains) == 0 {
		fmt.Fprintln(os.Stderr, "verify requires at least one --chain")
		return 2
	}
	chain, err := grants.DecodeChainB64(chains)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	roots := map[string]string{}
	for _, t := range trusts {
		parts := strings.SplitN(t, "=", 2)
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "bad --trust %q, want did=tenant\n", t)
			return 2
		}
		roots[parts[0]] = parts[1]
	}
	vg, err := grants.VerifyChain(chain, grants.VerifyOptions{
		TrustedRoots:  roots,
		RequiredScope: *scope,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "REFUSED:", err)
		return 1
	}
	out, _ := json.Marshal(map[string]any{
		"ok": true, "root_cid": vg.RootCID, "leaf_cid": vg.LeafCID,
		"granting_tenant": vg.GrantingTenant, "target_tenant": vg.TargetTenant,
		"max_budget_usd": vg.MaxBudgetUSD, "scopes": vg.Scopes, "command": vg.Command,
	})
	fmt.Println(string(out))
	return 0
}

func grantsDID(args []string) int {
	fs := flag.NewFlagSet("did", flag.ContinueOnError)
	keyFile := fs.String("key-file", "", "path to hex private key file (0600)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *keyFile == "" {
		fmt.Fprintln(os.Stderr, "did requires --key-file")
		return 2
	}
	privHex, err := readKeyFile(*keyFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	didStr, err := grants.DIDForPrivkey(privHex)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Println(didStr)
	return 0
}
