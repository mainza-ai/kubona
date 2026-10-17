package session

// S5b/C2 tests. Fixture lines below were written by the REAL Python
// Ledger (kubona/provenance/ledger.py) — only ts edited (cosmetic:
// hashes cover payload, never ts). They prove Go reads genuine Python
// bytes and replays them verbatim.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const s5bLine1 = `{"content_hash": "61c01e66f75fac842c96ed306c7f5d62ffbac2e139862a76921d37a58684361d", "kind": "workflow_step", "label": "tenant/default", "payload": {"result_summary": "{'ok': True}", "session_id": "s5b-sess", "spent_usd": 0.0, "step": "run", "tool": "execution/submit_job", "trigger": "manual", "workflow": "vqe-sweep"}, "prev_hash": "0000000000000000000000000000000000000000000000000000000000000000", "record_hash": "38a5a6cba2c919eab7b002fb1432281f128f0b5d77858b009e4dcae7169204fc", "record_id": "6fda9201a2a2425c", "ts": 1792245722.5}`

const s5bLine2 = `{"content_hash": "f7a24d438c57977c5ece173d7ef8fc2b58daccf3b8ab601fe432ecd898974c56", "kind": "workflow_step", "label": "tenant/default", "payload": {"result_summary": "{'ok': True}", "session_id": "s5b-sess", "spent_usd": 0.0, "step": "record", "tool": "provenance/write_record", "trigger": "manual", "workflow": "vqe-sweep"}, "prev_hash": "38a5a6cba2c919eab7b002fb1432281f128f0b5d77858b009e4dcae7169204fc", "record_hash": "51a87660ccd6f12a0dbf1fb0faee6e13f32e014ec5ebdc98b2a5f5156f37c789", "record_id": "2c9ab1c06bdd44b4", "ts": 1792245722.6}`

const s5bLineOther = `{"content_hash": "641134264142a86e2f221c6823cf5d91006739808ca9a7f786abf8c833dc6241", "kind": "workflow_dry_run", "label": "tenant/default", "payload": {"compensations_planned": [], "planned_steps": [], "session_id": "other-sess", "trigger": "manual", "unresolved": [], "workflow": "other"}, "prev_hash": "51a87660ccd6f12a0dbf1fb0faee6e13f32e014ec5ebdc98b2a5f5156f37c789", "record_hash": "7f298b6e84acf38cb49417c076a8222565df094f10bcac9a7b0f456d81b2fad0", "record_id": "684ac71935da4d19", "ts": 1792245722.7}`

func TestPythonSafeTenant(t *testing.T) {
	for in, want := range map[string]string{
		"lab-alpha": "lab-alpha", "a/b..c": "abc", "": "", "a_b-c9": "a_b-c9",
	} {
		if got := pythonSafeTenant(in); got != want {
			t.Errorf("pythonSafeTenant(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPythonSidecarPath(t *testing.T) {
	got := pythonSidecarPath("/state/ledger.jsonl", "lab-alpha")
	if got != filepath.Join("/state", "ledger.lab-alpha.jsonl") {
		t.Fatalf("sidecar path = %q", got)
	}
	// Matches Python: base.parent / f"{stem}.{safe(tenant)}{suffix}".
	if got := pythonSidecarPath("/s/kubona-ledger.jsonl", "a/b"); got != filepath.Join("/s", "kubona-ledger.ab.jsonl") {
		t.Fatalf("sidecar path with unsafe tenant = %q", got)
	}
}

func writeS5bSidecar(t *testing.T, dir string, lines ...string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "ledger.default.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
}

func TestExportPythonRecordsFilters(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KUBONA_LEDGER", filepath.Join(dir, "ledger.jsonl"))
	writeS5bSidecar(t, dir, s5bLine1, s5bLine2, s5bLineOther)
	recs, err := exportPythonRecords("s5b-sess", "default")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 session rows, got %d", len(recs))
	}
	// Payload bytes verbatim (Python serialization untouched).
	var p1 map[string]any
	if err := json.Unmarshal(recs[0].Payload, &p1); err != nil {
		t.Fatalf("payload parse: %v", err)
	}
	if p1["step"] != "run" || recs[0].Kind != "workflow_step" || recs[0].Label != "tenant/default" {
		t.Fatalf("row mangled: %+v", recs[0])
	}
	if !strings.Contains(string(recs[0].Payload), `"session_id": "s5b-sess"`) {
		t.Fatalf("payload not verbatim: %s", recs[0].Payload)
	}
}

func TestExportPythonRecordsMissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KUBONA_LEDGER", filepath.Join(dir, "ledger.jsonl"))
	recs, err := exportPythonRecords("s5b-sess", "default")
	if err != nil {
		t.Fatalf("missing sidecar must not error: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("missing sidecar must export empty, got %d", len(recs))
	}
}

func TestExportPythonRecordsMalformedRefuses(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KUBONA_LEDGER", filepath.Join(dir, "ledger.jsonl"))
	writeS5bSidecar(t, dir, s5bLine1, `{"kind": "broken",`)
	if _, err := exportPythonRecords("s5b-sess", "default"); err == nil {
		t.Fatal("malformed sidecar line must refuse (never silently dropped)")
	}
	writeS5bSidecar(t, dir, `{"kind": "../../evil", "payload": {}, "label": "x", "content_hash": "h", "record_hash": "h", "record_id": "h", "ts": 1, "prev_hash": "h"}`)
	if _, err := exportPythonRecords("s5b-sess", "default"); err == nil {
		t.Fatal("invalid kind must refuse")
	}
}

func checkChain(t *testing.T, path string, wantLines int) []PythonRecord {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read replayed: %v", err)
	}
	var recs []PythonRecord
	prev := strings.Repeat("0", 64)
	for i, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var rec PythonRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("line %d not JSON: %v", i, err)
		}
		if rec.PrevHash != prev {
			t.Fatalf("line %d prev linkage broken", i)
		}
		sum := sha256.Sum256([]byte(rec.PrevHash + ":" + rec.ContentHash))
		if hex.EncodeToString(sum[:]) != rec.RecordHash {
			t.Fatalf("line %d record_hash formula wrong", i)
		}
		prev = rec.RecordHash
		recs = append(recs, rec)
	}
	if len(recs) != wantLines {
		t.Fatalf("want %d lines, got %d", wantLines, len(recs))
	}
	return recs
}

func TestReplayRoundTripIdempotent(t *testing.T) {
	src := t.TempDir()
	t.Setenv("KUBONA_LEDGER", filepath.Join(src, "ledger.jsonl"))
	writeS5bSidecar(t, src, s5bLine1, s5bLine2, s5bLineOther)
	recs, err := exportPythonRecords("s5b-sess", "default")
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	dst := t.TempDir()
	t.Setenv("KUBONA_LEDGER", filepath.Join(dst, "ledger.jsonl"))
	n, err := replayPythonRecords("default", recs)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if n != 2 {
		t.Fatalf("want 2 appended, got %d", n)
	}
	replayed := checkChain(t, filepath.Join(dst, "ledger.default.jsonl"), 2)
	// Content preserved: payload/kind/label/ts/content_hash identical.
	// (Payload BYTES are compacted by Go's marshal — JSON-semantically
	// identical, and chain validity never depends on payload whitespace:
	// Python's validate_chain re-serializes on read. content_hash, the
	// tamper-evident seal, is preserved as an opaque string, never
	// recomputed.)
	for i, r := range replayed {
		var got, want map[string]any
		if err := json.Unmarshal(r.Payload, &got); err != nil {
			t.Fatalf("replayed payload %d not JSON: %v", i, err)
		}
		if err := json.Unmarshal(recs[i].Payload, &want); err != nil {
			t.Fatalf("source payload %d not JSON: %v", i, err)
		}
		if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
			t.Fatalf("row %d payload content changed", i)
		}
		if r.Kind != recs[i].Kind || r.ContentHash != recs[i].ContentHash ||
			r.Label != recs[i].Label || r.RecordID != recs[i].RecordID {
			t.Fatalf("row %d not preserved", i)
		}
	}

	// Double-activate replays, never duplicates.
	before, _ := os.ReadFile(filepath.Join(dst, "ledger.default.jsonl"))
	n2, err := replayPythonRecords("default", recs)
	if err != nil {
		t.Fatalf("re-replay: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("re-replay must append 0, got %d", n2)
	}
	after, _ := os.ReadFile(filepath.Join(dst, "ledger.default.jsonl"))
	if string(before) != string(after) {
		t.Fatal("re-replay modified the file")
	}
}

func TestManifestPythonLedgerRoundTrip(t *testing.T) {
	src := t.TempDir()
	t.Setenv("KUBONA_LEDGER", filepath.Join(src, "ledger.jsonl"))
	writeS5bSidecar(t, src, s5bLine1, s5bLine2, s5bLineOther)
	m1, _ := mustOpenCascade(t)
	seedCascadeSession(t, m1, m1.ledger, "s5b-sess")
	man, err := m1.ExportSession("s5b-sess")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(man.PythonLedger) != 2 {
		t.Fatalf("manifest section has %d rows, want 2", len(man.PythonLedger))
	}
	if _, ok := man.Digests["python_ledger"]; !ok {
		t.Fatal("ninth digest row missing")
	}
	if _, ok := man.SectionEnvelopes["python_ledger"]; !ok {
		t.Fatal("ninth section envelope missing")
	}

	dst := t.TempDir()
	t.Setenv("KUBONA_LEDGER", filepath.Join(dst, "ledger.jsonl"))
	m2, _ := mustOpenCascade(t)
	res, err := m2.ImportSession(man)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.PythonReplayed != 2 {
		t.Fatalf("PythonReplayed = %d, want 2", res.PythonReplayed)
	}
	checkChain(t, filepath.Join(dst, "ledger.default.jsonl"), 2)

	// Same manifest into the same lab: session exists AND rows present —
	// replay is a no-op receipt.
	res2, err := m2.ImportSession(man)
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if !res2.Existing || res2.PythonReplayed != 0 {
		t.Fatalf("re-import must be a no-op receipt, got %+v", res2)
	}
}

func TestManifestPythonLedgerTamperRefused(t *testing.T) {
	src := t.TempDir()
	t.Setenv("KUBONA_LEDGER", filepath.Join(src, "ledger.jsonl"))
	writeS5bSidecar(t, src, s5bLine1)
	m1, _ := mustOpenCascade(t)
	seedCascadeSession(t, m1, m1.ledger, "s5b-sess")
	man, err := m1.ExportSession("s5b-sess")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(man.PythonLedger) == 0 {
		t.Fatal("export must carry the session sidecar rows")
	}
	tampered := *man
	tampered.PythonLedger = make([]PythonRecord, len(man.PythonLedger))
	copy(tampered.PythonLedger, man.PythonLedger)
	tampered.PythonLedger[0].Payload = json.RawMessage(`{"session_id": "s5b-sess", "forged": true}`)
	dst := t.TempDir()
	t.Setenv("KUBONA_LEDGER", filepath.Join(dst, "ledger.jsonl"))
	m2, _ := mustOpenCascade(t)
	if _, err := m2.ImportSession(&tampered); err == nil {
		t.Fatal("tampered python_ledger section must refuse")
	}
	if _, err := os.Stat(filepath.Join(dst, "ledger.default.jsonl")); !os.IsNotExist(err) {
		t.Fatal("refused import must not create a sidecar")
	}
}

func TestManifestStrictRoster(t *testing.T) {
	m1, _ := mustOpenCascade(t)
	seedCascadeSession(t, m1, m1.ledger, "s5b-roster")
	t.Setenv("KUBONA_LEDGER", filepath.Join(t.TempDir(), "ledger.jsonl"))
	man, err := m1.ExportSession("s5b-roster")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	m2, _ := mustOpenCascade(t)

	stripped := *man
	stripped.Digests = map[string]string{}
	for k, v := range man.Digests {
		stripped.Digests[k] = v
	}
	delete(stripped.Digests, "python_ledger")
	stripped.PythonLedger = nil
	if _, err := m2.ImportSession(&stripped); err == nil {
		t.Fatal("stripped section + stripped digest row must refuse (silent history loss)")
	}

	extra := *man
	extra.Digests = map[string]string{}
	for k, v := range man.Digests {
		extra.Digests[k] = v
	}
	extra.Digests["smuggled"] = "00"
	if _, err := m2.ImportSession(&extra); err == nil {
		t.Fatal("unknown digest row must refuse")
	}
}
