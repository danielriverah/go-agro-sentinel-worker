package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"agro-sentinel-worker/internal/config"
	apphttp "agro-sentinel-worker/internal/http"
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

	db, err := database.NewConnection(cfg.MySQL)
	if err != nil {
		l.Error("connecting to mysql failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	awsCfg, err := aws.NewSession(cfg.AWS)
	if err != nil {
		l.Error("creating aws session failed", "error", err)
		os.Exit(1)
	}

	prodRepo := database.NewProductionRepo(db)
	sceneRepo := database.NewSceneRepo(db)
	fileRepo := database.NewFileRepo(db)
	s3Client := aws.NewS3Client(awsCfg)

	var syncer apphttp.Syncer
	dynamoClient := aws.NewDynamoDBClient(awsCfg)
	polygonRepo := database.NewPolygonRepo(db, sync.CalculateBBoxFromWKT)
	syncer = sync.New(
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

	h := &apphttp.Handlers{
		Productions: prodRepo,
		Scenes:      sceneRepo,
		Files:       fileRepo,
		S3:          s3Client,
		S3Bucket:    cfg.S3.Bucket,
		Sync:        syncer,
	}

	router := apphttp.NewRouter(l, h)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	l.Info("API server starting", "addr", addr)

	if err := http.ListenAndServe(addr, router); err != nil {
		l.Error("server failed", "error", err)
		os.Exit(1)
	}
}
