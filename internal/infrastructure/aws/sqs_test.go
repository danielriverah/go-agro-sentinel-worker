package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

func TestSQSClient_SendReceiveDelete(t *testing.T) {
	cfg := requireLocalstack(t)

	awsCfg, err := NewSession(cfg)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	rawClient := sqs.NewFromConfig(awsCfg)
	ctx := context.Background()

	queueName := "agro-sentinel-worker-test-queue"
	createOut, err := rawClient.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: &queueName})
	if err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}
	queueURL := *createOut.QueueUrl

	client := NewSQSClient(awsCfg)

	if err := client.SendMessage(ctx, queueURL, "hello agro sentinel"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	var messages []SQSMessage
	for i := 0; i < 5 && len(messages) == 0; i++ {
		messages, err = client.ReceiveMessages(ctx, queueURL, 10, 2)
		if err != nil {
			t.Fatalf("ReceiveMessages: %v", err)
		}
	}

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Body != "hello agro sentinel" {
		t.Fatalf("unexpected message body: %q", messages[0].Body)
	}
	if messages[0].ReceiptHandle == "" {
		t.Fatal("expected non-empty receipt handle")
	}

	if err := client.DeleteMessage(ctx, queueURL, messages[0].ReceiptHandle); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}

	remaining, err := client.ReceiveMessages(ctx, queueURL, 10, 1)
	if err != nil {
		t.Fatalf("ReceiveMessages after delete: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected queue to be empty after delete, got %d messages", len(remaining))
	}
}
