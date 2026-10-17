package jobs

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateQASMAcceptsBell(t *testing.T) {
	bell := "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\ncreg c[2];\nh q[0];\ncx q[0], q[1];\nmeasure q[0] -> c[0];\nmeasure q[1] -> c[1];\n"
	if err := ValidateQASM(bell); err != nil {
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
		err := ValidateQASM(tc.qasm)
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

func TestValidateQASMSemanticsAccepts(t *testing.T) {
	cases := []struct {
		name string
		qasm string
	}{
		{"bell", "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\ncreg c[2];\nh q[0];\ncx q[0], q[1];\nmeasure q[0] -> c[0];\nmeasure q[1] -> c[1];\n"},
		{"param expressions", "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nrz(pi/2) q[0];\nu3(0.1,0.2,0.3) q[0];\nmeasure q[0] -> c[0];\n"},
		{"custom gate def and use", "OPENQASM 2.0;\nqreg q[2];\ngate mygate(theta) a, b {\nh a;\ncx a, b;\n}\nmygate(0.5) q[0], q[1];\n"},
		{"opaque decl and use", "OPENQASM 2.0;\nqreg q[1];\nopaque phase(theta) q;\nphase(0.1) q[0];\n"},
		{"whole-register barrier", "OPENQASM 2.0;\nqreg q[2];\ncreg c[2];\nh q[0];\nbarrier q;\nmeasure q[0] -> c[0];\nmeasure q[1] -> c[1];\n"},
		{"whole-register measure", "OPENQASM 2.0;\nqreg q[2];\ncreg c[2];\nh q[0];\nmeasure q -> c;\n"},
		{"conditional", "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];\nif (c==1) x q[0];\n"},
		{"reset", "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nreset q[0];\nmeasure q[0] -> c[0];\n"},
		{"unknown gate allowed", "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nfuturegate q[0];\nmeasure q[0] -> c[0];\n"},
		{"multiple registers", "OPENQASM 2.0;\nqreg a[1];\nqreg b[1];\ncreg c[2];\ncx a[0], b[0];\nmeasure a[0] -> c[0];\nmeasure b[0] -> c[1];\n"},
	}
	for _, tc := range cases {
		if err := ValidateQASM(tc.qasm); err != nil {
			t.Errorf("%s: valid program refused: %v", tc.name, err)
		}
	}
}

func TestValidateQASMSemanticsRefusals(t *testing.T) {
	cases := []struct {
		name string
		qasm string
		want string
	}{
		{"index out of range", "OPENQASM 2.0;\nqreg q[2];\nh q[5];\n", "out of range"},
		{"undeclared register", "OPENQASM 2.0;\nqreg q[1];\nh r[0];\n", "undeclared register"},
		{"measure into qreg", "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nmeasure q[0] -> q[0];\n", "undeclared register"},
		{"measure width mismatch", "OPENQASM 2.0;\nqreg q[2];\ncreg c[2];\nmeasure q -> c[0];\n", "width mismatch"},
		{"cx needs two qubits", "OPENQASM 2.0;\nqreg q[2];\ncx q[0];\n", "takes 2 qubits"},
		{"h takes one qubit", "OPENQASM 2.0;\nqreg q[2];\nh q[0], q[1];\n", "takes 1 qubits"},
		{"rz needs a parameter", "OPENQASM 2.0;\nqreg q[1];\nrz q[0];\n", "takes 1 parameters"},
		{"h takes no parameters", "OPENQASM 2.0;\nqreg q[1];\nh(0.5) q[0];\n", "takes 0 parameters"},
		{"duplicate qreg", "OPENQASM 2.0;\nqreg q[1];\nqreg q[2];\n", "redeclared"},
		{"qreg/creg clash", "OPENQASM 2.0;\nqreg q[1];\ncreg q[1];\n", "already declared as qreg"},
		{"header not first", "qreg q[1];\nOPENQASM 2.0;\nh q[0];\n", "first statement must be"},
		{"qasm3 refused honestly", "OPENQASM 3.0;\nqubit[2] q;\nh q[0];\n", "not yet admitted"},
		{"barrier on creg", "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nbarrier c[0];\n", "used as qubit"},
		{"if on undeclared creg", "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nif (d==1) x q[0];\n", "undeclared creg"},
		{"mixed register and indexed", "OPENQASM 2.0;\nqreg a[2];\nqreg b[2];\ncx a, b[0];\n", "mixes register and indexed"},
		{"custom gate arity", "OPENQASM 2.0;\nqreg q[2];\ngate g a, b {\ncx a, b;\n}\ng q[0];\n", "takes 2 qubits"},
		{"zero-size register", "OPENQASM 2.0;\nqreg q[0];\n", "non-positive size"},
	}
	for _, tc := range cases {
		err := ValidateQASM(tc.qasm)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want refusal containing %q", tc.name, err, tc.want)
		}
	}
}

func TestValidateQASMRefusesLiteralBackslashN(t *testing.T) {
	// Wire format (JSON) decodes \n to real newlines before dispatch; a
	// program that still carries literal backslash-n sequences is malformed
	// text, not a program. Several fixtures made this mistake (2026-10-11).
	qasm := "OPENQASM 2.0;\\nqreg q[1];\\ncreg c[1];\\n"
	if err := ValidateQASM(qasm); err == nil {
		t.Error("literal backslash-n program must refuse")
	} else if !strings.Contains(err.Error(), "unrecognized statement") {
		t.Errorf("refusal must name the statement, got: %v", err)
	}
}

func TestValidateQASMSemanticsStrictNumbers(t *testing.T) {
	cases := []struct {
		name string
		qasm string
		want string
	}{
		// Sscanf-family parsers accept leading-digit garbage; indices and
		// sizes must be digits-only.
		{"register size garbage", "OPENQASM 2.0;\nqreg q[12abc];\n", "non-positive size"},
		{"index garbage", "OPENQASM 2.0;\nqreg q[2];\nh q[1x];\n", "malformed qubit index"},
		{"negative index", "OPENQASM 2.0;\nqreg q[2];\nh q[-1];\n", "malformed qubit"},
		// Empty parens declare zero parameters, not one.
		{"custom gate empty parens", "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\ngate g() a {\nh a;\n}\ng q[0];\nmeasure q[0] -> c[0];\n", ""},
		{"custom gate param mismatch", "OPENQASM 2.0;\nqreg q[1];\ngate g(theta) a {\nh a;\n}\ng q[0];\n", "takes 1 parameters"},
	}
	for _, tc := range cases {
		err := ValidateQASM(tc.qasm)
		if tc.want == "" {
			if err != nil {
				t.Errorf("%s: valid program refused: %v", tc.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want refusal containing %q", tc.name, err, tc.want)
		}
	}
}
