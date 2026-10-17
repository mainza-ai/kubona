package jobs

import "testing"

// W2.5: two-qubit counting covers the full vocabulary (safe direction) and
// comments never score.
func TestCountQasmGatesVocabulary(t *testing.T) {
	qasm := `OPENQASM 2.0;
include "qelib1.inc";
qreg q[3];
creg c[3];
h q[0];
// cx q[0], q[1];
cnot q[0], q[1];
cy q[0], q[2];
ch q[1], q[2];
crx(0.5) q[0], q[1];
rxx(0.1) q[1], q[2];
swap q[0], q[2];
measure q[0] -> c[0];
`
	qubits, depth, twoQ := countQasmGates(qasm)
	if qubits != 3 {
		t.Fatalf("qubits: got %d", qubits)
	}
	// h + cnot + cy + ch + crx + rxx + swap = 7 depth (measure skipped).
	if depth != 7 {
		t.Fatalf("depth: got %d", depth)
	}
	// All six entanglers count (commented cx must not).
	if twoQ != 6 {
		t.Fatalf("twoQ: got %d (commented cx must not score)", twoQ)
	}
	// Unknown 2-operand gate counts entangling (fail-closed).
	q, d, t2 := countQasmGates("OPENQASM 2.0;\nqreg q[2];\nfoo q[0], q[1];\n")
	if q != 2 || d != 1 || t2 != 1 {
		t.Fatalf("unknown 2q gate must count entangling: %d %d %d", q, d, t2)
	}
}
