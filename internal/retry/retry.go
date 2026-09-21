package retry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// RetryConfig defines retry behavior.
type RetryConfig struct {
	MaxAttempts   int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	Multiplier    float64
	TotalTimeout  time.Duration
}

// WithBackoff retries fn with exponential backoff according to cfg.
// It differentiates retryable errors (transient) from fatal errors.
// Returns nil on success or the last error after exhausting attempts.
func WithBackoff(ctx context.Context, cfg RetryConfig, fn func() error) error {
	var lastErr error
	delay := cfg.InitialDelay
	startTime := time.Now()

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		// Check if total timeout exceeded.
		if time.Since(startTime) > cfg.TotalTimeout {
			return fmt.Errorf("total timeout exceeded after %d attempts: %w", attempt-1, lastErr)
		}

		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Non-retryable errors fail immediately.
		if !isRetryable(err) {
			return fmt.Errorf("non-retryable error: %w", err)
		}

		if attempt < cfg.MaxAttempts {
			// Calculate next delay before sleeping to respect total timeout.
			nextDelay := time.Duration(float64(delay) * cfg.Multiplier)
			if nextDelay > cfg.MaxDelay {
				nextDelay = cfg.MaxDelay
			}

			// Check if sleeping would exceed total timeout.
			if time.Since(startTime)+delay > cfg.TotalTimeout {
				return fmt.Errorf("total timeout exceeded after %d attempts: %w", attempt, lastErr)
			}

			time.Sleep(delay)
			delay = nextDelay
		}
	}

	return fmt.Errorf("failed after %d attempts: %w", cfg.MaxAttempts, lastErr)
}

// isRetryable reports whether err represents a transient error that should be retried.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Retryable error patterns.
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"connection timeout",
		"timeout",
		"503",
		"service unavailable",
		"too many connections",
		"i/o timeout",
		"temporary failure",
		"resource temporarily unavailable",
	}

	errMsg := strings.ToLower(err.Error())
	for _, pattern := range retryablePatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	// Check for context timeout (retryable).
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	return false
}

// PresetConfigs for common retry scenarios.
var (
	// SyncConfig: aggressive retry for sync operations (multiple services).
	SyncConfig = RetryConfig{
		MaxAttempts:  4,
		InitialDelay: 2 * time.Second,
		MaxDelay:     60 * time.Second,
		Multiplier:   2.0,
		TotalTimeout: 5 * time.Minute,
	}

	// WorkerConfig: moderate retry for worker operations.
	WorkerConfig = RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		TotalTimeout: 3 * time.Minute,
	}

	// APIConfig: quick retry for API startup.
	APIConfig = RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		TotalTimeout: 2 * time.Minute,
	}
)
