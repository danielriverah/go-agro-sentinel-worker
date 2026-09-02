// Package jobs implements the SQS-driven job queue that feeds scenes into
// the worker's processing pipeline: receiving job messages, checking
// idempotency and retry limits, invoking the worker, and acknowledging
// (deleting) messages once handled.
package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/infrastructure/aws"
)

// JobMessage is the JSON payload carried by each SQS message: the scene to
// process, identified by its production and scene id.
type JobMessage struct {
	JobID        string `json:"job_id"`
	ProduccionID int64  `json:"produccion_id"`
	SceneID      string `json:"scene_id"`
}

// SQSQueue is the subset of aws.SQSClient's behavior the processor and
// queue helpers depend on.
type SQSQueue interface {
	ReceiveMessages(ctx context.Context, queueURL string, maxMessages int, waitSeconds int) ([]aws.SQSMessage, error)
	DeleteMessage(ctx context.Context, queueURL string, receiptHandle string) error
	SendMessage(ctx context.Context, queueURL string, body string) error
}

// Enqueue marshals msg as JSON and sends it to queueURL.
func Enqueue(ctx context.Context, queue SQSQueue, queueURL string, msg JobMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return &domain.ProcessingError{Type: domain.ErrValidation, Message: "marshaling job message", Wrapped: err}
	}

	if err := queue.SendMessage(ctx, queueURL, string(body)); err != nil {
		return err
	}

	return nil
}

// parseJobMessage unmarshals an SQS message body into a JobMessage.
func parseJobMessage(body string) (JobMessage, error) {
	var msg JobMessage
	if err := json.Unmarshal([]byte(body), &msg); err != nil {
		return JobMessage{}, &domain.ProcessingError{Type: domain.ErrValidation, Message: fmt.Sprintf("parsing job message: %q", body), Wrapped: err}
	}
	return msg, nil
}
