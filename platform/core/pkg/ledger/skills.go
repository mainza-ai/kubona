package ledger

import (
	"strings"
	"time"
)

// DisabledSkills returns the operator-disabled skill names. The skills store
// owns validation; this table only survives restarts.
func (l *Ledger) DisabledSkills() ([]string, error) {
	rows, err := l.db.Query(`SELECT name FROM skill_disabled ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, nil
}

// SetSkillEnabled persists the operator's enable/disable decision.
func (l *Ledger) SetSkillEnabled(name string, enabled bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	if enabled {
		_, err := l.db.Exec(`DELETE FROM skill_disabled WHERE name = ?`, name)
		return err
	}
	_, err := l.db.Exec(`INSERT INTO skill_disabled (name, disabled_at) VALUES (?, ?)
		ON CONFLICT(name) DO UPDATE SET disabled_at = excluded.disabled_at`,
		name, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
