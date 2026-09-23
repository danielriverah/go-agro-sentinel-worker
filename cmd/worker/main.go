package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/daemon"
	"agro-sentinel-worker/internal/health"
	"agro-sentinel-worker/internal/ia"
	"agro-sentinel-worker/internal/infrastructure/aws"
	"agro-sentinel-worker/internal/infrastructure/bands"
	"agro-sentinel-worker/internal/infrastructure/database"
	"agro-sentinel-worker/internal/infrastructure/gdal"
	"agro-sentinel-worker/internal/jobs"
	"agro-sentinel-worker/internal/logger"
	"agro-sentinel-worker/internal/retry"
	"agro-sentinel-worker/internal/worker"
)

// lockReporter implements worker.ProgressReporter, writing WorkerStatus JSON
// next to the daemon lock file after every event.
type lockReporter struct {
	lock   *daemon.Lock
	status daemon.WorkerStatus
}

func (r *lockReporter) SetTotal(total int) {
	r.status.ScenesTotal = total
	r.lock.WriteStatus(r.status)
}

func (r *lockReporter) SetPhase(phase string) {
	r.status.Phase = phase
	r.lock.WriteStatus(r.status)
}

func (r *lockReporter) SceneStarted(scene string, produccionID int64) {
	r.status.CurrentScene = scene
	r.status.CurrentProduccionID = produccionID
	r.lock.WriteStatus(r.status)
}

func (r *lockReporter) SceneCompleted(scene string, _ int64) {
	r.status.ScenesDone++
	r.status.CurrentScene = scene
	r.lock.WriteStatus(r.status)
}

func (r *lockReporter) SceneFailed(scene string, _ int64) {
	r.status.ScenesFailed++
	r.status.CurrentScene = scene
	r.lock.WriteStatus(r.status)
}

func (r *lockReporter) SetNextSchedule(t time.Time) {
	r.status.NextScheduleAt = &t
	r.lock.WriteStatus(r.status)
}

func main() {
	produccionID := flag.Int64("production", 0, "produccion_id to process")
	sceneID := flag.String("scene", "", "scene_id to process")
	regenMode := flag.Bool("regen", false, "regenerate params+ia_req for all COMPLETED scenes of --production (no images rebuilt)")
	autoMode := flag.Bool("auto", false, "global scheduler: process all pending scenes in chronological order; runs as daemon on schedule")
	runAllMode := flag.Bool("run-all", false, "process all pending scenes once (same as auto but without the scheduler daemon)")
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Validate dependencies with timeout.
	validateCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Connect to MySQL with retry.
	db, err := connectMySQLWithRetry(validateCtx, cfg.MySQL, l)
	if err != nil {
		l.Error("could not connect to mysql — worker will not start", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create AWS session for SQS (general AWS credentials).
	awsCfg, err := aws.NewSession(cfg.AWS)
	if err != nil {
		l.Error("creating aws session failed", "error", err)
		os.Exit(1)
	}

	// Create S3 session (with S3-specific credentials from .env).
	s3Session, err := aws.NewS3Session(cfg.S3, cfg.AWS)
	if err != nil {
		l.Error("creating s3 session failed", "error", err)
		os.Exit(1)
	}

	executor := gdal.NewExecutor(cfg.GDAL.TimeoutSeconds)
	s3Client := aws.NewS3Client(s3Session)

	// Validate S3 bucket access.
	if err := health.CheckS3(validateCtx, s3Client, cfg.S3.Bucket); err != nil {
		l.Error("s3 health check failed", "error", err)
		os.Exit(1)
	}
	l.Info("s3 health check passed")

	sceneRepo := database.NewSceneRepo(db)
	fileRepo := database.NewFileRepo(db)
	bandResolver := bands.New(sceneRepo)
	iaResultRepo := database.NewIAResultRepository(db)

	// IA Analyzer — optional; only wired when Bedrock credentials are present.
	var iaAnalyzer worker.IAAnalyzer
	hasSecret := cfg.IA.Bedrock.SecretAccessKey != "" || cfg.IA.Bedrock.APIKey != ""
	if cfg.IA.Bedrock.AccessKeyID != "" && hasSecret && cfg.IA.Bedrock.ModelID != "" {
		bedrockClient, bedrockErr := aws.NewBedrockClient(cfg.IA.Bedrock)
		if bedrockErr != nil {
			l.Warn("Bedrock not configured — ia_auto disabled", "error", bedrockErr)
		} else {
			iaAnalyzer = ia.New(ia.Deps{
				S3:          s3Client,
				S3Up:        s3Client,
				Files:       fileRepo,
				Bedrock:     bedrockClient,
				Results:     iaResultRepo,
				Scenes:      sceneRepo,
				SceneInfo:   sceneRepo,
				Productions: database.NewProductionRepo(db),
				Alertas:     database.NewAlertaRepo(db),
				Bucket:      cfg.S3.Bucket,
				TempDir:     cfg.Processing.TempDir,
				Logger:      l,
			})
			l.Info("Bedrock IA analyzer configured for ia_auto", "model", cfg.IA.Bedrock.ModelID)
		}
	}

	deps := worker.WorkerDeps{
		Productions: database.NewProductionRepo(db),
		Scenes:      sceneRepo,
		Files:       fileRepo,
		IAResults:   iaResultRepo,
		IAAnalyzer:  iaAnalyzer,
		S3:          s3Client,
		Executor:    executor,
		Bands:       bandResolver,
		Processing:  cfg.Processing,
		Sentinel:    cfg.Sentinel,
		S3Config:    cfg.S3,
		Logger:      l,
		ShouldStop:  daemon.ConsumeStopTrigger,
	}

	w := worker.New(deps)

	// --run-all: process all pending scenes once and exit (no scheduler).
	if *runAllMode {
		if daemon.CleanStaleGlobalLock() {
			l.Info("cleaned stale global lock")
		}
		if n := daemon.CleanStaleProductionLocks(); n > 0 {
			l.Info("cleaned stale production locks", "count", n)
		}
		lock, err := daemon.AcquireGlobal()
		if err != nil {
			l.Error("cannot start: another run is in progress", "error", err)
			os.Exit(1)
		}
		if lock == nil {
			// Read the global lock to show which PID is holding it.
			if data, readErr := os.ReadFile("/tmp/worker-global.lock"); readErr == nil {
				l.Error("cannot start: global lock held", "pid_in_lock", strings.TrimSpace(string(data)), "current_pid", os.Getpid())
			} else {
				l.Error("cannot start: another run is in progress (lock nil, no lock file)")
			}
			os.Exit(1)
		}
		now := time.Now()
		reporter := &lockReporter{
			lock: lock,
			status: daemon.WorkerStatus{
				Mode:      "manual-all",
				PID:       os.Getpid(),
				StartedAt: now,
				Phase:     daemon.PhaseProcessing,
			},
		}
		lock.WriteStatus(reporter.status)
		w.SetProgress(reporter)

		runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		l.Info("run-all starting")
		runErr := w.ProcessAllPending(runCtx)
		lock.Release()
		if runErr != nil {
			l.Error("run-all failed", "error", runErr)
			os.Exit(1)
		}
		l.Info("run-all completed")
		return
	}

	// --auto: daemon scheduler driven by cfg.Worker.Schedule (cron expression).
	// Falls back to "0 1 * * *" (01:00 America/Mexico_City) when not configured.
	// Acquires the global lock before each run so that any concurrent
	// --production / --scene / --regen command is rejected while --auto is active.
	if *autoMode {
		runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		tzName := cfg.Worker.Timezone
		if tzName == "" {
			tzName = "America/Mexico_City"
		}
		loc, err := time.LoadLocation(tzName)
		if err != nil {
			l.Error("invalid worker timezone, falling back to UTC", "timezone", tzName, "error", err)
			loc = time.UTC
			tzName = "UTC"
		}

		schedule := cfg.Worker.Schedule
		if schedule == "" {
			schedule = "0 1 * * *"
		}

		runOnce := func(lock *daemon.Lock) {
			now := time.Now()
			reporter := &lockReporter{
				lock: lock,
				status: daemon.WorkerStatus{
					Mode:      "auto",
					PID:       os.Getpid(),
					StartedAt: now,
					Phase:     daemon.PhaseProcessing,
				},
			}
			lock.WriteStatus(reporter.status)
			w.SetProgress(reporter)

			l.Info("auto run starting")
			if err := w.ProcessAllPending(runCtx); err != nil {
				l.Error("auto processing failed", "error", err)
			} else {
				l.Info("auto processing completed")
			}
			w.SetProgress(nil)

			completed := time.Now()
			reporter.status.LastCompletedAt = &completed
			reporter.status.Phase = daemon.PhaseSleeping
			lock.WriteStatus(reporter.status)
		}

		// robfig/cron uses 6-field expressions with seconds; prepend "0 " for standard 5-field.
		expr := schedule
		if len(strings.Fields(expr)) == 5 {
			expr = "0 " + expr
		}

		c := cron.New(cron.WithLocation(loc), cron.WithSeconds())
		_, err = c.AddFunc(expr, func() {
			if daemon.CleanStaleGlobalLock() {
				l.Info("cleaned stale global lock")
			}
			if n := daemon.CleanStaleProductionLocks(); n > 0 {
				l.Info("cleaned stale production locks", "count", n)
			}
			// Always advance next_schedule_at regardless of whether we run.
			nextEntries := c.Entries()
			var nextTime time.Time
			if len(nextEntries) > 0 {
				nextTime = nextEntries[0].Next
			}

			lock, err := daemon.AcquireGlobal()
			if err != nil {
				l.Error("acquiring global lock failed", "error", err)
				if !nextTime.IsZero() {
					daemon.AdvanceSchedulerNextRun(nextTime)
				}
				return
			}
			if lock == nil {
				l.Info("auto run skipped — previous run still in progress")
				if !nextTime.IsZero() {
					daemon.AdvanceSchedulerNextRun(nextTime)
				}
				return
			}
			defer lock.Release()

			runOnce(lock)

			if !nextTime.IsZero() {
				l.Info("next auto run scheduled", "at", nextTime.In(loc).Format(time.RFC3339))
				daemon.UpdateSchedulerStateAfterRun(nextTime)
			}
		})
		if err != nil {
			l.Error("invalid worker schedule expression", "schedule", schedule, "error", err)
			os.Exit(1)
		}

		c.Start()

		entries := c.Entries()
		if len(entries) > 0 {
			next := entries[0].Next
			l.Info("worker auto scheduler started",
				"schedule", schedule,
				"timezone", tzName,
				"next_run", next.In(loc).Format(time.RFC3339),
			)
			daemon.WriteSchedulerState(next)
		}

		// Poll for on-demand triggers (global and per-production) written by the API.
		// Fires every 30 s while sleeping; skipped if the global lock is held.
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
					// Global trigger: run all pending productions immediately.
					if daemon.ConsumeGlobalTrigger() {
						if daemon.CleanStaleGlobalLock() {
							l.Info("global trigger: cleaned stale global lock")
						}
						lock, err := daemon.AcquireGlobal()
						if err != nil {
							l.Error("global trigger: cannot acquire global lock", "error", err)
						} else if lock == nil {
							l.Info("global trigger: skipped — previous run still in progress")
						} else {
							runOnce(lock)
							lock.Release()
							entries := c.Entries()
							if len(entries) > 0 {
								daemon.UpdateSchedulerStateAfterRun(entries[0].Next)
							}
						}
					}

					ids := daemon.ReadProdTriggers()
					for _, prodID := range ids {
						prodID := prodID // capture
						go func() {
							if daemon.CleanStaleProductionLocks() > 0 {
								l.Info("trigger: cleaned stale production locks")
							}
							lock, err := daemon.AcquireProduction(prodID)
							if err != nil {
								l.Warn("trigger: cannot acquire production lock", "produccion_id", prodID, "error", err)
								return
							}
							now := time.Now()
							reporter := &lockReporter{
								lock: lock,
								status: daemon.WorkerStatus{
									Mode:                "trigger-production",
									PID:                 os.Getpid(),
									StartedAt:           now,
									Phase:               daemon.PhaseProcessing,
									CurrentProduccionID: prodID,
								},
							}
							lock.WriteStatus(reporter.status)
							w.SetProgress(reporter)

							l.Info("trigger: processing production", "produccion_id", prodID)
							if err := w.ProcessProduction(runCtx, prodID); err != nil {
								l.Error("trigger: production failed", "produccion_id", prodID, "error", err)
							} else {
								l.Info("trigger: production completed", "produccion_id", prodID)
							}
							w.SetProgress(nil)
							lock.Release()
						}()
					}
				}
			}
		}()

		<-runCtx.Done()
		l.Info("worker auto daemon shutting down")
		c.Stop()
		daemon.ClearSchedulerState()
		return
	}

	// --production --regen: regenerate params+ia_req for all completed scenes.
	// --production --scene: process one specific scene.
	// --production: process all pending scenes for that production.
	// All three acquire a per-production lock so the same production cannot be
	// processed by two workers simultaneously, and are rejected if --auto is
	// running (global lock held).
	if *produccionID != 0 {
		lock, err := daemon.AcquireProduction(*produccionID)
		if err != nil {
			l.Error("cannot start", "error", err)
			os.Exit(1)
		}
		mode := "production"
		if *regenMode {
			mode = "regen"
		} else if *sceneID != "" {
			mode = "scene"
		}
		reporter := &lockReporter{
			lock: lock,
			status: daemon.WorkerStatus{
				Mode:                mode,
				PID:                 os.Getpid(),
				StartedAt:           time.Now(),
				Phase:               daemon.PhaseProcessing,
				CurrentProduccionID: *produccionID,
			},
		}
		lock.WriteStatus(reporter.status)
		w.SetProgress(reporter)

		var runErr error
		if *regenMode {
			runErr = w.RegenAllCompleted(ctx, *produccionID)
		} else if *sceneID != "" {
			runErr = w.ProcessScene(ctx, *produccionID, *sceneID)
		} else {
			runErr = w.ProcessProduction(ctx, *produccionID)
		}

		lock.Release()

		if runErr != nil {
			l.Error("worker failed", "mode", mode, "produccion_id", *produccionID, "error", runErr)
			os.Exit(1)
		}
		l.Info("worker completed", "mode", mode, "produccion_id", *produccionID)
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

// connectMySQLWithRetry connects to MySQL with exponential backoff retry logic.
func connectMySQLWithRetry(ctx context.Context, cfg config.MySQLConfig, l interface {
	Info(string, ...any)
	Error(string, ...any)
}) (*sql.DB, error) {
	cfg2 := retry.WorkerConfig
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
