package api

// C1 — run parts projection: recorded parts plus the CURRENT state of any job
// they dispatched. The part's recorded state is the state at emission; the
// projection upgrades it from the job row so a finished job never renders as
// still running.

import (
	"net/http"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	pb "github.com/mainza-ai/kubona/core/proto"
)

type loopPartView struct {
	supervisor.LoopPart
	// Need rides from the linked job (C2); a part with no known remedy has
	// none, and the lane shows only the job's status text.
	Need *jobs.Need `json:"need,omitempty"`
	// JobStatus is the live lowercase status when a job is linked.
	JobStatus string `json:"job_status,omitempty"`
}

func loopPartStateForJob(status pb.JobStatus) string {
	switch status {
	case pb.JobStatus_JOB_STATUS_COMPLETED:
		return "done"
	case pb.JobStatus_JOB_STATUS_FAILED:
		return "failed"
	case pb.JobStatus_JOB_STATUS_CANCELLED, pb.JobStatus_JOB_STATUS_INTERRUPTED:
		return "stopped"
	case pb.JobStatus_JOB_STATUS_RUNNING:
		return "running"
	case pb.JobStatus_JOB_STATUS_QUEUED, pb.JobStatus_JOB_STATUS_PAUSED,
		pb.JobStatus_JOB_STATUS_OUTCOME_UNKNOWN:
		return "waiting"
	default:
		return "waiting"
	}
}

// writeLoopParts serves GET /api/loops/{id}/parts. The caller has already
// bound the loop's session to the requester.
func (s *Server) writeLoopParts(w http.ResponseWriter, r *http.Request, loop *supervisor.ManagedLoop) {
	if r.Method != http.MethodGet {
		WriteAPIError(w, "method_not_allowed", "run parts are GET-only", nil)
		return
	}
	recorded, err := s.supervisor.LoopParts(loop.ID)
	if err != nil {
		WriteAPIError(w, "internal", "run parts read failed", nil)
		return
	}
	parts := make([]loopPartView, 0, len(recorded))
	for _, part := range recorded {
		view := loopPartView{LoopPart: part}
		if part.JobID != "" && s.jobRunner != nil {
			job, jerr := s.jobRunner.GetJob(part.JobID)
			if jerr == nil && job != nil {
				view.State = loopPartStateForJob(job.Status)
				view.JobStatus = jobStatusWire(job.Status)
				view.Now = view.JobStatus
				if job.CompletedAt != nil {
					view.EndedMS = job.CompletedAt.UnixMilli()
				}
				view.Need = needFromResult(job.ResultJSON)
			}
		}
		parts = append(parts, view)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "loop_id": loop.ID, "parts": parts, "count": len(parts),
	})
}
