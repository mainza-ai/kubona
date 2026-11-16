package api

// B5 pins: citation checking is deterministic and the stream filter never
// tears a citation mark. No model judge is involved.

import (
	"encoding/json"
	"strings"
	"testing"
)

func observation(id, tool, data string) ToolObservation {
	return ToolObservation{ID: id, Tool: tool, Data: json.RawMessage(data)}
}

func TestCheckCitationsClassifiesClaims(t *testing.T) {
	observations := []ToolObservation{
		observation("o1", "get_job", `{"energy":-1.13727,"shots":1024,"params":{"depth":3}}`),
	}
	reply := strings.Join([]string{
		"The measured energy is -1.13727 Ha [o1].",
		"An uncited figure 0.7414 appears here.",
		"A bad citation 5.5000 [o9].",
		"A wrong citation -2.0 [o1].",
		"Shots 1024 [o1] were used; 2 qubits were active.",
		"```qasm\nOPENQASM 2.0;\nrz(0.785) q[0];\n```",
	}, "\n")
	report := CheckCitations(reply, observations)

	statuses := map[string]string{}
	for _, claim := range report.Claims {
		statuses[claim.Number+"|"+claim.Citation] = claim.Status
	}
	if statuses["-1.13727|o1"] != "verified" {
		t.Fatalf("energy must verify: %+v", report.Claims)
	}
	if statuses["0.7414|"] != "uncited" {
		t.Fatalf("uncited decimal must be flagged: %+v", report.Claims)
	}
	if statuses["5.5000|o9"] != "unknown_observation" {
		t.Fatalf("unknown observation must be flagged: %+v", report.Claims)
	}
	if statuses["-2.0|o1"] != "not_in_observation" {
		t.Fatalf("mismatched number must be flagged: %+v", report.Claims)
	}
	if statuses["1024|o1"] != "verified" {
		t.Fatalf("large counts are checked: %+v", report.Claims)
	}
	// Small prose integers and fenced machine code are not claims.
	for number := range statuses {
		if strings.HasPrefix(number, "2|") || strings.HasPrefix(number, "0.785|") || strings.HasPrefix(number, "2.0|") {
			t.Fatalf("non-claim number was checked: %s", number)
		}
	}
	if report.Verified != 2 || report.Unverified != 3 {
		t.Fatalf("counts: verified=%d unverified=%d (%+v)", report.Verified, report.Unverified, report.Claims)
	}
}

func TestCheckCitationsTolerance(t *testing.T) {
	observations := []ToolObservation{observation("o1", "get_job", `{"bond_distance":0.7414}`)}
	report := CheckCitations("Bond 0.7414001 A [o1].", observations)
	if len(report.Claims) != 1 || report.Claims[0].Status != "verified" {
		t.Fatalf("within-tolerance value must verify: %+v", report.Claims)
	}
	report = CheckCitations("Bond 0.7500 A [o1].", observations)
	if report.Claims[0].Status != "not_in_observation" {
		t.Fatalf("outside-tolerance value must not verify: %+v", report.Claims)
	}
}

func TestCheckCitationsEmptyObservationsAreUnverified(t *testing.T) {
	report := CheckCitations("Energy -1.13727 Ha.", nil)
	if len(report.Claims) != 1 || report.Claims[0].Status != "uncited" || report.Unverified != 1 {
		t.Fatalf("no observations means no verified citation: %+v", report)
	}
}

func TestCitationStreamFilterPreservesTextAndMarks(t *testing.T) {
	text := "Energy -1.13727 [o1], shots 1024 [o12]."
	filter := &CitationStreamFilter{}
	var out strings.Builder
	var emitted []string
	for _, chunk := range []string{"Energy -1.1", "3727 [o", "1], shots 10", "24 [o1", "2]."} {
		part := filter.Push(chunk)
		emitted = append(emitted, part)
		out.WriteString(part)
	}
	out.WriteString(filter.Flush())
	if out.String() != text {
		t.Fatalf("filter must preserve the stream exactly:\n got %q\nwant %q", out.String(), text)
	}
	for _, part := range emitted {
		if incompleteCitation(part[strings.LastIndex(part, "[")+1:]) && strings.Contains(part, "[") {
			// Only a trailing partial mark is held back.
			idx := strings.LastIndex(part, "[")
			if incompleteCitation(part[idx:]) {
				t.Fatalf("emitted chunk tore a citation: %q", part)
			}
		}
	}
}

func TestCitationReportPersistsOnMessage(t *testing.T) {
	srv, l, cleanup := loopTestServer(t, "test_citations_msg.db")
	defer cleanup()
	sess, err := srv.sessionMgr.CreateSession("citations", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	msg, err := srv.sessionMgr.AddMessage(sess.ID, "assistant", "Energy -1.1 [o1]", nil)
	if err != nil {
		t.Fatal(err)
	}
	report, raw := srv.citationReport("Energy -1.1 [o1]", []ToolObservation{
		observation("o1", "get_job", `{"energy":-1.1}`),
	})
	if raw == nil || report.Verified != 1 {
		t.Fatalf("report: %+v %v", report, raw)
	}
	if err := srv.sessionMgr.SetMessageCitations(msg.ID, raw); err != nil {
		t.Fatal(err)
	}
	msgs, err := srv.sessionMgr.GetMessages(sess.ID)
	if err != nil || len(msgs) != 1 || msgs[0].Citations == nil {
		t.Fatalf("citations must reload: %v %+v", err, msgs)
	}
	var decoded CitationReport
	if err := json.Unmarshal(*msgs[0].Citations, &decoded); err != nil || decoded.Verified != 1 {
		t.Fatalf("persisted citations: %v %+v", err, decoded)
	}
	_ = l
}

func TestCheckCitationsScansTableCells(t *testing.T) {
	observations := []ToolObservation{observation("o1", "get_job", `{"energy":-1.13727}`)}
	reply := "| Quantity | Value |\n|---|---|\n| Energy | -1.13727 [o1] |"
	report := CheckCitations(reply, observations)
	found := false
	for _, claim := range report.Claims {
		if claim.Number == "-1.13727" {
			found = true
			if claim.Status != "verified" {
				t.Fatalf("table value must verify: %+v", claim)
			}
		}
	}
	if !found {
		t.Fatalf("table value was not scanned: %+v", report.Claims)
	}
}

func TestCheckCitationsScansMathSource(t *testing.T) {
	observations := []ToolObservation{observation("o1", "get_job", `{"energy":-1.13727}`)}
	report := CheckCitations("Energy $E = -1.13727 [o1]$ Ha.", observations)
	found := false
	for _, claim := range report.Claims {
		if claim.Number == "-1.13727" {
			found = true
			if claim.Status != "verified" {
				t.Fatalf("math source value must verify: %+v", claim)
			}
		}
	}
	if !found {
		t.Fatalf("math source value was not scanned: %+v", report.Claims)
	}
}
