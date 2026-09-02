package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	awssdk "github.com/aws/aws-sdk-go-v2/config"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/infrastructure/aws"
	"agro-sentinel-worker/internal/infrastructure/database"
	"agro-sentinel-worker/internal/infrastructure/gdal"
	"agro-sentinel-worker/internal/infrastructure/ia"
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

	if *produccionID == 0 || *sceneID == "" {
		fmt.Println("Usage: go run ./cmd/worker -production 1234 -scene S2A_xxx")
		os.Exit(1)
	}

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

	deps := worker.WorkerDeps{
		Productions: database.NewProductionRepo(db),
		Scenes:      database.NewSceneRepo(db),
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

	if err := w.ProcessScene(ctx, *produccionID, *sceneID); err != nil {
		l.Error("scene processing failed", "produccion_id", *produccionID, "scene_id", *sceneID, "error", err)
		os.Exit(1)
	}

	l.Info("scene processing completed", "produccion_id", *produccionID, "scene_id", *sceneID)
}
