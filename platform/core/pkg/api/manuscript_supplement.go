package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/verify"
)

// enrichJobFromParams extracts compact circuit/execution provenance from
// j.ParamsJSON and j.ResultJSON into the campaign row WITHOUT embedding raw
// QASM text or full counts maps (Phase 0: preserves verification-v1 isolation
// so QASM rotation literals never pollute numeric evidence).
func enrichJobFromParams(row map[string]any, paramsJSON, resultJSON []byte) {
	if len(paramsJSON) > 0 {
		var params map[string]any
		if json.Unmarshal(paramsJSON, &params) == nil && params != nil {
			for _, k := range []string{"seed", "shots", "layout", "transpile", "basis", "active_space", "geometry", "layers"} {
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
			if counts, ok := res["counts"].(map[string]any); ok && len(counts) > 0 {
				rawCounts, _ := json.Marshal(counts)
				sum := sha256.Sum256(rawCounts)
				row["counts_sha256"] = hex.EncodeToString(sum[:])
				row["counts_unique_bitstrings"] = len(counts)
				row["top_k_counts"] = topKCounts(counts, 8)
			}
		}
	}
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
		if allJobs, err := s.jobRunner.ListJobs(sessionID, 200); err == nil {
			for _, j := range allJobs {
				if j != nil {
					jobsByID[j.ID] = j
				}
			}
		}
	}

	const maxCountsFileBytes = 256 * 1024
	datasheetRows := make([]map[string]any, 0, len(exps))
	var csvBuf bytes.Buffer
	cw := csv.NewWriter(&csvBuf)
	_ = cw.Write([]string{
		"job_id", "kind", "backend", "hardware", "mitigated", "shots", "seed",
		"energy", "anchor_energy", "delta_mha", "ci95_mha", "usage_seconds", "actual_usd",
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
						for _, k := range []string{"bond_distance", "energy", "delta_mha", "energy_exact", "fci_exact_energy"} {
							if v, has := pm[k]; has && v != nil {
								row[k] = v
							}
						}
						pointsSummary = append(pointsSummary, row)
					}
				}
			}
		}
		if j != nil && len(j.ResultJSON) > 0 {
			var res map[string]any
			if json.Unmarshal(j.ResultJSON, &res) == nil && res != nil {
				if counts, ok := res["counts"].(map[string]any); ok && len(counts) > 0 {
					if raw, err := json.Marshal(counts); err == nil && len(raw) <= maxCountsFileBytes {
						countsPath = "supplement/counts/" + jid + ".json"
						artifacts[countsPath] = raw
					}
				}
			}
		}

		dsEntry := map[string]any{
			"job_id":          jid,
			"kind":            r["kind"],
			"backend":         r["backend"],
			"hardware":        r["hardware"],
			"mitigated":       r["mitigated"],
			"shots":           r["shots"],
			"seed":            r["seed"],
			"layout":          r["layout"],
			"transpile":       r["transpile"],
			"circuit_sha256":  r["circuit_sha256"],
			"qasm_path":       qasmPath,
			"qasm_paths":      qasmPaths,
			"qasm_points":     r["qasm_points"],
			"points":          pointsSummary,
			"status":          r["status"],
			"counts_sha256":   r["counts_sha256"],
			"counts_path":     countsPath,
			"calibration":     r["calibration"],
			"energy":          r["energy"],
			"anchor_energy":   firstNonNil(r, "fci_exact_energy", "active_space_fci_energy", "exact_energy", "energy_exact"),
			"delta_mha":       firstNonNil(r, "delta_mha", "delta"),
			"ci95_mha":        r["ci95_mha"],
			"usage_seconds":   r["usage_seconds"],
			"actual_usd":      r["actual_usd"],
			"vendor_ref":      r["vendor_ref"],
			"qasm_gate_lines": r["qasm_gate_lines"],
			"two_qubit_gates": r["two_qubit_gate_count"],
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
