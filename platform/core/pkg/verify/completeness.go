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
		rep.HardwareJobs++
		jid, _ := r["job_id"].(string)
		if _, hasSeed := r["seed"]; !hasSeed {
			missingSeed++
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

	// 4. Scholarly metadata & reference count (warning on draft; checked on submission)
	var ev map[string]any
	_ = json.Unmarshal([]byte(evidenceJSON), &ev)
	hasMeta := false
	if ev != nil {
		if meta, ok := ev["_scholarly"].(map[string]any); ok && len(meta) > 0 {
			hasMeta = true
		}
	}
	if !hasMeta {
		rep.Warnings = append(rep.Warnings, "scholarly metadata using default Kubona affiliations/COI/funding")
	}
	if citationsCount < 8 {
		msg := fmt.Sprintf("manuscript has %d citation(s); recommended >= 8 for archival submission", citationsCount)
		rep.Warnings = append(rep.Warnings, msg)
		rep.Checks = append(rep.Checks, CompletenessCheck{
			ID: "references_count", Passed: false, Severity: "warning", Detail: msg,
		})
	} else {
		rep.Checks = append(rep.Checks, CompletenessCheck{
			ID: "references_count", Passed: true, Severity: "warning", Detail: fmt.Sprintf("%d citations attached", citationsCount),
		})
	}

	return rep
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
