package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	awssdk "github.com/aws/aws-sdk-go-v2/config"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/infrastructure/aws"
	"agro-sentinel-worker/internal/infrastructure/database"
	"agro-sentinel-worker/internal/infrastructure/gdal"
	"agro-sentinel-worker/internal/infrastructure/ia"
	"agro-sentinel-worker/internal/jobs"
	"agro-sentinel-worker/internal/logger"
	"agro-sentinel-worker/internal/worker"
)

// stacBandResolver is a placeholder worker.BandResolver: full STAC/COG
// discovery is not implemented yet. It fails clearly rather than silently
// producing an empty band set.
type stacBandResolver struct{}

func (stacBandResolver) ResolveBands(ctx context.Context, produccionID int64, sceneID string) ([]domain.BandInfo, string, error) {
	return nil, "", &domain.ProcessingError{
		Type:    domain.ErrValidation,
		Message: "band resolution (STAC/COG discovery) is not wired up yet",
	}
}

func main() {
	produccionID := flag.Int64("production", 0, "produccion_id to process")
	sceneID := flag.String("scene", "", "scene_id to process")
	flag.Parse()

	cfgPath := "configs/config.yaml"
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		cfgPath = p
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	l := logger.New(cfg.Logging)
	l.Info("worker starting", "name", cfg.App.Name)

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
	sceneRepo := database.NewSceneRepo(db)

	deps := worker.WorkerDeps{
		Productions: database.NewProductionRepo(db),
		Scenes:      sceneRepo,
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

	// -production/-scene run a single scene once and exit, for manual
	// invocation and debugging. With no flags, the worker runs the
	// SQS-driven job queue loop until it receives a termination signal.
	if *produccionID != 0 && *sceneID != "" {
		if err := w.ProcessScene(ctx, *produccionID, *sceneID); err != nil {
			l.Error("scene processing failed", "produccion_id", *produccionID, "scene_id", *sceneID, "error", err)
			os.Exit(1)
		}
		l.Info("scene processing completed", "produccion_id", *produccionID, "scene_id", *sceneID)
		return
	}

	if cfg.SQS.QueueURL == "" {
		log.Fatal("sqs.queue_url is not configured; set it or pass -production/-scene for a single-scene run")
	}

	sqsClient := aws.NewSQSClient(awsCfg)
	processor := jobs.New(sqsClient, w, sceneRepo, jobs.ProcessorConfig{
		QueueURL:    cfg.SQS.QueueURL,
		MaxMessages: cfg.SQS.MaxMessages,
		WaitSeconds: cfg.SQS.WaitSeconds,
		MaxRetries:  cfg.SQS.MaxRetries,
	}, l)

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	l.Info("worker listening for jobs", "queue_url", cfg.SQS.QueueURL)
	if err := processor.Run(runCtx); err != nil {
		l.Error("job processor stopped with error", "error", err)
		os.Exit(1)
	}

	l.Info("worker shut down gracefully")
}
