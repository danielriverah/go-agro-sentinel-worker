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
	"time"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/processing"
	"agro-sentinel-worker/internal/storage"
)

// cloudCoverThreshold is the SCL cloud-cover-over-bbox percentage below
// which a scene gets full processing (all compositions, indices, stats and
// params.json). At or above it, only natural.png is generated.
const cloudCoverThreshold = 23.0

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
	SetBloqueado(ctx context.Context, produccionID int64, motivo string) error
}

// SceneRepository is the subset of database.SceneRepo the worker needs.
type SceneRepository interface {
	GetByProduccionAndSceneID(ctx context.Context, produccionID int64, sceneID string) (*domain.Scene, error)
	UpdateStatus(ctx context.Context, id int64, status domain.JobStatus) error
	SetError(ctx context.Context, id int64, errType string, errMsg string) error
	Upsert(ctx context.Context, s *domain.Scene) error
	GetPreviousValidScene(ctx context.Context, produccionID int64, beforeDate time.Time) (*domain.Scene, error)
}

// FileRepository is the subset of database.FileRepo the worker needs.
type FileRepository interface {
	Create(ctx context.Context, f *domain.SceneFile) error
	GetByType(ctx context.Context, escenaID int64, fileType domain.FileType) (*domain.SceneFile, error)
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

// IAClient analyzes a fully-processed scene's outputs and produces an
// analisis.json document. It is optional: Task 13 provides the real
// implementation. IA_ERROR is treated as a partial failure — the scene still
// completes without analisis.json.
type IAClient interface {
	Analyze(ctx context.Context, input IAInput) (*domain.AnalysisResult, error)
}

// IAInput carries what the IA client needs to analyze a processed scene.
type IAInput struct {
	ProduccionID  int64
	SceneID       string
	MultibandPath string
	Params        *processing.Params
}

// WorkerDeps bundles all dependencies ProcessScene needs. Every dependency
// is expressed as a local interface so tests can supply mocks.
type WorkerDeps struct {
	Productions ProductionRepository
	Scenes      SceneRepository
	Files       FileRepository
	S3          S3Client
	Executor    GDALExecutor
	Bands       BandResolver
	IA          IAClient // optional; nil skips analysis

	Processing config.ProcessingConfig
	Sentinel   config.SentinelConfig
	S3Config   config.S3Config

	Logger *slog.Logger
}

// Worker runs the scene processing pipeline.
type Worker struct {
	deps WorkerDeps
	mb   *processing.MultibandBuilder
	log  *slog.Logger
}

// New creates a Worker from deps.
func New(deps WorkerDeps) *Worker {
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
		perr := asProcessingError(procErr)
		if setErr := w.deps.Scenes.SetError(ctx, scene.ID, string(perr.Type), perr.Error()); setErr != nil {
			w.log.Error("failed to record scene error", "scene_id", sceneID, "error", setErr)
		}
		return procErr
	}

	return nil
}

func (w *Worker) fetchAndValidate(ctx context.Context, produccionID int64, sceneID string) (*domain.Production, *domain.Scene, error) {
	production, err := w.deps.Productions.GetByProduccionID(ctx, produccionID)
	if err != nil {
		return nil, nil, &domain.ProcessingError{Type: domain.ErrMySQL, Message: "fetching production", Wrapped: err}
	}
	if production == nil {
		return nil, nil, &domain.ProcessingError{Type: domain.ErrValidation, Message: fmt.Sprintf("production %d not found", produccionID)}
	}

	scene, err := w.deps.Scenes.GetByProduccionAndSceneID(ctx, produccionID, sceneID)
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
		case production.BBox == nil:
			reason = "production has no bbox"
		}
		if setErr := w.deps.Scenes.SetError(ctx, scene.ID, string(domain.ErrValidation), reason); setErr != nil {
			w.log.Error("failed to record scene validation error", "scene_id", sceneID, "error", setErr)
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
	jobID := fmt.Sprintf("%d_%s", production.ProduccionID, scene.SceneID)
	jobDir := storage.New(baseDir, jobID)

	if err := jobDir.Create(); err != nil {
		return &domain.ProcessingError{Type: domain.ErrDisk, Message: "creating job directory", Wrapped: err}
	}
	defer func() {
		if err := jobDir.Cleanup(); err != nil {
			w.log.Warn("job directory cleanup failed", "job_id", jobID, "error", err)
		}
	}()

	bands, sclHref, err := w.deps.Bands.ResolveBands(ctx, production.ProduccionID, scene.SceneID)
	if err != nil {
		return err
	}

	targetResolution := production.TargetResolution
	if targetResolution <= 0 {
		targetResolution = w.deps.Processing.TargetResolution
	}
	if targetResolution <= 0 {
		targetResolution = 10
	}

	bbox := *production.BBox

	multibandPath, err := w.mb.Build(ctx, jobDir.Root(), bbox, bands, targetResolution)
	if err != nil {
		return err
	}

	cloudCoverBBox, coverage, err := processing.CalculateCloudCover(ctx, w.deps.Executor, sclHref, bbox, jobDir.Work())
	if err != nil {
		return err
	}

	outputs := []outputFile{
		{fileType: domain.FileMultiband, path: multibandPath, name: "multiband.tif"},
	}

	threshold := w.deps.Sentinel.CloudCoverProductionMax
	if threshold <= 0 {
		threshold = cloudCoverThreshold
	}
	passesQuality := cloudCoverBBox <= threshold

	var iaAnalysis *domain.AnalysisResult
	var params *processing.Params

	if passesQuality {
		imgOutputs, err := w.generateFullOutputs(ctx, production, scene, multibandPath, jobDir, cloudCoverBBox, coverage)
		if err != nil {
			return err
		}
		outputs = append(outputs, imgOutputs.files...)
		params = imgOutputs.params

		iaAnalysis = w.runIA(ctx, production, scene, multibandPath, params)
		if iaAnalysis != nil {
			analisisPath := filepath.Join(jobDir.Output(), "analisis.json")
			if err := writeJSON(analisisPath, iaAnalysis); err != nil {
				w.log.Error("failed to write analisis.json", "scene_id", scene.SceneID, "error", err)
				iaAnalysis = nil
			} else {
				outputs = append(outputs, outputFile{fileType: domain.FileAnalisis, path: analisisPath, name: "analisis.json"})
			}

			if iaAnalysis != nil && iaAnalysis.PosibleCosecha {
				if err := w.deps.Productions.SetBloqueado(ctx, production.ProduccionID, "posible_cosecha detectada por IA"); err != nil {
					w.log.Error("failed to block production after posible_cosecha detection", "produccion_id", production.ProduccionID, "scene_id", scene.SceneID, "error", err)
				}
			}
		}
	} else {
		naturalPath, err := w.generateNaturalOnly(ctx, multibandPath, jobDir)
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
	finalScene.PassesQuality = passesQuality
	finalScene.CloudCoverBBox = &cloudCoverBBox
	finalScene.HasMultiband = true
	finalScene.HasParams = params != nil
	finalScene.HasRGB = true
	finalScene.HasAnalisis = iaAnalysis != nil
	finalScene.ErrorType = ""
	finalScene.ErrorMessage = ""
	now := time.Now().UTC()
	finalScene.ProcessedAt = &now

	if err := w.deps.Scenes.Upsert(ctx, &finalScene); err != nil {
		return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "marking scene completed", Wrapped: err}
	}

	return nil
}

// fullOutputs bundles the generated image files and the params document for
// a fully-processed (below cloud-cover-threshold) scene.
type fullOutputs struct {
	files  []outputFile
	params *processing.Params
}

func (w *Worker) generateFullOutputs(ctx context.Context, production *domain.Production, scene *domain.Scene, multibandPath string, jobDir *storage.JobDir, cloudCoverBBox float64, coverage processing.CoverageStats) (*fullOutputs, error) {
	outDir := jobDir.Output()
	var files []outputFile

	for _, comp := range processing.AllCompositions() {
		outPath := filepath.Join(outDir, string(comp.Type)+".png")
		red := bandNumber(comp.RedBand)
		green := bandNumber(comp.GreenBand)
		blue := bandNumber(comp.BlueBand)
		if err := processing.GenerateRGB(ctx, w.deps.Executor, multibandPath, outPath, red, green, blue); err != nil {
			return nil, err
		}
		files = append(files, outputFile{fileType: comp.Type, path: outPath, name: string(comp.Type) + ".png"})
	}

	indices := processing.AllIndices()
	for _, idx := range indices {
		outPath := filepath.Join(outDir, string(idx.Type)+".png")
		if err := processing.GenerateIndex(ctx, w.deps.Executor, multibandPath, outPath, idx.Type); err != nil {
			return nil, err
		}
		files = append(files, outputFile{fileType: idx.Type, path: outPath, name: string(idx.Type) + ".png"})
	}

	bandStats, err := processing.CalculateBandStatistics(ctx, w.deps.Executor, multibandPath)
	if err != nil {
		return nil, err
	}

	indexStats, err := processing.CalculateIndexStatistics(ctx, w.deps.Executor, multibandPath, indices)
	if err != nil {
		return nil, err
	}

	previousParams := w.fetchPreviousParams(ctx, production.ProduccionID, scene)

	var fechaPlantacion time.Time
	if production.FechaPlantacion != nil {
		fechaPlantacion = *production.FechaPlantacion
	}

	params := processing.BuildParams(processing.ParamsInput{
		ProduccionID:    production.ProduccionID,
		SceneID:         scene.SceneID,
		SceneDate:       scene.SceneDate,
		FechaPlantacion: fechaPlantacion,
		CloudCoverBBox:  cloudCoverBBox,
		Indices:         indexStats,
		BandStats:       bandStats,
		Coverage:        coverage,
	}, previousParams)

	paramsPath := filepath.Join(outDir, "params.json")
	if err := writeJSON(paramsPath, params); err != nil {
		return nil, &domain.ProcessingError{Type: domain.ErrDisk, Message: "writing params.json", Wrapped: err}
	}
	files = append(files, outputFile{fileType: domain.FileParams, path: paramsPath, name: "params.json"})

	return &fullOutputs{files: files, params: params}, nil
}

func (w *Worker) generateNaturalOnly(ctx context.Context, multibandPath string, jobDir *storage.JobDir) (string, error) {
	outPath := filepath.Join(jobDir.Output(), string(domain.FileNatural)+".png")
	if err := processing.GenerateRGB(ctx, w.deps.Executor, multibandPath, outPath,
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
	prevScene, err := w.deps.Scenes.GetPreviousValidScene(ctx, produccionID, scene.SceneDate)
	if err != nil {
		w.log.Warn("failed to look up previous valid scene", "scene_id", scene.SceneID, "error", err)
		return nil
	}
	if prevScene == nil {
		return nil
	}

	prevFile, err := w.deps.Files.GetByType(ctx, prevScene.ID, domain.FileParams)
	if err != nil {
		w.log.Warn("failed to look up previous params file", "scene_id", scene.SceneID, "error", err)
		return nil
	}
	if prevFile == nil {
		return nil
	}

	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("prev-params-%d-%s.json", produccionID, prevScene.SceneID))
	defer os.Remove(tmpPath)

	if err := w.deps.S3.Download(ctx, prevFile.S3Bucket, prevFile.S3Key, tmpPath); err != nil {
		w.log.Warn("failed to download previous params.json", "scene_id", scene.SceneID, "error", err)
		return nil
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		w.log.Warn("failed to read downloaded previous params.json", "scene_id", scene.SceneID, "error", err)
		return nil
	}

	var previous processing.Params
	if err := json.Unmarshal(data, &previous); err != nil {
		w.log.Warn("failed to parse previous params.json", "scene_id", scene.SceneID, "error", err)
		return nil
	}

	return &previous
}

// runIA invokes the IA client, if configured, tolerating errors: an IA
// failure is logged and treated as a partial result (the scene still
// completes without analisis.json).
func (w *Worker) runIA(ctx context.Context, production *domain.Production, scene *domain.Scene, multibandPath string, params *processing.Params) *domain.AnalysisResult {
	if w.deps.IA == nil {
		return nil
	}

	result, err := w.deps.IA.Analyze(ctx, IAInput{
		ProduccionID:  production.ProduccionID,
		SceneID:       scene.SceneID,
		MultibandPath: multibandPath,
		Params:        params,
	})
	if err != nil {
		w.log.Error("IA analysis failed, continuing without analisis.json", "scene_id", scene.SceneID, "error", err)
		return nil
	}

	return result
}

func (w *Worker) uploadAndRegister(ctx context.Context, production *domain.Production, scene *domain.Scene, outputs []outputFile) error {
	bucket := w.deps.S3Config.Bucket
	prefix := w.deps.S3Config.Prefix

	for _, out := range outputs {
		key := w.deps.S3.BuildKey(prefix, production.ProduccionID, scene.SceneID, out.name)

		if err := w.deps.S3.Upload(ctx, bucket, key, out.path); err != nil {
			return err
		}

		var size int64
		if info, statErr := os.Stat(out.path); statErr == nil {
			size = info.Size()
		}

		sceneFile := &domain.SceneFile{
			EscenaID:      scene.ID,
			FileType:      out.fileType,
			FileName:      out.name,
			S3Key:         key,
			S3Bucket:      bucket,
			FileSizeBytes: size,
		}

		if err := w.deps.Files.Create(ctx, sceneFile); err != nil {
			return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "registering scene file: " + out.name, Wrapped: err}
		}
	}

	return nil
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

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
