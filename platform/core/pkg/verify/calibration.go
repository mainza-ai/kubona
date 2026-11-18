package verify

// Reviewer calibration (AG-5).
//
// The verifier is deterministic, so "calibration" means: does its verdict on a
// labelled set agree with the labels? The shipped set is maintainer-labelled
// and says so — it is a regression and honesty check, not an independent human
// study. The report exposes the labeler string so nobody can cite it as more.
//
// A case fails when the verifier's status sequence differs from the expected
// sequence, so removing a finding or inventing one both count against it.

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed testdata/calibration.json
var calibrationFixture []byte

// Calibrate runs the shipped labelled set.
func Calibrate() (CalibrationReport, error) { return EvaluateCalibration(calibrationFixture) }

type calibrationFile struct {
	Version string            `json:"version"`
	Labeler string            `json:"labeler"`
	Cases   []calibrationCase `json:"cases"`
}

type calibrationCase struct {
	ID                string         `json:"id"`
	Note              string         `json:"note,omitempty"`
	Text              string         `json:"text"`
	Evidence          []Document     `json:"evidence,omitempty"`
	Citations         []Citation     `json:"citations,omitempty"`
	Resolved          []ResolvedWork `json:"resolved,omitempty"`
	ResolverError     bool           `json:"resolver_error,omitempty"`
	ExpectedNumbers   []string       `json:"expected_numbers"`
	ExpectedCitations []string       `json:"expected_citations,omitempty"`
}

// CalibrationCase is one evaluated case.
type CalibrationCase struct {
	ID         string   `json:"id"`
	Agreed     bool     `json:"agreed"`
	Expected   []string `json:"expected"`
	Observed   []string `json:"observed"`
	Difference string   `json:"difference,omitempty"`
}

// CalibrationReport is the agreement summary.
type CalibrationReport struct {
	Version     string            `json:"version"`
	Labeler     string            `json:"labeler"`
	Cases       int               `json:"cases"`
	Agreed      int               `json:"agreed"`
	Disagreed   int               `json:"disagreed"`
	Accuracy    float64           `json:"accuracy"`
	FailureList []CalibrationCase `json:"failures,omitempty"`
	Disclosure  string            `json:"disclosure"`
}

// EvaluateCalibration runs an arbitrary labelled set (used by tests and by the
// CLI against the embedded file).
func EvaluateCalibration(data []byte) (CalibrationReport, error) {
	var file calibrationFile
	if err := json.Unmarshal(data, &file); err != nil {
		return CalibrationReport{}, fmt.Errorf("calibration file is not JSON: %w", err)
	}
	if len(file.Cases) == 0 {
		return CalibrationReport{}, fmt.Errorf("calibration file has no cases")
	}
	rep := CalibrationReport{
		Version: file.Version, Labeler: file.Labeler, Cases: len(file.Cases),
		Disclosure: "Labels are maintainer-authored for regression and honesty checks; this is not an independent human annotation study.",
	}
	for _, c := range file.Cases {
		observed := []string{}
		for _, f := range VerifyNumbers(c.Text, Evidence{Documents: c.Evidence}) {
			observed = append(observed, f.Status)
		}
		expected := c.ExpectedNumbers
		if len(expected) == 0 {
			expected = []string{}
		}
		failures := diff(expected, observed)
		if len(c.Citations) > 0 || len(c.ExpectedCitations) > 0 {
			cites := VerifyCitations(context.Background(), c.Citations, fixtureResolver(c))
			citeStatuses := []string{}
			for _, f := range cites {
				citeStatuses = append(citeStatuses, f.Status)
			}
			failures = append(failures, diff(c.ExpectedCitations, citeStatuses)...)
		}
		if len(failures) == 0 {
			rep.Agreed++
			continue
		}
		rep.Disagreed++
		rep.FailureList = append(rep.FailureList, CalibrationCase{
			ID: c.ID, Agreed: false, Expected: expected, Observed: observed,
			Difference: strings.Join(failures, "; "),
		})
	}
	rep.Accuracy = float64(rep.Agreed) / float64(rep.Cases)
	return rep, nil
}

type fixtureResolver calibrationCase

func (f fixtureResolver) Resolve(_ context.Context, _ string) ([]ResolvedWork, error) {
	if f.ResolverError {
		return nil, fmt.Errorf("fixture resolver unavailable")
	}
	return f.Resolved, nil
}

func diff(expected, observed []string) []string {
	out := []string{}
	for i := 0; i < len(expected) || i < len(observed); i++ {
		var e, o string
		if i < len(expected) {
			e = expected[i]
		}
		if i < len(observed) {
			o = observed[i]
		}
		if e != o {
			out = append(out, fmt.Sprintf("[%d] expected %q observed %q", i, e, o))
		}
	}
	return out
}
