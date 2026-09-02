package logger

import (
	"bytes"
	"strings"
	"testing"

	"agro-sentinel-worker/internal/config"
)

func TestNewLogger(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(config.LoggingConfig{Level: "info", Format: "json"}, &buf)

	l.Info("test message", "job_id", "123")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("log output should contain message, got: %s", output)
	}
	if !strings.Contains(output, "123") {
		t.Errorf("log output should contain job_id value, got: %s", output)
	}
}

func TestNewLoggerTextFormat(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(config.LoggingConfig{Level: "debug", Format: "text"}, &buf)

	l.Debug("debug message")

	output := buf.String()
	if !strings.Contains(output, "debug message") {
		t.Errorf("debug log should appear at debug level, got: %s", output)
	}
}
