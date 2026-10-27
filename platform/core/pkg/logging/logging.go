// Package logging configures the process-wide structured logger.
//
// Production defaults to JSON on stderr (ingestible by Loki/CloudWatch/etc.);
// dev defaults to text. KUBONA_LOG_FORMAT=json|text and KUBONA_LOG_LEVEL=
// debug|info|warn|error override explicitly.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

func Setup() {
	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(os.Getenv("KUBONA_LOG_LEVEL"))) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}
	format := strings.ToLower(strings.TrimSpace(os.Getenv("KUBONA_LOG_FORMAT")))
	if format == "" {
		if os.Getenv("KUBONA_ENV") == "production" {
			format = "json"
		} else {
			format = "text"
		}
	}
	if format == "json" {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, opts)))
		return
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, opts)))
}
