// Package worker implements the processing worker orchestration: the
// end-to-end pipeline that takes a (produccion, escena) pair from PENDING
// through multiband generation, cloud cover evaluation, image/index
// generation, statistics, params.json construction, S3 upload, and database
// bookkeeping.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/processing"
	"agro-sentinel-worker/internal/storage"
)

// cloudCoverThreshold is the SCL cloud-cover-over-bbox percentage below
// which a scene gets full processing (all compositions, indices, stats and
// params.json). At or above it, only natural.png is generated.
const cloudCoverThreshold = 15.0

// maxRetries is the maximum number of times a scene may be retried after a
// FAILED processing attempt.
const maxRetries = 3

// GDALExecutor is the subset of gdal.Executor's behavior the worker's
// processing calls depend on. It matches processing.GDALExecutor
// structurally so any *gdal.Executor (or test double) satisfies both.
type GDALExecutor interface {
	Run(ctx context.Context, command string, args []string) (stdout string, stderr string, err error)
}

// ProductionRepository is the subset of database.ProductionRepo the worker needs.
type ProductionRepository interface {
	GetByProduccionID(ctx context.Context, produccionID int64) (*domain.Production, error)
	GetByMonitoringID(ctx context.Context, monitoringID uint) (*domain.Production, error)
	// bloqueado no se escribe nunca: lo deriva un trigger de posible_cosecha.
	UpdatePosibleCosecha(ctx context.Context, produccionID int64, posible bool) error
	GetERPFolioRancho(ctx context.Context, produccionID int64) (folio, rancho string, err error)
}

// SceneRepository is the subset of database.SceneRepo the worker needs.
type SceneRepository interface {
	GetByProduccionAndSceneName(ctx context.Context, produccionID int64, sceneName string) (*domain.Scene, error)
	UpdateStatus(ctx context.Context, id uint64, status domain.JobStatus) error
	SetFailed(ctx context.Context, id uint64) error
	Upsert(ctx context.Context, s *domain.Scene) error
	GetPreviousUsableScene(ctx context.Context, monitoringProduccionID uint, beforeDate time.Time) (*domain.Scene, error)
	ListByMonitoringProduccion(ctx context.Context, monitoringProduccionID uint) ([]*domain.Scene, error)
	GetOldestPendingByProduccion(ctx context.Context, monitoringProduccionID uint) (*domain.Scene, error)
	ListFromDateByProduccion(ctx context.Context, monitoringProduccionID uint, fromDate time.Time) ([]*domain.Scene, error)
	ResetProcessingToPending(ctx context.Context, monitoringProduccionID uint) (int64, error)
	ResetAllProcessingToPending(ctx context.Context) (int64, error)
	// FindMultibandSources returns origin scenes (multiband_ref_escena_id IS NULL,
	// truth_tif_exists = 1) for the given scene_name in OTHER productions, along
	// with their production's tile_bbox so the caller can check containment.
	FindMultibandSources(ctx context.Context, sceneName string, excludeProduccionID int64) ([]*domain.MultibandSource, error)
	// CountScenesToProcess returns how many production-scene rows this run will
	// walk, matching ListAllBySceneName's filters so the progress total agrees
	// with what actually gets processed.
	CountScenesToProcess(ctx context.Context) (int, error)
	// FindOldestPendingSceneName returns the scene_name of the globally oldest
	// PENDING/FAILED scene across all monitored productions. Returns ("", zero, nil)
	// when there is nothing left to process.
	FindOldestPendingSceneName(ctx context.Context) (sceneName string, fecha time.Time, err error)
	// ListAllBySceneName returns every scene with the given scene_name across all
	// monitored productions, with truth_tif_exists=1 rows first (for reuse).
	ListAllBySceneName(ctx context.Context, sceneName string) ([]*domain.Scene, error)
}

// FileRepository is the subset of database.FileRepo the worker needs.
type FileRepository interface {
	Create(ctx context.Context, f *domain.SceneFile) error
	GetByTipo(ctx context.Context, escenaID uint64, tipo string) (*domain.SceneFile, error)
}

// S3Client is the subset of aws.S3Client the worker needs to persist output
// files and fetch previous params.json documents.
type S3Client interface {
	Upload(ctx context.Context, bucket, key, filePath string) error
	HeadObject(ctx context.Context, bucket, key string) (bool, int64, error)
	Download(ctx context.Context, bucket, key, destPath string) error
	BuildKey(prefix string, produccionID int64, sceneID string, fileName string) string
}

// BandResolver resolves the COG hrefs for a scene's spectral bands and its
// SCL (scene classification layer) band, used to build multiband.tif and
// compute cloud cover respectively.
type BandResolver interface {
	ResolveBands(ctx context.Context, produccionID int64, sceneID string) (bands []domain.BandInfo, sclHref string, err error)
}

// IAResultRepository is the subset of database.IAResultRepository the worker
// needs to fetch the previous IA analysis for the ia_req.json payload.
type IAResultRepository interface {
	GetByEscenaID(ctx context.Context, escenaID uint64) (*domain.IAResultSummary, error)
}

// IAAnalyzer is the subset of ia.Analyzer the worker needs for auto-analysis.
type IAAnalyzer interface {
	Analyze(ctx context.Context, escenaID uint64, s3Key string, produccionID int64) (*domain.IAResultSummary, error)
	DryRunRaw(ctx context.Context, s3Key string) error
}

// WorkerDeps bundles all dependencies ProcessScene needs. Every dependency
// is expressed as a local interface so tests can supply mocks.
type WorkerDeps struct {
	Productions ProductionRepository
	Scenes      SceneRepository
	Files       FileRepository
	IAResults   IAResultRepository // optional; nil skips ultimo_analisis lookup
	IAAnalyzer  IAAnalyzer         // optional; nil disables auto-IA even when production.IAuto=true
	S3          S3Client
	Executor    GDALExecutor
	Bands       BandResolver

	Processing config.ProcessingConfig
	Sentinel   config.SentinelConfig
	S3Config   config.S3Config

	Logger   *slog.Logger
	Progress ProgressReporter // optional; nil uses noopReporter

	// ShouldStop is called between scenes to check for a graceful stop signal.
	// Return true to stop after the current scene completes. nil = never stop.
	ShouldStop func() bool
}

// Worker runs the scene processing pipeline.
type Worker struct {
	deps WorkerDeps
	mb   *processing.MultibandBuilder
	log  *slog.Logger
}

// SetProgress replaces the active ProgressReporter at runtime.
// Pass nil to revert to the no-op reporter.
func (w *Worker) SetProgress(r ProgressReporter) {
	if r == nil {
		w.deps.Progress = noopReporter{}
	} else {
		w.deps.Progress = r
	}
}

// shouldStop consumes the stop trigger and returns true if a graceful stop was
// requested. Consuming the trigger atomically prevents double-stop.
func (w *Worker) shouldStop() bool {
	if w.deps.ShouldStop == nil {
		return false
	}
	return w.deps.ShouldStop()
}

// New creates a Worker from deps.
func New(deps WorkerDeps) *Worker {
	if deps.Progress == nil {
		deps.Progress = noopReporter{}
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	mb := processing.New(deps.Executor, deps.S3, deps.Processing, logger)

	return &Worker{deps: deps, mb: mb, log: logger}
}

// outputFile describes one generated file pending upload/registration.
type outputFile struct {
	fileType domain.FileType
	path     string
	name     string
}

// ProcessScene runs the full processing pipeline for one scene, per the
// spec's Processing Worker Flow:
//
//  1. Fetch production and scene
//  2. Validate production (monitoring, bbox, not bloqueado)
//  3. Mark scene PROCESSING
//  4. Create JobDir (cleaned up on return)
//  5. Build multiband.tif from COG bands
//  6. Calculate cloud cover from SCL
//  7. Below threshold: full processing (compositions, indices, stats, params.json)
//  8. At/above threshold: natural.png only
//  9. Upload outputs to S3
//  10. Register files in the database
//  11. Mark scene COMPLETED
//
// Any error along the way is classified and stored on the scene record,
// which is marked FAILED with an incremented retry_count.
func (w *Worker) ProcessScene(ctx context.Context, produccionID int64, sceneID string) error {
	production, scene, err := w.fetchAndValidate(ctx, produccionID, sceneID)
	if err != nil {
		return err
	}

	if err := w.deps.Scenes.UpdateStatus(ctx, scene.ID, domain.StatusProcessing); err != nil {
		return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "updating scene status to PROCESSING", Wrapped: err}
	}

	if procErr := w.process(ctx, production, scene); procErr != nil {
		if setErr := w.deps.Scenes.SetFailed(ctx, scene.ID); setErr != nil {
			w.log.Error("failed to mark scene as failed", "scene_name", sceneID, "error", setErr)
		}
		return procErr
	}

	return nil
}

// ProcessProduction processes all scenes for the given production in
// chronological order, starting from the oldest PENDING scene:
//
//   - Scenes with status PENDING → full processing (TIF + images + params + ia_req)
//   - Scenes with status COMPLETED → only regenerate params + ia_req so the
//     historical chain stays correct after older scenes are (re)processed
//
// This guarantees that each scene's historical summary reflects the correct
// chain of previously processed scenes.
func (w *Worker) ProcessProduction(ctx context.Context, produccionID int64) error {
	production, err := w.deps.Productions.GetByProduccionID(ctx, produccionID)
	if err != nil {
		return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "fetching production", Wrapped: err}
	}
	if production == nil {
		return &domain.ProcessingError{Type: domain.ErrValidation, Message: fmt.Sprintf("production %d not found", produccionID)}
	}

	// Reset any PROCESSING scenes to PENDING — they were left stuck by a
	// previous run that was killed mid-flight. Safe here because the
	// per-production lock guarantees no other worker is active.
	if n, err := w.deps.Scenes.ResetProcessingToPending(ctx, production.ID); err != nil {
		return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "resetting stuck PROCESSING scenes", Wrapped: err}
	} else if n > 0 {
		w.log.Info("reset stuck PROCESSING scenes to PENDING", "produccion_id", produccionID, "count", n)
	}

	// Find the oldest pending scene — that's our starting point.
	oldest, err := w.deps.Scenes.GetOldestPendingByProduccion(ctx, production.ID)
	if err != nil {
		return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "fetching oldest pending scene", Wrapped: err}
	}
	if oldest == nil {
		w.log.Info("no pending scenes for production", "produccion_id", produccionID)
		return nil
	}

	fromDate := time.Time{}
	if oldest.Fecha != nil {
		fromDate = *oldest.Fecha
	}

	// Get all scenes from that date forward, ordered ASC so we process oldest→newest.
	scenes, err := w.deps.Scenes.ListFromDateByProduccion(ctx, production.ID, fromDate)
	if err != nil {
		return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "listing scenes from oldest pending", Wrapped: err}
	}

	w.log.Info("processing production scenes", "produccion_id", produccionID, "scene_count", len(scenes), "from_date", fromDate.Format("2006-01-02"))
	w.deps.Progress.SetTotal(len(scenes))
	w.deps.Progress.SetPhase("processing")

	for _, scene := range scenes {
		sceneName := scene.SceneName
		w.deps.Progress.SceneStarted(sceneName, produccionID)
		switch scene.Status {
		case domain.StatusCompleted:
			// Already fully processed — only regenerate params + ia_req so the
			// historical chain is correct with whatever new scenes were added before.
			w.log.Info("regenerating params+ia_req for completed scene", "scene_name", sceneName)
			if err := w.regenerateParamsAndIA(ctx, production, scene); err != nil {
				w.log.Error("params regeneration failed", "scene_name", sceneName, "error", err)
				w.deps.Progress.SceneFailed(sceneName, produccionID)
				// Non-fatal: continue with the next scene.
			} else {
				w.deps.Progress.SceneCompleted(sceneName, produccionID)
			}
		case domain.StatusFailed, domain.StatusPending:
			w.log.Info("full processing for scene", "scene_name", sceneName, "status", scene.Status)
			if err := w.ProcessScene(ctx, produccionID, sceneName); err != nil {
				w.log.Error("scene processing failed", "scene_name", sceneName, "error", err)
				w.deps.Progress.SceneFailed(sceneName, produccionID)
				// Non-fatal: continue so later scenes still get their historical updated.
			} else {
				w.deps.Progress.SceneCompleted(sceneName, produccionID)
			}
		default:
			w.log.Warn("skipping scene with unexpected status", "scene_name", sceneName, "status", scene.Status)
		}

		// Graceful stop: check after each scene so no work is left half-done.
		if w.shouldStop() {
			w.log.Info("graceful stop triggered — stopping after current scene", "scene_name", sceneName)
			return nil
		}
	}

	return nil
}

// RegenAllCompleted regenerates params.json + ia_req.json for every COMPLETED
// scene of the given production in strict chronological order (oldest first),
// so the historical chain is fully rebuilt from scratch without touching the
// multiband.tif or any image files. Useful after a params schema change.
func (w *Worker) RegenAllCompleted(ctx context.Context, produccionID int64) error {
	production, err := w.deps.Productions.GetByProduccionID(ctx, produccionID)
	if err != nil {
		return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "fetching production", Wrapped: err}
	}
	if production == nil {
		return &domain.ProcessingError{Type: domain.ErrValidation, Message: fmt.Sprintf("production %d not found", produccionID)}
	}

	scenes, err := w.deps.Scenes.ListByMonitoringProduccion(ctx, production.ID)
	if err != nil {
		return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "listing scenes", Wrapped: err}
	}

	// Keep only completed scenes. DB returns DESC; reverse to oldest → newest
	// so the historical chain builds correctly.
	var completed []*domain.Scene
	for _, s := range scenes {
		if s.Status == domain.StatusCompleted {
			completed = append(completed, s)
		}
	}
	for i, j := 0, len(completed)-1; i < j; i, j = i+1, j-1 {
		completed[i], completed[j] = completed[j], completed[i]
	}

	w.log.Info("regenerating params+ia_req for all completed scenes",
		"produccion_id", produccionID, "count", len(completed))
	w.deps.Progress.SetTotal(len(completed))
	w.deps.Progress.SetPhase("processing")

	for _, scene := range completed {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		w.log.Info("regen", "scene_name", scene.SceneName)
		w.deps.Progress.SceneStarted(scene.SceneName, produccionID)
		if err := w.regenerateParamsAndIA(ctx, production, scene); err != nil {
			w.log.Error("regen failed", "scene_name", scene.SceneName, "error", err)
			w.deps.Progress.SceneFailed(scene.SceneName, produccionID)
			// Non-fatal: continue so the chain is as complete as possible.
		} else {
			w.deps.Progress.SceneCompleted(scene.SceneName, produccionID)
		}
	}

	w.log.Info("regen complete", "produccion_id", produccionID)
	return nil
}

// regenerateParamsAndIA rebuilds multiband.params.json and multiband.ia_req.json
// for an already-completed scene using the existing multiband.tif in S3.
// It does NOT re-download COG bands or regenerate images.
func (w *Worker) regenerateParamsAndIA(ctx context.Context, production *domain.Production, scene *domain.Scene) error {
	baseDir := w.deps.Processing.TempDir
	if baseDir == "" {
		baseDir = os.TempDir()
	}
	jobID := fmt.Sprintf("regen_%d_%s", production.ProduccionID, scene.SceneName)
	jobDir := storage.New(baseDir, jobID)
	if err := jobDir.Create(); err != nil {
		return &domain.ProcessingError{Type: domain.ErrDisk, Message: "creating regen job directory", Wrapped: err}
	}
	defer func() {
		if err := jobDir.Cleanup(); err != nil {
			w.log.Warn("regen job directory cleanup failed", "job_id", jobID, "error", err)
		}
	}()

	// Download the existing multiband.tif from S3.
	bucket := w.deps.S3Config.Bucket
	multibandKey := strings.TrimRight(production.Prefix, "/") + "/" + scene.SceneName + "/multiband.tif"
	multibandPath := filepath.Join(jobDir.Root(), "multiband.tif")
	if err := w.deps.S3.Download(ctx, bucket, multibandKey, multibandPath); err != nil {
		return &domain.ProcessingError{Type: domain.ErrS3, Message: "downloading multiband.tif for regen", Wrapped: err}
	}

	polygonPath, err := processing.WritePolygonGeoJSON(production, jobDir.Work())
	if err != nil {
		return err
	}

	maskedPath, err := processing.MaskMultiband(ctx, w.deps.Executor, multibandPath, polygonPath, jobDir.Work())
	if err != nil {
		return err
	}

	bandStats, err := processing.CalculateBandStatistics(ctx, w.deps.Executor, maskedPath)
	if err != nil {
		return err
	}

	indices := processing.AllIndices()
	for _, idx := range indices {
		rawOut := filepath.Join(jobDir.Work(), string(idx.Type)+"_masked.png")
		if err := processing.GenerateIndex(ctx, w.deps.Executor, maskedPath, rawOut, idx.Type); err != nil {
			return err
		}
	}

	indexStats, err := processing.CalculateIndexStatistics(ctx, w.deps.Executor, maskedPath, indices)
	if err != nil {
		return err
	}

	// Reuse the stored cloud cover — no need to re-download the SCL band.
	cloudCoverPoly := 0.0
	if scene.ProductionCloud != nil {
		cloudCoverPoly = *scene.ProductionCloud
	}

	// Recover the original coverage from the existing params.json so regen
	// does not overwrite it with zeros (SCL is not re-downloaded on regen).
	existingCoverage := w.fetchExistingCoverage(ctx, production, scene)

	previousParams := w.fetchPreviousParams(ctx, production.ProduccionID, scene)

	var sceneDate, fechaPlantacion time.Time
	if scene.Fecha != nil {
		sceneDate = *scene.Fecha
	}
	if production.FechaPlantacion != nil {
		fechaPlantacion = *production.FechaPlantacion
	}

	params := processing.BuildParams(processing.ParamsInput{
		ProduccionID:    production.ProduccionID,
		SceneID:         scene.SceneName,
		SceneDate:       sceneDate,
		FechaPlantacion: fechaPlantacion,
		CloudCoverBBox:  cloudCoverPoly,
		Indices:         indexStats,
		BandStats:       bandStats,
		Coverage:        existingCoverage,
	}, previousParams)

	outDir := jobDir.Output()
	paramsPath := filepath.Join(outDir, "multiband.params.json")
	if err := writeJSON(paramsPath, params); err != nil {
		return &domain.ProcessingError{Type: domain.ErrDisk, Message: "writing regen params", Wrapped: err}
	}

	var outputs []outputFile
	outputs = append(outputs, outputFile{fileType: domain.FileParams, path: paramsPath, name: "multiband.params.json"})

	if iaReqFiles, iaErr := w.generateIARequest(ctx, production, scene, params, jobDir); iaErr != nil {
		w.log.Warn("failed to generate ia_req.json during regen", "scene_name", scene.SceneName, "error", iaErr)
	} else {
		outputs = append(outputs, iaReqFiles...)
	}

	return w.uploadAndRegister(ctx, production, scene, outputs)
}

// ProcessAllPending is the Mode 4 scheduler. It iterates all pending scenes
// across every monitored production in strict chronological order (oldest
// scene first) so that the historical params chain is always built correctly.
//
// For each scene_name it finds:
//   - Productions with a PENDING/FAILED scene → full processing
//   - Productions with a COMPLETED scene → params + ia_req regeneration
//     (because an earlier scene in the chain may have just been filled in)
//
// Productions that already have a multiband.tif are processed first so later
// productions can reuse it, minimising COG band downloads.
//
// The loop runs until all scenes are processed, then returns. The caller
// (main.go) is responsible for sleeping and calling again periodically.
func (w *Worker) ProcessAllPending(ctx context.Context) error {
	if n, err := w.deps.Scenes.ResetAllProcessingToPending(ctx); err != nil {
		return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "resetting stuck PROCESSING scenes", Wrapped: err}
	} else if n > 0 {
		w.log.Info("reset stuck PROCESSING scenes to PENDING", "count", n)
	}

	// Set the total so the frontend can render a deterministic progress bar.
	if total, err := w.deps.Scenes.CountScenesToProcess(ctx); err != nil {
		w.log.Warn("could not count scenes to process", "error", err)
	} else {
		w.deps.Progress.SetTotal(total)
	}

	for {
		// Find the globally oldest scene with at least one pending production.
		sceneName, _, err := w.deps.Scenes.FindOldestPendingSceneName(ctx)
		if err != nil {
			return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "finding oldest pending scene", Wrapped: err}
		}
		if sceneName == "" {
			w.log.Info("all scenes up to date — nothing pending")
			return nil
		}

		// Fetch all productions that have this scene, with already-generated
		// multibands first so reuse kicks in for subsequent productions.
		scenes, err := w.deps.Scenes.ListAllBySceneName(ctx, sceneName)
		if err != nil {
			return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "listing scenes by name: " + sceneName, Wrapped: err}
		}

		w.log.Info("processing scene across productions",
			"scene_name", sceneName,
			"production_count", len(scenes))

		for _, scene := range scenes {
			prod, err := w.deps.Productions.GetByMonitoringID(ctx, scene.MonitoringProduccionID)
			if err != nil || prod == nil {
				w.log.Warn("could not fetch production for scene",
					"scene_name", sceneName,
					"monitoring_produccion_id", scene.MonitoringProduccionID)
				continue
			}

			w.deps.Progress.SceneStarted(sceneName, prod.ProduccionID)
			switch scene.Status {
			case domain.StatusCompleted:
				w.log.Info("regenerating params+ia_req for completed scene",
					"scene_name", sceneName, "produccion_id", prod.ProduccionID)
				if err := w.regenerateParamsAndIA(ctx, prod, scene); err != nil {
					w.log.Error("regen failed", "scene_name", sceneName,
						"produccion_id", prod.ProduccionID, "error", err)
					w.deps.Progress.SceneFailed(sceneName, prod.ProduccionID)
				} else {
					w.deps.Progress.SceneCompleted(sceneName, prod.ProduccionID)
				}
			case domain.StatusPending, domain.StatusFailed:
				w.log.Info("full processing",
					"scene_name", sceneName, "produccion_id", prod.ProduccionID)
				if err := w.ProcessScene(ctx, prod.ProduccionID, sceneName); err != nil {
					w.log.Error("processing failed", "scene_name", sceneName,
						"produccion_id", prod.ProduccionID, "error", err)
					w.deps.Progress.SceneFailed(sceneName, prod.ProduccionID)
				} else {
					w.deps.Progress.SceneCompleted(sceneName, prod.ProduccionID)
				}
			default:
				// PROCESSING — skip, otra instancia del worker lo tiene.
			}

			if ctx.Err() != nil {
				return ctx.Err()
			}

			// Graceful stop: check after each scene-batch so no work is left half-done.
			if w.shouldStop() {
				w.log.Info("graceful stop triggered — stopping after current scene batch", "scene_name", sceneName)
				return nil
			}
		}
	}
}

func (w *Worker) fetchAndValidate(ctx context.Context, produccionID int64, sceneID string) (*domain.Production, *domain.Scene, error) {
	production, err := w.deps.Productions.GetByProduccionID(ctx, produccionID)
	if err != nil {
		return nil, nil, &domain.ProcessingError{Type: domain.ErrMySQL, Message: "fetching production", Wrapped: err}
	}
	if production == nil {
		return nil, nil, &domain.ProcessingError{Type: domain.ErrValidation, Message: fmt.Sprintf("production %d not found", produccionID)}
	}

	scene, err := w.deps.Scenes.GetByProduccionAndSceneName(ctx, produccionID, sceneID)
	if err != nil {
		return nil, nil, &domain.ProcessingError{Type: domain.ErrMySQL, Message: "fetching scene", Wrapped: err}
	}
	if scene == nil {
		return nil, nil, &domain.ProcessingError{Type: domain.ErrValidation, Message: fmt.Sprintf("scene %s not found for production %d", sceneID, produccionID)}
	}

	if !production.ShouldProcess() {
		reason := "production is not eligible for processing"
		switch {
		case !production.Monitoring:
			reason = "production has monitoring=false"
		case production.Bloqueado:
			reason = "production is bloqueado"
		}
		if setErr := w.deps.Scenes.SetFailed(ctx, scene.ID); setErr != nil {
			w.log.Error("failed to mark scene failed on validation", "scene_name", sceneID, "error", setErr)
		}
		return nil, nil, &domain.ProcessingError{Type: domain.ErrValidation, Message: reason}
	}

	return production, scene, nil
}

// process runs steps 4-11 of the pipeline (jobdir creation through
// completion), assuming production/scene have already been validated and
// the scene marked PROCESSING.
func (w *Worker) process(ctx context.Context, production *domain.Production, scene *domain.Scene) error {
	baseDir := w.deps.Processing.TempDir
	if baseDir == "" {
		baseDir = os.TempDir()
	}
	jobID := fmt.Sprintf("%d_%s", production.ProduccionID, scene.SceneName)
	jobDir := storage.New(baseDir, jobID)

	if err := jobDir.Create(); err != nil {
		return &domain.ProcessingError{Type: domain.ErrDisk, Message: "creating job directory", Wrapped: err}
	}
	defer func() {
		if err := jobDir.Cleanup(); err != nil {
			w.log.Warn("job directory cleanup failed", "job_id", jobID, "error", err)
		}
	}()

	targetResolution := w.deps.Processing.TargetResolution
	if targetResolution <= 0 {
		targetResolution = 10
	}

	// tile_bbox is the download window — a fixed-size square around the field
	// center. pbox is not used for processing; the polygon mask handles stats.
	tileBBox := production.ParseTileBBox()
	if tileBBox == nil {
		return &domain.ProcessingError{Type: domain.ErrValidation, Message: "production has no tile_bbox — run sync first"}
	}

	// Write the production polygon to a temp GeoJSON for GDAL cutline use.
	// Cloud cover and statistics are measured only within the polygon.
	polygonPath, err := processing.WritePolygonGeoJSON(production, jobDir.Work())
	if err != nil {
		return err
	}

	// ResolveBands resolves remote COG URLs — needed for SCL (cloud cover) and
	// for building the multiband if no reusable source is found.
	bands, sclHref, err := w.deps.Bands.ResolveBands(ctx, production.ProduccionID, scene.SceneName)
	if err != nil {
		return err
	}

	// --- Multiband reuse: check if another production already downloaded a
	//     multiband.tif whose tile_bbox contains our polygon_bbox. ---
	var multibandRefID *uint64
	// imageExtent is the real geographic extent of the multiband, and therefore
	// of every image derived from it: this production's tile normally, or the
	// origin production's tile when the raster was reused from another one.
	multibandPath, multibandRefID, imageExtent := w.tryReuseMultiband(ctx, production, scene, jobDir)
	if imageExtent == nil {
		imageExtent = production.ParseTileBBox()
	}

	if multibandPath == "" {
		// No reusable multiband found — build from COG bands.
		// Append SCL as band 11 so the multiband carries scene classification
		// alongside the 10 spectral bands (B02..B12).
		bandsWithSCL := append(bands, domain.BandInfo{
			Name:       domain.BandSCL,
			Resolution: domain.BandSCL.Resolution(),
			Href:       sclHref,
		})
		multibandPath, err = w.mb.Build(ctx, jobDir.Root(), *tileBBox, bandsWithSCL, targetResolution)
		if err != nil {
			return err
		}
	}

	cloudCoverPoly, coverage, err := processing.CalculateCloudCover(ctx, w.deps.Executor, sclHref, *tileBBox, jobDir.Work(), polygonPath)
	if err != nil {
		var procErr *domain.ProcessingError
		if errors.As(err, &procErr) && procErr.Type == domain.ErrNoData {
			// SCL has no valid pixels — sensor did not acquire data for this area.
			// Still generate natural.png (multiband is already built) and upload
			// both so the scene is visually browsable. Mark production_cloud=101
			// (distinct from 100% cloud) and COMPLETED so it is never retried.
			const noDataCloud = 101.0
			w.log.Warn("scene has no valid SCL pixels — generating natural.png and marking nodata (101)",
				"scene_name", scene.SceneName, "produccion_id", production.ProduccionID)

			noDataOutputs := []outputFile{
				{fileType: domain.FileMultiband, path: multibandPath, name: "multiband.tif"},
			}
			if naturalPath, natErr := w.generateNaturalOnly(ctx, multibandPath, jobDir, imageExtent); natErr != nil {
				w.log.Warn("failed to generate natural.png for nodata scene", "scene_name", scene.SceneName, "error", natErr)
			} else {
				noDataOutputs = append(noDataOutputs, outputFile{fileType: domain.FileNatural, path: naturalPath, name: "natural.png"})
			}
			if uploadErr := w.uploadAndRegister(ctx, production, scene, noDataOutputs); uploadErr != nil {
				return uploadErr
			}

			noData := noDataCloud
			finalScene := *scene
			finalScene.Status = domain.StatusCompleted
			finalScene.Usable = false
			finalScene.ProductionCloud = &noData
			finalScene.TruthTifExists = true
			finalScene.RenderTifExists = true
			now := time.Now().UTC()
			finalScene.UltimaSincronizacion = &now
			if upsertErr := w.deps.Scenes.Upsert(ctx, &finalScene); upsertErr != nil {
				return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "marking nodata scene completed", Wrapped: upsertErr}
			}
			return nil
		}
		return err
	}

	outputs := []outputFile{
		{fileType: domain.FileMultiband, path: multibandPath, name: "multiband.tif"},
	}

	threshold := w.deps.Sentinel.CloudCoverProductionMax
	if threshold <= 0 {
		threshold = cloudCoverThreshold
	}
	passesQuality := cloudCoverPoly <= threshold

	var params *processing.Params

	if passesQuality {
		imgOutputs, err := w.generateFullOutputs(ctx, production, scene, multibandPath, jobDir, cloudCoverPoly, coverage, polygonPath, imageExtent)
		if err != nil {
			return err
		}
		outputs = append(outputs, imgOutputs.files...)
		params = imgOutputs.params

		// Generate ia_req.json — the compact payload ready to send to the IA
		// service. The IA analysis itself is a separate process.
		if iaReqFiles, iaErr := w.generateIARequest(ctx, production, scene, params, jobDir); iaErr != nil {
			w.log.Warn("failed to generate ia_req.json", "scene_name", scene.SceneName, "error", iaErr)
		} else {
			outputs = append(outputs, iaReqFiles...)
		}
	} else {
		naturalPath, err := w.generateNaturalOnly(ctx, multibandPath, jobDir, imageExtent)
		if err != nil {
			return err
		}
		outputs = append(outputs, outputFile{fileType: domain.FileNatural, path: naturalPath, name: "natural.png"})
	}

	if err := w.uploadAndRegister(ctx, production, scene, outputs); err != nil {
		return err
	}

	finalScene := *scene
	finalScene.Status = domain.StatusCompleted
	finalScene.Usable = passesQuality
	finalScene.ProductionCloud = &cloudCoverPoly
	finalScene.TruthTifExists = true
	finalScene.ParamsExists = params != nil
	finalScene.RenderTifExists = true
	finalScene.MultibandRefEscenaID = multibandRefID
	// Cuando el multiband viene de otra producción, las imágenes heredan SU
	// extensión. Se guarda en la escena para que la georreferencia no dependa
	// de que esa producción siga existiendo.
	if multibandRefID != nil && imageExtent != nil {
		finalScene.ImageBBox = imageExtent.MarshalJSONColumn()
	}
	now := time.Now().UTC()
	finalScene.UltimaSincronizacion = &now

	if err := w.deps.Scenes.Upsert(ctx, &finalScene); err != nil {
		return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "marking scene completed", Wrapped: err}
	}

	w.maybeRunIA(ctx, production, &finalScene)

	return nil
}

// maybeRunIA triggers IA analysis when production.IAuto=true, scene is usable,
// ia_req.json exists, and an IAAnalyzer is configured.
// Currently runs in dry-run mode — set dryRun=false to persist results.
func (w *Worker) maybeRunIA(ctx context.Context, production *domain.Production, scene *domain.Scene) {
	const dryRun = true

	if !production.IAuto || !scene.Usable || w.deps.IAAnalyzer == nil {
		return
	}

	iaReqFile, err := w.deps.Files.GetByTipo(ctx, scene.ID, string(domain.FileIAReq))
	if err != nil || iaReqFile == nil {
		w.log.Info("ia_auto skipped: ia_req.json not found", "scene_id", scene.ID, "scene_name", scene.SceneName)
		return
	}

	s3Key := strings.TrimRight(production.Prefix, "/") + "/" + scene.SceneName + "/multiband.ia_req.json"

	if dryRun {
		if err := w.deps.IAAnalyzer.DryRunRaw(ctx, s3Key); err != nil {
			w.log.Error("ia_auto dry-run failed", "scene_id", scene.ID, "scene_name", scene.SceneName, "error", err)
		} else {
			w.log.Info("ia_auto dry-run ok", "scene_id", scene.ID, "scene_name", scene.SceneName)
		}
		return
	}

	result, err := w.deps.IAAnalyzer.Analyze(ctx, scene.ID, s3Key, production.ProduccionID)
	if err != nil {
		w.log.Error("ia_auto analysis failed", "scene_id", scene.ID, "scene_name", scene.SceneName, "error", err)
		return
	}
	w.log.Info("ia_auto analysis completed", "scene_id", scene.ID, "scene_name", scene.SceneName, "estado", result.EstadoGeneral)
}

// fullOutputs bundles the generated image files and the params document for
// a fully-processed (below cloud-cover-threshold) scene.
type fullOutputs struct {
	files  []outputFile
	params *processing.Params
}

func (w *Worker) generateFullOutputs(ctx context.Context, production *domain.Production, scene *domain.Scene, multibandPath string, jobDir *storage.JobDir, cloudCoverPoly float64, coverage processing.CoverageStats, polygonPath string, imageExtent *domain.BBox) (*fullOutputs, error) {
	outDir := jobDir.Output()
	var files []outputFile

	// Images for display are built from a WGS84 copy clipped to imageExtent —
	// the multiband's real extent, which is the origin production's tile when
	// the raster was reused. The map positions them with that same rectangle,
	// so they line up exactly. Statistics below keep using the UTM multiband.
	vizPath := multibandPath
	if imageExtent != nil {
		warped := filepath.Join(jobDir.Work(), "multiband_wgs84.tif")
		if err := processing.WarpToWGS84(ctx, w.deps.Executor, multibandPath, warped, *imageExtent); err != nil {
			return nil, err
		}
		vizPath = warped
	} else {
		w.log.Warn("production has no tile_bbox — images stay in UTM and will be slightly offset on the map",
			"produccion_id", production.ProduccionID)
	}

	// PNG images are generated from the full tile (no polygon mask) so they
	// can be overlaid on maps at their natural extent. Each PNG gets:
	//   1. 4× upscale with lanczos (smoother, no data change)
	//   2. Polygon outline burned in yellow
	for _, comp := range processing.AllCompositions() {
		outPath := filepath.Join(outDir, string(comp.Type)+".png")
		red := bandNumber(comp.RedBand)
		green := bandNumber(comp.GreenBand)
		blue := bandNumber(comp.BlueBand)
		if err := processing.GenerateRGB(ctx, w.deps.Executor, vizPath, outPath, red, green, blue); err != nil {
			return nil, err
		}
		if err := processing.OverlayPolygon(ctx, w.deps.Executor, outPath, polygonPath, outPath); err != nil {
			w.log.Warn("polygon overlay failed", "file", comp.Type, "error", err)
		}
		files = append(files, outputFile{fileType: comp.Type, path: outPath, name: string(comp.Type) + ".png"})
	}

	indices := processing.AllIndices()
	for _, idx := range indices {
		outPath := filepath.Join(outDir, string(idx.Type)+".png")
		if err := processing.GenerateIndex(ctx, w.deps.Executor, vizPath, outPath, idx.Type); err != nil {
			return nil, err
		}
		if err := processing.OverlayPolygon(ctx, w.deps.Executor, outPath, polygonPath, outPath); err != nil {
			w.log.Warn("polygon overlay failed", "file", idx.Type, "error", err)
		}
		files = append(files, outputFile{fileType: idx.Type, path: outPath, name: string(idx.Type) + ".png"})
	}

	// Band and index statistics are computed on polygon-masked rasters so the
	// numbers represent only pixels inside the production, not the full tile.
	//
	// maskedMultiband is used for band stats (pixels inside the polygon).
	// For index stats we need masked raw tifs alongside the masked multiband,
	// so we regenerate them from maskedMultiband into work/ and pass that path
	// to CalculateIndexStatistics (which derives raw paths from the dir).
	maskedPath, err := processing.MaskMultiband(ctx, w.deps.Executor, multibandPath, polygonPath, jobDir.Work())
	if err != nil {
		return nil, err
	}

	bandStats, err := processing.CalculateBandStatistics(ctx, w.deps.Executor, maskedPath)
	if err != nil {
		return nil, err
	}

	// Generate masked raw index tifs into work/ for statistics.
	for _, idx := range indices {
		rawOut := filepath.Join(jobDir.Work(), string(idx.Type)+"_masked.png")
		if err := processing.GenerateIndex(ctx, w.deps.Executor, maskedPath, rawOut, idx.Type); err != nil {
			return nil, err
		}
	}

	// CalculateIndexStatistics derives raw tif paths from maskedPath's dir.
	indexStats, err := processing.CalculateIndexStatistics(ctx, w.deps.Executor, maskedPath, indices)
	if err != nil {
		return nil, err
	}

	previousParams := w.fetchPreviousParams(ctx, production.ProduccionID, scene)

	var fechaPlantacion time.Time
	if production.FechaPlantacion != nil {
		fechaPlantacion = *production.FechaPlantacion
	}

	var sceneDate time.Time
	if scene.Fecha != nil {
		sceneDate = *scene.Fecha
	}

	params := processing.BuildParams(processing.ParamsInput{
		ProduccionID:    production.ProduccionID,
		SceneID:         scene.SceneName,
		SceneDate:       sceneDate,
		FechaPlantacion: fechaPlantacion,
		CloudCoverBBox:  cloudCoverPoly,
		Indices:         indexStats,
		BandStats:       bandStats,
		Coverage:        coverage,
	}, previousParams)

	paramsPath := filepath.Join(outDir, "multiband.params.json")
	if err := writeJSON(paramsPath, params); err != nil {
		return nil, &domain.ProcessingError{Type: domain.ErrDisk, Message: "writing multiband.params.json", Wrapped: err}
	}
	files = append(files, outputFile{fileType: domain.FileParams, path: paramsPath, name: "multiband.params.json"})

	return &fullOutputs{files: files, params: params}, nil
}

func (w *Worker) generateNaturalOnly(ctx context.Context, multibandPath string, jobDir *storage.JobDir, imageExtent *domain.BBox) (string, error) {
	// Same WGS84 reprojection as generateFullOutputs so the cloudy-scene
	// preview lines up on the map too.
	vizPath := multibandPath
	if imageExtent != nil {
		warped := filepath.Join(jobDir.Work(), "multiband_wgs84.tif")
		if err := processing.WarpToWGS84(ctx, w.deps.Executor, multibandPath, warped, *imageExtent); err != nil {
			return "", err
		}
		vizPath = warped
	}

	outPath := filepath.Join(jobDir.Output(), string(domain.FileNatural)+".png")
	if err := processing.GenerateRGB(ctx, w.deps.Executor, vizPath, outPath,
		bandNumber(domain.BandB04), bandNumber(domain.BandB03), bandNumber(domain.BandB02)); err != nil {
		return "", err
	}
	return outPath, nil
}

// fetchPreviousParams looks up the most recent quality-passing scene before
// this one and, if it has a params.json registered, downloads and parses it
// to seed the historical chain. Any failure along the way is logged and
// treated as "no previous params" rather than failing the pipeline.
func (w *Worker) fetchPreviousParams(ctx context.Context, produccionID int64, scene *domain.Scene) *processing.Params {
	var beforeDate time.Time
	if scene.Fecha != nil {
		beforeDate = *scene.Fecha
	}
	prevScene, err := w.deps.Scenes.GetPreviousUsableScene(ctx, scene.MonitoringProduccionID, beforeDate)
	if err != nil {
		w.log.Warn("failed to look up previous usable scene", "scene_name", scene.SceneName, "error", err)
		return nil
	}
	if prevScene == nil {
		return nil
	}

	prevFile, err := w.deps.Files.GetByTipo(ctx, prevScene.ID, string(domain.FileParams))
	if err != nil {
		w.log.Warn("failed to look up previous params file", "scene_name", scene.SceneName, "error", err)
		return nil
	}
	if prevFile == nil {
		return nil
	}

	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("prev-params-%d-%s.json", produccionID, prevScene.SceneName))
	defer os.Remove(tmpPath)

	if err := w.deps.S3.Download(ctx, w.deps.S3Config.Bucket, prevFile.S3Key, tmpPath); err != nil {
		w.log.Warn("failed to download previous params.json", "scene_name", scene.SceneName, "error", err)
		return nil
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		w.log.Warn("failed to read downloaded previous params.json", "scene_name", scene.SceneName, "error", err)
		return nil
	}

	var previous processing.Params
	if err := json.Unmarshal(data, &previous); err != nil {
		w.log.Warn("failed to parse previous params.json", "scene_name", scene.SceneName, "error", err)
		return nil
	}

	return &previous
}

// fetchExistingCoverage downloads the current params.json for a scene from S3
// and returns its coverage stats so regen preserves the original SCL-derived
// values. Returns zero-value CoverageStats on any error (non-fatal).
func (w *Worker) fetchExistingCoverage(ctx context.Context, production *domain.Production, scene *domain.Scene) processing.CoverageStats {
	paramsFile, err := w.deps.Files.GetByTipo(ctx, scene.ID, string(domain.FileParams))
	if err != nil || paramsFile == nil {
		return processing.CoverageStats{}
	}
	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("existing-params-%d-%s.json", production.ProduccionID, scene.SceneName))
	defer os.Remove(tmpPath)
	if err := w.deps.S3.Download(ctx, w.deps.S3Config.Bucket, paramsFile.S3Key, tmpPath); err != nil {
		return processing.CoverageStats{}
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return processing.CoverageStats{}
	}
	var p processing.Params
	if err := json.Unmarshal(data, &p); err != nil {
		return processing.CoverageStats{}
	}
	return p.Coverage
}

// generateIARequest builds the ia_req.json payload (compact, IA-ready) and
// returns it as an outputFile slice so it can be uploaded alongside the other
// scene outputs. Errors are non-fatal — the scene completes without ia_req.json.
func (w *Worker) generateIARequest(ctx context.Context, production *domain.Production, scene *domain.Scene, params *processing.Params, jobDir *storage.JobDir) ([]outputFile, error) {
	// Fetch the previous IA analysis for this scene, if any.
	var ultimoAnalisis *domain.IAResultSummary
	if w.deps.IAResults != nil {
		if prev, err := w.deps.IAResults.GetByEscenaID(ctx, scene.ID); err != nil {
			w.log.Warn("failed to fetch ultimo_analisis for ia_req", "scene_name", scene.SceneName, "error", err)
		} else {
			ultimoAnalisis = prev
		}
	}

	// Enrich production with folio and rancho from ERP tables (best-effort).
	enriched := *production
	if folio, rancho, erpErr := w.deps.Productions.GetERPFolioRancho(ctx, production.ProduccionID); erpErr == nil {
		enriched.Folio = folio
		enriched.Rancho = rancho
	}

	req := processing.BuildIARequest(processing.IARequestInput{
		Production:     &enriched,
		Scene:          scene,
		Params:         params,
		FullHistorico:  params.Historico,
		UltimoAnalisis: ultimoAnalisis,
	})

	data, err := processing.MarshalIARequest(req)
	if err != nil {
		return nil, &domain.ProcessingError{Type: domain.ErrDisk, Message: "marshalling ia_req.json", Wrapped: err}
	}

	iaReqPath := filepath.Join(jobDir.Output(), "multiband.ia_req.json")
	if err := os.WriteFile(iaReqPath, data, 0o644); err != nil {
		return nil, &domain.ProcessingError{Type: domain.ErrDisk, Message: "writing multiband.ia_req.json", Wrapped: err}
	}

	return []outputFile{
		{fileType: domain.FileIAReq, path: iaReqPath, name: "multiband.ia_req.json"},
	}, nil
}

func (w *Worker) uploadAndRegister(ctx context.Context, production *domain.Production, scene *domain.Scene, outputs []outputFile) error {
	bucket := w.deps.S3Config.Bucket

	for _, out := range outputs {
		// Use the production's own prefix so the S3 path matches the sync
		// index structure: {production.Prefix}/{scene.SceneName}/{filename}
		key := strings.TrimRight(production.Prefix, "/") + "/" + scene.SceneName + "/" + out.name

		if err := w.deps.S3.Upload(ctx, bucket, key, out.path); err != nil {
			return err
		}

		var size int64
		if info, statErr := os.Stat(out.path); statErr == nil {
			size = info.Size()
		}

		ext := strings.TrimPrefix(filepath.Ext(out.name), ".")

		uri := "s3://" + bucket + "/" + key
		sf := &domain.SceneFile{
			EscenaID:  scene.ID,
			Tipo:      dbTipo(out.fileType),
			S3Key:     key,
			S3Uri:     uri,
			Extension: ext,
			SizeBytes: size,
			Existe:    true,
		}

		// For JSON outputs, store the file content in json_content.
		if ext == "json" {
			if data, readErr := os.ReadFile(out.path); readErr == nil {
				sf.JsonContent = string(data)
			}
		}

		if err := w.deps.Files.Create(ctx, sf); err != nil {
			return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "registering scene file: " + out.name, Wrapped: err}
		}
	}

	return nil
}

// tryReuseMultiband checks if any other production already has a multiband.tif
// for this scene_name whose tile_bbox fully contains this production's polygon_bbox.
// If found, it downloads the multiband to jobDir and returns its local path, the
// origin escena_id, and the origin production's tile_bbox — which is the actual
// geographic extent of the downloaded raster, and therefore the extent the
// generated images must carry. Returns ("", nil, nil) when no source exists.
func (w *Worker) tryReuseMultiband(ctx context.Context, production *domain.Production, scene *domain.Scene, jobDir *storage.JobDir) (path string, refID *uint64, srcTile *domain.BBox) {
	polyBBox := production.ParsePolygonBBox()
	if polyBBox == nil {
		// Fallback: pbox is the same concept populated by older sync versions.
		polyBBox = production.ParsePBox()
	}
	if polyBBox == nil {
		return "", nil, nil
	}

	sources, err := w.deps.Scenes.FindMultibandSources(ctx, scene.SceneName, production.ProduccionID)
	if err != nil {
		w.log.Warn("failed to query multiband sources", "scene_name", scene.SceneName, "error", err)
		return "", nil, nil
	}

	for _, src := range sources {
		if !bboxContains(src.TileBBox, *polyBBox) {

			continue
		}

		destPath := filepath.Join(jobDir.Root(), "multiband.tif")
		if dlErr := w.deps.S3.Download(ctx, w.deps.S3Config.Bucket, src.MultibandKey, destPath); dlErr != nil {
			w.log.Warn("failed to download reuse multiband", "key", src.MultibandKey, "error", dlErr)
			continue
		}

		w.log.Info("reusing multiband from source escena", "scene_name", scene.SceneName, "source_escena_id", src.EscenaID, "key", src.MultibandKey)
		id := src.EscenaID
		tile := src.TileBBox
		return destPath, &id, &tile
	}

	return "", nil, nil
}

// bboxContains returns true when outer fully contains inner (inclusive edges).
func bboxContains(outer, inner domain.BBox) bool {
	return outer.MinX <= inner.MinX &&
		outer.MinY <= inner.MinY &&
		outer.MaxX >= inner.MaxX &&
		outer.MaxY >= inner.MaxY
}

// bandNumber returns the 1-based band index of b within multiband.tif,
// matching the fixed order documented in processing/multiband.go.
func bandNumber(b domain.Band) int {
	for i, sb := range domain.AllSpectralBands() {
		if sb == b {
			return i + 1
		}
	}
	return 0
}

// asProcessingError coerces any error into a *domain.ProcessingError,
// wrapping unclassified errors as VALIDATION_ERROR.
func asProcessingError(err error) *domain.ProcessingError {
	var perr *domain.ProcessingError
	if errors.As(err, &perr) {
		return perr
	}
	return &domain.ProcessingError{Type: domain.ErrValidation, Message: "unclassified processing error", Wrapped: err}
}

// dbTipo maps internal FileType constants to the canonical tipo values stored
// in s3_monitoring_escena_archivos. All PNG images (indices + compositions)
// share the "image" tipo; other files use their specific type string.
func dbTipo(ft domain.FileType) string {
	switch ft {
	case domain.FileMultiband:
		return "truth_tif"
	case domain.FileNatural, domain.FileFalseColor, domain.FileRedEdge, domain.FileSWIR,
		domain.FileNDVI, domain.FileNDRE, domain.FileEVI, domain.FileGNDVI,
		domain.FileNBR, domain.FileNDMI, domain.FileSAVI:
		return "image"
	case domain.FileParams:
		return "params"
	case domain.FileIAReq:
		return "ia_req"
	case domain.FileIAResult, domain.FileAnalisis:
		return "ia"
	default:
		return string(ft)
	}
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
