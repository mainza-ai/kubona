package api

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/experimentresult"
)

const maxExperimentExportBytes = 8 * 1024 * 1024

type experimentExportJob struct {
	JobID         string                    `json:"job_id"`
	JobType       string                    `json:"job_type"`
	Status        string                    `json:"status"`
	ResultVersion string                    `json:"result_version,omitempty"`
	ResultSHA256  string                    `json:"result_sha256,omitempty"`
	Receipt       *experimentresult.Receipt `json:"result_receipt,omitempty"`
	Result        json.RawMessage           `json:"result,omitempty"`
}

func experimentExportKind(jobType string) bool {
	switch jobType {
	case "vqe_run", "geometry_optimization", "vqe_repeats", "vqe_pes", "qaoa_run",
		"excited_state", "reaction", "spin_chain", "thermodynamic_limit",
		"lattice_dynamics", "conjecture_run", "hardware_experiment", "hardware_pes",
		"qaoa_hardware", "spin_chain_hardware", "krylov_json", "mitigate_json":
		return true
	default:
		return false
	}
}

// handleExperimentExport serves a portable, signed experiment bundle. The
// envelope signs the canonical JSON payload (campaign + selected jobs and
// their receipts); it never rewrites or re-signs the underlying results.
func (s *Server) handleExperimentExport(w http.ResponseWriter, r *http.Request, sessionID string) {
	if _, ok := s.authorizeKnownSession(w, r, sessionID); !ok {
		return
	}
	rawState, err := s.ledger.GetCampaignState(sessionID)
	if err != nil {
		WriteAPIError(w, "internal_error", fmt.Sprintf("campaign read failed: %v", err), nil)
		return
	}
	list, err := s.jobRunner.ListJobs(sessionID, 100)
	if err != nil {
		WriteAPIError(w, "internal_error", fmt.Sprintf("job read failed: %v", err), nil)
		return
	}
	exportJobs := make([]experimentExportJob, 0, len(list))
	total := 0
	for _, job := range list {
		if job == nil || !experimentExportKind(job.JobType) || len(job.ResultJSON) == 0 {
			continue
		}
		total += len(job.ResultJSON)
		if total > maxExperimentExportBytes {
			WriteAPIError(w, "payload_too_large",
				fmt.Sprintf("experiment export exceeds %d bytes; refuse a silently truncated bundle", maxExperimentExportBytes), nil)
			return
		}
		var head struct {
			ResultVersion string `json:"result_version"`
		}
		_ = json.Unmarshal(job.ResultJSON, &head)
		hash := sha256.Sum256(job.ResultJSON)
		exportJobs = append(exportJobs, experimentExportJob{
			JobID: job.ID, JobType: job.JobType, Status: job.Status.String(),
			ResultVersion: head.ResultVersion,
			ResultSHA256:  hex.EncodeToString(hash[:]),
			Receipt:       experimentresult.ReceiptFromJSON(job.ResultJSON),
			Result:        job.ResultJSON,
		})
	}
	payload := map[string]any{
		"export_version": "experiment-export-v1",
		"session_id":     sessionID,
		"exported_at":    time.Now().UTC().Format(time.RFC3339),
		"campaign":       json.RawMessage(rawState),
		"jobs":           exportJobs,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		WriteAPIError(w, "internal_error", "export payload could not be encoded", nil)
		return
	}
	digest := sha256.Sum256(payloadBytes)
	envelope := map[string]any{
		"algorithm":      "ed25519",
		"domain":         "experiment-export-v1",
		"payload_sha256": hex.EncodeToString(digest[:]),
		"signed":         false,
		"signature":      "",
		"signer_pubkey":  "",
		"note":           "no signing key configured; payload SHA-256 is still recorded",
	}
	if len(s.signerPrivKey) == ed25519.PrivateKeySize {
		sig := ed25519.Sign(s.signerPrivKey, payloadBytes)
		envelope["signed"] = true
		envelope["signature"] = hex.EncodeToString(sig)
		envelope["signer_pubkey"] = hex.EncodeToString(s.signerPrivKey.Public().(ed25519.PublicKey))
		envelope["note"] = "signed over the exact payload bytes before this envelope was attached"
	}
	response := map[string]any{
		"ok": true, "payload": payload, "envelope": envelope,
	}
	_ = s.ledger.AppendEvent(sessionID, "experiment_exported", map[string]any{
		"export_version": "experiment-export-v1",
		"jobs":           len(exportJobs),
		"payload_sha256": envelope["payload_sha256"],
		"signed":         envelope["signed"],
	})
	writeJSON(w, http.StatusOK, response)
}

// experimentExportPath recognizes /api/sessions/{id}/experiment-export.
func experimentExportPath(id string) (string, bool) {
	const suffix = "/experiment-export"
	if strings.HasSuffix(id, suffix) {
		return strings.TrimSuffix(id, suffix), true
	}
	return "", false
}
