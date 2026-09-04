diff --git a/cmd/worker/main.go b/cmd/worker/main.go
index e3e9ca3..1fcf49c 100644
--- a/cmd/worker/main.go
+++ b/cmd/worker/main.go
@@ -1,27 +1,29 @@
 package main
 
 import (
 	"context"
 	"flag"
-	"fmt"
 	"log"
 	"os"
+	"os/signal"
+	"syscall"
 
 	awssdk "github.com/aws/aws-sdk-go-v2/config"
 
 	"agro-sentinel-worker/internal/config"
 	"agro-sentinel-worker/internal/domain"
 	"agro-sentinel-worker/internal/infrastructure/aws"
 	"agro-sentinel-worker/internal/infrastructure/database"
 	"agro-sentinel-worker/internal/infrastructure/gdal"
 	"agro-sentinel-worker/internal/infrastructure/ia"
+	"agro-sentinel-worker/internal/jobs"
 	"agro-sentinel-worker/internal/logger"
 	"agro-sentinel-worker/internal/worker"
 )
 
 // stacBandResolver is a placeholder worker.BandResolver: full STAC/COG
 // discovery is not implemented yet. It fails clearly rather than silently
 // producing an empty band set.
 type stacBandResolver struct{}
 
 func (stacBandResolver) ResolveBands(ctx context.Context, produccionID int64, sceneID string) ([]domain.BandInfo, string, error) {
@@ -42,54 +44,78 @@ func main() {
 	}
 
 	cfg, err := config.Load(cfgPath)
 	if err != nil {
 		log.Fatalf("loading config: %v", err)
 	}
 
 	l := logger.New(cfg.Logging)
 	l.Info("worker starting", "name", cfg.App.Name)
 
-	if *produccionID == 0 || *sceneID == "" {
-		fmt.Println("Usage: go run ./cmd/worker -production 1234 -scene S2A_xxx")
-		os.Exit(1)
-	}
-
 	ctx := context.Background()
 
 	db, err := database.NewConnection(cfg.MySQL)
 	if err != nil {
 		log.Fatalf("connecting to database: %v", err)
 	}
 	defer db.Close()
 
 	awsCfg, err := awssdk.LoadDefaultConfig(ctx)
 	if err != nil {
 		log.Fatalf("loading AWS config: %v", err)
 	}
 
 	executor := gdal.NewExecutor(cfg.GDAL.TimeoutSeconds)
 	s3Client := aws.NewS3Client(awsCfg)
+	sceneRepo := database.NewSceneRepo(db)
 
 	deps := worker.WorkerDeps{
 		Productions: database.NewProductionRepo(db),
-		Scenes:      database.NewSceneRepo(db),
+		Scenes:      sceneRepo,
 		Files:       database.NewFileRepo(db),
 		S3:          s3Client,
 		Executor:    executor,
 		Bands:       stacBandResolver{},
 		IA:          ia.New(cfg.IA),
 		Processing:  cfg.Processing,
 		Sentinel:    cfg.Sentinel,
 		S3Config:    cfg.S3,
 		Logger:      l,
 	}
 
 	w := worker.New(deps)
 
-	if err := w.ProcessScene(ctx, *produccionID, *sceneID); err != nil {
-		l.Error("scene processing failed", "produccion_id", *produccionID, "scene_id", *sceneID, "error", err)
+	// -production/-scene run a single scene once and exit, for manual
+	// invocation and debugging. With no flags, the worker runs the
+	// SQS-driven job queue loop until it receives a termination signal.
+	if *produccionID != 0 && *sceneID != "" {
+		if err := w.ProcessScene(ctx, *produccionID, *sceneID); err != nil {
+			l.Error("scene processing failed", "produccion_id", *produccionID, "scene_id", *sceneID, "error", err)
+			os.Exit(1)
+		}
+		l.Info("scene processing completed", "produccion_id", *produccionID, "scene_id", *sceneID)
+		return
+	}
+
+	if cfg.SQS.QueueURL == "" {
+		log.Fatal("sqs.queue_url is not configured; set it or pass -production/-scene for a single-scene run")
+	}
+
+	sqsClient := aws.NewSQSClient(awsCfg)
+	processor := jobs.New(sqsClient, w, sceneRepo, jobs.ProcessorConfig{
+		QueueURL:    cfg.SQS.QueueURL,
+		MaxMessages: cfg.SQS.MaxMessages,
+		WaitSeconds: cfg.SQS.WaitSeconds,
+		MaxRetries:  cfg.SQS.MaxRetries,
+	}, l)
+
+	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
+	defer stop()
+
+	l.Info("worker listening for jobs", "queue_url", cfg.SQS.QueueURL)
+	if err := processor.Run(runCtx); err != nil {
+		l.Error("job processor stopped with error", "error", err)
 		os.Exit(1)
 	}
 
-	l.Info("scene processing completed", "produccion_id", *produccionID, "scene_id", *sceneID)
+	l.Info("worker shut down gracefully")
 }
diff --git a/internal/config/config.go b/internal/config/config.go
index 4a7ade2..14093e8 100644
--- a/internal/config/config.go
+++ b/internal/config/config.go
@@ -55,21 +55,24 @@ type AWSConfig struct {
 	Region   string `yaml:"region"`
 	Endpoint string `yaml:"endpoint"`
 }
 
 type S3Config struct {
 	Bucket string `yaml:"bucket"`
 	Prefix string `yaml:"prefix"`
 }
 
 type SQSConfig struct {
-	QueueURL string `yaml:"queue_url"`
+	QueueURL    string `yaml:"queue_url"`
+	MaxMessages int    `yaml:"max_messages"`
+	WaitSeconds int    `yaml:"wait_seconds"`
+	MaxRetries  int    `yaml:"max_retries"`
 }
 
 type DynamoDBConfig struct {
 	TableProducciones string `yaml:"table_producciones"`
 	TableEscenas      string `yaml:"table_escenas"`
 }
 
 type MySQLConfig struct {
 	Host     string `yaml:"host"`
 	Port     int    `yaml:"port"`
diff --git a/internal/domain/errors.go b/internal/domain/errors.go
index 49325b5..23e5381 100644
--- a/internal/domain/errors.go
+++ b/internal/domain/errors.go
@@ -7,20 +7,21 @@ type ErrType string
 const (
 	ErrGDAL       ErrType = "GDAL_ERROR"
 	ErrS3         ErrType = "S3_ERROR"
 	ErrTimeout    ErrType = "TIMEOUT"
 	ErrValidation ErrType = "VALIDATION_ERROR"
 	ErrSTAC       ErrType = "STAC_ERROR"
 	ErrIA         ErrType = "IA_ERROR"
 	ErrMySQL      ErrType = "MYSQL_ERROR"
 	ErrDynamoDB   ErrType = "DYNAMODB_ERROR"
 	ErrDisk       ErrType = "DISK_ERROR"
+	ErrSQS        ErrType = "SQS_ERROR"
 )
 
 type ProcessingError struct {
 	Type    ErrType
 	Message string
 	Wrapped error
 }
 
 func (e *ProcessingError) Error() string {
 	if e.Wrapped != nil {
diff --git a/internal/infrastructure/aws/sqs.go b/internal/infrastructure/aws/sqs.go
new file mode 100644
index 0000000..08ab1c5
--- /dev/null
+++ b/internal/infrastructure/aws/sqs.go
@@ -0,0 +1,81 @@
+package aws
+
+import (
+	"context"
+	"fmt"
+
+	awssdk "github.com/aws/aws-sdk-go-v2/aws"
+	"github.com/aws/aws-sdk-go-v2/service/sqs"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// SQSMessage is a single message received from an SQS queue: its body and
+// the receipt handle needed to delete it once processed.
+type SQSMessage struct {
+	Body          string
+	ReceiptHandle string
+}
+
+// SQSClient wraps the AWS SDK v2 SQS client with the operations needed by
+// the job processor: long-polling receive, delete, and send.
+type SQSClient struct {
+	client *sqs.Client
+}
+
+// NewSQSClient builds an SQSClient from an already-resolved AWS SDK config.
+func NewSQSClient(awsCfg awssdk.Config) *SQSClient {
+	return &SQSClient{client: sqs.NewFromConfig(awsCfg)}
+}
+
+// ReceiveMessages long-polls queueURL for up to maxMessages messages,
+// waiting up to waitSeconds for at least one to arrive.
+func (c *SQSClient) ReceiveMessages(ctx context.Context, queueURL string, maxMessages int, waitSeconds int) ([]SQSMessage, error) {
+	out, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
+		QueueUrl:            awssdk.String(queueURL),
+		MaxNumberOfMessages: int32(maxMessages),
+		WaitTimeSeconds:     int32(waitSeconds),
+	})
+	if err != nil {
+		return nil, &domain.ProcessingError{Type: domain.ErrSQS, Message: fmt.Sprintf("receiving messages from %s", queueURL), Wrapped: err}
+	}
+
+	messages := make([]SQSMessage, 0, len(out.Messages))
+	for _, m := range out.Messages {
+		var body, receiptHandle string
+		if m.Body != nil {
+			body = *m.Body
+		}
+		if m.ReceiptHandle != nil {
+			receiptHandle = *m.ReceiptHandle
+		}
+		messages = append(messages, SQSMessage{Body: body, ReceiptHandle: receiptHandle})
+	}
+
+	return messages, nil
+}
+
+// DeleteMessage removes a processed message from queueURL so it is not
+// redelivered.
+func (c *SQSClient) DeleteMessage(ctx context.Context, queueURL string, receiptHandle string) error {
+	_, err := c.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
+		QueueUrl:      awssdk.String(queueURL),
+		ReceiptHandle: awssdk.String(receiptHandle),
+	})
+	if err != nil {
+		return &domain.ProcessingError{Type: domain.ErrSQS, Message: fmt.Sprintf("deleting message from %s", queueURL), Wrapped: err}
+	}
+	return nil
+}
+
+// SendMessage enqueues body onto queueURL.
+func (c *SQSClient) SendMessage(ctx context.Context, queueURL string, body string) error {
+	_, err := c.client.SendMessage(ctx, &sqs.SendMessageInput{
+		QueueUrl:    awssdk.String(queueURL),
+		MessageBody: awssdk.String(body),
+	})
+	if err != nil {
+		return &domain.ProcessingError{Type: domain.ErrSQS, Message: fmt.Sprintf("sending message to %s", queueURL), Wrapped: err}
+	}
+	return nil
+}
diff --git a/internal/infrastructure/aws/sqs_test.go b/internal/infrastructure/aws/sqs_test.go
new file mode 100644
index 0000000..6119423
--- /dev/null
+++ b/internal/infrastructure/aws/sqs_test.go
@@ -0,0 +1,63 @@
+package aws
+
+import (
+	"context"
+	"testing"
+
+	"github.com/aws/aws-sdk-go-v2/service/sqs"
+)
+
+func TestSQSClient_SendReceiveDelete(t *testing.T) {
+	cfg := requireLocalstack(t)
+
+	awsCfg, err := NewSession(cfg)
+	if err != nil {
+		t.Fatalf("NewSession: %v", err)
+	}
+
+	rawClient := sqs.NewFromConfig(awsCfg)
+	ctx := context.Background()
+
+	queueName := "agro-sentinel-worker-test-queue"
+	createOut, err := rawClient.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: &queueName})
+	if err != nil {
+		t.Fatalf("CreateQueue: %v", err)
+	}
+	queueURL := *createOut.QueueUrl
+
+	client := NewSQSClient(awsCfg)
+
+	if err := client.SendMessage(ctx, queueURL, "hello agro sentinel"); err != nil {
+		t.Fatalf("SendMessage: %v", err)
+	}
+
+	var messages []SQSMessage
+	for i := 0; i < 5 && len(messages) == 0; i++ {
+		messages, err = client.ReceiveMessages(ctx, queueURL, 10, 2)
+		if err != nil {
+			t.Fatalf("ReceiveMessages: %v", err)
+		}
+	}
+
+	if len(messages) != 1 {
+		t.Fatalf("expected 1 message, got %d", len(messages))
+	}
+	if messages[0].Body != "hello agro sentinel" {
+		t.Fatalf("unexpected message body: %q", messages[0].Body)
+	}
+	if messages[0].ReceiptHandle == "" {
+		t.Fatal("expected non-empty receipt handle")
+	}
+
+	if err := client.DeleteMessage(ctx, queueURL, messages[0].ReceiptHandle); err != nil {
+		t.Fatalf("DeleteMessage: %v", err)
+	}
+
+	remaining, err := client.ReceiveMessages(ctx, queueURL, 10, 1)
+	if err != nil {
+		t.Fatalf("ReceiveMessages after delete: %v", err)
+	}
+	if len(remaining) != 0 {
+		t.Fatalf("expected queue to be empty after delete, got %d messages", len(remaining))
+	}
+}
diff --git a/internal/jobs/processor.go b/internal/jobs/processor.go
new file mode 100644
index 0000000..fa9bdc5
--- /dev/null
+++ b/internal/jobs/processor.go
@@ -0,0 +1,151 @@
+package jobs
+
+import (
+	"context"
+	"log/slog"
+
+	"agro-sentinel-worker/internal/domain"
+	"agro-sentinel-worker/internal/infrastructure/aws"
+)
+
+// defaultMaxMessages and defaultWaitSeconds are used when ProcessorConfig
+// leaves the corresponding field unset (zero).
+const (
+	defaultMaxMessages = 10
+	defaultWaitSeconds = 20
+	defaultMaxRetries  = 3
+)
+
+// SceneProcessor is the subset of worker.Worker's behavior the processor
+// depends on: running the full processing pipeline for one scene.
+type SceneProcessor interface {
+	ProcessScene(ctx context.Context, produccionID int64, sceneID string) error
+}
+
+// SceneRepository is the subset of database.SceneRepo's behavior the
+// processor needs to enforce idempotency and retry limits before handing a
+// job to the worker.
+type SceneRepository interface {
+	GetByProduccionAndSceneID(ctx context.Context, produccionID int64, sceneID string) (*domain.Scene, error)
+}
+
+// ProcessorConfig configures a Processor's polling and retry behavior.
+type ProcessorConfig struct {
+	QueueURL    string
+	MaxMessages int
+	WaitSeconds int
+	MaxRetries  int
+}
+
+// Processor consumes job messages from SQS and drives them through the
+// worker's scene processing pipeline, honoring idempotency (a COMPLETED
+// scene is skipped) and retry limits (a scene that has exhausted its
+// retries is skipped) before calling the worker.
+type Processor struct {
+	queue  SQSQueue
+	worker SceneProcessor
+	scenes SceneRepository
+	cfg    ProcessorConfig
+	log    *slog.Logger
+}
+
+// New creates a Processor from its dependencies. Zero-valued MaxMessages,
+// WaitSeconds or MaxRetries in cfg fall back to sensible defaults.
+func New(sqsClient SQSQueue, worker SceneProcessor, sceneRepo SceneRepository, cfg ProcessorConfig, logger *slog.Logger) *Processor {
+	if cfg.MaxMessages <= 0 {
+		cfg.MaxMessages = defaultMaxMessages
+	}
+	if cfg.WaitSeconds <= 0 {
+		cfg.WaitSeconds = defaultWaitSeconds
+	}
+	if cfg.MaxRetries <= 0 {
+		cfg.MaxRetries = defaultMaxRetries
+	}
+	if logger == nil {
+		logger = slog.Default()
+	}
+
+	return &Processor{
+		queue:  sqsClient,
+		worker: worker,
+		scenes: sceneRepo,
+		cfg:    cfg,
+		log:    logger,
+	}
+}
+
+// Run long-polls the configured queue in an infinite loop until ctx is
+// canceled, processing each received message and returning nil once the
+// context is done (graceful shutdown).
+func (p *Processor) Run(ctx context.Context) error {
+	for {
+		select {
+		case <-ctx.Done():
+			return nil
+		default:
+		}
+
+		messages, err := p.queue.ReceiveMessages(ctx, p.cfg.QueueURL, p.cfg.MaxMessages, p.cfg.WaitSeconds)
+		if err != nil {
+			if ctx.Err() != nil {
+				return nil
+			}
+			p.log.Error("receiving messages failed", "error", err)
+			continue
+		}
+
+		for _, m := range messages {
+			p.handleMessage(ctx, m)
+		}
+	}
+}
+
+// handleMessage parses, validates and processes a single SQS message,
+// deleting it once handling completes successfully or is determined to be
+// unnecessary (idempotency/retry skip). A message is left undeleted only
+// when ProcessScene itself fails, so SQS redelivers it after the visibility
+// timeout.
+func (p *Processor) handleMessage(ctx context.Context, m aws.SQSMessage) {
+	msg, err := parseJobMessage(m.Body)
+	if err != nil {
+		p.log.Error("dropping unparseable job message", "error", err, "body", m.Body)
+		p.deleteMessage(ctx, m.ReceiptHandle)
+		return
+	}
+
+	log := p.log.With("job_id", msg.JobID, "produccion_id", msg.ProduccionID, "scene_id", msg.SceneID)
+
+	scene, err := p.scenes.GetByProduccionAndSceneID(ctx, msg.ProduccionID, msg.SceneID)
+	if err != nil {
+		log.Error("failed to look up scene, leaving message for redelivery", "error", err)
+		return
+	}
+
+	if scene != nil {
+		if scene.Status == domain.StatusCompleted {
+			log.Info("scene already completed, skipping")
+			p.deleteMessage(ctx, m.ReceiptHandle)
+			return
+		}
+
+		if scene.Status == domain.StatusFailed && !scene.CanRetry(p.cfg.MaxRetries) {
+			log.Warn("scene has exhausted retries, skipping", "retry_count", scene.RetryCount, "max_retries", p.cfg.MaxRetries)
+			p.deleteMessage(ctx, m.ReceiptHandle)
+			return
+		}
+	}
+
+	if err := p.worker.ProcessScene(ctx, msg.ProduccionID, msg.SceneID); err != nil {
+		log.Error("scene processing failed, leaving message for redelivery", "error", err)
+		return
+	}
+
+	log.Info("scene processed successfully")
+	p.deleteMessage(ctx, m.ReceiptHandle)
+}
+
+func (p *Processor) deleteMessage(ctx context.Context, receiptHandle string) {
+	if err := p.queue.DeleteMessage(ctx, p.cfg.QueueURL, receiptHandle); err != nil {
+		p.log.Error("failed to delete message", "error", err)
+	}
+}
diff --git a/internal/jobs/processor_test.go b/internal/jobs/processor_test.go
new file mode 100644
index 0000000..1955cbf
--- /dev/null
+++ b/internal/jobs/processor_test.go
@@ -0,0 +1,242 @@
+package jobs
+
+import (
+	"context"
+	"encoding/json"
+	"io"
+	"log/slog"
+	"sync"
+	"testing"
+	"time"
+
+	"agro-sentinel-worker/internal/domain"
+	"agro-sentinel-worker/internal/infrastructure/aws"
+)
+
+// mockQueue is an in-memory SQSQueue: ReceiveMessages returns the queued
+// messages once each (simulating consumption), then blocks until ctx is
+// canceled (simulating long-polling an empty queue) so Run's loop doesn't
+// spin forever in tests.
+type mockQueue struct {
+	mu       sync.Mutex
+	messages []aws.SQSMessage
+	deleted  []string
+	sent     []string
+}
+
+func (q *mockQueue) ReceiveMessages(ctx context.Context, queueURL string, maxMessages int, waitSeconds int) ([]aws.SQSMessage, error) {
+	q.mu.Lock()
+	if len(q.messages) > 0 {
+		msgs := q.messages
+		q.messages = nil
+		q.mu.Unlock()
+		return msgs, nil
+	}
+	q.mu.Unlock()
+
+	// Simulate long polling on an empty queue until context cancellation.
+	<-ctx.Done()
+	return nil, ctx.Err()
+}
+
+func (q *mockQueue) DeleteMessage(ctx context.Context, queueURL string, receiptHandle string) error {
+	q.mu.Lock()
+	defer q.mu.Unlock()
+	q.deleted = append(q.deleted, receiptHandle)
+	return nil
+}
+
+func (q *mockQueue) SendMessage(ctx context.Context, queueURL string, body string) error {
+	q.mu.Lock()
+	defer q.mu.Unlock()
+	q.sent = append(q.sent, body)
+	return nil
+}
+
+func (q *mockQueue) wasDeleted(receiptHandle string) bool {
+	q.mu.Lock()
+	defer q.mu.Unlock()
+	for _, d := range q.deleted {
+		if d == receiptHandle {
+			return true
+		}
+	}
+	return false
+}
+
+// mockWorker records ProcessScene calls and returns a configured error.
+type mockWorker struct {
+	mu    sync.Mutex
+	calls []string
+	err   error
+}
+
+func (w *mockWorker) ProcessScene(ctx context.Context, produccionID int64, sceneID string) error {
+	w.mu.Lock()
+	defer w.mu.Unlock()
+	w.calls = append(w.calls, sceneID)
+	return w.err
+}
+
+func (w *mockWorker) callCount() int {
+	w.mu.Lock()
+	defer w.mu.Unlock()
+	return len(w.calls)
+}
+
+// mockSceneRepo returns a fixed scene (or nil) for lookups.
+type mockSceneRepo struct {
+	scene *domain.Scene
+	err   error
+}
+
+func (r *mockSceneRepo) GetByProduccionAndSceneID(ctx context.Context, produccionID int64, sceneID string) (*domain.Scene, error) {
+	return r.scene, r.err
+}
+
+func testLogger() *slog.Logger {
+	return slog.New(slog.NewTextHandler(io.Discard, nil))
+}
+
+func jobBody(t *testing.T, msg JobMessage) string {
+	t.Helper()
+	b, err := json.Marshal(msg)
+	if err != nil {
+		t.Fatalf("marshaling job message: %v", err)
+	}
+	return string(b)
+}
+
+// runUntilIdle runs p.Run in a goroutine, waits for the queue to have
+// nothing left to deliver from its initial batch, then cancels and waits
+// for Run to return.
+func runUntilIdle(t *testing.T, p *Processor, w *mockWorker, wantCalls int) {
+	t.Helper()
+	ctx, cancel := context.WithCancel(context.Background())
+
+	done := make(chan error, 1)
+	go func() { done <- p.Run(ctx) }()
+
+	deadline := time.Now().Add(2 * time.Second)
+	for time.Now().Before(deadline) {
+		if w.callCount() >= wantCalls {
+			break
+		}
+		time.Sleep(5 * time.Millisecond)
+	}
+
+	cancel()
+	select {
+	case err := <-done:
+		if err != nil {
+			t.Fatalf("Run returned error: %v", err)
+		}
+	case <-time.After(2 * time.Second):
+		t.Fatal("Run did not return after context cancellation")
+	}
+}
+
+func TestProcessor_ReceivedMessage_ProcessedAndDeleted(t *testing.T) {
+	msg := JobMessage{JobID: "job-1", ProduccionID: 1234, SceneID: "S2A_1"}
+	queue := &mockQueue{messages: []aws.SQSMessage{{Body: jobBody(t, msg), ReceiptHandle: "rh-1"}}}
+	worker := &mockWorker{}
+	repo := &mockSceneRepo{scene: &domain.Scene{ID: 1, Status: domain.StatusPending}}
+
+	p := New(queue, worker, repo, ProcessorConfig{QueueURL: "q", MaxRetries: 3}, testLogger())
+	runUntilIdle(t, p, worker, 1)
+
+	if worker.callCount() != 1 {
+		t.Fatalf("expected ProcessScene called once, got %d", worker.callCount())
+	}
+	if !queue.wasDeleted("rh-1") {
+		t.Fatal("expected message to be deleted after successful processing")
+	}
+}
+
+func TestProcessor_AlreadyCompleted_SkipsAndDeletes(t *testing.T) {
+	msg := JobMessage{JobID: "job-2", ProduccionID: 1234, SceneID: "S2A_2"}
+	queue := &mockQueue{messages: []aws.SQSMessage{{Body: jobBody(t, msg), ReceiptHandle: "rh-2"}}}
+	worker := &mockWorker{}
+	repo := &mockSceneRepo{scene: &domain.Scene{ID: 2, Status: domain.StatusCompleted}}
+
+	p := New(queue, worker, repo, ProcessorConfig{QueueURL: "q", MaxRetries: 3}, testLogger())
+
+	ctx, cancel := context.WithCancel(context.Background())
+	done := make(chan error, 1)
+	go func() { done <- p.Run(ctx) }()
+
+	deadline := time.Now().Add(2 * time.Second)
+	for time.Now().Before(deadline) && !queue.wasDeleted("rh-2") {
+		time.Sleep(5 * time.Millisecond)
+	}
+	cancel()
+	<-done
+
+	if worker.callCount() != 0 {
+		t.Fatalf("expected ProcessScene NOT called for completed scene, got %d calls", worker.callCount())
+	}
+	if !queue.wasDeleted("rh-2") {
+		t.Fatal("expected message to be deleted for already-completed scene")
+	}
+}
+
+func TestProcessor_ProcessSceneFails_MessageNotDeleted(t *testing.T) {
+	msg := JobMessage{JobID: "job-3", ProduccionID: 1234, SceneID: "S2A_3"}
+	queue := &mockQueue{messages: []aws.SQSMessage{{Body: jobBody(t, msg), ReceiptHandle: "rh-3"}}}
+	worker := &mockWorker{err: &domain.ProcessingError{Type: domain.ErrGDAL, Message: "boom"}}
+	repo := &mockSceneRepo{scene: &domain.Scene{ID: 3, Status: domain.StatusPending}}
+
+	p := New(queue, worker, repo, ProcessorConfig{QueueURL: "q", MaxRetries: 3}, testLogger())
+	runUntilIdle(t, p, worker, 1)
+
+	if worker.callCount() != 1 {
+		t.Fatalf("expected ProcessScene called once, got %d", worker.callCount())
+	}
+	if queue.wasDeleted("rh-3") {
+		t.Fatal("expected message NOT to be deleted after failed processing (so SQS redelivers)")
+	}
+}
+
+func TestProcessor_FailedWithRetriesRemaining_Reprocesses(t *testing.T) {
+	msg := JobMessage{JobID: "job-4", ProduccionID: 1234, SceneID: "S2A_4"}
+	queue := &mockQueue{messages: []aws.SQSMessage{{Body: jobBody(t, msg), ReceiptHandle: "rh-4"}}}
+	worker := &mockWorker{}
+	repo := &mockSceneRepo{scene: &domain.Scene{ID: 4, Status: domain.StatusFailed, RetryCount: 1}}
+
+	p := New(queue, worker, repo, ProcessorConfig{QueueURL: "q", MaxRetries: 3}, testLogger())
+	runUntilIdle(t, p, worker, 1)
+
+	if worker.callCount() != 1 {
+		t.Fatalf("expected ProcessScene called for retryable FAILED scene, got %d calls", worker.callCount())
+	}
+	if !queue.wasDeleted("rh-4") {
+		t.Fatal("expected message to be deleted after successful retry")
+	}
+}
+
+func TestProcessor_FailedExhaustedRetries_SkipsAndDeletes(t *testing.T) {
+	msg := JobMessage{JobID: "job-5", ProduccionID: 1234, SceneID: "S2A_5"}
+	queue := &mockQueue{messages: []aws.SQSMessage{{Body: jobBody(t, msg), ReceiptHandle: "rh-5"}}}
+	worker := &mockWorker{}
+	repo := &mockSceneRepo{scene: &domain.Scene{ID: 5, Status: domain.StatusFailed, RetryCount: 3}}
+
+	p := New(queue, worker, repo, ProcessorConfig{QueueURL: "q", MaxRetries: 3}, testLogger())
+
+	ctx, cancel := context.WithCancel(context.Background())
+	done := make(chan error, 1)
+	go func() { done <- p.Run(ctx) }()
+
+	deadline := time.Now().Add(2 * time.Second)
+	for time.Now().Before(deadline) && !queue.wasDeleted("rh-5") {
+		time.Sleep(5 * time.Millisecond)
+	}
+	cancel()
+	<-done
+
+	if worker.callCount() != 0 {
+		t.Fatalf("expected ProcessScene NOT called for retry-exhausted scene, got %d calls", worker.callCount())
+	}
+	if !queue.wasDeleted("rh-5") {
+		t.Fatal("expected message to be deleted for retry-exhausted scene")
+	}
+}
diff --git a/internal/jobs/queue.go b/internal/jobs/queue.go
new file mode 100644
index 0000000..2f447da
--- /dev/null
+++ b/internal/jobs/queue.go
@@ -0,0 +1,53 @@
+// Package jobs implements the SQS-driven job queue that feeds scenes into
+// the worker's processing pipeline: receiving job messages, checking
+// idempotency and retry limits, invoking the worker, and acknowledging
+// (deleting) messages once handled.
+package jobs
+
+import (
+	"context"
+	"encoding/json"
+	"fmt"
+
+	"agro-sentinel-worker/internal/domain"
+	"agro-sentinel-worker/internal/infrastructure/aws"
+)
+
+// JobMessage is the JSON payload carried by each SQS message: the scene to
+// process, identified by its production and scene id.
+type JobMessage struct {
+	JobID        string `json:"job_id"`
+	ProduccionID int64  `json:"produccion_id"`
+	SceneID      string `json:"scene_id"`
+}
+
+// SQSQueue is the subset of aws.SQSClient's behavior the processor and
+// queue helpers depend on.
+type SQSQueue interface {
+	ReceiveMessages(ctx context.Context, queueURL string, maxMessages int, waitSeconds int) ([]aws.SQSMessage, error)
+	DeleteMessage(ctx context.Context, queueURL string, receiptHandle string) error
+	SendMessage(ctx context.Context, queueURL string, body string) error
+}
+
+// Enqueue marshals msg as JSON and sends it to queueURL.
+func Enqueue(ctx context.Context, queue SQSQueue, queueURL string, msg JobMessage) error {
+	body, err := json.Marshal(msg)
+	if err != nil {
+		return &domain.ProcessingError{Type: domain.ErrValidation, Message: "marshaling job message", Wrapped: err}
+	}
+
+	if err := queue.SendMessage(ctx, queueURL, string(body)); err != nil {
+		return err
+	}
+
+	return nil
+}
+
+// parseJobMessage unmarshals an SQS message body into a JobMessage.
+func parseJobMessage(body string) (JobMessage, error) {
+	var msg JobMessage
+	if err := json.Unmarshal([]byte(body), &msg); err != nil {
+		return JobMessage{}, &domain.ProcessingError{Type: domain.ErrValidation, Message: fmt.Sprintf("parsing job message: %q", body), Wrapped: err}
+	}
+	return msg, nil
+}
