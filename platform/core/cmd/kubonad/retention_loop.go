package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
)

// startRetentionLoop runs the configured retention pass once per interval.
// It is off by default (local development should not silently delete chat
// text); production compose sets KUBONA_RETENTION_ENABLED=1.
func startRetentionLoop(l *ledger.Ledger, sup *supervisor.Supervisor) func() {
	if strings.TrimSpace(os.Getenv("KUBONA_RETENTION_ENABLED")) != "1" {
		return func() {}
	}
	cfg := ledger.DefaultRetentionConfig()
	if v := envInt("KUBONA_RETENTION_DAYS", 0); v > 0 {
		cfg.ChatMessageDays = v
	}
	if v := envInt("KUBONA_RETENTION_TOMBSTONE_DAYS", 0); v > 0 {
		cfg.TombstoneDays = v
	}
	hours := envInt("KUBONA_RETENTION_INTERVAL_HOURS", 24)
	if hours < 1 {
		hours = 24
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(time.Duration(hours) * time.Hour)
		defer ticker.Stop()
		// First pass after one interval, not at boot: startup should not race
		// migrations and preflight.
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				res, err := l.RunRetention(cfg)
				if err != nil {
					slog.Warn("[Kubona Core] retention failed", "error", err)
					continue
				}
				if res.ChatMessages+res.Tombstones > 0 {
					slog.Info("[Kubona Core] retention pass", "chat_messages", res.ChatMessages, "tombstones", res.Tombstones)
				}
				if sup != nil {
					if n := sup.EvictTerminalLoops(time.Now().UTC().AddDate(0, 0, -90)); n > 0 {
						slog.Info("[Kubona Core] evicted terminal loops", "count", n)
					}
				}
			}
		}
	}()
	return cancel
}

func envInt(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}
