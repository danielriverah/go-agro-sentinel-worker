package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/infrastructure/aws"
	"agro-sentinel-worker/internal/infrastructure/database"
	"agro-sentinel-worker/internal/logger"
	"agro-sentinel-worker/internal/sync"
)

func main() {
	cfgPath := "configs/config.yaml"
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		cfgPath = p
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	l := logger.New(cfg.Logging)
	l.Info("sync starting", "name", cfg.App.Name, "interval_minutes", cfg.Sync.IntervalMinutes)

	db, err := database.NewConnection(cfg.MySQL)
	if err != nil {
		l.Error("connecting to mysql failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := database.RunMigrations(db); err != nil {
		l.Error("running migrations failed", "error", err)
		os.Exit(1)
	}

	awsCfg, err := aws.NewSession(cfg.AWS)
	if err != nil {
		l.Error("creating aws session failed", "error", err)
		os.Exit(1)
	}

	dynamoClient := aws.NewDynamoDBClient(awsCfg)
	prodRepo := database.NewProductionRepo(db)
	sceneRepo := database.NewSceneRepo(db)
	polygonRepo := database.NewPolygonRepo(db, sync.CalculateBBoxFromWKT)

	svc := sync.New(
		dynamoClient,
		prodRepo,
		sceneRepo,
		polygonRepo,
		cfg.Sync,
		cfg.Sentinel,
		cfg.DynamoDB.TableProducciones,
		cfg.DynamoDB.TableEscenas,
		l,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := svc.RunLoop(ctx); err != nil && ctx.Err() == nil {
		l.Error("sync loop exited with error", "error", err)
		os.Exit(1)
	}

	l.Info("sync stopped")
}
