// Package gdal provides a thin wrapper around the GDAL command-line tools.
// GDAL is always invoked as an external process (never as a Go library
// binding) so the worker has no cgo dependency on libgdal.
package gdal

import (
	"bytes"
	"context"
	"os/exec"
	"time"

	"agro-sentinel-worker/internal/domain"
)

// Executor runs GDAL command-line tools with a configurable timeout.
type Executor struct {
	// DefaultTimeout is used when the caller's context has no deadline.
	DefaultTimeout time.Duration
}

// NewExecutor creates an Executor whose default timeout is timeoutSeconds.
// If timeoutSeconds is <= 0, a default of 300 seconds is used.
func NewExecutor(timeoutSeconds int) *Executor {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}
	return &Executor{DefaultTimeout: time.Duration(timeoutSeconds) * time.Second}
}

// Run executes a GDAL command (e.g. "gdalinfo") with the given args,
// capturing stdout and stderr. If ctx has no deadline, the Executor's
// DefaultTimeout is applied. A non-zero exit code, a timeout, or a failure
// to start the process are all reported as *domain.ProcessingError with
// Type domain.ErrGDAL.
func (e *Executor) Run(ctx context.Context, command string, args []string) (string, string, error) {
	runCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		timeout := e.DefaultTimeout
		if timeout <= 0 {
			timeout = 300 * time.Second
		}
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, command, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return stdout.String(), stderr.String(), &domain.ProcessingError{
				Type:    domain.ErrGDAL,
				Message: command + " timed out",
				Wrapped: runCtx.Err(),
			}
		}
		return stdout.String(), stderr.String(), &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: command + " failed: " + stderr.String(),
			Wrapped: err,
		}
	}

	return stdout.String(), stderr.String(), nil
}
