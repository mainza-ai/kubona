package ledger

// Coarse operational counts for /metrics. These are scrape-time queries over
// indexed status columns; they are not a work queue and never drive control
// flow. A failure returns an error so the caller omits the gauge rather than
// reporting a fabricated zero.

func (l *Ledger) JobStatusCounts() (map[string]int64, error) {
	return l.countBy(`SELECT status, COUNT(*) FROM jobs GROUP BY status`)
}

func (l *Ledger) ManuscriptStateCounts() (map[string]int64, error) {
	return l.countBy(`SELECT state, COUNT(*) FROM manuscripts GROUP BY state`)
}

func (l *Ledger) HypothesisStatusCounts() (map[string]int64, error) {
	return l.countBy(`SELECT status, COUNT(*) FROM hypotheses GROUP BY status`)
}

func (l *Ledger) PlanStatusCounts() (map[string]int64, error) {
	return l.countBy(`SELECT status, COUNT(*) FROM experiment_plans GROUP BY status`)
}

func (l *Ledger) countBy(query string) (map[string]int64, error) {
	rows, err := l.DB().Query(query)
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

func (l *Ledger) ManuscriptArtifactBytes() (int64, error) {
	var n int64
	err := l.DB().QueryRow(`SELECT COALESCE(SUM(LENGTH(content)), 0) FROM manuscript_artifacts`).Scan(&n)
	return n, err
}
