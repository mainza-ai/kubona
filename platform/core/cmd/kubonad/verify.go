package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// kubonad verify --db PATH [--session ID]: audit the ledger hash chain(s)
// through a read-only handle. Exit 0 + "clean" when no faults; exit 1
// listing every fault otherwise. Any copy of the ledger must verify before
// it is trusted (read replicas, backups, per-tenant shards).
func runVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	dbPath := fs.String("db", "kubona.db", "path to SQLite WAL ledger")
	session := fs.String("session", "", "audit one session only (default: all)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	l, err := ledger.OpenLedgerReadOnly(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer l.Close()
	faults, err := l.VerifyChain(*session)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if len(faults) == 0 {
		scope := "all sessions"
		if *session != "" {
			scope = "session " + *session
		}
		fmt.Printf("chain clean (%s)\n", scope)
		return 0
	}
	out, _ := json.MarshalIndent(faults, "", "  ")
	fmt.Printf("%d chain fault(s):\n%s\n", len(faults), out)
	return 1
}
