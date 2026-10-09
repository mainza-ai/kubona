package qir

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// CircuitSummary holds compiled QIR circuit analysis metrics
type CircuitSummary struct {
	NumQubits  int    `json:"num_qubits"`
	Depth      int    `json:"depth"`
	GateCount  int    `json:"gate_count"`
	TCount     int    `json:"t_count"`
	QIRProfile string `json:"qir_profile"`
	LLVMIR     string `json:"llvm_ir,omitempty"`
	// CompilerUsed names the backend that produced this summary
	// ("rust-qir-core" or "go-fallback-estimate"). FallbackEstimate is true
	// when the Rust core was unavailable and the numbers below are regex
	// estimates, not compiler-verified metrics: Depth is an upper bound
	// (gate count), not scheduled depth, and linearity is unchecked.
	CompilerUsed     string `json:"compiler_used"`
	FallbackEstimate bool   `json:"fallback_estimate"`
}

// Compiler manages lowering of OpenQASM circuits to QIR BaseProfile LLVM IR
type Compiler struct {
	QIRCoreBinaryPath string
}

func NewCompiler() *Compiler {
	return &Compiler{
		QIRCoreBinaryPath: "../crates/qir-core/target/release/qir-core",
	}
}

// LowerToLLVM lowers OpenQASM to QIR BaseProfile LLVM IR
func (c *Compiler) LowerToLLVM(qasm string) (string, error) {
	if strings.TrimSpace(qasm) == "" {
		return "", errors.New("empty QASM input")
	}

	// Try executing native Rust qir-core binary if available
	candidates := []string{
		c.QIRCoreBinaryPath,
		"../../crates/qir-core/target/release/qir-core",
		"platform/crates/qir-core/target/release/qir-core",
	}

	for _, bin := range candidates {
		abs, err := filepath.Abs(bin)
		if err == nil {
			cmd := exec.Command(abs, "--llvm")
			cmd.Stdin = strings.NewReader(qasm)
			var out bytes.Buffer
			cmd.Stdout = &out
			if err := cmd.Run(); err == nil && out.Len() > 0 {
				return out.String(), nil
			}
		}
	}

	// Native pure Go BaseProfile lowerer fallback
	return c.nativeLower(qasm)
}

// Analyze returns structured circuit summary metrics
func (c *Compiler) Analyze(qasm string) (*CircuitSummary, error) {
	candidates := []string{
		c.QIRCoreBinaryPath,
		"../../crates/qir-core/target/release/qir-core",
		"platform/crates/qir-core/target/release/qir-core",
	}

	for _, bin := range candidates {
		abs, err := filepath.Abs(bin)
		if err == nil {
			cmd := exec.Command(abs, "--json")
			cmd.Stdin = strings.NewReader(qasm)
			var out bytes.Buffer
			cmd.Stdout = &out
			if err := cmd.Run(); err == nil && out.Len() > 0 {
				var raw struct {
					InstructionCount int `json:"instruction_count"`
					QubitAllocations int `json:"qubit_allocations"`
					GateDepth        int `json:"gate_depth"`
					SingleQubitGates int `json:"single_qubit_gates"`
					TwoQubitGates    int `json:"two_qubit_gates"`
					TotalGates       int `json:"total_gates"`
				}
				if err := json.Unmarshal(out.Bytes(), &raw); err == nil && (raw.QubitAllocations > 0 || raw.TotalGates > 0) {
					llvm, _ := c.LowerToLLVM(qasm)
					return &CircuitSummary{
						NumQubits:        raw.QubitAllocations,
						Depth:            raw.GateDepth,
						GateCount:        raw.TotalGates,
						TCount:           0,
						QIRProfile:       "BaseProfile",
						LLVMIR:           llvm,
						CompilerUsed:     "rust-qir-core",
						FallbackEstimate: false,
					}, nil
				}
			}
		}
	}

	// Pure Go fallback parser
	lines := strings.Split(qasm, "\n")
	numQubits := 0
	gateCount := 0
	tCount := 0

	qregRe := regexp.MustCompile(`qreg\s+\w+\[(\d+)\]`)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if matches := qregRe.FindStringSubmatch(line); len(matches) > 1 {
			var n int
			fmt.Sscanf(matches[1], "%d", &n)
			if n > numQubits {
				numQubits = n
			}
		} else if strings.HasPrefix(line, "h ") || strings.HasPrefix(line, "cx ") ||
			strings.HasPrefix(line, "x ") || strings.HasPrefix(line, "rz") {
			gateCount++
		} else if strings.HasPrefix(line, "t ") || strings.HasPrefix(line, "tdg ") {
			gateCount++
			tCount++
		}
	}

	if numQubits == 0 {
		numQubits = 2
	}

	llvm, _ := c.nativeLower(qasm)

	return &CircuitSummary{
		NumQubits:        numQubits,
		Depth:            gateCount,
		GateCount:        gateCount,
		TCount:           tCount,
		QIRProfile:       "BaseProfile-Estimate",
		LLVMIR:           llvm,
		CompilerUsed:     "go-fallback-estimate",
		FallbackEstimate: true,
	}, nil
}

func (c *Compiler) nativeLower(qasm string) (string, error) {
	var sb strings.Builder
	sb.WriteString("; ModuleID = 'kubona_qir_module'\n")
	sb.WriteString("source_filename = \"circuit.qasm\"\n\n")
	sb.WriteString("%Qubit = type opaque\n")
	sb.WriteString("%Result = type opaque\n\n")
	sb.WriteString("declare void @__quantum__qis__h__body(%Qubit*)\n")
	sb.WriteString("declare void @__quantum__qis__cnot__body(%Qubit*, %Qubit*)\n")
	sb.WriteString("declare void @__quantum__qis__rz__body(double, %Qubit*)\n")
	sb.WriteString("declare void @__quantum__qis__mz__body(%Qubit*, %Result*)\n")
	sb.WriteString("declare void @__quantum__rt__array_record_output(i64, i8*)\n\n")
	sb.WriteString("define void @main() #0 {\nentry:\n")

	// Parse simple instructions, tracking the declared register width so the
	// required_num_qubits attribute reflects the input instead of a constant.
	numQubits := 0
	qregRe := regexp.MustCompile(`qreg\s+\w+\[(\d+)\]`)
	lines := strings.Split(qasm, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if matches := qregRe.FindStringSubmatch(l); len(matches) > 1 {
			var n int
			fmt.Sscanf(matches[1], "%d", &n)
			if n > numQubits {
				numQubits = n
			}
			continue
		}
		if strings.HasPrefix(l, "h q[") {
			var q int
			fmt.Sscanf(l, "h q[%d];", &q)
			sb.WriteString(fmt.Sprintf("  call void @__quantum__qis__h__body(%%Qubit* inttoptr (i64 %d to %%Qubit*))\n", q))
		} else if strings.HasPrefix(l, "cx q[") {
			var q0, q1 int
			fmt.Sscanf(l, "cx q[%d], q[%d];", &q0, &q1)
			sb.WriteString(fmt.Sprintf("  call void @__quantum__qis__cnot__body(%%Qubit* inttoptr (i64 %d to %%Qubit*), %%Qubit* inttoptr (i64 %d to %%Qubit*))\n", q0, q1))
		}
	}

	sb.WriteString("  ret void\n}\n\n")
	sb.WriteString(fmt.Sprintf("attributes #0 = { \"qir_profiles\"=\"custom\" \"required_num_qubits\"=\"%d\" }\n", numQubits))
	return sb.String(), nil
}
