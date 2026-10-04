package mcp

import (
	"testing"
)

func TestGatewayPermissions(t *testing.T) {
	gw := NewGateway()

	// Standard profile allows submit_job
	allowed, msg := gw.CanCallTool("standard", "submit_job")
	if !allowed {
		t.Fatalf("expected submit_job allowed in standard profile, got: %s", msg)
	}

	// Low trust review denies submit_job
	allowed, msg = gw.CanCallTool("low_trust_review", "submit_job")
	if allowed {
		t.Fatalf("expected submit_job denied in low_trust_review profile")
	}

	// Low trust review allows get_job_status
	allowed, msg = gw.CanCallTool("low_trust_review", "get_job_status")
	if !allowed {
		t.Fatalf("expected get_job_status allowed in low_trust_review, got: %s", msg)
	}

	// Unknown tool rejected
	allowed, _ = gw.CanCallTool("standard", "unknown_tool")
	if allowed {
		t.Fatalf("expected unknown_tool rejected")
	}

	// Unknown profile rejected
	allowed, _ = gw.CanCallTool("unknown_profile", "get_job_status")
	if allowed {
		t.Fatalf("expected unknown_profile rejected")
	}
}

func TestGatewayListTools(t *testing.T) {
	gw := NewGateway()
	tools := gw.ListTools("low_trust_review")
	if len(tools) != 1 || tools[0].Name != "get_job_status" {
		t.Fatalf("expected only get_job_status visible in low_trust_review, got: %+v", tools)
	}
}
