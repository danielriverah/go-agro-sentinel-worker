package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/daemon"
	"agro-sentinel-worker/internal/health"
	"agro-sentinel-worker/internal/infrastructure/aws"
	"agro-sentinel-worker/internal/infrastructure/database"
	"agro-sentinel-worker/internal/logger"
	"agro-sentinel-worker/internal/retry"
	"agro-sentinel-worker/internal/sync"
)

func main() {
	scenesOnly := flag.Bool("scenes-only", false, "Sincroniza solo escenas (fase 2) sin tocar producciones")
	autoMode := flag.Bool("auto", false, "daemon scheduler: corre solo en el horario configurado")
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
	l.Info("sync starting", "name", cfg.App.Name, "interval_minutes", cfg.Sync.IntervalMinutes)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Connect to MySQL with retry.
	db, err := connectMySQLWithRetry(ctx, cfg.MySQL, l)
	if err != nil {
		l.Error("could not connect to mysql — sync will not start", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// RunMigrations is a no-op; schema is managed by the DBA.
	_ = database.RunMigrations(db)

	// Create AWS session.
	awsCfg, err := aws.NewSession(cfg.AWS)
	if err != nil {
		l.Error("creating aws session failed", "error", err)
		os.Exit(1)
	}

	// Create DynamoDB session.
	dynamoCfg, err := aws.NewDynamoDBSession(cfg.DynamoDB)
	if err != nil {
		l.Error("creating dynamodb session failed", "error", err)
		os.Exit(1)
	}

	dynamoEndpoint := cfg.DynamoDB.Endpoint
	if dynamoEndpoint == "" {
		dynamoEndpoint = "aws:" + cfg.DynamoDB.Region
	}
	l.Info("dynamodb target", "endpoint", dynamoEndpoint, "table_producciones", cfg.DynamoDB.TableProducciones)

	s3Endpoint := os.Getenv("AWS_ENDPOINT_URL")
	if s3Endpoint == "" {
		s3Endpoint = "aws:real"
	}
	l.Info("s3 target", "endpoint", s3Endpoint, "bucket", cfg.S3.Bucket)

	// Validate all dependencies before continuing.
	dynamoClient := aws.NewDynamoDBClient(dynamoCfg)
	s3Client := aws.NewS3Client(awsCfg)

	if err := health.CheckDynamoDB(ctx, dynamoClient, cfg.DynamoDB.TableProducciones); err != nil {
		l.Error("dynamodb health check failed", "error", err)
		os.Exit(1)
	}
	l.Info("dynamodb health check passed")

	if err := health.CheckS3(ctx, s3Client, cfg.S3.Bucket); err != nil {
		l.Error("s3 health check failed", "error", err)
		os.Exit(1)
	}
	l.Info("s3 health check passed")

	prodRepo := database.NewProductionRepo(db)
	sceneRepo := database.NewSceneRepo(db)
	polygonRepo := database.NewPolygonRepo(db, sync.CalculateBBoxFromWKT)
	fileRepo := database.NewFileRepo(db)
	iaResultRepo := database.NewIAResultRepository(db)

	svc := sync.New(
		dynamoClient,
		prodRepo,
		sceneRepo,
		polygonRepo,
		s3Client,
		fileRepo,
		iaResultRepo,
		cfg.Sync,
		cfg.Sentinel,
		cfg.DynamoDB.TableProducciones,
		cfg.DynamoDB.TableEscenas,
		cfg.S3.Bucket,
		l,
	)

	if *scenesOnly {
		l.Info("modo: sync escenas solamente")
		if err := svc.RunScenesOnly(ctx); err != nil {
			l.Error("sync escenas failed", "error", err)
			os.Exit(1)
		}
		l.Info("sync escenas completado")
		return
	}

	// --auto: daemon scheduler that runs a full sync only at the configured
	// schedule. It does not run on startup. Skips a run if a previous sync
	// process is still running (file lock).
	if *autoMode {
		scheduleHours := []int{4, 22}
		tz := daemon.MustLoadLocation("America/Mexico_City")
		lockPath := "/tmp/sync-auto.lock"

		runOnce := func() {
			lock, err := daemon.Acquire(lockPath)
			if err != nil {
				l.Error("acquiring sync lock failed", "error", err)
				return
			}
			if lock == nil {
				l.Info("sync run skipped — previous run still in progress")
				return
			}
			defer lock.Release()

			l.Info("sync auto run starting")
			if err := svc.RunOnce(ctx); err != nil && ctx.Err() == nil {
				l.Error("sync auto run failed", "error", err)
			} else {
				l.Info("sync auto run completed")
			}
		}

		for {
			next := daemon.NextSchedule(scheduleHours, tz)
			l.Info("next sync scheduled", "in", next.Round(time.Minute).String())

			select {
			case <-ctx.Done():
				l.Info("sync daemon shutting down")
				return
			case <-time.After(next):
				runOnce()
			}
		}
	}

	if err := svc.RunLoop(ctx); err != nil && ctx.Err() == nil {
		l.Error("sync loop exited with error", "error", err)
		os.Exit(1)
	}

	l.Info("sync stopped gracefully")
}

// connectMySQLWithRetry connects to MySQL with exponential backoff retry logic.
func connectMySQLWithRetry(ctx context.Context, cfg config.MySQLConfig, l interface {
	Info(string, ...any)
	Error(string, ...any)
}) (*sql.DB, error) {
	cfg2 := retry.SyncConfig
	attempt := 0
	var db *sql.DB

	err := retry.WithBackoff(ctx, cfg2, func() error {
		attempt++
		var connErr error
		db, connErr = database.NewConnection(cfg)
		if connErr == nil {
			l.Info("mysql connected", "attempt", attempt)
			return nil
		}
		if attempt < cfg2.MaxAttempts {
			l.Error("mysql connection failed, retrying",
				"attempt", attempt,
				"max", cfg2.MaxAttempts,
				"error", connErr)
		}
		return connErr
	})

	if err != nil {
		return nil, err
	}

	return db, nil
}
