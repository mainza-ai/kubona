package main

import "testing"

func TestVerifyCalibrationCLIPassesItsGate(t *testing.T) {
	if got := runVerifyCalibration(nil); got != 0 {
		t.Fatalf("calibration exit = %d, want 0", got)
	}
	if got := runVerifyCalibration([]string{"--json"}); got != 0 {
		t.Fatalf("json calibration exit = %d, want 0", got)
	}
	if got := runVerifyCalibration([]string{"--min-accuracy", "1.01"}); got != 1 {
		t.Fatalf("unreachable gate exit = %d, want 1", got)
	}
}
