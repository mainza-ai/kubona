package verify

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CompletenessCheck records one auditable criterion in the Phase 6 publication
// completeness gate.
type CompletenessCheck struct {
	ID       string `json:"id"`
	Passed   bool   `json:"passed"`
	Severity string `json:"severity"` // "blocking" | "warning"
	Detail   string `json:"detail"`
}

// CompletenessReport is the fail-closed publication completeness verdict
// (Phase 6). Empirical deposits with hardware results fail closed (409
// incomplete_manuscript) when any blocking check fails.
type CompletenessReport struct {
	Complete                bool                `json:"complete"`
	HardwareJobs            int                 `json:"hardware_jobs"`
	HardwareCircuitsBundled int                 `json:"hardware_circuits_bundled"`
	SupplementFiles         int                 `json:"supplement_files"`
	Blocking                []string            `json:"blocking"`
	Warnings                []string            `json:"warnings"`
	Checks                  []CompletenessCheck `json:"checks"`
}

// CheckPublicationCompleteness verifies that a built manuscript bundle carries
// every required circuit, datasheet, environment, and scholarly artifact.
func CheckPublicationCompleteness(
	campaignJSON string,
	evidenceJSON string,
	citationsCount int,
	bibliographyCount int,
	artifactNames map[string]string,
) CompletenessReport {
	rep := CompletenessReport{
		Complete: true,
		Blocking: []string{},
		Warnings: []string{},
		Checks:   []CompletenessCheck{},
	}

	var camp map[string]any
	_ = json.Unmarshal([]byte(campaignJSON), &camp)
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

	for name := range artifactNames {
		if strings.HasPrefix(name, "supplement/") {
			rep.SupplementFiles++
		}
	}

	// 1. Hardware circuits bundled (blocking when hardware jobs exist)
	missingQASM := []string{}
	missingSeed := 0
	for _, r := range exps {
		isHW, _ := r["hardware"].(bool)
		kind, _ := r["kind"].(string)
		if !isHW && !strings.Contains(strings.ToLower(kind), "hardware") {
			continue
		}
		// A failed/cancelled submission has no settled result to reproduce;
		// requiring its circuit would make every campaign with a retried job
		// permanently incomplete. Only settled rows are gated.
		if !isSettledRow(r) {
			continue
		}
		rep.HardwareJobs++
		jid, _ := r["job_id"].(string)
		if seed, hasSeed := r["seed"]; !hasSeed || seed == nil {
			missingSeed++
		}
		points := intValue(r["qasm_points"])
		if points > 0 {
			// Batch-shaped jobs (hardware PES) carry one circuit per point.
			allPresent := true
			for i := 0; i < points; i++ {
				if artifactNames[fmt.Sprintf("supplement/qasm/%s_p%d.qasm", jid, i)] == "" {
					allPresent = false
					break
				}
			}
			if allPresent {
				rep.HardwareCircuitsBundled++
			} else {
				missingQASM = append(missingQASM, fmt.Sprintf("%s (point circuits)", jid))
			}
			continue
		}
		qasmPath := "supplement/qasm/" + jid + ".qasm"
		cHash, _ := r["circuit_sha256"].(string)
		if cHash != "" && artifactNames[qasmPath] != "" {
			rep.HardwareCircuitsBundled++
		} else {
			missingQASM = append(missingQASM, jid)
		}
	}
	if rep.HardwareJobs > 0 {
		ok := len(missingQASM) == 0
		detail := fmt.Sprintf("%d/%d hardware jobs have bundled QASM + circuit_sha256", rep.HardwareCircuitsBundled, rep.HardwareJobs)
		if !ok {
			detail = fmt.Sprintf("missing QASM bundle for %d hardware job(s): %s", len(missingQASM), strings.Join(missingQASM[:minInt(5, len(missingQASM))], ", "))
			rep.Blocking = append(rep.Blocking, "hardware_circuits_bundled: "+detail)
			rep.Complete = false
		}
		rep.Checks = append(rep.Checks, CompletenessCheck{
			ID: "hardware_circuits_bundled", Passed: ok, Severity: "blocking", Detail: detail,
		})
	} else {
		rep.Checks = append(rep.Checks, CompletenessCheck{
			ID: "hardware_circuits_bundled", Passed: true, Severity: "blocking", Detail: "no hardware jobs in campaign",
		})
	}

	// 1b. At least one circuit figure when hardware results are claimed (R1).
	circuitFigures := 0
	for name := range artifactNames {
		if strings.HasPrefix(name, "figures/fig_circuit_") {
			circuitFigures++
		}
	}
	if rep.HardwareJobs > 0 {
		ok := circuitFigures > 0
		detail := fmt.Sprintf("%d circuit schematic figure(s) embedded", circuitFigures)
		if !ok {
			detail = "hardware results are claimed but no figures/fig_circuit_*.png schematic is embedded"
			rep.Blocking = append(rep.Blocking, "hardware_circuit_figures: "+detail)
			rep.Complete = false
		}
		rep.Checks = append(rep.Checks, CompletenessCheck{
			ID: "hardware_circuit_figures", Passed: ok, Severity: "blocking", Detail: detail,
		})
	}

	// 2. Experiment Datasheet + Jobs CSV + Environment JSON (blocking when any settled job exists)
	hasDatasheet := artifactNames["supplement/datasheet.json"] != ""
	hasJobsCSV := artifactNames["supplement/jobs.csv"] != ""
	hasEnv := artifactNames["supplement/environment.json"] != ""
	dataBundleOK := hasDatasheet && hasJobsCSV && hasEnv
	if len(exps) > 0 && !dataBundleOK {
		missing := []string{}
		if !hasDatasheet {
			missing = append(missing, "supplement/datasheet.json")
		}
		if !hasJobsCSV {
			missing = append(missing, "supplement/jobs.csv")
		}
		if !hasEnv {
			missing = append(missing, "supplement/environment.json")
		}
		msg := "missing supplement files: " + strings.Join(missing, ", ")
		rep.Blocking = append(rep.Blocking, "datasheet_bundle: "+msg)
		rep.Complete = false
		rep.Checks = append(rep.Checks, CompletenessCheck{
			ID: "datasheet_bundle", Passed: false, Severity: "blocking", Detail: msg,
		})
	} else {
		rep.Checks = append(rep.Checks, CompletenessCheck{
			ID: "datasheet_bundle", Passed: true, Severity: "blocking", Detail: "datasheet.json, jobs.csv, and environment.json present",
		})
	}

	// 3. Seed provenance warning
	if missingSeed > 0 {
		msg := fmt.Sprintf("%d hardware job(s) missing seed metadata", missingSeed)
		rep.Warnings = append(rep.Warnings, msg)
		rep.Checks = append(rep.Checks, CompletenessCheck{
			ID: "seeds_recorded", Passed: false, Severity: "warning", Detail: msg,
		})
	} else {
		rep.Checks = append(rep.Checks, CompletenessCheck{
			ID: "seeds_recorded", Passed: true, Severity: "warning", Detail: "all hardware jobs carry seed metadata",
		})
	}

	// 4. Scholarly metadata & reference count (warning on draft; enforced at deposit)
	missingMeta := CheckScholarlyMetadata(evidenceJSON)
	if len(missingMeta) > 0 {
		msg := "scholarly metadata missing or using placeholder defaults: " + strings.Join(missingMeta, ", ")
		rep.Warnings = append(rep.Warnings, msg)
		rep.Checks = append(rep.Checks, CompletenessCheck{
			ID: "scholarly_metadata", Passed: false, Severity: "warning", Detail: msg,
		})
	} else {
		rep.Checks = append(rep.Checks, CompletenessCheck{
			ID: "scholarly_metadata", Passed: true, Severity: "warning", Detail: "operator scholarly metadata supplied",
		})
	}
	if citationsCount < 8 && bibliographyCount < 8 {
		msg := fmt.Sprintf("manuscript has %d citation(s) and %d printed bibliography entr(ies); recommended >= 8 for archival submission", citationsCount, bibliographyCount)
		rep.Warnings = append(rep.Warnings, msg)
		rep.Checks = append(rep.Checks, CompletenessCheck{
			ID: "references_count", Passed: false, Severity: "warning", Detail: msg,
		})
	} else {
		rep.Checks = append(rep.Checks, CompletenessCheck{
			ID: "references_count", Passed: true, Severity: "warning",
			Detail: fmt.Sprintf("%d printed bibliography entr(ies) (%d caller-supplied)", bibliographyCount, citationsCount),
		})
	}

	return rep
}

// isSettledRow reports whether a campaign row reached a terminal state that
// produced a result. Failed, cancelled, and rejected submissions are excluded
// from circuit/seed completeness requirements.
func isSettledRow(r map[string]any) bool {
	st, _ := r["status"].(string)
	switch strings.ToLower(strings.TrimSpace(st)) {
	case "failed", "error", "cancelled", "canceled", "rejected", "aborted":
		return false
	}
	return true
}

func intValue(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	default:
		return 0
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// CheckScholarlyMetadata returns the list of required scholarly metadata fields
// that are missing or empty in evidenceJSON._scholarly (R6 deposit gate).
func CheckScholarlyMetadata(evidenceJSON string) []string {
	required := []string{"affiliation", "corresponding_author", "competing_interests", "funding", "license"}
	var ev map[string]any
	if err := json.Unmarshal([]byte(evidenceJSON), &ev); err != nil || ev == nil {
		return required
	}
	meta, _ := ev["_scholarly"].(map[string]any)
	if len(meta) == 0 {
		return required
	}
	var missing []string
	for _, k := range required {
		val, _ := meta[k].(string)
		trimmed := strings.TrimSpace(val)
		if trimmed == "" || strings.EqualFold(trimmed, "placeholder") || strings.EqualFold(trimmed, "tbd") {
			missing = append(missing, k)
		}
	}
	return missing
}
