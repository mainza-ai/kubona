package api

import (
	"testing"

	pb "github.com/mainza-ai/kubona/core/proto"
)

func TestJobStatusWireIsClosedLowercase(t *testing.T) {
	cases := map[pb.JobStatus]string{
		pb.JobStatus_JOB_STATUS_QUEUED:      "queued",
		pb.JobStatus_JOB_STATUS_RUNNING:     "running",
		pb.JobStatus_JOB_STATUS_COMPLETED:   "completed",
		pb.JobStatus_JOB_STATUS_FAILED:      "failed",
		pb.JobStatus_JOB_STATUS_PAUSED:      "paused",
		pb.JobStatus_JOB_STATUS_CANCELLED:   "cancelled",
		pb.JobStatus_JOB_STATUS_UNSPECIFIED: "unspecified",
	}
	for status, want := range cases {
		if got := jobStatusWire(status); got != want {
			t.Errorf("%v: got %q want %q", status, got, want)
		}
	}
}
