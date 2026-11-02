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
}

func jobStatusWire(status pb.JobStatus) string {
	name := strings.TrimPrefix(status.String(), "JOB_STATUS_")
	if name == "" {
		return "unspecified"
	}
	return strings.ToLower(name)
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
	}
}

func jobViews(items []*jobs.Job) []*jobView {
	out := make([]*jobView, 0, len(items))
	for _, item := range items {
		out = append(out, jobViewFrom(item))
	}
	return out
}
