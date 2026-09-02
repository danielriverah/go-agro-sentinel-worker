package logger

import (
	"io"
	"log/slog"
	"os"

	"agro-sentinel-worker/internal/config"
)

func New(cfg config.LoggingConfig) *slog.Logger {
	return NewWithWriter(cfg, os.Stdout)
}

func NewWithWriter(cfg config.LoggingConfig, w io.Writer) *slog.Logger {
	level := parseLevel(cfg.Level)

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level}

	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}

	return slog.New(handler)
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
