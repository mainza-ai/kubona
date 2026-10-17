package qir

import (
	"strings"
	"testing"
)

func TestQIRCompilerLowerToLLVM(t *testing.T) {
	c := NewCompiler()
	qasm := `OPENQASM 2.0;
include "qelib1.inc";
qreg q[2];
creg c[2];
h q[0];
cx q[0], q[1];
`
	llvm, err := c.LowerToLLVM(qasm)
	if err != nil {
		t.Fatalf("LowerToLLVM failed: %v", err)
	}

	if !strings.Contains(llvm, "@__quantum__qis__h__body") {
		t.Errorf("expected @__quantum__qis__h__body in LLVM IR, got:\n%s", llvm)
	}
	if !strings.Contains(llvm, "@__quantum__qis__cnot__body") {
		t.Errorf("expected @__quantum__qis__cnot__body in LLVM IR, got:\n%s", llvm)
	}
}

func TestQIRCompilerAnalyze(t *testing.T) {
	c := NewCompiler()
	qasm := `OPENQASM 2.0;
include "qelib1.inc";
qreg q[3];
h q[0];
cx q[0], q[1];
t q[2];
`
	summary, err := c.Analyze(qasm)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if summary.NumQubits < 2 {
		t.Errorf("expected at least 2 qubits, got %d", summary.NumQubits)
	}
	if summary.GateCount == 0 {
		t.Errorf("expected gate count > 0")
	}
	if !strings.HasPrefix(summary.QIRProfile, "BaseProfile") {
		t.Errorf("expected BaseProfile dialect target, got %s", summary.QIRProfile)
	}
	if summary.CompilerUsed == "" {
		t.Errorf("compiler backend must be labeled, got empty CompilerUsed")
	}
}

func TestQIRCompilerFallbackIsLabeled(t *testing.T) {
	c := &Compiler{QIRCoreBinaryPath: "/nonexistent/qir-core-test-binary"}
	qasm := `OPENQASM 2.0;
include "qelib1.inc";
qreg q[4];
h q[0];
cx q[0], q[1];
`
	summary, err := c.Analyze(qasm)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if !summary.FallbackEstimate || summary.CompilerUsed != "go-fallback-estimate" {
		t.Errorf("fallback must be labeled: %+v", summary)
	}
	if summary.NumQubits != 4 {
		t.Errorf("fallback must report declared register width, got %d", summary.NumQubits)
	}
	llvm, err := c.LowerToLLVM(qasm)
	if err != nil {
		t.Fatalf("LowerToLLVM failed: %v", err)
	}
	if !strings.Contains(llvm, `"required_num_qubits"="4"`) {
		t.Errorf("fallback IR must carry the declared qubit count, got:\n%s", llvm)
	}
}

// W1.9: the closed set lowers fully; anything outside refuses (never
// silently dropped); T-count is source-parsed; qubit counts are declared,
// never invented.
func TestQIRFallbackClosedSet(t *testing.T) {
	c := &Compiler{QIRCoreBinaryPath: "/nonexistent/qir-core-test-binary"}
	qasm := `OPENQASM 2.0;
include "qelib1.inc";
qreg q[2];
creg c[2];
h q[0];
x q[1];
rx(pi/2) q[0];
cz q[0], q[1];
swap q[0], q[1];
t q[0];
tdg q[1];
reset q[0];
measure q[0] -> c[0];
`
	summary, err := c.Analyze(qasm)
	if err != nil {
		t.Fatalf("closed-set analyze: %v", err)
	}
	if summary.GateCount != 9 {
		t.Fatalf("gate count must cover all 9 statements, got %d", summary.GateCount)
	}
	if summary.TCount != 2 {
		t.Fatalf("T-count must parse t+tdg, got %d", summary.TCount)
	}
	llvm, err := c.LowerToLLVM(qasm)
	if err != nil {
		t.Fatalf("closed-set lower: %v", err)
	}
	for _, want := range []string{"__quantum__qis__x__body", "__quantum__qis__rx__body",
		"__quantum__qis__cz__body", "__quantum__qis__cnot__body",
		"__quantum__qis__reset__body", "__quantum__qis__mz__body"} {
		if !strings.Contains(llvm, want) {
			t.Errorf("IR must declare+emit %s", want)
		}
	}
	// Every emitted call resolves: no call without a declare.
	declares := map[string]bool{}
	calls := map[string]bool{}
	for _, line := range strings.Split(llvm, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "declare ") {
			declares[line] = true
		}
		if strings.HasPrefix(line, "call ") {
			start := strings.Index(line, "@")
			end := strings.Index(line, "(")
			if start >= 0 && end > start {
				calls[line[start:end]] = true
			}
		}
	}
	for callee := range calls {
		found := false
		for d := range declares {
			if strings.Contains(d, callee) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("emitted call %s has no declare", callee)
		}
	}
}

func TestQIRFallbackRefusals(t *testing.T) {
	c := &Compiler{QIRCoreBinaryPath: "/nonexistent/qir-core-test-binary"}
	for _, tc := range []struct {
		name string
		qasm string
	}{
		{"u-gate", "OPENQASM 2.0;\nqreg q[1];\nu1(0.5) q[0];\n"},
		{"ch-gate", "OPENQASM 2.0;\nqreg q[2];\nch q[0], q[1];\n"},
		{"no-register", "OPENQASM 2.0;\nh q[0];\n"},
		{"bad-index", "OPENQASM 2.0;\nqreg q[1];\nh q[5];\n"},
		{"bad-angle", "OPENQASM 2.0;\nqreg q[1];\nrx(bogus) q[0];\n"},
	} {
		if _, err := c.Analyze(tc.qasm); err == nil {
			t.Errorf("%s must refuse", tc.name)
		}
		if _, err := c.LowerToLLVM(tc.qasm); err == nil {
			t.Errorf("%s lowering must refuse", tc.name)
		}
	}
}
