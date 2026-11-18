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
