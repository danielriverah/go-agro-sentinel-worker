package aws

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"agro-sentinel-worker/internal/domain"
)

// SQSMessage is a single message received from an SQS queue: its body and
// the receipt handle needed to delete it once processed.
type SQSMessage struct {
	Body          string
	ReceiptHandle string
}

// SQSClient wraps the AWS SDK v2 SQS client with the operations needed by
// the job processor: long-polling receive, delete, and send.
type SQSClient struct {
	client *sqs.Client
}

// NewSQSClient builds an SQSClient from an already-resolved AWS SDK config.
func NewSQSClient(awsCfg awssdk.Config) *SQSClient {
	return &SQSClient{client: sqs.NewFromConfig(awsCfg)}
}

// ReceiveMessages long-polls queueURL for up to maxMessages messages,
// waiting up to waitSeconds for at least one to arrive.
func (c *SQSClient) ReceiveMessages(ctx context.Context, queueURL string, maxMessages int, waitSeconds int) ([]SQSMessage, error) {
	out, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            awssdk.String(queueURL),
		MaxNumberOfMessages: int32(maxMessages),
		WaitTimeSeconds:     int32(waitSeconds),
	})
	if err != nil {
		return nil, &domain.ProcessingError{Type: domain.ErrSQS, Message: fmt.Sprintf("receiving messages from %s", queueURL), Wrapped: err}
	}

	messages := make([]SQSMessage, 0, len(out.Messages))
	for _, m := range out.Messages {
		var body, receiptHandle string
		if m.Body != nil {
			body = *m.Body
		}
		if m.ReceiptHandle != nil {
			receiptHandle = *m.ReceiptHandle
		}
		messages = append(messages, SQSMessage{Body: body, ReceiptHandle: receiptHandle})
	}

	return messages, nil
}

// DeleteMessage removes a processed message from queueURL so it is not
// redelivered.
func (c *SQSClient) DeleteMessage(ctx context.Context, queueURL string, receiptHandle string) error {
	_, err := c.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      awssdk.String(queueURL),
		ReceiptHandle: awssdk.String(receiptHandle),
	})
	if err != nil {
		return &domain.ProcessingError{Type: domain.ErrSQS, Message: fmt.Sprintf("deleting message from %s", queueURL), Wrapped: err}
	}
	return nil
}

// SendMessage enqueues body onto queueURL.
func (c *SQSClient) SendMessage(ctx context.Context, queueURL string, body string) error {
	_, err := c.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    awssdk.String(queueURL),
		MessageBody: awssdk.String(body),
	})
	if err != nil {
		return &domain.ProcessingError{Type: domain.ErrSQS, Message: fmt.Sprintf("sending message to %s", queueURL), Wrapped: err}
	}
	return nil
}
