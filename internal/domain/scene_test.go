package domain

import "testing"

func TestSceneNeedsProcessing(t *testing.T) {
	s := Scene{Status: StatusPending}
	if !s.NeedsProcessing() {
		t.Error("PENDING scene should need processing")
	}

	s.Status = StatusCompleted
	if s.NeedsProcessing() {
		t.Error("COMPLETED scene should not need processing")
	}

	s.Status = StatusFailed
	s.RetryCount = 2
	if !s.CanRetry(3) {
		t.Error("FAILED scene with retry_count=2 and max=3 should be retryable")
	}

	s.RetryCount = 3
	if s.CanRetry(3) {
		t.Error("FAILED scene with retry_count=3 and max=3 should not be retryable")
	}
}
