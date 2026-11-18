package api

// Science telemetry (AG-8b): stage outcomes, cost per accepted claim, human
// interventions, and queue/loop health. Every value is a scrape-time read of
// durable rows. An unmeasurable value stays null rather than becoming zero —
// in particular, human minutes are not recorded by the platform, so the field
// is null with an explicit status.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/session"
)

type stageOutcome struct {
	Stage   string `json:"stage"`
	Outcome string `json:"outcome"`
	Count   int64  `json:"count"`
}

type costPerAcceptedClaim struct {
	VerifiedClaims   int64    `json:"verified_claims"`
	ClaimsWithJob    int64    `json:"claims_with_job"`
	ClaimsWithoutJob int64    `json:"claims_without_job"`
	TotalCostUSD     float64  `json:"total_cost_usd"`
	CostPerClaimUSD  *float64 `json:"cost_per_claim_usd"`
	Basis            string   `json:"basis"`
}

type humanInterventions struct {
	PlanApprovals          int64    `json:"plan_approvals"`
	Signoffs               int64    `json:"signoffs"`
	AutonomyUpdates        int64    `json:"autonomy_updates"`
	Halts                  int64    `json:"halts"`
	ClaimReviews           int64    `json:"claim_reviews"`
	Total                  int64    `json:"total"`
	HumanMinutes           *float64 `json:"human_minutes"`
	HumanMinutesStatus     string   `json:"human_minutes_status"`
	ApprovalLatencyMinutes *float64 `json:"approval_latency_minutes,omitempty"`
}

type queueHealth struct {
	JobsByStatus               map[string]int64 `json:"jobs_by_status"`
	LiveJobs                   int64            `json:"live_jobs"`
	OldestLiveJobAgeSeconds    *float64         `json:"oldest_live_job_age_seconds,omitempty"`
	LoopsByStatus              map[string]int64 `json:"loops_by_status"`
	ActiveLoops                int64            `json:"active_loops"`
	OldestActiveLoopAgeSeconds *float64         `json:"oldest_active_loop_age_seconds,omitempty"`
}

type scienceMetricsReport struct {
	Stages       []stageOutcome        `json:"stages"`
	Cost         costPerAcceptedClaim  `json:"cost"`
	Human        humanInterventions    `json:"human"`
	Queue        queueHealth           `json:"queue"`
	GeneratedAt  string                `json:"generated_at"`
	Disclosure   string                `json:"disclosure"`
}

func (s *Server) handleScienceMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteAPIError(w, "method_not_allowed", "science metrics are GET-only", nil)
		return
	}
	report := scienceMetricsReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Disclosure: "Stage counts are ledger rows; cost uses settled actual_usd when present. Human minutes are not recorded and stay null; approval latency is a measured proxy, not effort.",
	}
	var err error
	if report.Stages, err = s.scienceStageOutcomes(); err != nil {
		WriteAPIError(w, "internal", "stage taxonomy failed: "+err.Error(), nil)
		return
	}
	if report.Cost, err = s.scienceCostPerAcceptedClaim(); err != nil {
		WriteAPIError(w, "internal", "cost metric failed: "+err.Error(), nil)
		return
	}
	if report.Human, err = s.scienceHumanInterventions(); err != nil {
		WriteAPIError(w, "internal", "human metrics failed: "+err.Error(), nil)
		return
	}
	if report.Queue, err = s.scienceQueueHealth(); err != nil {
		WriteAPIError(w, "internal", "queue health failed: "+err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) scienceStageOutcomes() ([]stageOutcome, error) {
	out := []stageOutcome{}
	add := func(stage string, counts map[string]int64, mapping map[string]string) {
		for status, n := range counts {
			outcome := mapping[status]
			if outcome == "" {
				outcome = status
			}
			out = append(out, stageOutcome{Stage: stage, Outcome: outcome, Count: n})
		}
	}
	hyps, err := s.countRows(`SELECT status, COUNT(*) FROM hypotheses GROUP BY status`)
	if err != nil {
		return nil, err
	}
	add("hypothesize", hyps, map[string]string{
		"proposed": "open", "shortlisted": "open", "collapsed": "success", "rejected": "negative",
	})
	plans, err := s.countRows(`SELECT status, COUNT(*) FROM experiment_plans GROUP BY status`)
	if err != nil {
		return nil, err
	}
	add("plan", plans, map[string]string{
		"proposed": "pending", "approved": "pending", "running": "running",
		"completed": "success", "rejected": "failure", "failed": "failure",
	})
	jobs, err := s.countRows(`SELECT status, COUNT(*) FROM jobs GROUP BY status`)
	if err != nil {
		return nil, err
	}
	add("execute", jobs, map[string]string{
		"JOB_STATUS_COMPLETED": "success", "JOB_STATUS_FAILED": "failure",
		"JOB_STATUS_CANCELLED": "failure", "JOB_STATUS_INTERRUPTED": "failure",
		"JOB_STATUS_OUTCOME_UNKNOWN": "failure", "JOB_STATUS_QUEUED": "running",
		"JOB_STATUS_RUNNING": "running",
	})
	verification, err := s.verificationOutcomes()
	if err != nil {
		return nil, err
	}
	add("verify", verification, map[string]string{
		"verified": "success", "blocked": "failure", "unverified": "unverified",
	})
	var signoffs int64
	if err := s.ledger.DB().QueryRow(`SELECT COUNT(*) FROM manuscript_signoffs`).Scan(&signoffs); err != nil {
		return nil, err
	}
	if signoffs > 0 {
		out = append(out, stageOutcome{Stage: "signoff", Outcome: "success", Count: signoffs})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Stage == out[j].Stage {
			return out[i].Outcome < out[j].Outcome
		}
		return out[i].Stage < out[j].Stage
	})
	return out, nil
}

func (s *Server) verificationOutcomes() (map[string]int64, error) {
	rows, err := s.ledger.DB().Query(`SELECT verification_json FROM manuscripts WHERE verification_json <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var report struct {
			Summary struct {
				Status string `json:"status"`
			} `json:"summary"`
		}
		if json.Unmarshal([]byte(raw), &report) == nil && report.Summary.Status != "" {
			out[report.Summary.Status]++
		}
	}
	return out, rows.Err()
}

func (s *Server) scienceCostPerAcceptedClaim() (costPerAcceptedClaim, error) {
	out := costPerAcceptedClaim{
		Basis: "actual_usd from the settled result when present, else the reserved amount",
	}
	rows, err := s.ledger.DB().Query(`SELECT evidence_json FROM claim_records WHERE verdict = 'verified' LIMIT 1000`)
	if err != nil {
		return out, err
	}
	jobIDs := []string{}
	for rows.Next() {
		var evidence string
		if err := rows.Scan(&evidence); err != nil {
			rows.Close()
			return out, err
		}
		out.VerifiedClaims++
		var ref struct {
			JobID string `json:"job_id"`
		}
		if json.Unmarshal([]byte(evidence), &ref) == nil && ref.JobID != "" {
			jobIDs = append(jobIDs, ref.JobID)
			out.ClaimsWithJob++
		} else {
			out.ClaimsWithoutJob++
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	for _, id := range jobIDs {
		var reservedMicro int64
		var resultJSON string
		err := s.ledger.DB().QueryRow(`SELECT COALESCE(reserved_amount_micro_usd, 0),
			COALESCE(result_json, '') FROM jobs WHERE id = ?`, id).Scan(&reservedMicro, &resultJSON)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return out, err
		}
		cost := float64(reservedMicro) / 1e6
		if actual, ok := actualUSD(resultJSON); ok {
			cost = actual
		}
		out.TotalCostUSD += cost
	}
	if out.VerifiedClaims > 0 {
		per := out.TotalCostUSD / float64(out.VerifiedClaims)
		out.CostPerClaimUSD = &per
	}
	return out, nil
}

func actualUSD(raw string) (float64, bool) {
	if raw == "" {
		return 0, false
	}
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return 0, false
	}
	return walkActualUSD(v, 0)
}

func walkActualUSD(v any, depth int) (float64, bool) {
	if depth > 8 {
		return 0, false
	}
	switch x := v.(type) {
	case map[string]any:
		if raw, ok := x["actual_usd"]; ok {
			if n, ok := numberValue(raw); ok && n >= 0 && !math.IsInf(n, 0) && !math.IsNaN(n) {
				return n, true
			}
		}
		for _, child := range x {
			if n, ok := walkActualUSD(child, depth+1); ok {
				return n, true
			}
		}
	case []any:
		for _, child := range x {
			if n, ok := walkActualUSD(child, depth+1); ok {
				return n, true
			}
		}
	}
	return 0, false
}

func numberValue(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func (s *Server) scienceHumanInterventions() (humanInterventions, error) {
	out := humanInterventions{HumanMinutesStatus: "unmeasured: the platform does not record operator wall-clock time"}
	count := func(query string) (int64, error) {
		var n int64
		err := s.ledger.DB().QueryRow(query).Scan(&n)
		return n, err
	}
	var err error
	if out.Signoffs, err = count(`SELECT COUNT(*) FROM ledger_events WHERE event_type = 'manuscript_signed'`); err != nil {
		return out, err
	}
	if out.AutonomyUpdates, err = count(`SELECT COUNT(*) FROM ledger_events WHERE event_type = 'autonomy_policy_updated'`); err != nil {
		return out, err
	}
	if out.Halts, err = count(`SELECT COUNT(*) FROM ledger_events WHERE event_type = 'autonomy_halted'`); err != nil {
		return out, err
	}
	if out.ClaimReviews, err = count(`SELECT COUNT(*) FROM claim_reviews`); err != nil {
		return out, err
	}
	approvals, latencies, err := s.planApprovalEvents()
	if err != nil {
		return out, err
	}
	out.PlanApprovals = approvals
	out.Total = out.PlanApprovals + out.Signoffs + out.AutonomyUpdates + out.Halts + out.ClaimReviews
	if len(latencies) > 0 {
		median := medianFloat(latencies)
		out.ApprovalLatencyMinutes = &median
	}
	return out, nil
}

func (s *Server) planApprovalEvents() (int64, []float64, error) {
	rows, err := s.ledger.DB().Query(`SELECT event_type, payload, timestamp FROM ledger_events
		WHERE event_type IN ('plan_created', 'plan_status_changed') ORDER BY id ASC LIMIT 20000`)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	type marks struct {
		created  time.Time
		approved time.Time
	}
	byPlan := map[string]*marks{}
	for rows.Next() {
		var eventType, payload, stamp string
		if err := rows.Scan(&eventType, &payload, &stamp); err != nil {
			return 0, nil, err
		}
		var body struct {
			PlanID string `json:"plan_id"`
			Status string `json:"status"`
		}
		if json.Unmarshal([]byte(payload), &body) != nil || body.PlanID == "" {
			continue
		}
		m := byPlan[body.PlanID]
		if m == nil {
			m = &marks{}
			byPlan[body.PlanID] = m
		}
		t, ok := parseLedgerTime(stamp)
		if !ok {
			continue
		}
		switch eventType {
		case "plan_created":
			if m.created.IsZero() {
				m.created = t
			}
		case "plan_status_changed":
			if body.Status == "approved" && m.approved.IsZero() {
				m.approved = t
			}
		}
	}
	if err := rows.Err(); err != nil {
		return 0, nil, err
	}
	var approvals int64
	latencies := []float64{}
	for _, m := range byPlan {
		if m.approved.IsZero() {
			continue
		}
		approvals++
		if !m.created.IsZero() && m.approved.After(m.created) {
			latencies = append(latencies, m.approved.Sub(m.created).Minutes())
		}
	}
	return approvals, latencies, nil
}

func (s *Server) scienceQueueHealth() (queueHealth, error) {
	out := queueHealth{JobsByStatus: map[string]int64{}, LoopsByStatus: map[string]int64{}}
	jobs, err := s.countRows(`SELECT status, COUNT(*) FROM jobs GROUP BY status`)
	if err != nil {
		return out, err
	}
	out.JobsByStatus = jobs
	for _, status := range session.LiveJobStatuses {
		out.LiveJobs += jobs[status]
	}
	clause, args := session.LiveJobStatusClause()
	var oldest any
	if err := s.ledger.DB().QueryRow(`SELECT MIN(created_at) FROM jobs WHERE status `+clause, args...).Scan(&oldest); err != nil {
		return out, err
	}
	if stamp := anyString(oldest); stamp != "" {
		if t, ok := parseLedgerTime(stamp); ok {
			age := time.Since(t).Seconds()
			out.OldestLiveJobAgeSeconds = &age
		}
	}
	loops, err := s.countRows(`SELECT status, COUNT(*) FROM loops WHERE status IS NOT NULL GROUP BY status`)
	if err != nil {
		return out, err
	}
	out.LoopsByStatus = loops
	terminal := map[string]bool{}
	for _, status := range session.TerminalLoopStatuses {
		terminal[status] = true
	}
	for status, n := range loops {
		if !terminal[status] {
			out.ActiveLoops += n
		}
	}
	if len(session.TerminalLoopStatuses) > 0 {
		placeholders := session.Placeholders(len(session.TerminalLoopStatuses))
		loopArgs := make([]any, 0, len(session.TerminalLoopStatuses))
		for _, status := range session.TerminalLoopStatuses {
			loopArgs = append(loopArgs, status)
		}
		var oldestLoop any
		if err := s.ledger.DB().QueryRow(`SELECT MIN(created_at) FROM loops WHERE status NOT IN (`+placeholders+`)`, loopArgs...).Scan(&oldestLoop); err != nil {
			return out, err
		}
		if stamp := anyString(oldestLoop); stamp != "" {
			if t, ok := parseLedgerTime(stamp); ok {
				age := time.Since(t).Seconds()
				out.OldestActiveLoopAgeSeconds = &age
			}
		}
	}
	return out, nil
}

func (s *Server) countRows(query string, args ...any) (map[string]int64, error) {
	rows, err := s.ledger.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var key string
		var n int64
		if err := rows.Scan(&key, &n); err != nil {
			return nil, err
		}
		out[key] = n
	}
	return out, rows.Err()
}

func parseLedgerTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{
		time.RFC3339Nano, time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func anyString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	case time.Time:
		return x.Format(time.RFC3339)
	}
	return ""
}

func medianFloat(values []float64) float64 {
	sort.Float64s(values)
	n := len(values)
	if n%2 == 1 {
		return values[n/2]
	}
	return (values[n/2-1] + values[n/2]) / 2
}
