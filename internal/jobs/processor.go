package jobs

import (
	"context"
	"log/slog"

	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/infrastructure/aws"
)

// defaultMaxMessages and defaultWaitSeconds are used when ProcessorConfig
// leaves the corresponding field unset (zero).
const (
	defaultMaxMessages = 10
	defaultWaitSeconds = 20
	defaultMaxRetries  = 3
)

// SceneProcessor is the subset of worker.Worker's behavior the processor
// depends on: running the full processing pipeline for one scene.
type SceneProcessor interface {
	ProcessScene(ctx context.Context, produccionID int64, sceneID string) error
}

// SceneRepository is the subset of database.SceneRepo's behavior the
// processor needs to enforce idempotency and retry limits before handing a
// job to the worker.
type SceneRepository interface {
	GetByProduccionAndSceneID(ctx context.Context, produccionID int64, sceneID string) (*domain.Scene, error)
}

// ProcessorConfig configures a Processor's polling and retry behavior.
type ProcessorConfig struct {
	QueueURL    string
	MaxMessages int
	WaitSeconds int
	MaxRetries  int
}

// Processor consumes job messages from SQS and drives them through the
// worker's scene processing pipeline, honoring idempotency (a COMPLETED
// scene is skipped) and retry limits (a scene that has exhausted its
// retries is skipped) before calling the worker.
type Processor struct {
	queue  SQSQueue
	worker SceneProcessor
	scenes SceneRepository
	cfg    ProcessorConfig
	log    *slog.Logger
}

// New creates a Processor from its dependencies. Zero-valued MaxMessages,
// WaitSeconds or MaxRetries in cfg fall back to sensible defaults.
func New(sqsClient SQSQueue, worker SceneProcessor, sceneRepo SceneRepository, cfg ProcessorConfig, logger *slog.Logger) *Processor {
	if cfg.MaxMessages <= 0 {
		cfg.MaxMessages = defaultMaxMessages
	}
	if cfg.WaitSeconds <= 0 {
		cfg.WaitSeconds = defaultWaitSeconds
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = defaultMaxRetries
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Processor{
		queue:  sqsClient,
		worker: worker,
		scenes: sceneRepo,
		cfg:    cfg,
		log:    logger,
	}
}

// Run long-polls the configured queue in an infinite loop until ctx is
// canceled, processing each received message and returning nil once the
// context is done (graceful shutdown).
func (p *Processor) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		messages, err := p.queue.ReceiveMessages(ctx, p.cfg.QueueURL, p.cfg.MaxMessages, p.cfg.WaitSeconds)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			p.log.Error("receiving messages failed", "error", err)
			continue
		}

		for _, m := range messages {
			p.handleMessage(ctx, m)
		}
	}
}

// handleMessage parses, validates and processes a single SQS message,
// deleting it once handling completes successfully or is determined to be
// unnecessary (idempotency/retry skip). A message is left undeleted only
// when ProcessScene itself fails, so SQS redelivers it after the visibility
// timeout.
func (p *Processor) handleMessage(ctx context.Context, m aws.SQSMessage) {
	msg, err := parseJobMessage(m.Body)
	if err != nil {
		p.log.Error("dropping unparseable job message", "error", err, "body", m.Body)
		p.deleteMessage(ctx, m.ReceiptHandle)
		return
	}

	log := p.log.With("job_id", msg.JobID, "produccion_id", msg.ProduccionID, "scene_id", msg.SceneID)

	scene, err := p.scenes.GetByProduccionAndSceneID(ctx, msg.ProduccionID, msg.SceneID)
	if err != nil {
		log.Error("failed to look up scene, leaving message for redelivery", "error", err)
		return
	}

	if scene != nil {
		if scene.Status == domain.StatusCompleted {
			log.Info("scene already completed, skipping")
			p.deleteMessage(ctx, m.ReceiptHandle)
			return
		}

		if scene.Status == domain.StatusFailed && !scene.CanRetry(p.cfg.MaxRetries) {
			log.Warn("scene has exhausted retries, skipping", "retry_count", scene.RetryCount, "max_retries", p.cfg.MaxRetries)
			p.deleteMessage(ctx, m.ReceiptHandle)
			return
		}
	}

	if err := p.worker.ProcessScene(ctx, msg.ProduccionID, msg.SceneID); err != nil {
		log.Error("scene processing failed, leaving message for redelivery", "error", err)
		return
	}

	log.Info("scene processed successfully")
	p.deleteMessage(ctx, m.ReceiptHandle)
}

func (p *Processor) deleteMessage(ctx context.Context, receiptHandle string) {
	if err := p.queue.DeleteMessage(ctx, p.cfg.QueueURL, receiptHandle); err != nil {
		p.log.Error("failed to delete message", "error", err)
	}
}
