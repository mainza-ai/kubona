package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/evals"
)

// runEvalLLM scores a recorded prediction file against the sha-pinned
// fixtures. It never calls a model and never invents a score: an incomplete
// file refuses, and a fixture/sha mismatch refuses.
//
//	kubonad eval-llm --role copilot --predictions recorded.json [--dir ~/.kubona/evals]
func runEvalLLM(args []string) int {
	fs := flag.NewFlagSet("eval-llm", flag.ContinueOnError)
	role := fs.String("role", "copilot", "fixture role to evaluate")
	predictionsPath := fs.String("predictions", "", "recorded predictions JSON (required)")
	dir := fs.String("dir", "", "report directory (default: ~/.kubona/evals or KUBONA_EVALS_DIR)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *predictionsPath == "" {
		fmt.Fprintln(os.Stderr, "usage: kubonad eval-llm --role copilot --predictions recorded.json [--dir DIR]")
		return 2
	}
	set, err := evals.Load(*role)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	input, err := evals.LoadPredictionFile(*predictionsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: predictions: %v\n", err)
		return 1
	}
	report, err := evals.Evaluate(set, input, time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	store := evals.Store{Dir: *dir}
	if store.Dir == "" {
		store.Dir = evals.DefaultDir()
	}
	if err := store.Write(report); err != nil {
		fmt.Fprintf(os.Stderr, "error: writing report: %v\n", err)
		return 1
	}
	fmt.Printf(
		"eval-llm: %s %s/%s scored %d (+%d unscored), Brier %.4f, ECE %.4f, fixtures %s\n",
		report.Role, report.Provider, report.Model, report.Total, report.Unscored,
		report.Brier, report.ECE, report.FixturesSHA256[:12],
	)
	return 0
}
