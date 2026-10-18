package agents

// R6 — transcript scoring pilot: per-role rubric measurements over
// recorded tribunal transcripts. Rule-based text signals ONLY (Q-26: no
// LLM judging, licence-clean evaluation): each signal is a documented
// substring/regex check on the turn's own content plus its recorded
// coverage/observation metadata. Scores are RECORDED, never enforced —
// measurement before gating. Nothing in the run path reads them.
//
// Rubric (per role, each signal 0/1, score = mean):
//   ansatz-scout: specificity (names qubit count, mapping, or depth),
//     quantification (carries a number for the design it proposes).
//   disconfirmation-officer: anchor_present (an energy value in Ha),
//     falsification_attempted (plateau/risk/disprov/no-advantage language
//     or a failed verdict — trying to refute counts, succeeding optional).
//   device-auditor: canary_checked (fidelity/canary + number),
//     constraints_stated (ceiling, T1/T2, or threshold language).
//   epistemic-scribe: proposal_emitted, coverage_complete (4/4 facets),
//     uncertainty_labeled (unverified/no-consensus/fallback named).
// A turn that never ran (absent from the transcript) scores 0 with all
// signals false — absence is measured, never interpolated.

import (
	"regexp"
	"strings"
)

var numberRe = regexp.MustCompile(`-?\d+(\.\d+)?`)

// ScoredTurn is the rubric input for one turn: content plus recorded
// metadata (no re-derivation — the seal already measured authorship).
type ScoredTurn struct {
	Role      AgentRole
	Content   string
	Verdict   string
	Authored  bool
	Coverage  map[string]CoverageDecl
	Observed  int
	FailedObs bool
}

// RoleScore is one role's measurement.
type RoleScore struct {
	Role    AgentRole
	Score   float64
	Signals map[string]bool
}

func has(s, sub string) bool { return strings.Contains(strings.ToLower(s), sub) }

func scoreScout(t ScoredTurn) RoleScore {
	spec := has(t.Content, "qubit") || has(t.Content, "mapping") || has(t.Content, "depth") || has(t.Content, "ansatz")
	quant := numberRe.MatchString(t.Content)
	return pack(RoleAnsatzScout, map[string]bool{"specificity": spec, "quantification": quant})
}

func scoreOfficer(t ScoredTurn) RoleScore {
	anchor := (has(t.Content, "ha") || has(t.Content, "fci") || has(t.Content, "anchor")) && numberRe.MatchString(t.Content)
	fals := has(t.Content, "plateau") || has(t.Content, "risk") || has(t.Content, "falsif") ||
		has(t.Content, "disprov") || (has(t.Content, "no ") && has(t.Content, "advantage")) ||
		t.Verdict == "REFUTED" || t.FailedObs
	return pack(RoleDisconfirmationOfficer, map[string]bool{"anchor_present": anchor, "falsification_attempted": fals})
}

func scoreAuditor(t ScoredTurn) RoleScore {
	canary := (has(t.Content, "canary") || has(t.Content, "fidelity")) && numberRe.MatchString(t.Content)
	constr := has(t.Content, "ceiling") || has(t.Content, "t1") || has(t.Content, "threshold") || has(t.Content, "approved")
	return pack(RoleDeviceAuditor, map[string]bool{"canary_checked": canary, "constraints_stated": constr})
}

func scoreScribe(t ScoredTurn, proposalEmitted bool) RoleScore {
	prop := proposalEmitted
	complete := len(t.Coverage) == 4
	if complete {
		for _, decl := range t.Coverage {
			if !decl.Available {
				complete = false
				break
			}
		}
	}
	uncertain := has(t.Content, "unverified") || has(t.Content, "did not") || has(t.Content, "not reach") ||
		has(t.Content, "fallback") || has(t.Content, "consensus")
	return pack(RoleEpistemicScribe, map[string]bool{
		"proposal_emitted": prop, "coverage_complete": complete, "uncertainty_labeled": uncertain})
}

func pack(role AgentRole, signals map[string]bool) RoleScore {
	hit := 0
	for _, v := range signals {
		if v {
			hit++
		}
	}
	return RoleScore{Role: role, Score: float64(hit) / float64(len(signals)), Signals: signals}
}

func zeroScore(role AgentRole, names []string) RoleScore {
	signals := map[string]bool{}
	for _, n := range names {
		signals[n] = false
	}
	return RoleScore{Role: role, Score: 0, Signals: signals}
}

// ScoreTranscript measures a completed run. turns carries the sealed
// turns (any order — matched by role); hasProposal mirrors the result.
func ScoreTranscript(turns []ScoredTurn, hasProposal bool) []RoleScore {
	byRole := map[AgentRole]ScoredTurn{}
	for _, t := range turns {
		byRole[t.Role] = t
	}
	out := make([]RoleScore, 0, 4)
	if t, ok := byRole[RoleAnsatzScout]; ok {
		out = append(out, scoreScout(t))
	} else {
		out = append(out, zeroScore(RoleAnsatzScout, []string{"specificity", "quantification"}))
	}
	if t, ok := byRole[RoleDisconfirmationOfficer]; ok {
		out = append(out, scoreOfficer(t))
	} else {
		out = append(out, zeroScore(RoleDisconfirmationOfficer, []string{"anchor_present", "falsification_attempted"}))
	}
	if t, ok := byRole[RoleDeviceAuditor]; ok {
		out = append(out, scoreAuditor(t))
	} else {
		out = append(out, zeroScore(RoleDeviceAuditor, []string{"canary_checked", "constraints_stated"}))
	}
	if t, ok := byRole[RoleEpistemicScribe]; ok {
		out = append(out, scoreScribe(t, hasProposal))
	} else {
		out = append(out, zeroScore(RoleEpistemicScribe, []string{"proposal_emitted", "coverage_complete", "uncertainty_labeled"}))
	}
	return out
}
