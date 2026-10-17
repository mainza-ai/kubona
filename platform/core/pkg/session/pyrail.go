package session

// S5b/C2 — Python-rail sidecar migration (ninth manifest section).
//
// The Python provenance ledger keeps tenant-scoped sidecar files
// (ledger.<tenant>.jsonl, see kubona/provenance/ledger.py) holding
// workflow_step / workflow_dry_run / workflow_compensation and friends.
// S5 exported Go tables only, so a migrated lab silently lost that
// history. This file sweeps the sidecar at export and replays it at
// import, digest-pinned like every other section.
//
// Trust layering (decided, see C2 landed-note): the closed kind registry
// is enforced at the Python APPEND path (R1a) — Go does not re-validate
// kinds here, it verifies structure + digest-pin, exactly as much as it
// trusts its own SQLite file. What Go guarantees:
//   - export selects only rows whose payload.session_id == the session
//     (file order preserved — chain order matters for replay);
//   - malformed lines refuse the export (never silently dropped);
//   - import replays content faithfully (kind/payload/label/ts/
//     content_hash/record_id preserved; payload whitespace is normalized
//     by Go's marshal — JSON-semantically identical, and safe because
//     NOTHING recomputes hashes from payload bytes: content_hash travels
//     as an opaque string, and Python's validate_chain re-serializes on
//     read) with only the chain links rewritten
//     (prev_hash := current tail, record_hash := sha256(prev:content),
//     the exact Python formula);
//   - replay dedups by content_hash (double-activate replays, never
//     duplicates) and holds Python's own <file>.lock while
//     tail-reading + appending (same lock the Python Ledger takes);
//   - an unreadable/invalid replay target refuses before any write.

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

// PythonRecord is one sidecar row. Payload stays json.RawMessage throughout:
// Go never re-serializes it, so Python's serialization (spaced separators,
// ascii-escaped) round-trips byte-identical.
type PythonRecord struct {
	RecordID    string          `json:"record_id"`
	PrevHash    string          `json:"prev_hash"`
	ContentHash string          `json:"content_hash"`
	RecordHash  string          `json:"record_hash"`
	Ts          float64         `json:"ts"`
	Kind        string          `json:"kind"`
	Payload     json.RawMessage `json:"payload"`
	Label       string          `json:"label"`
}

var pythonKindAlphabet = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.\-]*$`)

// pythonSafeTenant mirrors kubona/provenance/ledger.py::_safe (drop every
// char outside [A-Za-z0-9_-]). Tenant IDs are validated to that alphabet
// anyway; this is defense-in-depth so a weird tenant can never escape the
// state dir via ".." in a filename.
func pythonSafeTenant(tenant string) string {
	var b strings.Builder
	for _, r := range tenant {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// resolvePythonLedgerBase mirrors kubona/config.py::ledger_path: the
// KUBONA_LEDGER file when set, else ~/.kubona/ledger.jsonl (the default
// state root both rails share). Custom --db layouts must set KUBONA_LEDGER
// to co-locate — one variable, both rails (documented, not guessed).
func resolvePythonLedgerBase() string {
	if env := strings.TrimSpace(os.Getenv("KUBONA_LEDGER")); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kubona", "ledger.jsonl")
}

// pythonSidecarPath builds ledger.<tenant>.jsonl beside the base file,
// exactly like Python (base.parent / f"{base.stem}.{safe(tenant)}{suffix}").
func pythonSidecarPath(base, tenant string) string {
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(filepath.Base(base), ext)
	return filepath.Join(filepath.Dir(base), stem+"."+pythonSafeTenant(tenant)+ext)
}

// exportPythonRecords sweeps the tenant sidecar for this session's rows.
// A missing sidecar is not an error (empty section, digest-pinned) —
// sessions without workflow history migrate exactly as before.
func exportPythonRecords(sessionID, tenantID string) ([]PythonRecord, error) {
	base := resolvePythonLedgerBase()
	if base == "" {
		return nil, fmt.Errorf("cannot resolve python ledger base (no home dir and KUBONA_LEDGER unset)")
	}
	path := pythonSidecarPath(base, tenantID)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []PythonRecord{}, nil
		}
		return nil, fmt.Errorf("read python sidecar: %w", err)
	}
	var out []PythonRecord
	for i, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec PythonRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("python sidecar line %d not valid JSON: %w", i+1, err)
		}
		if rec.Kind == "" || !pythonKindAlphabet.MatchString(rec.Kind) {
			return nil, fmt.Errorf("python sidecar line %d has invalid kind %q: refusing (not silently dropped)", i+1, rec.Kind)
		}
		if len(rec.Payload) == 0 || rec.ContentHash == "" || rec.RecordHash == "" || rec.RecordID == "" {
			return nil, fmt.Errorf("python sidecar line %d missing required fields: refusing", i+1)
		}
		var payload struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(rec.Payload, &payload); err != nil {
			return nil, fmt.Errorf("python sidecar line %d payload not an object: %w", i+1, err)
		}
		if payload.SessionID == sessionID {
			out = append(out, rec)
		}
	}
	if out == nil {
		out = []PythonRecord{}
	}
	return out, nil
}

// replayPythonRecords merges exported rows into the tenant sidecar:
// rows whose content_hash already exists are skipped (idempotent —
// double-activate replays), new rows append with rewritten chain links.
// Holds Python's <file>.lock across tail-read + append, like the Python
// Ledger does. Returns appended count.
func replayPythonRecords(tenantID string, recs []PythonRecord) (int, error) {
	if len(recs) == 0 {
		return 0, nil
	}
	base := resolvePythonLedgerBase()
	if base == "" {
		return 0, fmt.Errorf("cannot resolve python ledger base (no home dir and KUBONA_LEDGER unset)")
	}
	path := pythonSidecarPath(base, tenantID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, fmt.Errorf("sidecar dir: %w", err)
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return 0, fmt.Errorf("sidecar lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return 0, fmt.Errorf("sidecar flock: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	seen := map[string]bool{}
	tail := strings.Repeat("0", 64)
	if raw, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var rec PythonRecord
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				return 0, fmt.Errorf("existing sidecar has invalid line: refusing to append onto a corrupt chain: %w", err)
			}
			if rec.ContentHash != "" {
				seen[rec.ContentHash] = true
			}
			if rec.RecordHash != "" {
				tail = rec.RecordHash
			}
		}
	} else if !os.IsNotExist(err) {
		return 0, fmt.Errorf("read sidecar: %w", err)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open sidecar for replay: %w", err)
	}
	defer f.Close()
	appended := 0
	w := bufio.NewWriter(f)
	for _, rec := range recs {
		if seen[rec.ContentHash] {
			continue
		}
		rec.PrevHash = tail
		sum := sha256.Sum256([]byte(tail + ":" + rec.ContentHash))
		rec.RecordHash = hex.EncodeToString(sum[:])
		line, err := json.Marshal(rec)
		if err != nil {
			return appended, fmt.Errorf("marshal replay row: %w", err)
		}
		// NOTE: struct marshal order (record_id, prev_hash, ...) differs
		// from Python's to_json key order — both are sorted-key readers,
		// and validate_chain parses JSON, never compares raw lines.
		if _, err := w.Write(append(line, '\n')); err != nil {
			return appended, fmt.Errorf("write replay row: %w", err)
		}
		tail = rec.RecordHash
		seen[rec.ContentHash] = true
		appended++
	}
	if err := w.Flush(); err != nil {
		return appended, fmt.Errorf("flush replay rows: %w", err)
	}
	return appended, nil
}
