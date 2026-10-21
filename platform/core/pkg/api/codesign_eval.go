// Codesign evaluator proxy (Phase 1 migration, ux-plan slice 5).
//
// The Python console_api.py module is unserved (no listener, no compose
// target); these handlers migrate its STATELESS compute routes behind the
// Go rail via runBridgePython, keeping Python as the single validator and
// the single implementation (no second copy of kickmix/TQ/frontier/budget
// math anywhere). Stateful duplicates (sessions/chat/loops/...) stay
// retired — this file migrates compute only.
//
// Threat model: same as the existing /api/codesign/* reads — read-only
// scoring, no spend, no jobs, no secrets. Public behind limiters; bounded
// compute (artifact caps + num_tests HTTP bound + ctx timeout); validity
// stays Python-owned (Go maps transport + status, never verdicts).
//
// Paths: /api/codesign/evaluate (single artifact + optional factory
// diagnostics), /api/codesign/evaluate-batch (artifacts[] with frontier),
// /api/codesign/budget (union-bound math). The existing
// /api/codesign/frontier (pre-scored entries) is untouched — different
// contract (no collision): entries there, artifacts here.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const codesignEvalMaxBody = 1 << 20

// evalSnippet runs one harness evaluation. Single validator: every bound
// below mirrors console_api (num_tests HTTP-capped at 4096 — larger runs
// belong in the local harness, not behind a request timeout).
const evalSnippet = `import json, sys
def strict_int(v, name, lo, hi):
    if isinstance(v, bool) or not isinstance(v, int) or not (lo <= v <= hi):
        raise ValueError(f"{name} must be an int in [{lo},{hi}]: refusing")
    return v
try:
    body = json.loads(sys.argv[1])
    if not isinstance(body, dict):
        raise ValueError("body must be a JSON object: refusing")
    from kubona.codesign import (CircuitCodesignHarness, build_spec,
                                  build_truth_table_spec, kappa_sweep)
    from kubona.codesign.factories import factory_report
    if "spec_table" in body and body["spec_table"] is not None:
        st = body["spec_table"]
        if not isinstance(st, dict):
            raise ValueError("spec_table must be an object: refusing")
        ins, outs, table = st.get("inputs", []), st.get("outputs", []), st.get("table", {})
        if not isinstance(ins, list) or not isinstance(outs, list) or not isinstance(table, dict):
            raise ValueError("spec_table needs inputs[]/outputs[]/table{}: refusing")
        spec = build_truth_table_spec(
            strict_int(st.get("num_qubits", 0), "num_qubits", 1, 64),
            [strict_int(q, "inputs[]", 0, 63) for q in ins],
            [strict_int(q, "outputs[]", 0, 63) for q in outs],
            table)
        label = "table"
    else:
        kind = body.get("spec_kind")
        spec = build_spec(str(kind))
        label = str(kind)
    num_tests = strict_int(body.get("num_tests", 64), "num_tests", 1, 4096)
    max_failures = strict_int(body.get("max_failures", 0), "max_failures", 0, num_tests)
    kappa = strict_int(body.get("kappa", 1), "kappa", 1, 3)
    seed = strict_int(body.get("seed", 7), "seed", 0, 2**31 - 1)
    h = CircuitCodesignHarness(spec, num_tests=num_tests,
                               max_failures=max_failures, kappa=kappa, seed=seed)
    res = h.run(body.get("artifact", []))
    if res.get("valid"):
        res["kappa_sweep"] = {str(k): v for k, v in
                              kappa_sweep(res["toffoli"], res["peak_qubits"]).items()}
    fac, cd, cu = body.get("factory"), body.get("code_distance"), body.get("cycle_us")
    if fac is None and (cd is not None or cu is not None):
        raise ValueError("factory diagnostics need factory + code_distance + cycle_us together: refusing")
    if fac is not None:
        if cd is None or cu is None:
            raise ValueError("factory diagnostics need factory + code_distance + cycle_us together: refusing (no silent physics)")
        res["factory"] = factory_report(toffoli=res["toffoli"], qubits=res["peak_qubits"],
                                        toffoli_depth=res.get("toffoli_depth", 0), factory=fac,
                                        code_distance=cd, cycle_us=cu)
    print(json.dumps({"ok": True, "spec_kind": label, "result": res}))
except ValueError as e:
    print(json.dumps({"ok": False, "error": str(e)[:280]}))
except Exception as e:
    print(json.dumps({"ok": False, "error": f"evaluation refused: {e}"[:280]}))
`

// batchSnippet mirrors codesign_frontier (fixed num_tests=32, artifacts<=64).
const batchSnippet = `import json, sys
def strict_int(v, name, lo, hi):
    if isinstance(v, bool) or not isinstance(v, int) or not (lo <= v <= hi):
        raise ValueError(f"{name} must be an int in [{lo},{hi}]: refusing")
    return v
try:
    body = json.loads(sys.argv[1])
    if not isinstance(body, dict):
        raise ValueError("body must be a JSON object: refusing")
    from kubona.codesign import (CircuitCodesignHarness, MapElitesFrontier,
                                  build_spec, build_truth_table_spec)
    if "spec_table" in body and body["spec_table"] is not None:
        st = body["spec_table"]
        if not isinstance(st, dict):
            raise ValueError("spec_table must be an object: refusing")
        ins, outs, table = st.get("inputs", []), st.get("outputs", []), st.get("table", {})
        if not isinstance(ins, list) or not isinstance(outs, list) or not isinstance(table, dict):
            raise ValueError("spec_table needs inputs[]/outputs[]/table{}: refusing")
        spec = build_truth_table_spec(
            strict_int(st.get("num_qubits", 0), "num_qubits", 1, 64),
            [strict_int(q, "inputs[]", 0, 63) for q in ins],
            [strict_int(q, "outputs[]", 0, 63) for q in outs],
            table)
        label = "table"
    else:
        kind = body.get("spec_kind")
        spec = build_spec(str(kind))
        label = str(kind)
    artifacts = body.get("artifacts", [])
    kappa = strict_int(body.get("kappa", 1), "kappa", 1, 3)
    if not isinstance(artifacts, list) or len(artifacts) > 64:
        raise ValueError("artifacts must be a list of <=64: refusing")
    h = CircuitCodesignHarness(spec, num_tests=32, kappa=kappa)
    f = MapElitesFrontier(kappa=kappa)
    details = []
    for art in artifacts:
        r = h.run(art)
        details.append({"valid": r["valid"], "toffoli": r["toffoli"],
                        "peak_qubits": r["peak_qubits"], "tq": r["tq"],
                        "eps_bound_97_5": r.get("eps_bound_97_5"),
                        "depth": r.get("depth"),
                        "toffoli_depth": r.get("toffoli_depth"),
                        "active_qubits": r.get("active_qubits")})
        if r["valid"]:
            f.offer(art, r["toffoli"], r["peak_qubits"], {"tq": r["tq"]})
    print(json.dumps({"ok": True, "spec_kind": label,
                      "frontier": f.to_dict(), "details": details}))
except ValueError as e:
    print(json.dumps({"ok": False, "error": str(e)[:280]}))
except Exception as e:
    print(json.dumps({"ok": False, "error": f"frontier refused: {e}"[:280]}))
`

// familiesSnippet runs one bounded evaluated family search (ux-plan slice 3):
// same spec for all tracks (cross-family comparison needs common ground),
// each artifact evaluated (invalid recorded, never scored), valid offers
// mature their family frontier, then share_rounds of cross-pollination,
// then the widen-nudge names the most stagnant track. Deterministic
// (sorted order, fixed seed basis); humain nudge reads the output.
const familiesSnippet = `import json, sys
def strict_int(v, name, lo, hi):
    if isinstance(v, bool) or not isinstance(v, int) or not (lo <= v <= hi):
        raise ValueError(f"{name} must be an int in [{lo},{hi}]: refusing")
    return v
try:
    body = json.loads(sys.argv[1])
    if not isinstance(body, dict):
        raise ValueError("body must be a JSON object: refusing")
    from kubona.codesign import (CircuitCodesignHarness, FamilySearch,
                                  build_spec, build_truth_table_spec)
    if "spec_table" in body and body["spec_table"] is not None:
        st = body["spec_table"]
        if not isinstance(st, dict):
            raise ValueError("spec_table must be an object: refusing")
        ins, outs, table = st.get("inputs", []), st.get("outputs", []), st.get("table", {})
        if not isinstance(ins, list) or not isinstance(outs, list) or not isinstance(table, dict):
            raise ValueError("spec_table needs inputs[]/outputs[]/table{}: refusing")
        spec = build_truth_table_spec(
            strict_int(st.get("num_qubits", 0), "num_qubits", 1, 64),
            [strict_int(q, "inputs[]", 0, 63) for q in ins],
            [strict_int(q, "outputs[]", 0, 63) for q in outs],
            table)
        label = "table"
    else:
        kind = body.get("spec_kind")
        spec = build_spec(str(kind))
        label = str(kind)
    families = body.get("families", [])
    kappa = strict_int(body.get("kappa", 1), "kappa", 1, 3)
    if not isinstance(families, list) or not (1 <= len(families) <= 8):
        raise ValueError("families must be a list of 1..8 distinct names: refusing")
    offers_in = body.get("offers", [])
    if not isinstance(offers_in, list) or len(offers_in) > 64:
        raise ValueError("offers must be a list of <=64: refusing")
    share_rounds = strict_int(body.get("share_rounds", 1), "share_rounds", 0, 3)
    num_tests = 32
    fs = FamilySearch(families, kappa=kappa)
    h = CircuitCodesignHarness(spec, num_tests=num_tests, max_failures=0,
                               kappa=kappa, seed=7)
    offers = []
    for o in offers_in:
        if not isinstance(o, dict) or "family" not in o or "artifact" not in o:
            raise ValueError("each offer needs {family, artifact}: refusing")
        fam = o["family"]
        r = h.run(o["artifact"])
        if not r["valid"]:
            offers.append({"family": fam, "valid": False})
            continue
        improved = fs.offer(fam, o["artifact"], r["toffoli"], r["peak_qubits"])
        offers.append({"family": fam, "valid": True,
                       "toffoli": r["toffoli"], "qubits": r["peak_qubits"],
                       "tq": r["tq"], "improved": improved})
    shares = []
    for _ in range(share_rounds):
        shares.append(fs.share_elites())
    print(json.dumps({"ok": True, "spec_kind": label, "kappa": kappa,
                      "offers": offers, "elites": fs.elites(),
                      "stagnation": dict(fs._stagnant),
                      "lineage_len": len(fs.lineage),
                      "lineage": fs.lineage[-128:],
                      "shares": shares, "widen_nudge": fs.widen_nudge()}))
except ValueError as e:
    print(json.dumps({"ok": False, "error": str(e)[:280]}))
except Exception as e:
    print(json.dumps({"ok": False, "error": f"families refused: {e}"[:280]}))
`

// budgetSnippet mirrors codesign_budget (entries<=32, Table-S4 math).
const budgetSnippet = `import json, sys
def strict_int(v, name, lo, hi):
    if isinstance(v, bool) or not isinstance(v, int) or not (lo <= v <= hi):
        raise ValueError(f"{name} must be an int in [{lo},{hi}]: refusing")
    return v
try:
    body = json.loads(sys.argv[1])
    if not isinstance(body, dict):
        raise ValueError("body must be a JSON object: refusing")
    from kubona.codesign import BudgetEntry, ledger_rows, union_bound
    raw = body.get("entries", [])
    if not isinstance(raw, list) or len(raw) > 32:
        raise ValueError("entries must be a list of <=32: refusing")
    entries = []
    for e in raw:
        if not isinstance(e, dict):
            raise ValueError(f"entry must be an object, got {e!r}: refusing")
        rate = e.get("per_use_rate", float("nan"))
        if isinstance(rate, bool) or not isinstance(rate, (int, float)):
            raise ValueError(f"per_use_rate must be a number, got {rate!r}: refusing")
        entries.append(BudgetEntry(str(e.get("source", "")),
                                   float(rate),
                                   strict_int(e.get("uses", -1), "uses", 0, 10**9)))
    print(json.dumps({"ok": True, "rows": ledger_rows(entries),
                      "union_bound": union_bound(entries)}))
except ValueError as e:
    print(json.dumps({"ok": False, "error": str(e)[:280]}))
except Exception as e:
    print(json.dumps({"ok": False, "error": f"budget refused: {e}"[:280]}))
`

// proxyCodesign is the shared rail: method gate, body cap, forward raw,
// map outer ok flag to status. Python owns all validation (single
// validator); Go owns transport + timeout + status mapping.
func (s *Server) proxyCodesign(w http.ResponseWriter, r *http.Request, script string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed: refusing", http.StatusMethodNotAllowed)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, codesignEvalMaxBody))
	if err != nil {
		WriteAPIError(w, "bad_request", "body too large or unreadable: refusing", nil)
		return
	}
	var probe any
	if json.Unmarshal(raw, &probe) != nil {
		WriteAPIError(w, "bad_request", "invalid JSON body: refusing", nil)
		return
	}
	out, _, err := runBridgePython(r.Context(), 120*time.Second, script, string(raw))
	if err != nil {
		http.Error(w, fmt.Sprintf("codesign evaluator unavailable: %v", err), http.StatusServiceUnavailable)
		return
	}
	cleanOut := bytes.TrimSpace(out)
	if idx := bytes.Index(cleanOut, []byte("{")); idx >= 0 {
		cleanOut = cleanOut[idx:]
	}
	var parsed struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if json.Unmarshal(cleanOut, &parsed) != nil {
		http.Error(w, "codesign evaluator returned unparseable output: refusing", http.StatusBadGateway)
		return
	}
	if !parsed.OK {
		msg := parsed.Error
		if msg == "" {
			msg = "evaluation refused"
		}
		WriteAPIError(w, "bad_request", msg, nil)
		return
	}
	var full any
	_ = json.Unmarshal(cleanOut, &full)
	writeJSON(w, http.StatusOK, full)
}

func (s *Server) handleCodesignEvaluate(w http.ResponseWriter, r *http.Request) {
	s.proxyCodesign(w, r, evalSnippet)
}

func (s *Server) handleCodesignEvaluateBatch(w http.ResponseWriter, r *http.Request) {
	s.proxyCodesign(w, r, batchSnippet)
}

func (s *Server) handleCodesignFamilies(w http.ResponseWriter, r *http.Request) {
	s.proxyCodesign(w, r, familiesSnippet)
}

func (s *Server) handleCodesignBudget(w http.ResponseWriter, r *http.Request) {
	s.proxyCodesign(w, r, budgetSnippet)
}
