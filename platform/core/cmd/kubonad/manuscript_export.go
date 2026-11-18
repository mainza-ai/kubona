package main

// kubonad manuscript-export — export a session's publication evidence.
//
// The ledger is the authority for manuscripts, signoffs, reviews, annotations
// and artifact bytes. Deleting a session leaves those rows unreachable through
// the API, so an operator needs a durable export path before pruning or
// removing a lab. This command is read-only: it opens the ledger read-only,
// writes a directory, verifies every artifact against its recorded SHA-256,
// and never mutates the database.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func runManuscriptExport(args []string) int {
	fs := flag.NewFlagSet("manuscript-export", flag.ContinueOnError)
	dbPath := fs.String("db", "", "ledger path (absolute)")
	sessionID := fs.String("session", "", "session id to export")
	outDir := fs.String("out", "", "output directory (absolute)")
	onlyID := fs.String("id", "", "export one manuscript id (optional)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dbPath == "" || !filepath.IsAbs(*dbPath) {
		fmt.Fprintln(os.Stderr, "error: --db must be an absolute path")
		return 1
	}
	if strings.TrimSpace(*sessionID) == "" {
		fmt.Fprintln(os.Stderr, "error: --session is required")
		return 1
	}
	if *outDir == "" || !filepath.IsAbs(*outDir) {
		fmt.Fprintln(os.Stderr, "error: --out must be an absolute path")
		return 1
	}
	l, err := ledger.OpenLedgerReadOnly(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: open ledger read-only:", err)
		return 1
	}
	defer l.Close()

	ids := []string{}
	if *onlyID != "" {
		ids = append(ids, *onlyID)
	} else {
		offset := 0
		for {
			page, hasMore, err := l.ListManuscriptsPage(*sessionID, 200, offset)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error: list manuscripts:", err)
				return 1
			}
			for _, m := range page {
				ids = append(ids, m.ID)
			}
			if !hasMore {
				break
			}
			offset += len(page)
		}
	}
	if len(ids) == 0 {
		fmt.Println("manuscript-export: no manuscripts for this session")
		return 0
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "error: create output dir:", err)
		return 1
	}
	for _, id := range ids {
		if err := exportManuscript(l, *sessionID, id, *outDir); err != nil {
			fmt.Fprintf(os.Stderr, "error: export %s: %v\n", id, err)
			return 1
		}
	}
	fmt.Printf("manuscript-export: %d manuscript(s) -> %s\n", len(ids), *outDir)
	return 0
}

func exportManuscript(l *ledger.Ledger, sessionID, id, outDir string) error {
	ms, err := l.GetManuscript(sessionID, id)
	if err != nil {
		return err
	}
	if strings.Contains(id, "/") || strings.Contains(id, "..") {
		return fmt.Errorf("unsafe manuscript id %q", id)
	}
	dir := filepath.Join(outDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	meta := map[string]any{
		"id": ms.ID, "session_id": ms.SessionID, "title": ms.Title,
		"abstract": ms.Abstract, "state": ms.State, "content_hash": ms.ContentHash,
		"build_hash": ms.BuildHash, "source_date_epoch": ms.SourceDateEpoch,
		"request_id": ms.RequestID, "created_at": ms.CreatedAt, "updated_at": ms.UpdatedAt,
		"review": json.RawMessage(ms.ReviewJSON),
	}
	if ms.Signoff != nil {
		meta["signoff"] = map[string]any{
			"signer_pubkey_hex": ms.Signoff.SignerPubKey,
			"signature_hex":     ms.Signoff.Signature,
			"content_hash":      ms.Signoff.ContentHash,
			"expiry":            ms.Signoff.Expiry,
			"signed_at":         ms.Signoff.SignedAt,
		}
	}
	if err := writeJSONFile(filepath.Join(dir, "metadata.json"), meta); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(dir, "sections.json"), ms.Sections); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(dir, "citations.json"), ms.Citations); err != nil {
		return err
	}
	reviews, err := l.ListManuscriptReviews(id)
	if err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(dir, "reviews.json"), reviews); err != nil {
		return err
	}
	annotations, err := l.ListManuscriptAnnotations(sessionID, id)
	if err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(dir, "annotations.json"), annotations); err != nil {
		return err
	}

	written := map[string]string{} // crate-relative path -> sha256
	for _, a := range ms.Artifacts {
		if a.Name == "" || strings.HasPrefix(a.Name, "/") || strings.Contains(a.Name, "..") {
			return fmt.Errorf("unsafe artifact name %q", a.Name)
		}
		raw, sum, err := l.GetManuscriptArtifact(sessionID, id, a.Name)
		if err != nil {
			return err
		}
		if a.SHA256 != "" && sum != a.SHA256 {
			return fmt.Errorf("artifact %s hash mismatch", a.Name)
		}
		rel := "artifacts/" + strings.TrimPrefix(filepath.ToSlash(a.Name), "/")
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, raw, 0o600); err != nil {
			return err
		}
		written[rel] = sum
	}
	// AG-6a: ship an RO-Crate 1.1 metadata document next to the artefacts so
	// an external consumer can reconstruct the research object and hashes.
	for _, rel := range []string{"metadata.json", "sections.json", "citations.json",
		"reviews.json", "annotations.json"} {
		raw, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		written[rel] = hex.EncodeToString(sum[:])
	}
	return writeROCrate(dir, ms, written)
}

// writeROCrate writes a minimal RO-Crate 1.1 metadata graph: the dataset, its
// files with sha256, the signoff/review as entities, and the content hash that
// binds them. It is intentionally small and stable rather than a full profile.
func writeROCrate(dir string, ms *ledger.Manuscript, files map[string]string) error {
	graph := []map[string]any{{
		"@id":        "ro-crate-metadata.json",
		"@type":      "CreativeWork",
		"conformsTo": map[string]any{"@id": "https://w3id.org/ro/crate/1.1"},
		"about":      map[string]any{"@id": "./"},
	}, {
		"@id":           "./",
		"@type":         "Dataset",
		"name":          ms.Title,
		"description":   ms.Abstract,
		"datePublished": ms.CreatedAt,
		"identifier":    ms.ID,
		"contentHash":   ms.ContentHash,
	}}
	if ms.Signoff != nil {
		graph = append(graph, map[string]any{
			"@id": "#signoff", "@type": "EndorsementRating",
			"signer": ms.Signoff.SignerPubKey, "signature": ms.Signoff.Signature,
			"contentHash": ms.Signoff.ContentHash, "expiry": ms.Signoff.Expiry,
			"datePublished": ms.Signoff.SignedAt,
		})
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	hasPart := make([]map[string]any, 0, len(names)+1)
	for _, name := range names {
		hasPart = append(hasPart, map[string]any{"@id": name})
		graph = append(graph, map[string]any{
			"@id": name, "@type": "File", "name": filepath.Base(name),
			"sha256": files[name],
		})
	}
	if ms.Signoff != nil {
		hasPart = append(hasPart, map[string]any{"@id": "#signoff"})
	}
	graph[1]["hasPart"] = hasPart
	doc := map[string]any{
		"@context": "https://w3id.org/ro/crate/1.1/context",
		"@graph":   graph,
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "ro-crate-metadata.json"), append(raw, '\n'), 0o600)
}

func writeJSONFile(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}
