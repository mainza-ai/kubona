package main

// kubonad verify-calibration reports how the manuscript claim verifier agrees
// with its shipped labelled set (AG-5). The set is maintainer-labelled and the
// report says so; this is a regression/honesty gate, not an independent study.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/mainza-ai/kubona/core/pkg/verify"
)

func runVerifyCalibration(args []string) int {
	fs := flag.NewFlagSet("verify-calibration", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print the full report as JSON")
	minAccuracy := fs.Float64("min-accuracy", 1.0, "fail when agreement is below this fraction")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rep, err := verify.Calibrate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	} else {
		fmt.Printf("verification calibration: %d/%d agreed (%.1f%%)\n",
			rep.Agreed, rep.Cases, rep.Accuracy*100)
		fmt.Printf("labeler: %s\n", rep.Labeler)
		fmt.Printf("disclosure: %s\n", rep.Disclosure)
		for _, f := range rep.FailureList {
			fmt.Printf("  FAIL %s: %s\n", f.ID, f.Difference)
		}
	}
	if rep.Accuracy < *minAccuracy {
		return 1
	}
	return 0
}
