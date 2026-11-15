package jobs

// C2 conformance: the typed need vocabulary IS the contract file (drift =
// red), classification uses structured evidence only, and a nil need means
// "no known remedy" rather than a guess. The last test proves the need
// survives the failure write and the job read-back.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type needContract struct {
	Kinds map[string]struct {
		Sentence string `json:"sentence"`
		Remedy   struct {
			ID      string `json:"id"`
			Label   string `json:"label"`
			Surface string `json:"surface"`
			Section string `json:"section"`
		} `json:"remedy"`
	} `json:"kinds"`
}

func loadNeedContract(t *testing.T) needContract {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "need-kinds.json"))
	if err != nil {
		t.Fatalf("reading need contract: %v", err)
	}
	var contract needContract
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatalf("parsing need contract: %v", err)
	}
	return contract
}

func TestNeedRegistryMatchesContract(t *testing.T) {
	contract := loadNeedContract(t)
	if len(contract.Kinds) != len(needKinds) {
		t.Fatalf("registry has %d kinds, contract has %d", len(needKinds), len(contract.Kinds))
	}
	for kind := range needKinds {
		entry, ok := contract.Kinds[kind]
		if !ok {
			t.Fatalf("kind %q is not in contracts/need-kinds.json", kind)
		}
		if strings.TrimSpace(entry.Sentence) == "" {
			t.Fatalf("kind %q needs a sentence", kind)
		}
		if entry.Remedy.ID == "" || entry.Remedy.Label == "" || entry.Remedy.Surface == "" {
			t.Fatalf("kind %q needs exactly one named remedy, got %+v", kind, entry.Remedy)
		}
	}
	for kind := range contract.Kinds {
		if !needKinds[kind] {
			t.Fatalf("contract kind %q is not in the registry", kind)
		}
	}
}

func TestClassifyNeedUsesStructuredEvidence(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"unavailable", status.Error(codes.Unavailable, "dial tcp: connection refused"), NeedBackendOffline},
		{"wrapped unavailable", fmt.Errorf("RunExperiment RPC failed: %w", status.Error(codes.Unavailable, "down")), NeedBackendOffline},
		{"bridge sentinel", fmt.Errorf("%w: refusing circuit_run without a compute plane", ErrBridgeUnavailable), NeedBackendOffline},
		{"unauthenticated", status.Error(codes.Unauthenticated, "401 from provider"), NeedCredentials},
		{"permission denied", status.Error(codes.PermissionDenied, "403 from provider"), NeedCredentials},
		{"resource exhausted", status.Error(codes.ResourceExhausted, "quota hit"), NeedQuotaExhausted},
		{"deadline", status.Error(codes.DeadlineExceeded, "took too long"), NeedRetry},
		{"aborted", status.Error(codes.Aborted, "aborted"), NeedRetry},
		{"internal", status.Error(codes.Internal, "worker bug"), NeedRetry},
		{"context deadline", context.DeadlineExceeded, NeedRetry},
		// Deterministic refusals: retrying would not help, so no remedy is
		// offered. Unknown-failure prose NEVER becomes a need.
		{"invalid argument", status.Error(codes.InvalidArgument, "bad qasm"), ""},
		{"failed precondition", status.Error(codes.FailedPrecondition, "not ready"), ""},
		{"not found", status.Error(codes.NotFound, "gone"), ""},
		{"unimplemented", status.Error(codes.Unimplemented, "nope"), ""},
		{"vendor prose", errors.New("provider says: quota exceeded, insert coin"), ""},
		{"plain failure", errors.New("experiment returned no success result: {}"), ""},
		{"nil", nil, ""},
	}
	contract := loadNeedContract(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyNeed(tc.err)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("must not invent a need, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("want %q, got nil", tc.want)
			}
			if got.Kind != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got.Kind)
			}
			if !got.Valid() {
				t.Fatalf("classified need must validate: %+v", got)
			}
			if _, ok := contract.Kinds[got.Kind]; !ok {
				t.Fatalf("classified kind %q is not catalogued", got.Kind)
			}
			if got.Reason == "" {
				t.Fatalf("need must carry the verbatim reason")
			}
		})
	}
}

func TestNeedValidRefusesUnknownKind(t *testing.T) {
	for _, need := range []*Need{
		nil,
		{Kind: "improve_everything", Reason: "made up"},
		{Kind: NeedRetry, Reason: ""},
	} {
		if need.Valid() {
			t.Fatalf("invalid need must refuse: %+v", need)
		}
	}
}

// TestFailJobPersistsNeed proves the projection the console reads: the need
// is written beside the error and survives read-back, with the backend target
// taken from admitted params (never invented).
func TestFailJobPersistsNeed(t *testing.T) {
	dbPath := "test_jobs_need.db"
	os.Remove(dbPath)
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer func() {
		l.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}()
	sm := session.NewManager(l)
	if _, err := sm.EnsureSessionWithID("sess-need", "need drill", 100.0, "default"); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	r := NewRunner(l, nil, 1)
	defer r.Stop()

	j, err := r.SubmitJob("sess-need", "circuit_run", map[string]any{
		"qasm":              "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n",
		"shots":             100,
		"requested_backend": "aer",
		"max_cost_usd":      1.0,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	stored, err := r.GetJob(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	execErr := fmt.Errorf("RunExperiment RPC failed: %w", status.Error(codes.Unavailable, "bridge down"))
	r.failJobWithNeed(stored, execErr.Error(), needForJob(stored, execErr))

	got, err := r.GetJob(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != pb.JobStatus_JOB_STATUS_FAILED {
		t.Fatalf("job must be terminal FAILED, got %v", got.Status)
	}
	var payload struct {
		Error string `json:"error"`
		Need  *Need  `json:"need"`
	}
	if err := json.Unmarshal(got.ResultJSON, &payload); err != nil {
		t.Fatalf("result must stay JSON: %v", err)
	}
	if payload.Error == "" {
		t.Fatal("error text must remain beside the need")
	}
	if payload.Need == nil || payload.Need.Kind != NeedBackendOffline {
		t.Fatalf("want backend_offline need, got %+v", payload.Need)
	}
	if payload.Need.Target != "aer" {
		t.Fatalf("target must come from admitted params, got %q", payload.Need.Target)
	}
	if !strings.Contains(payload.Need.Reason, "bridge down") {
		t.Fatalf("reason must be verbatim, got %q", payload.Need.Reason)
	}
}
