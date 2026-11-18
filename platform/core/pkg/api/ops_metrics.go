package api

// Operational gauges for /metrics (R6).
//
// The HTTP counters answer "did a request fail"; they cannot answer "is the
// queue stuck", "did the backup stop", or "is anything publishable in
// flight". These gauges are computed at scrape time from the ledger and the
// backup directory (no background goroutine, no stale cache). A gauge is
// omitted rather than fabricated when its source is not configured.

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

func (s *Server) renderOpsMetrics(w io.Writer) {
	writeGaugeMap(w, "kubona_jobs", "Settled/queued jobs by status (ledger).",
		"status", s.ledger.JobStatusCounts)
	writeGaugeMap(w, "kubona_manuscripts", "Manuscripts by state (ledger).",
		"state", s.ledger.ManuscriptStateCounts)
	writeGaugeMap(w, "kubona_hypotheses", "Hypotheses by status (ledger).",
		"status", s.ledger.HypothesisStatusCounts)
	writeGaugeMap(w, "kubona_experiment_plans", "Experiment plans by status (ledger).",
		"status", s.ledger.PlanStatusCounts)

	// AG-8b: science-stage taxonomy, cost, human actions, and queue health.
	if stages, err := s.scienceStageOutcomes(); err == nil {
		fmt.Fprintln(w, "# HELP kubona_science_stage Research-stage rows by stage and outcome.")
		fmt.Fprintln(w, "# TYPE kubona_science_stage gauge")
		for _, st := range stages {
			fmt.Fprintf(w, "kubona_science_stage{stage=%q,outcome=%q} %d\n", st.Stage, st.Outcome, st.Count)
		}
	}
	if cost, err := s.scienceCostPerAcceptedClaim(); err == nil {
		fmt.Fprintln(w, "# HELP kubona_verified_claims Claims with a verified verdict.")
		fmt.Fprintln(w, "# TYPE kubona_verified_claims gauge")
		fmt.Fprintf(w, "kubona_verified_claims %d\n", cost.VerifiedClaims)
		if cost.CostPerClaimUSD != nil {
			fmt.Fprintln(w, "# HELP kubona_cost_per_accepted_claim_usd Recorded cost divided by verified claims (null when none).")
			fmt.Fprintln(w, "# TYPE kubona_cost_per_accepted_claim_usd gauge")
			fmt.Fprintf(w, "kubona_cost_per_accepted_claim_usd %f\n", *cost.CostPerClaimUSD)
		}
	}
	if human, err := s.scienceHumanInterventions(); err == nil {
		fmt.Fprintln(w, "# HELP kubona_human_interventions Operator actions by kind.")
		fmt.Fprintln(w, "# TYPE kubona_human_interventions gauge")
		fmt.Fprintf(w, "kubona_human_interventions{kind=%q} %d\n", "plan_approvals", human.PlanApprovals)
		fmt.Fprintf(w, "kubona_human_interventions{kind=%q} %d\n", "signoffs", human.Signoffs)
		fmt.Fprintf(w, "kubona_human_interventions{kind=%q} %d\n", "autonomy_updates", human.AutonomyUpdates)
		fmt.Fprintf(w, "kubona_human_interventions{kind=%q} %d\n", "halts", human.Halts)
		fmt.Fprintf(w, "kubona_human_interventions{kind=%q} %d\n", "claim_reviews", human.ClaimReviews)
		if human.ApprovalLatencyMinutes != nil {
			fmt.Fprintln(w, "# HELP kubona_plan_approval_latency_minutes Median plan-created to approved latency.")
			fmt.Fprintln(w, "# TYPE kubona_plan_approval_latency_minutes gauge")
			fmt.Fprintf(w, "kubona_plan_approval_latency_minutes %f\n", *human.ApprovalLatencyMinutes)
		}
		fmt.Fprintln(w, "# HELP kubona_pending_approvals Proposed plans waiting for a decision.")
		fmt.Fprintln(w, "# TYPE kubona_pending_approvals gauge")
		fmt.Fprintf(w, "kubona_pending_approvals %d\n", human.PendingApprovals)
		fmt.Fprintln(w, "# HELP kubona_deferred_approvals Proposed plans parked until a future time.")
		fmt.Fprintln(w, "# TYPE kubona_deferred_approvals gauge")
		fmt.Fprintf(w, "kubona_deferred_approvals %d\n", human.DeferredApprovals)
		if human.PlanRejectionRate != nil {
			fmt.Fprintln(w, "# HELP kubona_plan_rejection_rate Rejected plans divided by decided plans.")
			fmt.Fprintln(w, "# TYPE kubona_plan_rejection_rate gauge")
			fmt.Fprintf(w, "kubona_plan_rejection_rate %f\n", *human.PlanRejectionRate)
		}
		if human.ApprovalLatencyP50 != nil {
			fmt.Fprintln(w, "# HELP kubona_plan_approval_latency_p50_minutes Nearest-rank p50 approval latency.")
			fmt.Fprintln(w, "# TYPE kubona_plan_approval_latency_p50_minutes gauge")
			fmt.Fprintf(w, "kubona_plan_approval_latency_p50_minutes %f\n", *human.ApprovalLatencyP50)
		}
		if human.ApprovalLatencyP90 != nil {
			fmt.Fprintln(w, "# HELP kubona_plan_approval_latency_p90_minutes Nearest-rank p90 approval latency.")
			fmt.Fprintln(w, "# TYPE kubona_plan_approval_latency_p90_minutes gauge")
			fmt.Fprintf(w, "kubona_plan_approval_latency_p90_minutes %f\n", *human.ApprovalLatencyP90)
		}
	}
	if queue, err := s.scienceQueueHealth(); err == nil {
		fmt.Fprintln(w, "# HELP kubona_live_jobs Jobs in queued/running state.")
		fmt.Fprintln(w, "# TYPE kubona_live_jobs gauge")
		fmt.Fprintf(w, "kubona_live_jobs %d\n", queue.LiveJobs)
		if queue.OldestLiveJobAgeSeconds != nil {
			fmt.Fprintln(w, "# HELP kubona_oldest_live_job_age_seconds Age of the oldest live job.")
			fmt.Fprintln(w, "# TYPE kubona_oldest_live_job_age_seconds gauge")
			fmt.Fprintf(w, "kubona_oldest_live_job_age_seconds %.0f\n", *queue.OldestLiveJobAgeSeconds)
		}
		fmt.Fprintln(w, "# HELP kubona_active_loops Autoresearch loops not in a terminal state.")
		fmt.Fprintln(w, "# TYPE kubona_active_loops gauge")
		fmt.Fprintf(w, "kubona_active_loops %d\n", queue.ActiveLoops)
		if queue.OldestActiveLoopAgeSeconds != nil {
			fmt.Fprintln(w, "# HELP kubona_oldest_active_loop_age_seconds Age of the oldest active loop.")
			fmt.Fprintln(w, "# TYPE kubona_oldest_active_loop_age_seconds gauge")
			fmt.Fprintf(w, "kubona_oldest_active_loop_age_seconds %.0f\n", *queue.OldestActiveLoopAgeSeconds)
		}
	}

	fmt.Fprintln(w, "# HELP kubona_bridge_configured Whether a bridge client is wired into this process.")
	fmt.Fprintln(w, "# TYPE kubona_bridge_configured gauge")
	fmt.Fprintf(w, "kubona_bridge_configured %d\n", boolGauge(s.bridgeClient() != nil))

	s.manuscriptMu.Lock()
	builds := make(map[string]int64, len(s.manuscriptBuilds))
	for k, v := range s.manuscriptBuilds {
		builds[k] = v
	}
	buildSeconds, buildCount := s.manuscriptBuildSeconds, s.manuscriptBuildCount
	s.manuscriptMu.Unlock()
	if len(builds) > 0 {
		fmt.Fprintln(w, "# HELP kubona_manuscript_builds_total Manuscript builds by outcome.")
		fmt.Fprintln(w, "# TYPE kubona_manuscript_builds_total counter")
		keys := make([]string, 0, len(builds))
		for k := range builds {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, "kubona_manuscript_builds_total{result=%q} %d\n", k, builds[k])
		}
		fmt.Fprintln(w, "# HELP kubona_manuscript_build_seconds_sum Total manuscript build time.")
		fmt.Fprintln(w, "# TYPE kubona_manuscript_build_seconds_sum counter")
		fmt.Fprintf(w, "kubona_manuscript_build_seconds_sum %f\n", buildSeconds)
		fmt.Fprintf(w, "kubona_manuscript_build_seconds_count %d\n", buildCount)
	}
	if bytes, err := s.ledger.ManuscriptArtifactBytes(); err == nil {
		fmt.Fprintln(w, "# HELP kubona_manuscript_artifact_bytes Bytes of stored manuscript artifacts.")
		fmt.Fprintln(w, "# TYPE kubona_manuscript_artifact_bytes gauge")
		fmt.Fprintf(w, "kubona_manuscript_artifact_bytes %d\n", bytes)
	}

	if dir := strings.TrimSpace(os.Getenv("KUBONA_BACKUP_DIR")); dir != "" {
		if count, age, ok := backupDirStatus(dir); ok {
			fmt.Fprintln(w, "# HELP kubona_backup_count Number of backup artifacts in KUBONA_BACKUP_DIR.")
			fmt.Fprintln(w, "# TYPE kubona_backup_count gauge")
			fmt.Fprintf(w, "kubona_backup_count %d\n", count)
			fmt.Fprintln(w, "# HELP kubona_backup_age_seconds Age of the newest backup artifact.")
			fmt.Fprintln(w, "# TYPE kubona_backup_age_seconds gauge")
			fmt.Fprintf(w, "kubona_backup_age_seconds %.0f\n", age)
		}
	}
}

func writeGaugeMap(w io.Writer, name, help, label string,
	counts func() (map[string]int64, error)) {
	m, err := counts()
	if err != nil {
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(w, "# HELP %s %s\n", name, help)
	fmt.Fprintf(w, "# TYPE %s gauge\n", name)
	for _, k := range keys {
		fmt.Fprintf(w, "%s{%s=%q} %d\n", name, label, k, m[k])
	}
}

func boolGauge(v bool) int {
	if v {
		return 1
	}
	return 0
}

// backupDirStatus finds the newest artifact (plaintext or .age) and its age.
// Sidecars are excluded. Missing/unreadable directory reports ok=false so the
// scrape omits the gauges instead of claiming a fresh backup.
func backupDirStatus(dir string) (count int, ageSeconds float64, ok bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, false
	}
	var newest time.Time
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.Contains(name, ".backup-") || strings.HasSuffix(name, ".sha256") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		count++
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	if count == 0 {
		return 0, 0, false
	}
	return count, time.Since(newest).Seconds(), true
}
