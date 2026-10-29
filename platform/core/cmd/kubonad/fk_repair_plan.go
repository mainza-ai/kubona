package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// kubonad fk-repair-plan --db /abs/ledger.db [--json]
//
// Dry run only: classify declared-FK orphans and print a repair plan. It
// never writes, never takes the writer lock, and never deletes a row. The
// operator decides whether to quarantine or delete; this command exists so
// that decision is made from evidence, not from guessing.
func runFKRepairPlan(args []string) int {
	fs := flag.NewFlagSet("fk-repair-plan", flag.ContinueOnError)
	dbPath := fs.String("db", "", "path to SQLite WAL ledger (must be absolute)")
	asJSON := fs.Bool("json", false, "emit machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dbPath == "" || !filepath.IsAbs(*dbPath) {
		fmt.Fprintln(os.Stderr, "error: --db must be an absolute path (relative ledger paths refuse: CWD-dependent plans prove nothing)")
		return 1
	}
	if _, err := os.Stat(*dbPath); err != nil {
		fmt.Fprintln(os.Stderr, "error: ledger not readable:", err)
		return 1
	}

	l, err := ledger.OpenLedgerReadOnly(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer l.Close()

	orphans, err := foreignKeyOrphans(l.DB())
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	plan := buildFKRepairPlan(orphans)
	if *asJSON {
		out, _ := json.MarshalIndent(plan, "", "  ")
		fmt.Println(string(out))
	} else {
		printFKRepairPlan(plan)
	}
	if plan.Orphans != 0 {
		return 1
	}
	return 0
}

type fkRepairItem struct {
	Table          string `json:"table"`
	RowID          int64  `json:"rowid"`
	Parent         string `json:"parent"`
	FKID           int64  `json:"fkid"`
	Class          string `json:"class"`
	Recommendation string `json:"recommendation"`
}

type fkRepairPlan struct {
	OK      bool           `json:"ok"`
	Orphans int            `json:"orphans"`
	ByTable map[string]int `json:"by_table"`
	Items   []fkRepairItem `json:"items"`
	Policy  string         `json:"policy"`
}

const fkRepairPolicy = "dry run only; take a verified backup, then quarantine or delete per class with operator approval. Jobs/loops may carry spend or reservations and need manual review before removal."

func buildFKRepairPlan(orphans []fkOrphan) fkRepairPlan {
	plan := fkRepairPlan{
		OK:      len(orphans) == 0,
		Orphans: len(orphans),
		ByTable: map[string]int{},
		Items:   make([]fkRepairItem, 0, len(orphans)),
		Policy:  fkRepairPolicy,
	}
	for _, o := range orphans {
		class, recommendation := classifyFKOrphan(o.Table)
		plan.ByTable[o.Table]++
		plan.Items = append(plan.Items, fkRepairItem{
			Table: o.Table, RowID: o.RowID, Parent: o.Parent, FKID: o.FKID,
			Class: class, Recommendation: recommendation,
		})
	}
	sort.Slice(plan.Items, func(i, j int) bool {
		if plan.Items[i].Table != plan.Items[j].Table {
			return plan.Items[i].Table < plan.Items[j].Table
		}
		return plan.Items[i].RowID < plan.Items[j].RowID
	})
	return plan
}

func classifyFKOrphan(table string) (class, recommendation string) {
	switch table {
	case "chat_messages", "campaign_states":
		return "content", "quarantine before delete; no spend fields, but content may be evidence"
	case "jobs":
		return "spend", "manual review; may carry reservations, estimates, or mandate references"
	case "loops":
		return "spend", "manual review; may carry reserved or consumed spend"
	default:
		return "unknown", "manual review; no automatic action"
	}
}

func printFKRepairPlan(plan fkRepairPlan) {
	if plan.Orphans == 0 {
		fmt.Println("fk-repair-plan: clean (no orphan rows; FK enforcement can be considered)")
		return
	}
	fmt.Printf("fk-repair-plan: %d orphan row(s) — dry run, no rows changed\n", plan.Orphans)
	tables := make([]string, 0, len(plan.ByTable))
	for table := range plan.ByTable {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		class, recommendation := classifyFKOrphan(table)
		fmt.Printf("  %s: %d (%s; %s)\n", table, plan.ByTable[table], class, recommendation)
	}
	fmt.Println("  policy:", plan.Policy)
	fmt.Println("  next: kubonad backup --db <db>, then choose quarantine/delete with operator approval")
}
