package jobs

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateQASMAcceptsBell(t *testing.T) {
	bell := "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\ncreg c[2];\nh q[0];\ncx q[0], q[1];\nmeasure q[0] -> c[0];\nmeasure q[1] -> c[1];\n"
	if err := validateQASM(bell); err != nil {
		t.Errorf("valid Bell QASM refused: %v", err)
	}
}

func TestValidateQASMRefusals(t *testing.T) {
	cases := []struct {
		name string
		qasm string
		want string
	}{
		{"empty", "   ", "must not be empty"},
		{"no header", "qreg q[2];\nh q[0];", "OPENQASM"},
		{"no qreg", "OPENQASM 2.0;\nh q[0];", "qreg"},
		{"unbalanced close", "OPENQASM 2.0;\nqreg q[2];\n}", "unbalanced"},
		{"unclosed open", "OPENQASM 2.0;\nqreg q[2];\ngate g a {", "unclosed"},
		{"comment brackets ignored", "OPENQASM 2.0;\n// } junk {\nqreg q[1];", ""},
	}
	for _, tc := range cases {
		err := validateQASM(tc.qasm)
		if tc.want == "" {
			if err != nil {
				t.Errorf("%s: should pass, got %v", tc.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want refusal containing %q", tc.name, err, tc.want)
		}
	}
}

func TestSubmitJobRefusesMalformedQASMWithoutResidue(t *testing.T) {
	r, cleanup := setupRunner(t, &fakeBridge{})
	defer cleanup()
	_, err := r.SubmitJob("sess-1", "circuit_run", map[string]any{
		"qasm":              "OPENQASM 2.0;\nqreg q[1];\n}",
		"requested_backend": "local_simulator",
	})
	if err == nil || !strings.Contains(err.Error(), "unbalanced") {
		t.Fatalf("expected structural refusal, got %v", err)
	}
	jobs, lerr := r.ListJobs("sess-1", 10)
	if lerr != nil {
		t.Fatal(lerr)
	}
	for _, j := range jobs {
		var p map[string]any
		_ = json.Unmarshal(j.ParamsJSON, &p)
		if q, _ := p["qasm"].(string); strings.Contains(q, "qreg q[1];\n}") {
			t.Error("refused submit must leave no row")
		}
	}
}
