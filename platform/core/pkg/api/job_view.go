package api

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	pb "github.com/mainza-ai/kubona/core/proto"
)

// jobView is the HTTP projection of a job. The wire status is the canonical
// lowercase string; status_code stays for one compatibility window so older
// clients can still read the numeric enum.
type jobView struct {
	ID               string          `json:"id"`
	SessionID        string          `json:"session_id"`
	Status           string          `json:"status"`
	StatusCode       int32           `json:"status_code"`
	JobType          string          `json:"job_type"`
	ParamsJSON       json.RawMessage `json:"params_json"`
	ResultJSON       json.RawMessage `json:"result_json,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	CompletedAt      *time.Time      `json:"completed_at,omitempty"`
	ReservedTenant   string          `json:"reserved_tenant,omitempty"`
	ReservedAmount   float64         `json:"reserved_amount,omitempty"`
	CompilerUsed     string          `json:"compiler_used,omitempty"`
	FallbackEstimate bool            `json:"fallback_estimate,omitempty"`
	MandateRef       string          `json:"mandate_ref,omitempty"`
	OwnerEpoch       int64           `json:"owner_epoch"`
	AttemptID        string          `json:"attempt_id,omitempty"`
	DispatchedAt     *time.Time      `json:"dispatched_at,omitempty"`
	VendorRef        string          `json:"vendor_ref,omitempty"`
	RequestID        string          `json:"request_id,omitempty"`
	// Need is the typed missing precondition (C2), persisted with the failure
	// result. Omitted when the cause is not known from structured evidence.
	Need *jobs.Need `json:"need,omitempty"`
}

func jobStatusWire(status pb.JobStatus) string {
	name := strings.TrimPrefix(status.String(), "JOB_STATUS_")
	if name == "" {
		return "unspecified"
	}
	return strings.ToLower(name)
}

// submittedJobView snapshots a job from the ledger before rendering a submit
// response. The pointer returned by SubmitJob is the live worker object; the
// worker mutates it as the job runs, so reading fields directly is a data
// race (caught by `go test -race`). A row read is a consistent snapshot.
func (s *Server) submittedJobView(job *jobs.Job) *jobView {
	if job == nil {
		return nil
	}
	if s != nil && s.jobRunner != nil {
		if snap, err := s.jobRunner.GetJob(job.ID); err == nil && snap != nil {
			return jobViewFrom(snap)
		}
	}
	return jobViewFrom(job)
}

func jobViewFrom(job *jobs.Job) *jobView {
	if job == nil {
		return nil
	}
	return &jobView{
		ID:               job.ID,
		SessionID:        job.SessionID,
		Status:           jobStatusWire(job.Status),
		StatusCode:       int32(job.Status),
		JobType:          job.JobType,
		ParamsJSON:       job.ParamsJSON,
		ResultJSON:       job.ResultJSON,
		CreatedAt:        job.CreatedAt,
		CompletedAt:      job.CompletedAt,
		ReservedTenant:   job.ReservedTenant,
		ReservedAmount:   job.ReservedAmount,
		CompilerUsed:     job.CompilerUsed,
		FallbackEstimate: job.FallbackEstimate,
		MandateRef:       job.MandateRef,
		OwnerEpoch:       job.OwnerEpoch,
		AttemptID:        job.AttemptID,
		DispatchedAt:     job.DispatchedAt,
		VendorRef:        job.VendorRef,
		RequestID:        job.RequestID,
		Need:             needFromResult(job.ResultJSON),
	}
}

// needFromResult reads the persisted need and refuse-ships anything whose
// kind is outside the closed vocabulary: an unknown kind is treated as
// absent, so the console never receives a remedy it cannot render.
func needFromResult(raw json.RawMessage) *jobs.Need {
	if len(raw) == 0 {
		return nil
	}
	var probe struct {
		Need *jobs.Need `json:"need"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil
	}
	if probe.Need == nil || !probe.Need.Valid() {
		return nil
	}
	return probe.Need
}

func jobViews(items []*jobs.Job) []*jobView {
	out := make([]*jobView, 0, len(items))
	for _, item := range items {
		out = append(out, jobViewFrom(item))
	}
	return out
}
