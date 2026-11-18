package api

import (
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/autonomy"
)

func TestToolPermissionRefusal(t *testing.T) {
	policy := autonomy.DefaultPolicy()
	if err := toolPermissionRefusal(policy, "run_vqe"); err != nil {
		t.Fatalf("default allow = %v", err)
	}
	policy.ToolPermissions = map[string]string{"run_vqe": "block", "submit_local_sim": "ask"}
	if err := toolPermissionRefusal(policy, "run_vqe"); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("block = %v", err)
	}
	if err := toolPermissionRefusal(policy, "submit_local_sim"); err == nil || !strings.Contains(err.Error(), "requires approval") {
		t.Fatalf("ask = %v", err)
	}
	if err := toolPermissionRefusal(policy, "read_jobs"); err != nil {
		t.Fatalf("unlisted tool = %v", err)
	}
}
