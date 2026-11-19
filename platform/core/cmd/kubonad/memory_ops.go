package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/memory"
)

// kubonad memory-status --db /abs/ledger.db [--json]
// Reports store pressure against the global budget and whether pgvector is
// configured. It does not activate the sidecar; MEM-20 stays gated on pressure.
func runMemoryStatus(args []string) int {
	fs := flag.NewFlagSet("memory-status", flag.ContinueOnError)
	dbPath := fs.String("db", "", "absolute path to the SQLite ledger")
	asJSON := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dbPath == "" || !filepath.IsAbs(*dbPath) {
		fmt.Fprintln(os.Stderr, "error: --db must be an absolute path")
		return 1
	}
	l, err := ledger.OpenLedger(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: open ledger:", err)
		return 1
	}
	defer l.Close()
	var count, contentBytes, vectorBytes int64
	_ = l.DB().QueryRow(`SELECT COUNT(*), COALESCE(SUM(LENGTH(content)),0), COALESCE(SUM(LENGTH(embedding_int8)),0) FROM hindsight_memories`).Scan(&count, &contentBytes, &vectorBytes)
	cfg := memory.DefaultStoreConfig()
	ratio := 0.0
	if cfg.MaxTotalItems > 0 {
		ratio = float64(count) / float64(cfg.MaxTotalItems)
	}
	pg := os.Getenv("KUBONA_PG_DSN") != ""
	report := map[string]any{
		"items":                count,
		"content_bytes":        contentBytes,
		"vector_bytes_int8":    vectorBytes,
		"max_total_items":      cfg.MaxTotalItems,
		"max_total_bytes":      cfg.MaxTotalBytes,
		"budget_ratio":         ratio,
		"pgvector_configured":  pg,
		"pgvector_recommended": ratio >= 0.8,
		"as_of":                time.Now().UTC().Format(time.RFC3339),
	}
	if *asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(report)
		return 0
	}
	fmt.Printf("memory items: %d / %d (%.1f%%)\n", count, cfg.MaxTotalItems, ratio*100)
	fmt.Printf("bytes: content=%d vector_int8=%d budget=%d\n", contentBytes, vectorBytes, cfg.MaxTotalBytes)
	fmt.Printf("pgvector configured: %v; recommended: %v\n", pg, ratio >= 0.8)
	return 0
}

// kubonad memory-feedback --db --tenant --id [--useful|--harmful]
func runMemoryFeedback(args []string) int {
	fs := flag.NewFlagSet("memory-feedback", flag.ContinueOnError)
	dbPath := fs.String("db", "", "absolute path to the SQLite ledger")
	tenant := fs.String("tenant", "default", "tenant id")
	id := fs.String("id", "", "memory id")
	useful := fs.Bool("useful", false, "record a useful outcome")
	harmful := fs.Bool("harmful", false, "record a harmful outcome")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dbPath == "" || *id == "" || (!*useful && !*harmful) || (*useful && *harmful) {
		fmt.Fprintln(os.Stderr, "usage: memory-feedback --db <abs> --id <id> --tenant <t> (--useful|--harmful)")
		return 2
	}
	l, err := ledger.OpenLedger(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: open ledger:", err)
		return 1
	}
	defer l.Close()
	store := memory.NewPersistentHindsightStore(l.DB())
	if err := store.FeedbackScoped(*tenant, *id, *useful); err != nil {
		fmt.Fprintln(os.Stderr, "error: feedback:", err)
		return 1
	}
	fmt.Printf("recorded %s for %s/%s\n", map[bool]string{true: "useful", false: "harmful"}[*useful], *tenant, *id)
	return 0
}

// kubonad memory-graph --db --tenant [--out file]
func runMemoryGraph(args []string) int {
	fs := flag.NewFlagSet("memory-graph", flag.ContinueOnError)
	dbPath := fs.String("db", "", "absolute path to the SQLite ledger")
	tenant := fs.String("tenant", "default", "tenant id")
	out := fs.String("out", "", "output file (default stdout)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "error: --db is required")
		return 1
	}
	l, err := ledger.OpenLedger(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: open ledger:", err)
		return 1
	}
	defer l.Close()
	store := memory.NewPersistentHindsightStore(l.DB())
	graph, err := store.TemporalGraph(*tenant)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: temporal graph:", err)
		return 1
	}
	raw, _ := json.MarshalIndent(graph, "", "  ")
	if *out == "" {
		fmt.Println(string(raw))
		return 0
	}
	if err := os.WriteFile(*out, raw, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "error: write graph:", err)
		return 1
	}
	fmt.Println("wrote", *out)
	return 0
}
