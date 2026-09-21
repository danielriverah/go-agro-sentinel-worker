package jobs

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/infrastructure/aws"
)

// mockQueue is an in-memory SQSQueue: ReceiveMessages returns the queued
// messages once each (simulating consumption), then blocks until ctx is
// canceled (simulating long-polling an empty queue) so Run's loop doesn't
// spin forever in tests.
type mockQueue struct {
	mu       sync.Mutex
	messages []aws.SQSMessage
	deleted  []string
	sent     []string
}

func (q *mockQueue) ReceiveMessages(ctx context.Context, queueURL string, maxMessages int, waitSeconds int) ([]aws.SQSMessage, error) {
	q.mu.Lock()
	if len(q.messages) > 0 {
		msgs := q.messages
		q.messages = nil
		q.mu.Unlock()
		return msgs, nil
	}
	q.mu.Unlock()

	// Simulate long polling on an empty queue until context cancellation.
	<-ctx.Done()
	return nil, ctx.Err()
}

func (q *mockQueue) DeleteMessage(ctx context.Context, queueURL string, receiptHandle string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.deleted = append(q.deleted, receiptHandle)
	return nil
}

func (q *mockQueue) SendMessage(ctx context.Context, queueURL string, body string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.sent = append(q.sent, body)
	return nil
}

func (q *mockQueue) wasDeleted(receiptHandle string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, d := range q.deleted {
		if d == receiptHandle {
			return true
		}
	}
	return false
}

// mockWorker records ProcessScene calls and returns a configured error.
type mockWorker struct {
	mu    sync.Mutex
	calls []string
	err   error
}

func (w *mockWorker) ProcessScene(ctx context.Context, produccionID int64, sceneID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls = append(w.calls, sceneID)
	return w.err
}

func (w *mockWorker) callCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.calls)
}

// mockSceneRepo returns a fixed scene (or nil) for lookups.
type mockSceneRepo struct {
	scene *domain.Scene
	err   error
}

func (r *mockSceneRepo) GetByProduccionAndSceneName(ctx context.Context, produccionID int64, sceneName string) (*domain.Scene, error) {
	return r.scene, r.err
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func jobBody(t *testing.T, msg JobMessage) string {
	t.Helper()
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshaling job message: %v", err)
	}
	return string(b)
}

// runUntilIdle runs p.Run in a goroutine, waits for the queue to have
// nothing left to deliver from its initial batch, then cancels and waits
// for Run to return.
func runUntilIdle(t *testing.T, p *Processor, w *mockWorker, wantCalls int) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if w.callCount() >= wantCalls {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

func TestProcessor_ReceivedMessage_ProcessedAndDeleted(t *testing.T) {
	msg := JobMessage{JobID: "job-1", ProduccionID: 1234, SceneID: "S2A_1"}
	queue := &mockQueue{messages: []aws.SQSMessage{{Body: jobBody(t, msg), ReceiptHandle: "rh-1"}}}
	worker := &mockWorker{}
	repo := &mockSceneRepo{scene: &domain.Scene{ID: 1, Status: domain.StatusPending}}

	p := New(queue, worker, repo, ProcessorConfig{QueueURL: "q", MaxRetries: 3}, testLogger())
	runUntilIdle(t, p, worker, 1)

	if worker.callCount() != 1 {
		t.Fatalf("expected ProcessScene called once, got %d", worker.callCount())
	}
	if !queue.wasDeleted("rh-1") {
		t.Fatal("expected message to be deleted after successful processing")
	}
}

func TestProcessor_AlreadyCompleted_SkipsAndDeletes(t *testing.T) {
	msg := JobMessage{JobID: "job-2", ProduccionID: 1234, SceneID: "S2A_2"}
	queue := &mockQueue{messages: []aws.SQSMessage{{Body: jobBody(t, msg), ReceiptHandle: "rh-2"}}}
	worker := &mockWorker{}
	repo := &mockSceneRepo{scene: &domain.Scene{ID: 2, Status: domain.StatusCompleted}}

	p := New(queue, worker, repo, ProcessorConfig{QueueURL: "q", MaxRetries: 3}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !queue.wasDeleted("rh-2") {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done

	if worker.callCount() != 0 {
		t.Fatalf("expected ProcessScene NOT called for completed scene, got %d calls", worker.callCount())
	}
	if !queue.wasDeleted("rh-2") {
		t.Fatal("expected message to be deleted for already-completed scene")
	}
}

func TestProcessor_ProcessSceneFails_MessageNotDeleted(t *testing.T) {
	msg := JobMessage{JobID: "job-3", ProduccionID: 1234, SceneID: "S2A_3"}
	queue := &mockQueue{messages: []aws.SQSMessage{{Body: jobBody(t, msg), ReceiptHandle: "rh-3"}}}
	worker := &mockWorker{err: &domain.ProcessingError{Type: domain.ErrGDAL, Message: "boom"}}
	repo := &mockSceneRepo{scene: &domain.Scene{ID: 3, Status: domain.StatusPending}}

	p := New(queue, worker, repo, ProcessorConfig{QueueURL: "q", MaxRetries: 3}, testLogger())
	runUntilIdle(t, p, worker, 1)

	if worker.callCount() != 1 {
		t.Fatalf("expected ProcessScene called once, got %d", worker.callCount())
	}
	if queue.wasDeleted("rh-3") {
		t.Fatal("expected message NOT to be deleted after failed processing (so SQS redelivers)")
	}
}

func TestProcessor_FailedWithRetriesRemaining_Reprocesses(t *testing.T) {
	msg := JobMessage{JobID: "job-4", ProduccionID: 1234, SceneID: "S2A_4"}
	queue := &mockQueue{messages: []aws.SQSMessage{{Body: jobBody(t, msg), ReceiptHandle: "rh-4"}}}
	worker := &mockWorker{}
	repo := &mockSceneRepo{scene: &domain.Scene{ID: 4, Status: domain.StatusFailed, RetryCount: 1}}

	p := New(queue, worker, repo, ProcessorConfig{QueueURL: "q", MaxRetries: 3}, testLogger())
	runUntilIdle(t, p, worker, 1)

	if worker.callCount() != 1 {
		t.Fatalf("expected ProcessScene called for retryable FAILED scene, got %d calls", worker.callCount())
	}
	if !queue.wasDeleted("rh-4") {
		t.Fatal("expected message to be deleted after successful retry")
	}
}

func TestProcessor_FailedExhaustedRetries_SkipsAndDeletes(t *testing.T) {
	msg := JobMessage{JobID: "job-5", ProduccionID: 1234, SceneID: "S2A_5"}
	queue := &mockQueue{messages: []aws.SQSMessage{{Body: jobBody(t, msg), ReceiptHandle: "rh-5"}}}
	worker := &mockWorker{}
	repo := &mockSceneRepo{scene: &domain.Scene{ID: 5, Status: domain.StatusFailed, RetryCount: 3}}

	p := New(queue, worker, repo, ProcessorConfig{QueueURL: "q", MaxRetries: 3}, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !queue.wasDeleted("rh-5") {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done

	if worker.callCount() != 0 {
		t.Fatalf("expected ProcessScene NOT called for retry-exhausted scene, got %d calls", worker.callCount())
	}
	if !queue.wasDeleted("rh-5") {
		t.Fatal("expected message to be deleted for retry-exhausted scene")
	}
}
