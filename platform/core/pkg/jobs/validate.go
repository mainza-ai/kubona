package jobs

import (
	"fmt"
	"strings"
)

// maxQASMBytes caps circuit text at dispatch. Larger programs go through
// the file/artifact path, not inline params.
const maxQASMBytes = 1 << 20

// validateQASM is the pre-dispatch structural gate for circuit_run jobs: it
// refuses malformed programs before admission, queue, ledger, or spend
// moves. Phase 1 is structural (shape); phase 2 is semantic (meaning).
// Neither is a compiler — the executor's parser remains the authority and
// its errors stay honest execution failures. Every refusal names its reason.
//
// Deliberate non-goal: gate-name resolution. Whether an unknown name
// resolves lives with the executor, which holds the include files; the
// pre-check owns only what it can decide with certainty (indices, arity on
// the core set, measure kinds, register declarations).
func validateQASM(qasm string) error {
	if strings.TrimSpace(qasm) == "" {
		return fmt.Errorf("qasm must not be empty")
	}
	if len(qasm) > maxQASMBytes {
		return fmt.Errorf("qasm exceeds %d bytes (%d): submit via artifact path", maxQASMBytes, len(qasm))
	}
	if strings.ContainsRune(qasm, 0) {
		return fmt.Errorf("qasm contains NUL bytes")
	}
	upper := strings.ToUpper(qasm)
	if !strings.Contains(upper, "OPENQASM") {
		return fmt.Errorf("qasm missing OPENQASM version header")
	}
	if strings.Contains(upper, "OPENQASM 3") {
		return fmt.Errorf("OPENQASM 3 programs not yet admitted; submit 2.0")
	}
	if !strings.Contains(strings.ToLower(qasm), "qreg") {
		return fmt.Errorf("qasm declares no quantum register (qreg)")
	}
	pairs := map[rune]rune{'{': '}', '[': ']', '(': ')'}
	// Strip // line comments first so their brackets never count.
	var code strings.Builder
	for _, line := range strings.Split(qasm, "\n") {
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = line[:idx]
		}
		code.WriteString(line)
		code.WriteByte('\n')
	}
	var stack []rune
	for _, r := range code.String() {
		if close, ok := pairs[r]; ok {
			stack = append(stack, close)
			continue
		}
		if r == '}' || r == ']' || r == ')' {
			if len(stack) == 0 || stack[len(stack)-1] != r {
				return fmt.Errorf("qasm has unbalanced %q", string(r))
			}
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) > 0 {
		return fmt.Errorf("qasm has unclosed delimiter (expecting %q)", string(stack[len(stack)-1]))
	}
	return validateQASMSemantics(code.String())
}

// coreGateArity is the qelib1.inc subset the pre-check is certain about:
// name -> (qubits, params). Anything else resolves at the executor.
var coreGateArity = map[string][2]int{
	"h":    {1, 0},
	"x":    {1, 0},
	"y":    {1, 0},
	"z":    {1, 0},
	"s":    {1, 0},
	"sdg":  {1, 0},
	"t":    {1, 0},
	"tdg":  {1, 0},
	"id":   {1, 0},
	"u0":   {1, 0},
	"cx":   {2, 0},
	"cz":   {2, 0},
	"cy":   {2, 0},
	"ch":   {2, 1},
	"swap": {2, 0},
	"rx":   {1, 1},
	"ry":   {1, 1},
	"rz":   {1, 1},
	"u1":   {1, 1},
	"p":    {1, 1},
	"u2":   {1, 2},
	"u3":   {1, 3},
	"U":    {1, 3},
	"u":    {1, 3},
}

type qasmRegs struct {
	qreg map[string]int
	creg map[string]int
}

// validateQASMSemantics parses declarations and checks every indexed
// reference, measure kind, and core-gate arity. Input is comment-stripped
// and delimiter-balanced by phase 1.
func validateQASMSemantics(code string) error {
	regs := &qasmRegs{qreg: map[string]int{}, creg: map[string]int{}}
	custom := map[string][2]int{} // gate name -> (qubits, params)

	var stmts []string
	var cur strings.Builder
	depth := 0
	for _, r := range code {
		switch r {
		case '{':
			depth++
			cur.WriteRune(r)
		case '}':
			depth--
			cur.WriteRune(r)
			// A gate definition ends at its closing brace — QASM puts no
			// semicolon there, so without this flush the next statement
			// merges into the definition and skips validation.
			if depth == 0 && strings.HasPrefix(strings.ToLower(strings.TrimSpace(cur.String())), "gate") {
				stmts = append(stmts, cur.String())
				cur.Reset()
			}
		case ';':
			if depth == 0 {
				stmts = append(stmts, cur.String())
				cur.Reset()
			} else {
				cur.WriteRune(r)
			}
		default:
			cur.WriteRune(r)
		}
	}

	first := true
	for _, raw := range stmts {
		stmt := strings.TrimSpace(raw)
		if stmt == "" {
			continue
		}
		if first {
			first = false
			upper := strings.ToUpper(stmt)
			if strings.Contains(upper, "OPENQASM 3") || strings.HasPrefix(strings.ToLower(stmt), "qubit") {
				return fmt.Errorf("OPENQASM 3 programs not yet admitted; submit 2.0")
			}
			if len(stmt) < 8 || !strings.EqualFold(stmt[:8], "OPENQASM") {
				return fmt.Errorf("first statement must be the OPENQASM 2.0 header, got %q", boundStmt(stmt))
			}
			if !strings.Contains(stmt, "2.0") {
				return fmt.Errorf("only OPENQASM 2.0 admitted, got %q", boundStmt(stmt))
			}
			continue
		}
		if err := checkQASMStmt(stmt, regs, custom); err != nil {
			return err
		}
	}
	return nil
}

func checkQASMStmt(stmt string, regs *qasmRegs, custom map[string][2]int) error {
	lower := strings.ToLower(stmt)

	if strings.HasPrefix(lower, "include") {
		return nil // name resolution belongs to the executor
	}
	if strings.HasPrefix(lower, "qreg") || strings.HasPrefix(lower, "creg") {
		kind := "qreg"
		table := regs.qreg
		if strings.HasPrefix(lower, "creg") {
			kind = "creg"
			table = regs.creg
		}
		name, size, err := parseRegDecl(stmt, kind)
		if err != nil {
			return err
		}
		if _, dup := table[name]; dup {
			return fmt.Errorf("register %q redeclared", name)
		}
		if _, clash := regs.qreg[name]; kind == "creg" && clash {
			return fmt.Errorf("register %q already declared as qreg", name)
		}
		if _, clash := regs.creg[name]; kind == "qreg" && clash {
			return fmt.Errorf("register %q already declared as creg", name)
		}
		table[name] = size
		return nil
	}
	if strings.HasPrefix(lower, "opaque") {
		name, arity, err := parseGateSig(stmt, "opaque")
		if err != nil {
			return err
		}
		custom[name] = arity
		return nil
	}
	if strings.HasPrefix(lower, "gate") {
		name, arity, err := parseGateSig(stmt, "gate")
		if err != nil {
			return err
		}
		custom[name] = arity
		return nil // body runs in the gate's own scope; executor checks it
	}
	if strings.HasPrefix(lower, "measure") {
		return checkMeasure(stmt, regs)
	}
	if strings.HasPrefix(lower, "barrier") {
		operands := strings.TrimSpace(stmt[len("barrier"):])
		for _, op := range splitTopLevel(operands, ',') {
			if err := checkQubitRef(strings.TrimSpace(op), regs); err != nil {
				return fmt.Errorf("barrier: %w", err)
			}
		}
		return nil
	}
	if strings.HasPrefix(lower, "reset") {
		op := strings.TrimSpace(stmt[len("reset"):])
		if err := checkQubitRef(op, regs); err != nil {
			return fmt.Errorf("reset: %w", err)
		}
		return nil
	}
	if strings.HasPrefix(lower, "if") {
		return checkConditional(stmt, regs, custom)
	}
	if strings.HasPrefix(lower, "qubit") || strings.Contains(stmt, "qubit[") {
		return fmt.Errorf("OPENQASM 3 programs not yet admitted; submit 2.0")
	}
	return checkGateApp(stmt, regs, custom)
}

func parseRegDecl(stmt, kind string) (string, int, error) {
	rest := strings.TrimSpace(stmt[len(kind):])
	open := strings.Index(rest, "[")
	close := strings.LastIndex(rest, "]")
	if open < 0 || close < 0 || close < open {
		return "", 0, fmt.Errorf("malformed %s declaration %q", kind, boundStmt(stmt))
	}
	name := strings.TrimSpace(rest[:open])
	if !isQASMIdent(name) {
		return "", 0, fmt.Errorf("malformed %s name %q", kind, name)
	}
	var size int
	if _, err := fmt.Sscanf(strings.TrimSpace(rest[open+1:close]), "%d", &size); err != nil || size <= 0 {
		return "", 0, fmt.Errorf("%s %q has non-positive size", kind, name)
	}
	if strings.TrimSpace(rest[close+1:]) != "" {
		return "", 0, fmt.Errorf("malformed %s declaration %q", kind, boundStmt(stmt))
	}
	return name, size, nil
}

// parseGateSig reads `gate|opaque name(params) qargs { ... }` and returns the
// qubit/param arity. Bodies are not entered.
func parseGateSig(stmt, kw string) (string, [2]int, error) {
	rest := strings.TrimSpace(stmt[len(kw):])
	name, rest := splitIdent(rest)
	if name == "" {
		return "", [2]int{}, fmt.Errorf("malformed %s declaration %q", kw, boundStmt(stmt))
	}
	params := 0
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "(") {
		end := matchParen(rest)
		if end < 0 {
			return "", [2]int{}, fmt.Errorf("malformed %s %q parameter list", kw, name)
		}
		params = len(splitTopLevel(strings.TrimSpace(rest[1:end]), ','))
		rest = strings.TrimSpace(rest[end+1:])
	}
	if idx := strings.Index(rest, "{"); idx >= 0 {
		rest = strings.TrimSpace(rest[:idx])
	}
	qargs := splitTopLevel(rest, ',')
	qubits := 0
	for _, q := range qargs {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		if !isQASMIdent(q) {
			return "", [2]int{}, fmt.Errorf("malformed %s %q qubit argument %q", kw, name, q)
		}
		qubits++
	}
	if qubits == 0 {
		return "", [2]int{}, fmt.Errorf("%s %q declares no qubits", kw, name)
	}
	return name, [2]int{qubits, params}, nil
}

func checkMeasure(stmt string, regs *qasmRegs) error {
	rest := strings.TrimSpace(stmt[len("measure"):])
	arrow := strings.Index(rest, "->")
	if arrow < 0 {
		return fmt.Errorf("measure missing '->' in %q", boundStmt(stmt))
	}
	qside := strings.TrimSpace(rest[:arrow])
	cside := strings.TrimSpace(rest[arrow+2:])
	qbits, err := resolveQubitCount(qside, regs.qreg, "measure qubit")
	if err != nil {
		return err
	}
	cbits, err := resolveQubitCount(cside, regs.creg, "measure clbit")
	if err != nil {
		// A clbit must name a creg; a qreg name here is a kind error, and
		// the resolver says so via the table it searched.
		return err
	}
	if qbits != cbits {
		return fmt.Errorf("measure width mismatch: %d qubits into %d clbits", qbits, cbits)
	}
	return nil
}

// checkQubitRef validates one qubit operand: indexed qreg bit or whole
// register (bare name). Classical registers are refused with the kind named.
func checkQubitRef(op string, regs *qasmRegs) error {
	if _, err := resolveQubitCount(op, regs.qreg, "qubit"); err != nil {
		if _, inCreg := regs.creg[bareName(op)]; inCreg {
			return fmt.Errorf("classical bit %q used as qubit", op)
		}
		return err
	}
	return nil
}

// resolveQubitCount validates an operand against one register table and
// returns how many bits it covers: `name[i]` is one bit, bare `name` is the
// whole register.
func resolveQubitCount(op string, table map[string]int, what string) (int, error) {
	op = strings.TrimSpace(op)
	if open := strings.Index(op, "["); open >= 0 {
		name := strings.TrimSpace(op[:open])
		if !isQASMIdent(name) || !strings.HasSuffix(op, "]") {
			return 0, fmt.Errorf("malformed %s reference %q", what, op)
		}
		size, ok := table[name]
		if !ok {
			return 0, fmt.Errorf("%s %q names undeclared register %q", what, op, name)
		}
		var idx int
		if _, err := fmt.Sscanf(strings.TrimSpace(op[open+1:len(op)-1]), "%d", &idx); err != nil {
			return 0, fmt.Errorf("malformed %s index in %q", what, op)
		}
		if idx < 0 || idx >= size {
			return 0, fmt.Errorf("%s %q out of range for register %q[%d]", what, op, name, size)
		}
		return 1, nil
	}
	if !isQASMIdent(op) {
		return 0, fmt.Errorf("malformed %s reference %q", what, op)
	}
	size, ok := table[op]
	if !ok {
		return 0, fmt.Errorf("%s %q names undeclared register %q", what, op, op)
	}
	return size, nil
}

func checkGateApp(stmt string, regs *qasmRegs, custom map[string][2]int) error {
	name, rest := splitIdent(stmt)
	if name == "" {
		return fmt.Errorf("unrecognized statement %q", boundStmt(stmt))
	}
	if name == "if" || name == "measure" || name == "barrier" || name == "reset" {
		return fmt.Errorf("malformed %s statement %q", name, boundStmt(stmt))
	}
	wantQ, wantP := -1, -1
	if arity, ok := coreGateArity[name]; ok {
		wantQ, wantP = arity[0], arity[1]
	} else if arity, ok := custom[name]; ok {
		wantQ, wantP = arity[0], arity[1]
	}
	// Unknown names resolve at the executor, which holds the includes.

	params := 0
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "(") {
		end := matchParen(rest)
		if end < 0 {
			return fmt.Errorf("gate %q has unbalanced parameter list", name)
		}
		inner := strings.TrimSpace(rest[1:end])
		if inner != "" {
			params = len(splitTopLevel(inner, ','))
		}
		rest = strings.TrimSpace(rest[end+1:])
	}
	if rest == "" {
		return fmt.Errorf("gate %q applies to no qubits", name)
	}
	operands := splitTopLevel(rest, ',')
	if wantQ >= 0 {
		indexed, whole := 0, 0
		for _, op := range operands {
			op = strings.TrimSpace(op)
			if op == "" {
				return fmt.Errorf("gate %q has empty qubit operand", name)
			}
			if strings.Contains(op, "[") {
				indexed++
			} else {
				whole++
			}
			if err := checkQubitRef(op, regs); err != nil {
				return fmt.Errorf("gate %q: %w", name, err)
			}
		}
		if indexed > 0 && whole > 0 {
			return fmt.Errorf("gate %q mixes register and indexed qubits", name)
		}
		if whole > 0 {
			if len(operands) != 1 || wantQ != 1 {
				// Whole-register application: one register per gate
				// qubit, sizes checked pairwise below. Multi-register
				// forms beyond that stay with the executor.
				if len(operands) != wantQ {
					return fmt.Errorf("gate %q takes %d qubits, got %d operands", name, wantQ, len(operands))
				}
				sizes := map[int]bool{}
				for _, op := range operands {
					sizes[regs.qreg[strings.TrimSpace(op)]] = true
				}
				if len(sizes) != 1 {
					return fmt.Errorf("gate %q register operands differ in size", name)
				}
			}
		} else if len(operands) != wantQ {
			return fmt.Errorf("gate %q takes %d qubits, got %d operands", name, wantQ, len(operands))
		}
		if params != wantP {
			return fmt.Errorf("gate %q takes %d parameters, got %d", name, wantP, params)
		}
	} else {
		// Unknown gate: operands must still name real qubits.
		for _, op := range operands {
			if err := checkQubitRef(strings.TrimSpace(op), regs); err != nil {
				return fmt.Errorf("gate %q: %w", name, err)
			}
		}
	}
	return nil
}

func checkConditional(stmt string, regs *qasmRegs, custom map[string][2]int) error {
	rest := strings.TrimSpace(stmt[len("if"):])
	if !strings.HasPrefix(rest, "(") {
		return fmt.Errorf("malformed if statement %q", boundStmt(stmt))
	}
	end := matchParen(rest)
	if end < 0 {
		return fmt.Errorf("if statement has unbalanced condition in %q", boundStmt(stmt))
	}
	cond := strings.TrimSpace(rest[1:end])
	eq := strings.Index(cond, "==")
	if eq < 0 {
		return fmt.Errorf("if condition must compare a creg with == in %q", boundStmt(stmt))
	}
	cregName := strings.TrimSpace(cond[:eq])
	if _, ok := regs.creg[cregName]; !ok {
		return fmt.Errorf("if condition names undeclared creg %q", cregName)
	}
	inner := strings.TrimSpace(rest[end+1:])
	if inner == "" {
		return fmt.Errorf("if statement has empty body in %q", boundStmt(stmt))
	}
	return checkGateApp(inner, regs, custom)
}

func splitIdent(s string) (string, string) {
	i := 0
	for i < len(s) && (s[i] == '_' || s[i] >= 'A' && s[i] <= 'Z' || s[i] >= 'a' && s[i] <= 'z' || s[i] >= '0' && s[i] <= '9') {
		i++
	}
	return s[:i], strings.TrimSpace(s[i:])
}

func isQASMIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	c := s[0]
	return c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

// splitTopLevel splits on sep ignoring separators nested in () or [].
func splitTopLevel(s string, sep rune) []string {
	var parts []string
	depth := 0
	var cur strings.Builder
	for _, r := range s {
		switch r {
		case '(', '[':
			depth++
			cur.WriteRune(r)
		case ')', ']':
			depth--
			cur.WriteRune(r)
		default:
			if r == sep && depth == 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			} else {
				cur.WriteRune(r)
			}
		}
	}
	parts = append(parts, cur.String())
	return parts
}

// matchParen returns the index of the close paren matching s[0] == '('.
func matchParen(s string) int {
	depth := 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func bareName(op string) string {
	if open := strings.Index(op, "["); open >= 0 {
		return strings.TrimSpace(op[:open])
	}
	return strings.TrimSpace(op)
}

func boundStmt(stmt string) string {
	t := strings.TrimSpace(stmt)
	if len([]rune(t)) > 80 {
		t = string([]rune(t)[:80]) + "…"
	}
	return t
}
