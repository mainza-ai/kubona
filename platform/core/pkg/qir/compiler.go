package qir

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
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

func resolveQIRCoreBinary() string {
	// 1. Explicit environment override
	if envBin := strings.TrimSpace(os.Getenv("KUBONA_QIR_CORE_BIN")); envBin != "" {
		if abs, err := filepath.Abs(envBin); err == nil {
			if _, err := os.Stat(abs); err == nil {
				return abs
			}
		}
	}

	// 2. Executable-relative lookup (for deployed daemon / CLI processes)
	if exePath, err := os.Executable(); err == nil && exePath != "" {
		exeDir := filepath.Dir(exePath)
		for _, rel := range []string{
			"qir-core",
			"../crates/qir-core/target/release/qir-core",
			"../../crates/qir-core/target/release/qir-core",
			"../../../crates/qir-core/target/release/qir-core",
		} {
			target := filepath.Join(exeDir, rel)
			if abs, err := filepath.Abs(target); err == nil {
				if _, err := os.Stat(abs); err == nil {
					return abs
				}
			}
		}
	}

	// 3. Known relative workspace search paths from working directory
	for _, rel := range []string{
		"../crates/qir-core/target/release/qir-core",
		"../../crates/qir-core/target/release/qir-core",
		"platform/crates/qir-core/target/release/qir-core",
	} {
		if abs, err := filepath.Abs(rel); err == nil {
			if _, err := os.Stat(abs); err == nil {
				return abs
			}
		}
	}

	return "../crates/qir-core/target/release/qir-core"
}

func NewCompiler() *Compiler {
	return &Compiler{
		QIRCoreBinaryPath: resolveQIRCoreBinary(),
	}
}

func (c *Compiler) binaryCandidates() []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if abs, err := filepath.Abs(p); err == nil && !seen[abs] {
			seen[abs] = true
			out = append(out, abs)
		}
	}
	if envBin := os.Getenv("KUBONA_QIR_CORE_BIN"); envBin != "" {
		add(envBin)
	}
	add(c.QIRCoreBinaryPath)
	if exePath, err := os.Executable(); err == nil && exePath != "" {
		exeDir := filepath.Dir(exePath)
		add(filepath.Join(exeDir, "qir-core"))
		add(filepath.Join(exeDir, "../crates/qir-core/target/release/qir-core"))
		add(filepath.Join(exeDir, "../../crates/qir-core/target/release/qir-core"))
	}
	add("../crates/qir-core/target/release/qir-core")
	add("../../crates/qir-core/target/release/qir-core")
	add("platform/crates/qir-core/target/release/qir-core")
	return out
}

// LowerToLLVM lowers OpenQASM to QIR BaseProfile LLVM IR
func (c *Compiler) LowerToLLVM(qasm string) (string, error) {
	if strings.TrimSpace(qasm) == "" {
		return "", errors.New("empty QASM input")
	}

	// Try executing native Rust qir-core binary if available
	candidates := c.binaryCandidates()

	for _, abs := range candidates {
		// W1.9: hung compiler binaries used to hang the API with them.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		cmd := exec.CommandContext(ctx, abs, "--llvm")
		cmd.Stdin = strings.NewReader(qasm)
		var out bytes.Buffer
		cmd.Stdout = &out
		err := cmd.Run()
		cancel()
		if err == nil && out.Len() > 0 {
			return out.String(), nil
		}
	}

	// Native pure Go BaseProfile lowerer fallback
	return c.nativeLower(qasm)
}

// Analyze returns structured circuit summary metrics
func (c *Compiler) Analyze(qasm string) (*CircuitSummary, error) {
	candidates := c.binaryCandidates()

	for _, abs := range candidates {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		cmd := exec.CommandContext(ctx, abs, "--json")
		cmd.Stdin = strings.NewReader(qasm)
		var out bytes.Buffer
		cmd.Stdout = &out
		runErr := cmd.Run()
		cancel()
		if runErr == nil && out.Len() > 0 {
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
					NumQubits: raw.QubitAllocations,
					Depth:     raw.GateDepth,
					GateCount: raw.TotalGates,
					// W1.9: qir-core emits no T-count — parsed from
					// source on both paths (t/tdg statements), never 0
					// by default (QEC consumed the fabricated zero).
					TCount:           countTStatements(qasm),
					QIRProfile:       "BaseProfile",
					LLVMIR:           llvm,
					CompilerUsed:     "rust-qir-core",
					FallbackEstimate: false,
				}, nil
			}
		}
	}

	// Pure Go fallback parser
	parsed, err := parseProgram(qasm)
	if err != nil {
		return nil, err
	}

	llvm, err := c.nativeLower(qasm)
	if err != nil {
		return nil, err
	}

	return &CircuitSummary{
		NumQubits:        parsed.numQubits,
		Depth:            parsed.gateCount,
		GateCount:        parsed.gateCount,
		TCount:           parsed.tCount,
		QIRProfile:       "CustomProfile-Estimate",
		LLVMIR:           llvm,
		CompilerUsed:     "go-fallback-estimate",
		FallbackEstimate: true,
	}, nil
}

// Closed lowered set (W1.9): the Go fallback lowers exactly these gates.
// Anything else refuses with the gate named — the old code silently dropped
// every non-h/cx gate while claiming BaseProfile. Use the rust core for
// full QASM. sdg/tdg decompose to S³/T⁷ (verified identities, standard
// intrinsics only — no s_adj/t_adj gamble); swap is 3×CNOT.
type parsedOp struct {
	name   string
	angle  float64
	qubits []int
	cbit   int
}

// parsedProgram is the single structural parse both Analyze (counts) and
// nativeLower (emission) consume — one parser, never two that rot apart.
type parsedProgram struct {
	numQubits int
	gateCount int
	tCount    int
	ops       []parsedOp
}

// tGateRe matches literal t/tdg statements for source T-counting.
var tGateRe = regexp.MustCompile(`(?m)^\s*(t|tdg)\s+`)

// countTStatements counts literal t/tdg gates in source (both Analyze
// paths — qir-core emits no T-count, and defaulting 0 fabricated it).
func countTStatements(qasm string) int {
	return len(tGateRe.FindAllString(qasm, -1))
}

var (
	qregRe       = regexp.MustCompile(`qreg\s+(\w+)\[(\d+)\]`)
	qasm3QubitRe = regexp.MustCompile(`qubit(?:\[(\d+)\])?\s+(\w+)`)
	qubitRe      = regexp.MustCompile(`(\w+)\[(\d+)\]`)
	// gateRe splits one statement into name, parenthesized params, operands.
	gateRe = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)\s*(\(([^)]*)\))?\s*(.*?)\s*;?\s*$`)
	// angleRe parses rotation angles: decimals or pi multiples (pi, pi/2,
	// -pi/4, 2*pi). Anything else refuses — angles are never guessed.
	angleRe = regexp.MustCompile(`^([+-]?)\s*(?:(\d+(?:\.\d+)?)\s*\*?\s*)?pi(?:\s*/\s*(\d+(?:\.\d+)?))?$`)
)

// parseAngle parses a rotation parameter into radians.
func parseAngle(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return f, nil
	}
	m := angleRe.FindStringSubmatch(raw)
	if m == nil {
		return 0, fmt.Errorf("unparseable angle %q: refusing (angles are never guessed)", raw)
	}
	sign := 1.0
	if m[1] == "-" {
		sign = -1.0
	}
	mult := 1.0
	if m[2] != "" {
		mult, _ = strconv.ParseFloat(m[2], 64)
	}
	div := 1.0
	if m[3] != "" {
		div, _ = strconv.ParseFloat(m[3], 64)
		if div == 0 {
			return 0, fmt.Errorf("zero divisor in angle %q", raw)
		}
	}
	return sign * mult * math.Pi / div, nil
}

// parseProgram structurally parses QASM once for the Go fallback: register
// map (multi-qreg via offsets), validated indices, per-gate classification.
func parseProgram(qasm string) (*parsedProgram, error) {
	regWidth := map[string]int{}
	regOffset := map[string]int{}
	total := 0
	for _, line := range strings.Split(qasm, "\n") {
		line = stripQASMComment(line)
		if matches := qregRe.FindStringSubmatch(line); len(matches) > 2 {
			n, _ := strconv.Atoi(matches[2])
			if n <= 0 {
				return nil, fmt.Errorf("invalid register width in %q", line)
			}
			if _, dup := regWidth[matches[1]]; dup {
				return nil, fmt.Errorf("duplicate register %q", matches[1])
			}
			regWidth[matches[1]] = n
			regOffset[matches[1]] = total
			total += n
		} else if matches := qasm3QubitRe.FindStringSubmatch(line); len(matches) > 2 {
			n := 1
			if matches[1] != "" {
				n, _ = strconv.Atoi(matches[1])
			}
			regName := matches[2]
			if n <= 0 {
				return nil, fmt.Errorf("invalid register width in %q", line)
			}
			if _, dup := regWidth[regName]; dup {
				return nil, fmt.Errorf("duplicate register %q", regName)
			}
			regWidth[regName] = n
			regOffset[regName] = total
			total += n
		}
	}
	if total == 0 {
		return nil, fmt.Errorf("no qubit register declared: refusing to invent qubits")
	}
	pp := &parsedProgram{numQubits: total}
	qidx := func(tok string) (int, error) {
		m := qubitRe.FindStringSubmatch(strings.TrimSpace(tok))
		if m == nil {
			return 0, fmt.Errorf("bad qubit operand %q", tok)
		}
		w, ok := regWidth[m[1]]
		if !ok {
			return 0, fmt.Errorf("qubit register %q not declared", m[1])
		}
		i, _ := strconv.Atoi(m[2])
		if i < 0 || i >= w {
			return 0, fmt.Errorf("qubit index %d out of range for %s[%d]", i, m[1], w)
		}
		return regOffset[m[1]] + i, nil
	}
	for _, rawLine := range strings.Split(qasm, "\n") {
		line := stripQASMComment(rawLine)
		if line == "" || isQASMDirective(line) {
			continue
		}
		m := gateRe.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("unparseable statement %q: refusing", rawLine)
		}
		name := strings.ToLower(m[1])
		var operands []string
		if strings.TrimSpace(m[4]) != "" {
			for _, o := range strings.Split(m[4], ",") {
				operands = append(operands, strings.TrimSpace(o))
			}
		}
		get := func(i int) (int, error) {
			if i >= len(operands) {
				return 0, fmt.Errorf("gate %q takes more operands (%d given)", name, len(operands))
			}
			return qidx(operands[i])
		}
		oneQubit := func() (int, error) {
			if len(operands) != 1 {
				return 0, fmt.Errorf("gate %q takes 1 qubit, got %d", name, len(operands))
			}
			return get(0)
		}
		twoQubit := func() (int, int, error) {
			if len(operands) != 2 {
				return 0, 0, fmt.Errorf("gate %q takes 2 qubits, got %d", name, len(operands))
			}
			a, err := get(0)
			if err != nil {
				return 0, 0, err
			}
			b, err := get(1)
			if err != nil {
				return 0, 0, err
			}
			return a, b, nil
		}
		switch name {
		case "h", "x", "y", "z", "s", "t":
			i, err := oneQubit()
			if err != nil {
				return nil, err
			}
			pp.ops = append(pp.ops, parsedOp{name: name, qubits: []int{i}})
		case "sdg":
			i, err := oneQubit()
			if err != nil {
				return nil, err
			}
			for k := 0; k < 3; k++ {
				pp.ops = append(pp.ops, parsedOp{name: "s", qubits: []int{i}})
			}
		case "tdg":
			i, err := oneQubit()
			if err != nil {
				return nil, err
			}
			for k := 0; k < 7; k++ {
				pp.ops = append(pp.ops, parsedOp{name: "t", qubits: []int{i}})
			}
		case "id":
			if _, err := oneQubit(); err != nil {
				return nil, err
			}
			pp.ops = append(pp.ops, parsedOp{name: "id"})
		case "rx", "ry", "rz":
			if strings.TrimSpace(m[3]) == "" {
				return nil, fmt.Errorf("gate %q takes an angle + 1 qubit", name)
			}
			ang, err := parseAngle(m[3])
			if err != nil {
				return nil, err
			}
			i, err := oneQubit()
			if err != nil {
				return nil, err
			}
			pp.ops = append(pp.ops, parsedOp{name: name, angle: ang, qubits: []int{i}})
		case "cx", "cz":
			a, b, err := twoQubit()
			if err != nil {
				return nil, err
			}
			pp.ops = append(pp.ops, parsedOp{name: name, qubits: []int{a, b}})
		case "swap":
			a, b, err := twoQubit()
			if err != nil {
				return nil, err
			}
			pp.ops = append(pp.ops, parsedOp{name: "cx", qubits: []int{a, b}},
				parsedOp{name: "cx", qubits: []int{b, a}},
				parsedOp{name: "cx", qubits: []int{a, b}})
		case "reset":
			i, err := oneQubit()
			if err != nil {
				return nil, err
			}
			pp.ops = append(pp.ops, parsedOp{name: "reset", qubits: []int{i}})
		case "measure":
			// measure q[i] -> c[j]: both sides validated (classical bit
			// threading is real here — the dropped-bit class is refused).
			if len(operands) != 1 {
				return nil, fmt.Errorf("measure takes q[i] -> c[j], got %q", rawLine)
			}
			parts := strings.Split(operands[0], "->")
			if len(parts) != 2 {
				return nil, fmt.Errorf("measure takes q[i] -> c[j], got %q", rawLine)
			}
			qi, err := qidx(strings.TrimSpace(parts[0]))
			if err != nil {
				return nil, err
			}
			cm := qubitRe.FindStringSubmatch(strings.TrimSpace(parts[1]))
			if cm == nil {
				return nil, fmt.Errorf("bad classical operand in %q", rawLine)
			}
			ci, _ := strconv.Atoi(cm[2])
			pp.ops = append(pp.ops, parsedOp{name: "measure", qubits: []int{qi}, cbit: ci})
		default:
			return nil, fmt.Errorf("gate %q is outside the Go fallback set (h,x,y,z,s,sdg,t,tdg,id,rx,ry,rz,cx,cz,swap,reset,measure): use the rust core for full QASM", name)
		}
		pp.gateCount++
		if name == "t" || name == "tdg" {
			pp.tCount++
		}
	}
	return pp, nil
}

func isQASMDirective(line string) bool {
	lower := strings.ToLower(line)
	for _, prefix := range []string{"openqasm", "include", "qreg", "creg", "barrier", "qubit", "bit"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// stripQASMComment removes // comments (block comments refuse elsewhere).
func stripQASMComment(line string) string {
	if i := strings.Index(line, "//"); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}

func (c *Compiler) nativeLower(qasm string) (string, error) {
	// Lower through the single structural parse: every op validated there,
	// so emission cannot silently drop semantics (W1.9).
	pp, err := parseProgram(qasm)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("; ModuleID = 'kubona_qir_module'\n")
	sb.WriteString("source_filename = \"circuit.qasm\"\n\n")
	sb.WriteString("%Qubit = type opaque\n")
	sb.WriteString("%Result = type opaque\n\n")
	// Declared intrinsics cover the whole closed lowered set — every
	// emitted call resolves (the old header declared rz/mz but emitted
	// only h/cnot, and any other gate vanished silently).
	for _, decl := range []string{
		"declare void @__quantum__qis__h__body(%Qubit*)",
		"declare void @__quantum__qis__x__body(%Qubit*)",
		"declare void @__quantum__qis__y__body(%Qubit*)",
		"declare void @__quantum__qis__z__body(%Qubit*)",
		"declare void @__quantum__qis__s__body(%Qubit*)",
		"declare void @__quantum__qis__t__body(%Qubit*)",
		"declare void @__quantum__qis__rx__body(double, %Qubit*)",
		"declare void @__quantum__qis__ry__body(double, %Qubit*)",
		"declare void @__quantum__qis__rz__body(double, %Qubit*)",
		"declare void @__quantum__qis__cnot__body(%Qubit*, %Qubit*)",
		"declare void @__quantum__qis__cz__body(%Qubit*, %Qubit*)",
		"declare void @__quantum__qis__reset__body(%Qubit*)",
		"declare void @__quantum__qis__mz__body(%Qubit*, %Result*)",
		"declare void @__quantum__rt__array_record_output(i64, i8*)",
	} {
		sb.WriteString(decl + "\n")
	}
	sb.WriteString("\ndefine void @main() #0 {\nentry:\n")
	q := func(i int) string {
		return fmt.Sprintf("%%Qubit* inttoptr (i64 %d to %%Qubit*)", i)
	}
	r := func(i int) string {
		return fmt.Sprintf("%%Result* inttoptr (i64 %d to %%Result*)", i)
	}
	for _, o := range pp.ops {
		switch o.name {
		case "h", "x", "y", "z", "s", "t":
			sb.WriteString(fmt.Sprintf("  call void @__quantum__qis__%s__body(%s)\n", o.name, q(o.qubits[0])))
		case "rx", "ry", "rz":
			sb.WriteString(fmt.Sprintf("  call void @__quantum__qis__%s__body(double %v, %s)\n", o.name, strconv.FormatFloat(o.angle, 'g', -1, 64), q(o.qubits[0])))
		case "cx":
			sb.WriteString(fmt.Sprintf("  call void @__quantum__qis__cnot__body(%s, %s)\n", q(o.qubits[0]), q(o.qubits[1])))
		case "cz":
			sb.WriteString(fmt.Sprintf("  call void @__quantum__qis__cz__body(%s, %s)\n", q(o.qubits[0]), q(o.qubits[1])))
		case "reset":
			sb.WriteString(fmt.Sprintf("  call void @__quantum__qis__reset__body(%s)\n", q(o.qubits[0])))
		case "measure":
			sb.WriteString(fmt.Sprintf("  call void @__quantum__qis__mz__body(%s, %s)\n", q(o.qubits[0]), r(o.cbit)))
		case "id":
			// identity: validated, counted, no emission.
		}
	}
	sb.WriteString("  ret void\n}\n\n")
	sb.WriteString(fmt.Sprintf("attributes #0 = { \"qir_profiles\"=\"custom\" \"required_num_qubits\"=\"%d\" }\n", pp.numQubits))
	return sb.String(), nil
}
