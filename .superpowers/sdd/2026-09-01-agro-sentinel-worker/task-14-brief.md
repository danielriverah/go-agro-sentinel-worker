### Task 14: SQS integration and job queue

**Files:**
- Create: `internal/infrastructure/aws/sqs.go`
- Create: `internal/jobs/processor.go`
- Create: `internal/jobs/queue.go`
- Modify: `cmd/worker/main.go` — switch to SQS-driven loop
- Test: `internal/infrastructure/aws/sqs_test.go`
- Test: `internal/jobs/processor_test.go`

**Interfaces:**
- Consumes: `config.SQSConfig`, `worker.Worker.ProcessScene`
- Produces:
  - `aws.SQSClient` struct with:
    - `ReceiveMessages(ctx, queueURL string, maxMessages int, waitSeconds int) ([]SQSMessage, error)`
    - `DeleteMessage(ctx, queueURL string, receiptHandle string) error`
    - `SendMessage(ctx, queueURL string, body string) error`
  - `SQSMessage` struct `{Body string, ReceiptHandle string}`
  - `jobs.JobMessage` struct `{JobID string, ProduccionID int64, SceneID string}`
  - `jobs.Processor` struct with:
    - `New(sqsClient SQSQueue, worker SceneProcessor, sceneRepo SceneRepository, cfg ProcessorConfig, logger *slog.Logger) *Processor`
    - `Run(ctx context.Context) error` — long-poll SQS loop, process each message, delete on success
  - Idempotency: before processing, check if scene status is already COMPLETED → skip and delete message

- [ ] **Step 1: Write processor test**

Test with mock SQS and mock worker:
- Message received → ProcessScene called → message deleted
- Already COMPLETED → ProcessScene NOT called → message deleted
- ProcessScene fails → message NOT deleted (SQS will redeliver)
- Retry logic: FAILED scene with retry_count < max → reprocess

- [ ] **Step 2: Implement SQS client and processor**

- [ ] **Step 3: Update cmd/worker/main.go for SQS loop**

- [ ] **Step 4: Run tests and commit**

```bash
go test ./internal/jobs/ -v
git add .
git commit -m "feat: SQS job queue with idempotent processing and retries"
```

---

