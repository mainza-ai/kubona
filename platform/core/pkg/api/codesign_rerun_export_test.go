package api

// Test-only bridge for the pure proposal parser. The route tests exercise
// the full path; this keeps the strict-parser pins in the external test
// package without widening the production API surface.
type CodesignOpForTest = codesignOp

func ParseCodesignProposalForTest(text string, numQubits, maxOps int) ([]codesignOp, error) {
	return parseCodesignProposal(text, numQubits, maxOps)
}
