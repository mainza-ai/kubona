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
// moves. It is a structural pre-check, not a compiler — the executor's
// parser remains the authority and its errors stay honest execution
// failures. Every refusal names its reason.
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
	return nil
}
