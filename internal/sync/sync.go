// Package sync implements the periodic sync cycle that pulls active
// producciones from DynamoDB and reconciles them into s3_monitoring_producciones,
// then syncs their escenas into s3_monitoring_escenas.
package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/infrastructure/aws"
	"agro-sentinel-worker/internal/infrastructure/database"
)

// DynamoReader is the subset of aws.DynamoDBClient the sync service needs.
type DynamoReader interface {
	ListActiveProducciones(ctx context.Context, tableName string) ([]aws.DynamoProduction, error)
	ListEscenas(ctx context.Context, tableName string, produccionID int64) ([]aws.DynamoScene, error)
	CloseProduccion(ctx context.Context, tableName string, produccionID int64, folio string) error
}

// ProductionRepository is the subset of database.ProductionRepo the sync service needs.
type ProductionRepository interface {
	Upsert(ctx context.Context, p *domain.Production) error
	UpdateSyncState(ctx context.Context, p *domain.Production) error
	GetByProduccionID(ctx context.Context, produccionID int64) (*domain.Production, error)
	ListActive(ctx context.Context) ([]*domain.Production, error)
	UpdateMonitoring(ctx context.Context, produccionID int64, monitoring bool) error
	GetERPDetails(ctx context.Context, produccionID int64) (*database.ERPProduccion, error)
	GetVariedades(ctx context.Context, produccionID int64) (string, error)
	ExistsInERP(ctx context.Context, produccionID int64) (bool, error)
}

// SceneRepository is the subset of database.SceneRepo the sync service needs.
type SceneRepository interface {
	Upsert(ctx context.Context, s *domain.Scene) error
	GetByProduccionAndSceneName(ctx context.Context, produccionID int64, sceneName string) (*domain.Scene, error)
	ListByMonitoringProduccion(ctx context.Context, monitoringProduccionID uint) ([]*domain.Scene, error)
	UpdateExistsFlags(ctx context.Context, escenaID uint64, truthTif, renderTif, params, ia bool) error
	// UpdateFromParams updates production_cloud, usable and analysis when params or IA
	// files are indexed — kept separate from UpdateExistsFlags so it is only called
	// when the payload is actually available to parse.
	UpdateFromParams(ctx context.Context, escenaID uint64, productionCloud *float64, usable, analysis *bool) error
	UpdateStatus(ctx context.Context, id uint64, status string) error
}

// PolygonRepository looks up the polygon for a produccion from asignaciones_zonas_producciones.
type PolygonRepository interface {
	GetPolygon(ctx context.Context, produccionID int64) (rawPolygon string, bbox *domain.BBox, err error)
}

// S3Indexer lists objects and reads file content from S3.
type S3Indexer interface {
	BucketExists(ctx context.Context, bucket string) (bool, error)
	ListObjects(ctx context.Context, bucket, prefix string) ([]aws.S3ObjectInfo, error)
	GetObjectContent(ctx context.Context, bucket, key string) ([]byte, error)
}

// SceneFileRepository indexes scene files in s3_monitoring_escena_archivos.
type SceneFileRepository interface {
	ExistsByKeyHash(ctx context.Context, escenaID uint64, hash string) (bool, error)
	Create(ctx context.Context, f *domain.SceneFile) error
}

// IAResultRepository persists IA analysis results reconstructed from S3.
type IAResultRepository interface {
	Upsert(ctx context.Context, result *domain.IAResultSummary) error
}

// SkipReason explains why a produccion seen in DynamoDB was not brought fully
// into MySQL. An empty value means the produccion was synced normally.
type SkipReason string

const (
	SkipNone         SkipReason = ""
	SkipSinPoligono  SkipReason = "sin_poligono"
	SkipSinFecha     SkipReason = "sin_fecha_plantacion"
	SkipNoExisteERP  SkipReason = "no_existe_en_erp"
	SkipBloqueada    SkipReason = "bloqueada"
	SkipFinMonitoreo SkipReason = "fin_monitoreo"
)

// SkippedProd identifies a produccion left out of the last sync cycle.
type SkippedProd struct {
	ProduccionID int64      `json:"produccion_id"`
	Folio        string     `json:"folio,omitempty"`
	Reason       SkipReason `json:"reason"`
}

// SyncReport summarises the outcome of the last completed sync cycle so the UI
// can tell whether MySQL is actually up to date with DynamoDB, instead of only
// knowing when the cycle last ran.
type SyncReport struct {
	StartedAt       time.Time     `json:"started_at"`
	FinishedAt      time.Time     `json:"finished_at"`
	ProdsInDynamo   int           `json:"prods_in_dynamo"`
	ProdsUpserted   int           `json:"prods_upserted"`
	ProdsSkipped    []SkippedProd `json:"prods_skipped,omitempty"`
	ProdsMonitoring int           `json:"prods_monitoring"`
	EscenasInserted int           `json:"escenas_inserted"`
	Errors          []string      `json:"errors,omitempty"`
}

// UpToDate reports whether the cycle finished leaving nothing behind.
func (r *SyncReport) UpToDate() bool {
	return r != nil && len(r.ProdsSkipped) == 0 && len(r.Errors) == 0
}

// ScheduleStatus holds the last and next execution times for observability.
type ScheduleStatus struct {
	Running  bool        `json:"running"`
	LastRun  *time.Time  `json:"last_run,omitempty"`
	NextRun  *time.Time  `json:"next_run,omitempty"`
	Schedule string      `json:"schedule"`
	Timezone string      `json:"timezone"`
	Report   *SyncReport `json:"report,omitempty"`
}

// Service runs the DynamoDB → MySQL sync cycle.
type Service struct {
	dynamo      DynamoReader
	prodRepo    ProductionRepository
	sceneRepo   SceneRepository
	polygonRepo PolygonRepository
	s3          S3Indexer
	fileRepo    SceneFileRepository
	iaRepo      IAResultRepository // optional; nil skips ia.json reconstruction

	cfg      config.SyncConfig
	sentinel config.SentinelConfig

	tableProducciones string
	tableEscenas      string
	s3Bucket          string

	logger *slog.Logger
	now    func() time.Time

	mu         sync.RWMutex
	lastRun    *time.Time
	nextRun    *time.Time
	running    bool
	lastReport *SyncReport
}

// New builds a Service with the given dependencies.
func New(
	dynamo DynamoReader,
	prodRepo ProductionRepository,
	sceneRepo SceneRepository,
	polygonRepo PolygonRepository,
	s3 S3Indexer,
	fileRepo SceneFileRepository,
	iaRepo IAResultRepository,
	cfg config.SyncConfig,
	sentinel config.SentinelConfig,
	tableProducciones string,
	tableEscenas string,
	s3Bucket string,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}

	return &Service{
		dynamo:            dynamo,
		prodRepo:          prodRepo,
		sceneRepo:         sceneRepo,
		polygonRepo:       polygonRepo,
		s3:                s3,
		fileRepo:          fileRepo,
		iaRepo:            iaRepo,
		cfg:               cfg,
		sentinel:          sentinel,
		tableProducciones: tableProducciones,
		tableEscenas:      tableEscenas,
		s3Bucket:          s3Bucket,
		logger:            logger,
		now:               time.Now,
	}
}

// CalculateFinMonitoreo returns the date monitoring should stop.
func CalculateFinMonitoreo(plantacion time.Time, maxDias, margen int) time.Time {
	return plantacion.AddDate(0, 0, maxDias+margen)
}

// RunOnce executes a single sync cycle in two independent phases:
//
//  1. Productions — pull every OPEN produccion from DynamoDB, upsert/update
//     in MySQL, and attempt to fill in polygon data (pbox, tile_bbox, poligono)
//     for any row that still lacks it.
//
//  2. Scenes — iterate all MySQL productions with monitoring=true that have the
//     required spatial fields, then insert any DynamoDB scenes not yet in MySQL.
//     Cloud-cover filtering is handled by a separate worker; here we only skip
//     scenes that already exist.
func (s *Service) RunOnce(ctx context.Context) error {
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		now := s.now()
		s.lastRun = &now
		s.mu.Unlock()
	}()

	s.logger.Info("sync cycle starting", "table_producciones", s.tableProducciones, "table_escenas", s.tableEscenas)

	report := &SyncReport{StartedAt: s.now()}

	// ── Phase 1: sync productions ────────────────────────────────────────────
	producciones, err := s.dynamo.ListActiveProducciones(ctx, s.tableProducciones)
	if err != nil {
		report.Errors = append(report.Errors, "listing active producciones: "+err.Error())
		s.finishReport(report)
		return &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: "listing active producciones", Wrapped: err}
	}

	report.ProdsInDynamo = len(producciones)
	s.logger.Info("sync cycle phase 1: producciones fetched from dynamodb", "count", len(producciones))

	for _, dp := range producciones {
		reason, err := s.syncProduccion(ctx, dp)
		if err != nil {
			s.logger.Error("syncing produccion failed", "produccion_id", dp.ProduccionID, "error", err)
			report.Errors = append(report.Errors,
				fmt.Sprintf("produccion %d: %v", dp.ProduccionID, err))
			continue
		}
		if reason != SkipNone {
			report.ProdsSkipped = append(report.ProdsSkipped, SkippedProd{
				ProduccionID: dp.ProduccionID, Folio: dp.Folio, Reason: reason,
			})
			continue
		}
		report.ProdsUpserted++
	}

	// ── Phase 2: sync scenes for all monitoring productions ──────────────────
	active, err := s.prodRepo.ListActive(ctx)
	if err != nil {
		s.logger.Error("listing active productions for scene sync failed", "error", err)
		report.Errors = append(report.Errors, "listing active productions: "+err.Error())
		s.finishReport(report)
		return nil // phase 1 succeeded; log and continue
	}
	report.ProdsMonitoring = len(active)

	var readyCount int
	for _, p := range active {
		if p.HasRequiredFields() {
			readyCount++
		}
	}
	s.logger.Info("sync cycle phase 2: syncing scenes", "monitoring_count", len(active), "ready_for_scenes", readyCount)

	for _, prod := range active {
		if !prod.HasRequiredFields() {
			s.logger.Info("skipping scene sync: missing required fields",
				"produccion_id", prod.ProduccionID,
				"has_pbox", len(prod.PBoxJSON) > 0,
				"has_tile_bbox", len(prod.TileBBoxJSON) > 0,
				"has_poligono", len(prod.PoligonoJSON) > 0,
				"has_fecha_plantacion", prod.FechaPlantacion != nil,
			)
			continue
		}
		inserted, err := s.syncEscenas(ctx, prod)
		if err != nil {
			s.logger.Error("syncing escenas failed", "produccion_id", prod.ProduccionID, "error", err)
			report.Errors = append(report.Errors,
				fmt.Sprintf("escenas de produccion %d: %v", prod.ProduccionID, err))
		}
		report.EscenasInserted += inserted
	}

	// ── Phase 3: index S3 files into s3_monitoring_escena_archivos ───────────
	if s.s3 != nil && s.fileRepo != nil {
		bucketOK, bucketErr := s.s3.BucketExists(ctx, s.s3Bucket)
		if bucketErr != nil {
			s.logger.Warn("phase 3 skipped: s3 bucket check failed", "bucket", s.s3Bucket, "error", bucketErr)
		} else if !bucketOK {
			s.logger.Info("phase 3 skipped: s3 bucket not yet created", "bucket", s.s3Bucket)
		} else {
			s.logger.Info("sync cycle phase 3: indexing s3 files")
			phase3Ctx, cancel3 := context.WithTimeout(ctx, 10*time.Minute)
			for _, prod := range active {
				if phase3Ctx.Err() != nil {
					s.logger.Warn("phase 3 timeout reached, stopping s3 indexing")
					break
				}
				if !prod.HasRequiredFields() {
					continue
				}
				scenes, err := s.sceneRepo.ListByMonitoringProduccion(phase3Ctx, prod.ID)
				if err != nil {
					s.logger.Error("listing scenes for s3 indexing failed",
						"produccion_id", prod.ProduccionID, "error", err)
					continue
				}
				for _, scene := range scenes {
					sceneCtx, sceneCancel := context.WithTimeout(phase3Ctx, 30*time.Second)
					s.indexSceneFiles(sceneCtx, prod, scene)
					sceneCancel()
				}
			}
			cancel3()
		}
	}

	s.finishReport(report)
	s.recordLastRun() // update last_run; next_run is managed by the cron/interval loop
	return nil
}

// RunScenesOnly ejecuta solo la fase 2: sincroniza escenas faltantes para todas
// las producciones activas que cumplen con los campos requeridos.
// Útil para forzar una sincronización de escenas sin tocar las producciones,
// por ejemplo después de importar escenas directamente a DynamoDB.
func (s *Service) RunScenesOnly(ctx context.Context) error {
	active, err := s.prodRepo.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("listing active productions: %w", err)
	}

	s.logger.Info("sync escenas: producciones activas", "count", len(active))

	for _, prod := range active {
		if !prod.HasRequiredFields() {
			s.logger.Info("skipping scene sync: missing required fields",
				"produccion_id", prod.ProduccionID,
				"has_pbox", len(prod.PBoxJSON) > 0,
				"has_tile_bbox", len(prod.TileBBoxJSON) > 0,
				"has_poligono", len(prod.PoligonoJSON) > 0,
				"has_fecha_plantacion", prod.FechaPlantacion != nil,
			)
			continue
		}
		if _, err := s.syncEscenas(ctx, prod); err != nil {
			s.logger.Error("syncing escenas failed", "produccion_id", prod.ProduccionID, "error", err)
		}
		if s.s3 != nil && s.fileRepo != nil {
			scenes, err := s.sceneRepo.ListByMonitoringProduccion(ctx, prod.ID)
			if err != nil {
				s.logger.Error("listing scenes for s3 indexing failed",
					"produccion_id", prod.ProduccionID, "error", err)
				continue
			}
			for _, scene := range scenes {
				sceneCtx, sceneCancel := context.WithTimeout(ctx, 30*time.Second)
				s.indexSceneFiles(sceneCtx, prod, scene)
				sceneCancel()
			}
		}
	}

	return nil
}

// Status returns the last and next execution times along with the active schedule.
// Returns any to satisfy the http.Syncer interface without an import cycle.
// nextRun is always computed from the cron expression so it is accurate even
// when Status is called from the API container (which never runs RunLoop).
func (s *Service) Status() any {
	s.mu.RLock()
	lastRun := s.lastRun
	nextRunStored := s.nextRun
	running := s.running
	report := s.lastReport
	s.mu.RUnlock()

	// Proceso API nunca ejecuta sync — leer last_run y el reporte del archivo
	// compartido en /tmp.
	if lastRun == nil {
		lastRun = readPersistedLastRun()
	}
	if report == nil {
		report = readPersistedReport()
	}

	tz := s.cfg.Timezone
	if tz == "" {
		tz = "UTC"
	}
	sched := s.cfg.Schedule
	if sched == "" {
		sched = fmt.Sprintf("every %dm", s.cfg.IntervalMinutes)
	}

	// Prefer stored nextRun (set by RunLoop). Fall back to computing from
	// the cron expression so the API container always shows a useful value.
	nextRun := nextRunStored
	if nextRun == nil && s.cfg.Schedule != "" {
		nextRun = s.computeNextRun(tz)
	}

	return ScheduleStatus{
		Running:  running,
		LastRun:  lastRun,
		NextRun:  nextRun,
		Schedule: sched,
		Timezone: tz,
		Report:   report,
	}
}

// computeNextRun calculates the next fire time from cfg.Schedule without
// starting a cron daemon — safe to call from any process at any time.
func (s *Service) computeNextRun(tzName string) *time.Time {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		loc = time.UTC
	}
	expr := s.cfg.Schedule
	if len(strings.Fields(expr)) == 5 {
		expr = "0 " + expr
	}
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(expr)
	if err != nil {
		return nil
	}
	t := schedule.Next(time.Now().In(loc))
	return &t
}

// IsRunning reports whether a sync cycle is currently in progress.
func (s *Service) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

const (
	lastRunPath = "/tmp/sync-last-run"
	reportPath  = "/tmp/sync-report.json"
)

// finishReport stamps the end time, stores the report in memory and persists it
// so the API process (which never runs a cycle) can serve it.
func (s *Service) finishReport(r *SyncReport) {
	r.FinishedAt = s.now()

	s.mu.Lock()
	s.lastReport = r
	s.mu.Unlock()

	b, err := json.Marshal(r)
	if err != nil {
		s.logger.Warn("marshaling sync report failed", "error", err)
		return
	}
	if err := os.WriteFile(reportPath, b, 0o600); err != nil {
		s.logger.Warn("persisting sync report failed", "error", err)
	}

	s.logger.Info("sync cycle report",
		"prods_in_dynamo", r.ProdsInDynamo,
		"prods_upserted", r.ProdsUpserted,
		"prods_skipped", len(r.ProdsSkipped),
		"escenas_inserted", r.EscenasInserted,
		"errors", len(r.Errors),
		"up_to_date", r.UpToDate())
}

func readPersistedReport() *SyncReport {
	b, err := os.ReadFile(reportPath)
	if err != nil {
		return nil
	}
	var r SyncReport
	if err := json.Unmarshal(b, &r); err != nil {
		return nil
	}
	return &r
}

func (s *Service) persistLastRun(t time.Time) {
	b, _ := t.MarshalText()
	_ = os.WriteFile(lastRunPath, b, 0o600)
}

func readPersistedLastRun() *time.Time {
	b, err := os.ReadFile(lastRunPath)
	if err != nil {
		return nil
	}
	var t time.Time
	if err := t.UnmarshalText(b); err != nil {
		return nil
	}
	return &t
}

func (s *Service) recordRun(next *time.Time) {
	s.mu.Lock()
	now := s.now()
	s.lastRun = &now
	s.nextRun = next
	s.mu.Unlock()
	s.persistLastRun(now)
}

func (s *Service) recordLastRun() {
	s.mu.Lock()
	now := s.now()
	s.lastRun = &now
	s.mu.Unlock()
	s.persistLastRun(now)
}

// RunLoop runs the sync on a schedule until ctx is cancelled.
// When cfg.Schedule is set it uses that cron expression in cfg.Timezone;
// otherwise it falls back to cfg.IntervalMinutes.
// An immediate first run always happens on start.
func (s *Service) RunLoop(ctx context.Context) error {
	// Always run once immediately on start.
	if err := s.RunOnce(ctx); err != nil {
		s.logger.Error("sync cycle failed", "error", err)
	}

	if s.cfg.Schedule != "" {
		return s.runLoopCron(ctx)
	}
	return s.runLoopInterval(ctx)
}

func (s *Service) runLoopCron(ctx context.Context) error {
	loc := time.UTC
	if s.cfg.Timezone != "" {
		var err error
		loc, err = time.LoadLocation(s.cfg.Timezone)
		if err != nil {
			return fmt.Errorf("invalid sync timezone %q: %w", s.cfg.Timezone, err)
		}
	}

	c := cron.New(cron.WithLocation(loc), cron.WithSeconds())
	// robfig/cron v3 with WithSeconds() uses 6-field expressions; we accept
	// standard 5-field (no seconds) by prepending "0 ".
	expr := s.cfg.Schedule
	if len(strings.Fields(expr)) == 5 {
		expr = "0 " + expr
	}

	_, err := c.AddFunc(expr, func() {
		if err := s.RunOnce(ctx); err != nil {
			s.logger.Error("sync cycle failed", "error", err)
		}
		// Compute next fire after this run.
		entries := c.Entries()
		var next *time.Time
		if len(entries) > 0 {
			t := entries[0].Next
			next = &t
		}
		s.recordRun(next)
		if next != nil {
			s.logger.Info("sync cycle done", "next_run", next.Format(time.RFC3339))
		}
	})
	if err != nil {
		return fmt.Errorf("invalid sync schedule %q: %w", s.cfg.Schedule, err)
	}

	// Set initial next-run before starting.
	c.Start()
	entries := c.Entries()
	if len(entries) > 0 {
		t := entries[0].Next
		s.mu.Lock()
		s.nextRun = &t
		s.mu.Unlock()
		s.logger.Info("sync scheduled", "schedule", s.cfg.Schedule, "timezone", s.cfg.Timezone, "next_run", t.Format(time.RFC3339))
	}

	<-ctx.Done()
	c.Stop()
	return ctx.Err()
}

func (s *Service) runLoopInterval(ctx context.Context) error {
	interval := time.Duration(s.cfg.IntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	next := s.now().Add(interval)
	s.mu.Lock()
	s.nextRun = &next
	s.mu.Unlock()
	s.logger.Info("sync scheduled", "interval_minutes", s.cfg.IntervalMinutes, "next_run", next.Format(time.RFC3339))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.RunOnce(ctx); err != nil {
				s.logger.Error("sync cycle failed", "error", err)
			}
			next = s.now().Add(interval)
			s.recordRun(&next)
			s.logger.Info("sync cycle done", "next_run", next.Format(time.RFC3339))
		}
	}
}

// syncProduccion brings one DynamoDB produccion into MySQL. It returns the
// reason the produccion was left out, or SkipNone when it synced normally.
func (s *Service) syncProduccion(ctx context.Context, dp aws.DynamoProduction) (SkipReason, error) {
	existing, err := s.prodRepo.GetByProduccionID(ctx, dp.ProduccionID)
	if err != nil {
		return SkipNone, &domain.ProcessingError{Type: domain.ErrMySQL, Message: "getting production", Wrapped: err}
	}

	if existing != nil && existing.Bloqueado {
		s.logger.Info("skipping blocked produccion", "produccion_id", dp.ProduccionID)
		return SkipBloqueada, nil
	}

	erp, err := s.prodRepo.GetERPDetails(ctx, dp.ProduccionID)
	if err != nil {
		s.logger.Warn("getting erp details failed, continuing with defaults",
			"produccion_id", dp.ProduccionID, "error", err)
	}

	prod := buildProduction(dp, existing, erp)

	// Variedades come from ERP planting records — always refresh from DB.
	if variedades, err := s.prodRepo.GetVariedades(ctx, dp.ProduccionID); err != nil {
		s.logger.Warn("getting variedades failed, keeping existing value",
			"produccion_id", dp.ProduccionID, "error", err)
	} else if variedades != "" {
		prod.Vaiedades = variedades
	}

	now := s.now()
	prod.UltimaSincronizacion = &now

	monitoring, pboxJSON, reason := s.evaluateMonitoring(ctx, &prod)
	prod.Monitoring = monitoring
	if pboxJSON != nil {
		prod.PBoxJSON = pboxJSON
	}
	finAlcanzado := reason == SkipFinMonitoreo

	if existing == nil {
		if finAlcanzado {
			// Period already ended before we ever inserted — close DynamoDB and skip.
			s.closeDynamo(ctx, dp.ProduccionID, dp.Folio)
			return SkipFinMonitoreo, nil
		}

		// s3_monitoring_producciones has a FK to the ERP producciones table, so
		// inserting a produccion_id that is not there fails with error 1452 on
		// every cycle. Report it as a skip instead of retrying forever.
		if ok, erpErr := s.prodRepo.ExistsInERP(ctx, dp.ProduccionID); erpErr != nil {
			s.logger.Warn("checking erp existence failed",
				"produccion_id", dp.ProduccionID, "error", erpErr)
		} else if !ok {
			s.logger.Info("skipping produccion: no existe en la tabla producciones del ERP",
				"produccion_id", dp.ProduccionID, "folio", dp.Folio)
			return SkipNoExisteERP, nil
		}

		// Insert with monitoring=false when fecha_plantacion or polygon is missing.
		// Future sync cycles will retry and activate monitoring once data is available.
		if err := s.prodRepo.Upsert(ctx, &prod); err != nil {
			return SkipNone, &domain.ProcessingError{Type: domain.ErrMySQL, Message: "inserting production", Wrapped: err}
		}
	} else {
		// Existing row — only update sync-controlled fields, never touch DBA-owned columns.
		if err := s.prodRepo.UpdateSyncState(ctx, &prod); err != nil {
			return SkipNone, &domain.ProcessingError{Type: domain.ErrMySQL, Message: "updating production sync state", Wrapped: err}
		}

		if finAlcanzado {
			s.closeDynamo(ctx, dp.ProduccionID, dp.Folio)
			return SkipFinMonitoreo, nil
		}
	}

	return reason, nil
}

// buildProduction merges DynamoDB + ERP data onto the existing MySQL row.
func buildProduction(dp aws.DynamoProduction, existing *domain.Production, erp *database.ERPProduccion) domain.Production {
	var prod domain.Production
	if existing != nil {
		prod = *existing
	}

	prod.ProduccionID = dp.ProduccionID
	prod.MaxDiasMonitoring = dp.DiasProduccion

	if dp.FechaPlantacion != "" {
		for _, layout := range []string{"02/01/2006", "2006-01-02"} {
			if t, err := time.Parse(layout, dp.FechaPlantacion); err == nil {
				prod.FechaPlantacion = &t
				break
			}
		}
	}


	// Enrich from ERP (articulos.nombre = cosecha, centros_costos.nombre used for prefix).
	if erp != nil {
		prod.ArticuloID = erp.ArticuloID
		prod.CentroCostoID = erp.CentroCostoID
		prod.Cosecha = erp.Cultivo
		prod.Vaiedades = erp.Cultivo
		prod.Folio = erp.Folio
		prod.Rancho = erp.NombreRancho
		if prod.Prefix == "" {
			prod.Prefix = buildPrefix(erp.Folio, dp.ProduccionID)
		}
	}

	if prod.Folio == "" {
		prod.Folio = dp.Folio
	}
	if prod.Cosecha == "" {
		prod.Cosecha = fmt.Sprintf("produccion-%d", dp.ProduccionID)
	}
	if prod.Vaiedades == "" {
		prod.Vaiedades = prod.Cosecha
	}
	if prod.Prefix == "" {
		prod.Prefix = buildPrefix(dp.Folio, dp.ProduccionID)
	}

	return prod
}

// buildPrefix derives the S3 prefix from the folio or produccion_id.
func buildPrefix(folio string, produccionID int64) string {
	if folio != "" {
		return "producciones/" + strings.ToLower(folio)
	}
	return fmt.Sprintf("producciones/%d", produccionID)
}

// evaluateMonitoring decides whether the production should be monitored.
// Returns (monitoring, pboxJSON to store — nil if unchanged, skip reason).
// A reason of SkipFinMonitoreo means the monitoring period has ended and
// DynamoDB + MySQL should be updated to reflect it is no longer active.
func (s *Service) evaluateMonitoring(ctx context.Context, prod *domain.Production) (bool, json.RawMessage, SkipReason) {
	if prod.FechaPlantacion == nil {
		s.logger.Info("monitoring disabled: no fecha_plantacion", "produccion_id", prod.ProduccionID)
		return false, nil, SkipSinFecha
	}

	fin := CalculateFinMonitoreo(*prod.FechaPlantacion, prod.MaxDiasMonitoring, s.cfg.DiasMargenMonitoreo)
	prod.FechaFin = &fin

	if s.now().After(fin) {
		s.logger.Info("monitoring disabled: fin de monitoreo alcanzado", "produccion_id", prod.ProduccionID)
		return false, nil, SkipFinMonitoreo
	}

	// If no pbox, try polygon from asignaciones_zonas_producciones.
	// Polygon never comes from DynamoDB — it is assigned by the DBA in MySQL.
	//
	// This early exit is load-bearing: a production that already has a pbox
	// keeps whatever geometry is stored, which is what lets the polygon editor
	// (PUT /producciones/{id}/poligono) persist an edit across sync cycles.
	// Making this branch unconditional would silently replace every edited
	// polygon with the ERP one on the next cycle. Guarded by
	// TestRunOnce_DoesNotOverwriteEditedPolygon.
	if prod.ParsePBox() == nil {
		rawPolygon, bbox, err := s.polygonRepo.GetPolygon(ctx, prod.ProduccionID)
		if err != nil {
			s.logger.Warn("polygon lookup failed", "produccion_id", prod.ProduccionID, "error", err)
		}
		if bbox != nil {
			type bboxDoc struct {
				MinLon float64   `json:"min_lon"`
				MinLat float64   `json:"min_lat"`
				MaxLon float64   `json:"max_lon"`
				MaxLat float64   `json:"max_lat"`
				PBox   []float64 `json:"pbox"`
			}

			// Tight polygon bbox → pbox.
			pboxDoc := bboxDoc{
				MinLon: bbox.MinX, MinLat: bbox.MinY,
				MaxLon: bbox.MaxX, MaxLat: bbox.MaxY,
				PBox: []float64{bbox.MinX, bbox.MinY, bbox.MaxX, bbox.MaxY},
			}
			pboxBytes, err := json.Marshal(pboxDoc)
			if err != nil {
				s.logger.Warn("marshaling pbox failed", "produccion_id", prod.ProduccionID, "error", err)
				return false, nil, SkipSinPoligono
			}

			// Center of polygon for tile calculations.
			centerLat := (bbox.MinY + bbox.MaxY) / 2.0
			centerLon := (bbox.MinX + bbox.MaxX) / 2.0
			prod.TileCenterLat = &centerLat
			prod.TileCenterLon = &centerLon

			// Expanded tile bbox (what scenes will use).
			edgeMeters := prod.TileEdgeMeters
			if edgeMeters == 0 {
				edgeMeters = 2000
			}
			tile := CalculateTileBBox(centerLat, centerLon, edgeMeters)
			tileBboxDoc := bboxDoc{
				MinLon: tile.MinX, MinLat: tile.MinY,
				MaxLon: tile.MaxX, MaxLat: tile.MaxY,
				PBox: []float64{tile.MinX, tile.MinY, tile.MaxX, tile.MaxY},
			}
			tileBboxBytes, err := json.Marshal(tileBboxDoc)
			if err != nil {
				s.logger.Warn("marshaling tile_bbox failed", "produccion_id", prod.ProduccionID, "error", err)
				return false, nil, SkipSinPoligono
			}

			prod.PBoxJSON = pboxBytes
			prod.PolygonBBoxJSON = pboxBytes // tight bbox of the field polygon
			prod.TileBBoxJSON = tileBboxBytes
			if pts := ParsePipePolygonToJSON(rawPolygon); pts != nil {
				prod.PoligonoJSON = pts
			}

			return true, pboxBytes, SkipNone
		}
		// No polygon yet — production is inserted with monitoring=false.
		// Next sync cycle will retry the polygon lookup and activate it.
		s.logger.Info("monitoring disabled: sin poligono/pbox (se reintentara en el siguiente ciclo)", "produccion_id", prod.ProduccionID)
		return false, nil, SkipSinPoligono
	}

	return true, nil, SkipNone
}

// syncEscenas inserts the DynamoDB scenes missing from MySQL for one
// production and returns how many were inserted.
func (s *Service) syncEscenas(ctx context.Context, prod *domain.Production) (int, error) {
	escenas, err := s.dynamo.ListEscenas(ctx, s.tableEscenas, prod.ProduccionID)
	if err != nil {
		return 0, &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: "listing escenas", Wrapped: err}
	}

	s.logger.Info("escenas found in dynamodb", "produccion_id", prod.ProduccionID, "table", s.tableEscenas, "count", len(escenas))

	var inserted int
	for _, de := range escenas {
		// Cloud-cover filtering is handled by the processing worker, not here.
		// We only skip scenes that are already recorded in MySQL.
		existing, err := s.sceneRepo.GetByProduccionAndSceneName(ctx, prod.ProduccionID, de.SceneID)
		if err != nil {
			s.logger.Error("getting scene failed",
				"produccion_id", prod.ProduccionID,
				"scene_name", de.SceneID,
				"error", err)
			continue
		}
		if existing != nil {
			continue
		}

		fecha, err := parseSceneDate(de.Date)
		if err != nil {
			s.logger.Error("parsing scene date failed",
				"scene_name", de.SceneID,
				"date", de.Date,
				"error", err)
			continue
		}

		cloudCover := de.CloudCover
		now := s.now()

		// Guarda la URL completa de una banda de referencia (con extensión)
		// para que el resolver pueda derivar todas las bandas por sustitución.
		// Prioriza stac_assets (formato STAC actual) y cae al legado si no existe.
		base := de.BaseBandURL()

		scene := &domain.Scene{
			MonitoringProduccionID: prod.ID,
			SceneName:              de.SceneID,
			Fecha:                  &fecha,
			CloudCover:             &cloudCover,
			Status:                 domain.StatusPending,
			UltimaSincronizacion:   &now,
			BaseBands:              base,
		}

		if err := s.sceneRepo.Upsert(ctx, scene); err != nil {
			s.logger.Error("upserting scene failed",
				"produccion_id", prod.ProduccionID,
				"scene_name", de.SceneID,
				"error", err)
			continue
		}
		inserted++
	}

	if inserted > 0 {
		s.logger.Info("escenas inserted", "produccion_id", prod.ProduccionID, "inserted", inserted)
	}
	return inserted, nil
}

func (s *Service) closeDynamo(ctx context.Context, produccionID int64, folio string) {
	if err := s.dynamo.CloseProduccion(ctx, s.tableProducciones, produccionID, folio); err != nil {
		s.logger.Warn("closing produccion in dynamodb failed", "produccion_id", produccionID, "error", err)
	} else {
		s.logger.Info("produccion closed in dynamodb", "produccion_id", produccionID)
	}
}

// firstNonEmpty returns the first non-empty string from the provided values.
// Kept for potential future use.
func firstNonEmpty(vals ...string) string { //nolint:unused
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func parseSceneDate(date string) (time.Time, error) {
	for _, layout := range []string{"02/01/2006", "2006-01-02", time.RFC3339} {
		if t, err := time.Parse(layout, date); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date format: %q", date)
}
