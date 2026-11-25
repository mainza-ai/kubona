package api

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/verify"
	"github.com/mainza-ai/kubona/core/pkg/version"
)

// enrichJobFromParams extracts compact circuit/execution provenance from
// j.ParamsJSON and j.ResultJSON into the campaign row WITHOUT embedding raw
// QASM text or full counts maps (Phase 0: preserves verification-v1 isolation
// so QASM rotation literals never pollute numeric evidence).
func enrichJobFromParams(row map[string]any, paramsJSON, resultJSON []byte) {
	var params map[string]any
	if len(paramsJSON) > 0 {
		if json.Unmarshal(paramsJSON, &params) == nil && params != nil {
			for _, k := range []string{"seed", "shots", "layout", "transpile", "basis", "active_space", "geometry", "bond_distance", "n_sites", "layers"} {
				if v, ok := params[k]; ok && v != nil {
					if cur, exists := row[k]; !exists || cur == nil {
						row[k] = v
					}
				}
			}
			if qasm, ok := params["qasm"].(string); ok && strings.TrimSpace(qasm) != "" {
				sum := sha256.Sum256([]byte(qasm))
				row["circuit_sha256"] = hex.EncodeToString(sum[:])
				row["qasm_bytes"] = len(qasm)
				lines, twoQ := summarizeQASMLines(qasm)
				if _, exists := row["qasm_gate_lines"]; !exists {
					row["qasm_gate_lines"] = lines
				}
				if _, exists := row["two_qubit_gate_count"]; !exists && twoQ > 0 {
					row["two_qubit_gate_count"] = twoQ
				}
			}
			if fq, ok := params["fidelity_qasm"].(string); ok && strings.TrimSpace(fq) != "" {
				sum := sha256.Sum256([]byte(fq))
				row["fidelity_circuit_sha256"] = hex.EncodeToString(sum[:])
			}
			if theta, ok := params["theta"].([]any); ok && len(theta) > 0 {
				row["theta_count"] = len(theta)
			}
			if opt, ok := params["optimizer"].(string); ok && strings.TrimSpace(opt) != "" {
				row["optimizer"] = opt
			}
			// Batch-shaped jobs (hardware PES) carry one circuit per point
			// rather than a single top-level qasm. Hash each point circuit so
			// the completeness gate can require every one of them in the
			// supplement instead of treating the whole job as circuit-less.
			if pts, ok := params["points"].([]any); ok && len(pts) > 0 {
				shas := make([]string, 0, len(pts))
				for _, p := range pts {
					pm, _ := p.(map[string]any)
					q, _ := pm["qasm"].(string)
					if strings.TrimSpace(q) == "" {
						continue
					}
					sum := sha256.Sum256([]byte(q))
					shas = append(shas, hex.EncodeToString(sum[:]))
				}
				if len(shas) > 0 {
					row["qasm_points"] = len(shas)
					row["point_circuits_sha256"] = shas
				}
			}
		}
	}
	if len(resultJSON) > 0 {
		var res map[string]any
		if json.Unmarshal(resultJSON, &res) == nil && res != nil {
			// Lattice/spin-chain results record `ci95` in Hartree while the
			// manuscript table expects `ci95_mha`. Without this mapping those
			// rows print as point estimates even though an interval exists.
			if row["ci95_mha"] == nil {
				if v, ok := res["ci95_mha"].(float64); ok && v > 0 {
					row["ci95_mha"] = v
				} else if v, ok := res["ci95"].(float64); ok && v > 0 {
					row["ci95_mha"] = v * 1000.0
				}
			}
			countsMap, _ := res["counts"].(map[string]any)
			if len(countsMap) == 0 {
				countsMap, _ = res["bitstring_counts"].(map[string]any)
			}
			countsByTerm, _ := res["counts_by_term"].(map[string]any)
			if len(countsMap) > 0 {
				rawCounts, _ := json.Marshal(countsMap)
				sum := sha256.Sum256(rawCounts)
				row["counts_sha256"] = hex.EncodeToString(sum[:])
				row["counts_unique_bitstrings"] = len(countsMap)
				row["top_k_counts"] = topKCounts(countsMap, 8)
				row["counts_status"] = "retained"
				// R2: a ratio interval must come from the sampled cut
				// distribution, not from treating the ratio as a binomial
				// proportion. The normal-approximation interval is derived here
				// from ledger counts and graph edges and recorded in the
				// campaign so verification-v1 can trace the printed bounds.
				if lo, hi, ok := ratioCIFromCounts(countsMap, params, res); ok {
					row["ratio_ci95"] = []float64{lo, hi}
					row["ratio_ci_method"] = "normal_approx_95"
				}
			} else if len(countsByTerm) > 0 {
				// Multi-term VQE/PES histograms are keyed by Pauli term; the
				// nested object is not a single bitstring distribution, so it
				// must not be reported as `counts_unique_bitstrings` or fed to
				// topKCounts (which would silently return an empty list).
				rawCounts, _ := json.Marshal(countsByTerm)
				sum := sha256.Sum256(rawCounts)
				row["counts_sha256"] = hex.EncodeToString(sum[:])
				row["counts_terms"] = len(countsByTerm)
				row["counts_status"] = "retained"
			} else if pts, ok := res["points"].([]any); ok && len(pts) > 0 {
				ptCounts := 0
				for _, p := range pts {
					if pm, ok := p.(map[string]any); ok {
						if pc, ok := pm["counts"].(map[string]any); ok && len(pc) > 0 {
							ptCounts++
						} else if cbt, ok := pm["counts_by_term"].(map[string]any); ok && len(cbt) > 0 {
							ptCounts++
						}
					}
				}
				if ptCounts > 0 {
					row["counts_status"] = "retained"
					row["counts_points"] = ptCounts
				} else {
					row["counts_status"] = "not_retained"
				}
			} else {
				row["counts_status"] = "not_retained"
			}
		}
	}
	if _, hasStatus := row["counts_status"]; !hasStatus {
		row["counts_status"] = "not_retained"
	}
}

// collectRepresentativeCircuits selects up to 6 representative QASM circuits
// (one per experiment kind, <= 32 KiB each) from the session's job ledger for
// transient circuit-diagram rendering (R1). Never persisted into CampaignJSON
// or EvidenceJSON so verification-v1 stays isolated from QASM rotation literals.
func (s *Server) collectRepresentativeCircuits(sessionID, campaignJSON string) map[string]string {
	if s.jobRunner == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	allJobs, err := s.jobRunner.ListAllSessionJobs(sessionID)
	if err != nil || len(allJobs) == 0 {
		return nil
	}
	jobsByID := make(map[string]*jobs.Job, len(allJobs))
	for _, j := range allJobs {
		if j != nil {
			jobsByID[j.ID] = j
		}
	}
	var camp map[string]any
	if json.Unmarshal([]byte(campaignJSON), &camp) != nil || camp == nil {
		return nil
	}
	rawList, _ := camp["experiments"].([]any)
	const maxSamples = 6
	const maxBytes = 32 * 1024
	out := map[string]string{}
	seenFamily := map[string]bool{}
	for _, item := range rawList {
		if len(out) >= maxSamples {
			break
		}
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		jid, _ := m["job_id"].(string)
		kind, _ := m["kind"].(string)
		fam := strings.ToLower(strings.TrimSpace(kind))
		if jid == "" || fam == "" || seenFamily[fam] {
			continue
		}
		j := jobsByID[jid]
		if j == nil || len(j.ParamsJSON) == 0 {
			continue
		}
		var params map[string]any
		if json.Unmarshal(j.ParamsJSON, &params) != nil || params == nil {
			continue
		}
		qasm, _ := params["qasm"].(string)
		if strings.TrimSpace(qasm) == "" {
			if pts, ok := params["points"].([]any); ok && len(pts) > 0 {
				if pm, ok := pts[0].(map[string]any); ok {
					qasm, _ = pm["qasm"].(string)
				}
			}
		}
		if strings.TrimSpace(qasm) == "" {
			continue
		}
		// R-D: for large transpiled circuits > 32 KiB, pass a clean newline-bounded
		// prefix so the schematic renderer still emits a folded diagram (full QASM
		// remains archived verbatim in supplement/qasm/).
		if len(qasm) > maxBytes {
			cut := strings.LastIndex(qasm[:maxBytes], "\n")
			if cut > 0 {
				qasm = "// KUBONA_PREFIX_TRUNCATED\n" + qasm[:cut]
			} else {
				qasm = "// KUBONA_PREFIX_TRUNCATED\n" + qasm[:maxBytes]
			}
		}
		seenFamily[fam] = true
		out[jid] = qasm
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ratioCIFromCounts derives a 95% normal-approximation interval for a QAOA
// approximation ratio from the sampled cut distribution.
//
// The ratio is not a binomial proportion: treating it as one (a Wilson
// interval over `shots`) is statistically invalid. Here the per-shot cut
// values are reconstructed from the recorded bitstrings and graph edges, the
// sample variance of the mean cut is estimated, and the interval is propagated
// through the ratio. The bounds are recorded in the campaign row so
// verification-v1 can trace the printed values back to ledger data.
func ratioCIFromCounts(counts map[string]any, params, res map[string]any) (float64, float64, bool) {
	edges := qaoaEdges(params, res)
	optimal := qaoaOptimalCut(params, res)
	if optimal <= 0 || len(edges) == 0 {
		return 0, 0, false
	}
	n := 0.0
	sum := 0.0
	sumSq := 0.0
	for bitstring, raw := range counts {
		cnt, ok := raw.(float64)
		if !ok || cnt <= 0 {
			continue
		}
		cut := float64(cutForBitstring(bitstring, edges))
		n += cnt
		sum += cnt * cut
		sumSq += cnt * cut * cut
	}
	if n < 2 {
		return 0, 0, false
	}
	mean := sum / n
	variance := (sumSq - n*mean*mean) / (n - 1)
	if variance < 0 {
		variance = 0
	}
	se := math.Sqrt(variance / n)
	ratio := mean / optimal
	half := 1.959963984540054 * se / optimal
	lo := math.Max(0, ratio-half)
	hi := math.Min(1, ratio+half)
	// Six decimals keep the campaign JSON stable while still rounding cleanly
	// to the three decimals printed in the paper.
	return math.Round(lo*1e6) / 1e6, math.Round(hi*1e6) / 1e6, true
}

func qaoaEdges(params, res map[string]any) []any {
	if params != nil {
		if e, ok := params["edges"].([]any); ok && len(e) > 0 {
			return e
		}
	}
	if res != nil {
		if g, ok := res["graph"].(map[string]any); ok {
			if e, ok := g["edges"].([]any); ok && len(e) > 0 {
				return e
			}
		}
	}
	return nil
}

func qaoaOptimalCut(params, res map[string]any) float64 {
	for _, m := range []map[string]any{res, params} {
		if m == nil {
			continue
		}
		if v, ok := m["optimal_cut"].(float64); ok && v > 0 {
			return v
		}
	}
	return 0
}

func cutForBitstring(bitstring string, edges []any) int {
	bits := []byte(strings.ReplaceAll(bitstring, " ", ""))
	for i, j := 0, len(bits)-1; i < j; i, j = i+1, j-1 {
		bits[i], bits[j] = bits[j], bits[i]
	}
	cut := 0
	for _, e := range edges {
		pair, ok := e.([]any)
		if !ok || len(pair) < 2 {
			continue
		}
		a, okA := pair[0].(float64)
		b, okB := pair[1].(float64)
		if !okA || !okB {
			continue
		}
		ia, ib := int(a), int(b)
		if ia < 0 || ib < 0 || ia >= len(bits) || ib >= len(bits) {
			continue
		}
		if bits[ia] != bits[ib] {
			cut++
		}
	}
	return cut
}

func summarizeQASMLines(qasm string) (gateLines int, twoQGates int) {
	for _, line := range strings.Split(qasm, "\n") {
		l := strings.TrimSpace(line)
		if l == "" || strings.HasPrefix(l, "//") || strings.HasPrefix(l, "OPENQASM") ||
			strings.HasPrefix(l, "include ") || strings.HasPrefix(l, "qreg ") ||
			strings.HasPrefix(l, "creg ") || strings.HasPrefix(l, "qubit") || strings.HasPrefix(l, "bit") {
			continue
		}
		gateLines++
		low := strings.ToLower(l)
		if strings.HasPrefix(low, "cx ") || strings.HasPrefix(low, "cz ") ||
			strings.HasPrefix(low, "ecr ") || strings.HasPrefix(low, "rzz") ||
			strings.HasPrefix(low, "swap ") || strings.HasPrefix(low, "ccx ") {
			twoQGates++
		}
	}
	return gateLines, twoQGates
}

func topKCounts(counts map[string]any, k int) []map[string]any {
	type kv struct {
		bit string
		val float64
	}
	items := make([]kv, 0, len(counts))
	for b, v := range counts {
		if f, ok := v.(float64); ok {
			items = append(items, kv{bit: b, val: f})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].val != items[j].val {
			return items[i].val > items[j].val
		}
		return items[i].bit < items[j].bit
	})
	if len(items) > k {
		items = items[:k]
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{"bitstring": it.bit, "count": int64(it.val)})
	}
	return out
}

// attachSupplementArtifacts stages the Phase 1/3 supplementary data bundle
// (QASM files, raw counts JSON, datasheet.json, jobs.csv, environment.json,
// and completeness_report.json) directly from the session's settled job ledger
// into ms.Artifacts so both Zenodo and RO-Crate exports carry full data parity.
func (s *Server) attachSupplementArtifacts(sessionID string, ms *ledger.Manuscript, artifacts map[string][]byte) {
	if artifacts == nil {
		return
	}
	var camp map[string]any
	_ = json.Unmarshal([]byte(ms.CampaignJSON), &camp)
	var exps []map[string]any
	if camp != nil {
		if rawList, ok := camp["experiments"].([]any); ok {
			for _, item := range rawList {
				if m, ok := item.(map[string]any); ok {
					exps = append(exps, m)
				}
			}
		}
	}

	jobsByID := map[string]*jobs.Job{}
	if s.jobRunner != nil {
		if allJobs, err := s.jobRunner.ListAllSessionJobs(sessionID); err == nil {
			for _, j := range allJobs {
				if j != nil {
					jobsByID[j.ID] = j
				}
			}
		}
	}

	maxCountsFileBytes := 256 * 1024
	if rawCap := strings.TrimSpace(os.Getenv("KUBONA_COUNTS_GZIP_THRESHOLD_BYTES")); rawCap != "" {
		if n, err := strconv.Atoi(rawCap); err == nil && n >= 1024 && n <= 2*1024*1024 {
			maxCountsFileBytes = n
		}
	}
	datasheetRows := make([]map[string]any, 0, len(exps))
	var csvBuf bytes.Buffer
	cw := csv.NewWriter(&csvBuf)
	_ = cw.Write([]string{
		"job_id", "kind", "backend", "hardware", "mitigated", "shots", "seed",
		"energy", "anchor_energy", "delta_mha", "ci95_mha", "ratio_ci95", "usage_seconds", "actual_usd",
		"circuit_sha256", "counts_sha256", "vendor_ref",
	})

	for _, r := range exps {
		jid, _ := r["job_id"].(string)
		if jid == "" {
			continue
		}
		j := jobsByID[jid]
		qasmPath := ""
		qasmPaths := []string{}
		var pointsSummary []map[string]any
		countsPath := ""
		if j != nil && len(j.ParamsJSON) > 0 {
			var params map[string]any
			if json.Unmarshal(j.ParamsJSON, &params) == nil && params != nil {
				if qasm, ok := params["qasm"].(string); ok && strings.TrimSpace(qasm) != "" {
					qasmPath = "supplement/qasm/" + jid + ".qasm"
					artifacts[qasmPath] = []byte(qasm)
					qasmPaths = append(qasmPaths, qasmPath)
				}
				if fq, ok := params["fidelity_qasm"].(string); ok && strings.TrimSpace(fq) != "" {
					artifacts["supplement/qasm/"+jid+"_fidelity.qasm"] = []byte(fq)
				}
				if pts, ok := params["points"].([]any); ok && len(pts) > 0 {
					for i, p := range pts {
						pm, _ := p.(map[string]any)
						q, _ := pm["qasm"].(string)
						if strings.TrimSpace(q) == "" {
							continue
						}
						path := fmt.Sprintf("supplement/qasm/%s_p%d.qasm", jid, i)
						artifacts[path] = []byte(q)
						qasmPaths = append(qasmPaths, path)
						row := map[string]any{"index": i, "qasm_path": path}
						for _, k := range []string{"bond_distance", "energy", "delta_mha", "energy_exact", "fci_exact_energy", "uncertainty"} {
							if v, has := pm[k]; has && v != nil {
								row[k] = v
							}
						}
						if u, ok := pm["uncertainty"].(float64); ok && u > 0 {
							row["ci95_mha"] = u * 1.959963984540054 * 1000.0
						}
						pointsSummary = append(pointsSummary, row)
					}
				}
			}
		}
		countsPaths := []string{}
		if j != nil && len(j.ResultJSON) > 0 {
			var res map[string]any
			if json.Unmarshal(j.ResultJSON, &res) == nil && res != nil {
				countsMap, _ := res["counts"].(map[string]any)
				if len(countsMap) == 0 {
					countsMap, _ = res["bitstring_counts"].(map[string]any)
				}
				if len(countsMap) == 0 {
					countsMap, _ = res["counts_by_term"].(map[string]any)
				}
				if len(countsMap) > 0 {
					if staged, ok := stageCountsArtifact("supplement/counts/"+jid, countsMap, maxCountsFileBytes, artifacts); ok {
						countsPath = staged
						countsPaths = append(countsPaths, staged)
					}
				}
				if pts, ok := res["points"].([]any); ok && len(pts) > 0 {
					for i, p := range pts {
						if pm, ok := p.(map[string]any); ok {
							countsObj, _ := pm["counts"].(map[string]any)
							if len(countsObj) == 0 {
								countsObj, _ = pm["counts_by_term"].(map[string]any)
							}
							if len(countsObj) > 0 {
								if staged, ok := stageCountsArtifact(fmt.Sprintf("supplement/counts/%s_p%d", jid, i), countsObj, maxCountsFileBytes, artifacts); ok {
									countsPaths = append(countsPaths, staged)
								}
							}
						}
					}
				}
			}
		}
		countsStatus := "not_retained"
		countsEncoding := ""
		countsFileSHA := ""
		countsArtifacts := []map[string]any{}
		if len(countsPaths) > 0 {
			countsStatus = "retained"
			for _, p := range countsPaths {
				enc, sha := countsArtifactMeta(p, artifacts)
				countsArtifacts = append(countsArtifacts, map[string]any{
					"path": p, "encoding": enc, "file_sha256": sha,
				})
			}
			primaryCounts := countsPath
			if primaryCounts == "" {
				primaryCounts = countsPaths[0]
			}
			countsEncoding, countsFileSHA = countsArtifactMeta(primaryCounts, artifacts)
		}

		// R4 + R-B: extract used physical qubits and slice per-qubit calibration if captured
		usedQubits := extractUsedQubits(r["layout"], r["transpile"])
		usedQubitCal := extractUsedQubitCalibration(r["calibration"], usedQubits)

		dsEntry := map[string]any{
			"job_id":                 jid,
			"kind":                   r["kind"],
			"backend":                r["backend"],
			"hardware":               r["hardware"],
			"mitigated":              r["mitigated"],
			"shots":                  r["shots"],
			"seed":                   r["seed"],
			"layout":                 r["layout"],
			"used_qubits":            usedQubits,
			"used_qubit_calibration": usedQubitCal,
			"transpile":              r["transpile"],
			"readout_mitigation":     r["readout_mitigation"],
			"error_suppression":      r["error_suppression"],
			"circuit_sha256":         r["circuit_sha256"],
			"qasm_path":              qasmPath,
			"qasm_paths":             qasmPaths,
			"qasm_points":            r["qasm_points"],
			"ratio_ci95":             r["ratio_ci95"],
			"ratio_ci_method":        r["ratio_ci_method"],
			"points":                 pointsSummary,
			"status":                 r["status"],
			"counts_status":          countsStatus,
			"counts_sha256":          r["counts_sha256"],
			"counts_encoding":        countsEncoding,
			"counts_file_sha256":     countsFileSHA,
			"counts_terms":           r["counts_terms"],
			"counts_path":            countsPath,
			"counts_paths":           countsPaths,
			"counts_artifacts":       countsArtifacts,
			"calibration":            r["calibration"],
			"energy":                 r["energy"],
			"anchor_energy":          firstNonNil(r, "fci_exact_energy", "active_space_fci_energy", "exact_energy", "energy_exact"),
			"delta_mha":              firstNonNil(r, "delta_mha", "delta"),
			"ci95_mha":               r["ci95_mha"],
			"usage_seconds":          r["usage_seconds"],
			"actual_usd":             r["actual_usd"],
			"vendor_ref":             r["vendor_ref"],
			"qasm_gate_lines":        r["qasm_gate_lines"],
			"two_qubit_gates":        r["two_qubit_gate_count"],
			"basis":                  r["basis"],
			"active_space":           r["active_space"],
			"geometry":               r["geometry"],
		}
		datasheetRows = append(datasheetRows, dsEntry)
		_ = cw.Write([]string{
			jid,
			fmtVal(r["kind"]),
			fmtVal(r["backend"]),
			fmtVal(r["hardware"]),
			fmtVal(r["mitigated"]),
			fmtVal(r["shots"]),
			fmtVal(r["seed"]),
			fmtVal(r["energy"]),
			fmtVal(dsEntry["anchor_energy"]),
			fmtVal(dsEntry["delta_mha"]),
			fmtVal(r["ci95_mha"]),
			fmtVal(r["ratio_ci95"]),
			fmtVal(r["usage_seconds"]),
			fmtVal(r["actual_usd"]),
			fmtVal(r["circuit_sha256"]),
			fmtVal(r["counts_sha256"]),
			fmtVal(r["vendor_ref"]),
		})
	}
	cw.Flush()

	if dsBytes, err := json.MarshalIndent(map[string]any{
		"schema":        "kubona-experiment-datasheet/v1",
		"manuscript_id": ms.ID,
		"session_id":    sessionID,
		"job_count":     len(datasheetRows),
		"jobs":          datasheetRows,
	}, "", "  "); err == nil {
		artifacts["supplement/datasheet.json"] = dsBytes
	}
	artifacts["supplement/jobs.csv"] = csvBuf.Bytes()

	envDoc := map[string]any{
		"schema":            "kubona-environment/v1",
		"go_version":        runtime.Version(),
		"ledger_schema":     ledger.CurrentSchemaVersion,
		"source_date_epoch": ms.SourceDateEpoch,
		"generated_at_utc":  time.Now().UTC().Format(time.RFC3339),
		"kubona_version":    version.Version,
	}
	if version.Commit != "" && version.Commit != "unknown" {
		envDoc["kubona_commit"] = version.Commit
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				envDoc["kubona_commit"] = s.Value
			case "vcs.time":
				envDoc["kubona_commit_time"] = s.Value
			case "vcs.modified":
				envDoc["kubona_commit_dirty"] = s.Value == "true"
			}
		}
	}
	if raw, ok := artifacts["supplement/tooling.json"]; ok {
		var tooling map[string]any
		if json.Unmarshal(raw, &tooling) == nil && tooling != nil {
			envDoc["tooling"] = tooling
			if lh, ok := tooling["uv_lock_sha256"].(string); ok && lh != "" {
				envDoc["uv_lock_sha256"] = lh
			}
			if bc, ok := tooling["bridge_commit"].(string); ok && bc != "" && bc != "unknown" {
				envDoc["bridge_commit"] = bc
			}
		}
	}
	if envBytes, err := json.MarshalIndent(envDoc, "", "  "); err == nil {
		artifacts["supplement/environment.json"] = envBytes
	}

	artIndex := make(map[string]string, len(artifacts))
	for name, data := range artifacts {
		sum := sha256.Sum256(data)
		artIndex[name] = hex.EncodeToString(sum[:])
	}
	// Count the bibliography entries actually emitted into main.bbl. The
	// canonical method references are \nocite'd, so this is larger than the
	// caller-supplied citation count; using it keeps the completeness warning
	// truthful about what a reader can see in the PDF.
	bibliographyCount := 0
	if bbl, ok := artifacts["main.bbl"]; ok {
		bibliographyCount = bytes.Count(bbl, []byte(`\bibitem`))
	}
	comp := verify.CheckPublicationCompleteness(ms.CampaignJSON, ms.EvidenceJSON, len(ms.Citations), bibliographyCount, artIndex)
	if compBytes, err := json.MarshalIndent(comp, "", "  "); err == nil {
		artifacts["supplement/completeness_report.json"] = compBytes
	}
}

func firstNonNil(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

func extractUsedQubits(layout, transpile any) []int {
	seen := map[int]bool{}
	// Layouts arrive in several shapes: a flat list of physical qubits, a
	// logical->physical map, or a nested {"initial": {...}, "final": {...}}
	// object. Descend recursively so the nested shape is not silently empty.
	var collect func(v any, depth int)
	collect = func(v any, depth int) {
		if depth > 4 {
			return
		}
		switch t := v.(type) {
		case []any:
			for _, x := range t {
				collect(x, depth+1)
			}
		case map[string]any:
			for _, x := range t {
				collect(x, depth+1)
			}
		case float64:
			if t >= 0 {
				seen[int(t)] = true
			}
		}
	}
	collect(layout, 0)
	if tm, ok := transpile.(map[string]any); ok {
		collect(tm["layout"], 0)
		collect(tm["final_layout"], 0)
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]int, 0, len(seen))
	for q := range seen {
		out = append(out, q)
	}
	sort.Ints(out)
	return out
}

// extractUsedQubitCalibration slices calibration.per_qubit down to the
// physical qubits that actually executed the circuit (R-B). Returns nil when
// the snapshot predates per-qubit capture.
func extractUsedQubitCalibration(cal any, usedQubits []int) map[string]any {
	cm, ok := cal.(map[string]any)
	if !ok || len(usedQubits) == 0 {
		return nil
	}
	pq, ok := cm["per_qubit"].(map[string]any)
	if !ok || len(pq) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, q := range usedQubits {
		key := strconv.Itoa(q)
		if v, exists := pq[key]; exists {
			out[key] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// stageCountsArtifact writes a shot histogram as `<base>.json` when <= plainCap
// bytes, or automatically gzip-compresses it as `<base>.json.gz` (up to 2 MiB
// compressed) so large multi-term VQE/PES histograms are never silently dropped (H3 / C-B).
func stageCountsArtifact(base string, obj any, plainCap int, artifacts map[string][]byte) (string, bool) {
	raw, err := json.Marshal(obj)
	if err != nil || len(raw) == 0 {
		return "", false
	}
	if len(raw) <= plainCap {
		p := base + ".json"
		artifacts[p] = raw
		return p, true
	}
	const maxGzipBytes = 2 * 1024 * 1024
	var buf bytes.Buffer
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return "", false
	}
	if _, err := gz.Write(raw); err != nil {
		_ = gz.Close()
		return "", false
	}
	if err := gz.Close(); err != nil || buf.Len() > maxGzipBytes {
		return "", false
	}
	p := base + ".json.gz"
	artifacts[p] = buf.Bytes()
	return p, true
}

// countsArtifactMeta returns the on-disk encoding ("json" or "gzip") and
// SHA-256 hex digest of the staged artifact bytes so downstream consumers can
// verify the archive file directly alongside the uncompressed counts_sha256 (C-C).
func countsArtifactMeta(stagedPath string, artifacts map[string][]byte) (string, string) {
	if stagedPath == "" {
		return "", ""
	}
	encoding := "json"
	if strings.HasSuffix(stagedPath, ".gz") {
		encoding = "gzip"
	}
	raw, ok := artifacts[stagedPath]
	if !ok || len(raw) == 0 {
		return encoding, ""
	}
	sum := sha256.Sum256(raw)
	return encoding, hex.EncodeToString(sum[:])
}

func fmtVal(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'g', 10, 64)
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func (s *Server) handleManuscriptCompleteness(w http.ResponseWriter, r *http.Request, sessionID, id string) {
	if r.Method != http.MethodGet {
		WriteAPIError(w, "method_not_allowed", "completeness check is GET-only", nil)
		return
	}
	ms, err := s.ledger.GetManuscript(sessionID, id)
	if errors.Is(err, ledger.ErrManuscriptNotFound) {
		WriteAPIError(w, "not_found", "unknown manuscript", nil)
		return
	}
	if err != nil {
		WriteAPIError(w, "internal", "manuscript read failed", nil)
		return
	}
	artIndex := make(map[string]string, len(ms.Artifacts))
	for _, a := range ms.Artifacts {
		artIndex[a.Name] = a.SHA256
	}
	bibliographyCount := 0
	if bbl, _, aerr := s.ledger.GetManuscriptArtifact(sessionID, id, "main.bbl"); aerr == nil {
		bibliographyCount = bytes.Count(bbl, []byte(`\bibitem`))
	}
	rep := verify.CheckPublicationCompleteness(ms.CampaignJSON, ms.EvidenceJSON, len(ms.Citations), bibliographyCount, artIndex)
	writeJSON(w, http.StatusOK, map[string]any{
		"manuscript_id": id,
		"session_id":    sessionID,
		"completeness":  rep,
	})
}
