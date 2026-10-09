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
