package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/alerts"
	"github.com/mainza-ai/kubona/core/pkg/fidelity"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

// startCalibrationWatch emits a ledger event and operator alert when a
// hardware device profile crosses the shared freshness budget. It does not
// fabricate a refresh: the operator/provider must refresh the contract. One
// event is emitted per stale/resumed transition, not per poll.
func startCalibrationWatch(stop <-chan struct{}, l *ledger.Ledger) {
	seconds := 3600
	if raw := os.Getenv("KUBONA_CALIBRATION_CHECK_SECONDS"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			seconds = n
		}
	}
	previous := map[string]bool{}
	check := func() {
		stale := fidelity.StaleCalibrations(time.Now())
		current := make(map[string]bool, len(stale))
		for _, p := range stale {
			current[p.Name] = true
			if previous[p.Name] {
				continue
			}
			age := "unknown"
			if at, err := time.Parse(time.RFC3339, p.CalibratedAt); err == nil {
				age = fmt.Sprintf("%.1fh", time.Since(at).Hours())
			}
			_ = l.AppendEvent("global", "calibration_stale", map[string]any{
				"device": p.Name, "calibrated_at": p.CalibratedAt,
				"age": age, "source": p.Source, "version": p.Version,
			})
			alerts.Notify(context.Background(), "calibration_stale", "critical",
				fmt.Sprintf("device=%s calibrated_at=%s age=%s max=%.0fh", p.Name, p.CalibratedAt, age, fidelity.MaxCalibrationAgeHours()))
			slog.Warn("[Kubona Core] calibration stale", "device", p.Name, "calibrated_at", p.CalibratedAt, "age", age)
		}
		for name := range previous {
			if current[name] {
				continue
			}
			_ = l.AppendEvent("global", "calibration_resumed", map[string]any{"device": name})
			slog.Info("[Kubona Core] calibration fresh again", "device", name)
		}
		previous = current
	}
	check()
	go func() {
		timer := time.NewTicker(time.Duration(seconds) * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-stop:
				return
			case <-timer.C:
				check()
			}
		}
	}()
}
