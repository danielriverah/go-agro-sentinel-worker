// Package ia implements the IA analysis pipeline:
// download ia_req.json from S3 → invoke Bedrock → persist result to MySQL.
package ia

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"agro-sentinel-worker/internal/domain"
)

// S3Downloader is the subset of aws.S3Client the analyzer needs.
type S3Downloader interface {
	Download(ctx context.Context, bucket, key, destPath string) error
}

// S3Uploader uploads a local file to S3.
type S3Uploader interface {
	Upload(ctx context.Context, bucket, key, srcPath string) error
}

// FileIndexer registers a file record in s3_monitoring_escena_archivos.
type FileIndexer interface {
	Create(ctx context.Context, f *domain.SceneFile) error
}

// BedrockAnalyzer invokes the IA model with a ia_req.json payload.
// PosibleCosecha is returned alongside the result so callers can update the production flag.
type BedrockAnalyzer interface {
	Analyze(ctx context.Context, escenaID uint64, iaReqJSON []byte) (*domain.IAResultSummary, bool, error)
	DryRunPrompt(iaReqJSON []byte) (system string, userMessage string, fullPayload []byte)
}

// IAResultRepo persists and retrieves IA analysis results.
type IAResultRepo interface {
	Upsert(ctx context.Context, result *domain.IAResultSummary) error
	GetByEscenaID(ctx context.Context, escenaID uint64) (*domain.IAResultSummary, error)
}

// SceneUpdater marks a scene's analysis flag after a successful IA run.
type SceneUpdater interface {
	SetAnalysis(ctx context.Context, escenaID uint64, analysis bool) error
}

// ProductionUpdater updates production-level flags derived from IA results.
type ProductionUpdater interface {
	UpdatePosibleCosecha(ctx context.Context, produccionID int64, posible bool) error
	SetBloqueado(ctx context.Context, produccionID int64, bloqueado bool) error
}

// Deps bundles Analyzer dependencies.
type Deps struct {
	S3          S3Downloader
	S3Up        S3Uploader   // optional; uploads result JSON to S3
	Files       FileIndexer  // optional; indexes result in s3_monitoring_escena_archivos
	Bedrock     BedrockAnalyzer
	Results     IAResultRepo
	Scenes      SceneUpdater
	Productions ProductionUpdater // optional; updates posible_cosecha and bloqueado
	Bucket      string
	TempDir     string
	Logger      *slog.Logger
}

// Analyzer orchestrates the IA pipeline and prevents concurrent analysis of
// the same scene via an in-memory set of active scene IDs.
type Analyzer struct {
	deps    Deps
	log     *slog.Logger
	mu      sync.Mutex
	running map[uint64]struct{} // escena IDs currently being analyzed
}

// New creates an Analyzer.
func New(deps Deps) *Analyzer {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Analyzer{
		deps:    deps,
		log:     logger,
		running: make(map[uint64]struct{}),
	}
}

// ErrAlreadyRunning is returned when an analysis for the same scene is already in progress.
var ErrAlreadyRunning = fmt.Errorf("ia analysis already in progress for this scene")

// deriveResultKey builds the S3 key for the IA result JSON from the ia_req key.
// e.g. "produccion/2044/.../multiband.ia_req.json" → "produccion/2044/.../multiband.ia.json"
func deriveResultKey(iaReqKey string) string {
	const old = "multiband.ia_req.json"
	const new = "multiband.ia.json"
	if len(iaReqKey) >= len(old) && iaReqKey[len(iaReqKey)-len(old):] == old {
		return iaReqKey[:len(iaReqKey)-len(old)] + new
	}
	// fallback: append suffix
	return iaReqKey + ".result.json"
}

// uploadResultJSON writes the raw Bedrock JSON (json_original) to S3 so the
// sync service can reconstruct s3_monitoring_escena_ia_resumen from S3 alone.
func (a *Analyzer) uploadResultJSON(ctx context.Context, result *domain.IAResultSummary, escenaID uint64, s3Key string) error {
	tmpFile := filepath.Join(a.deps.TempDir, fmt.Sprintf("ia_result_%d.json", escenaID))
	defer os.Remove(tmpFile)

	// Upload the raw Bedrock response so sync can re-parse it independently.
	payload := result.JSONOriginal
	if payload == "" {
		// Fallback: serialize the summary itself if raw is missing.
		b, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling result: %w", err)
		}
		payload = string(b)
	}
	data := []byte(payload)
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		return fmt.Errorf("writing result tmp file: %w", err)
	}

	if err := a.deps.S3Up.Upload(ctx, a.deps.Bucket, s3Key, tmpFile); err != nil {
		return fmt.Errorf("uploading to S3: %w", err)
	}

	now := time.Now().UTC()
	s3Uri := "s3://" + a.deps.Bucket + "/" + s3Key
	sf := &domain.SceneFile{
		EscenaID:      escenaID,
		Tipo:          string(domain.FileIAResult),
		S3Key:         s3Key,
		S3Uri:         s3Uri,
		Extension:     "json",
		SizeBytes:     int64(len(data)),
		LastModified:  &now,
		Existe:        true,
		JsonContent:   result.JSONOriginal,
		FechaCreacion: now,
	}
	if err := a.deps.Files.Create(ctx, sf); err != nil {
		return fmt.Errorf("indexing result file: %w", err)
	}

	a.log.Info("ia: result JSON uploaded and indexed", "escena_id", escenaID, "s3_key", s3Key)
	return nil
}

// DryRunResult holds the prompt that would be sent to Bedrock, for inspection.
type DryRunResult struct {
	S3Key       string `json:"s3_key"`
	IAReqJSON   string `json:"ia_req_json"`
	System      string `json:"system_prompt"`
	UserMessage string `json:"user_message"`
	FullPayload any    `json:"full_bedrock_payload"`
}

// DryRun downloads ia_req.json from S3 and returns the fully assembled prompt
// without making any Bedrock API call. Safe to run concurrently and at any time.
func (a *Analyzer) DryRun(ctx context.Context, s3Key string) (*DryRunResult, error) {
	tmpDir := a.deps.TempDir
	if tmpDir == "" {
		tmpDir = os.TempDir()
	}
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("dryrun_ia_req_%d.json", time.Now().UnixNano()))
	defer os.Remove(tmpFile)

	if err := a.deps.S3.Download(ctx, a.deps.Bucket, s3Key, tmpFile); err != nil {
		return nil, fmt.Errorf("ia dry-run: downloading ia_req.json: %w", err)
	}
	iaReqJSON, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("ia dry-run: reading ia_req.json: %w", err)
	}

	system, userMsg, payload := a.deps.Bedrock.DryRunPrompt(iaReqJSON)

	// Unmarshal payload so the response is a proper JSON object, not an escaped string.
	var payloadObj any
	_ = json.Unmarshal(payload, &payloadObj)

	return &DryRunResult{
		S3Key:       s3Key,
		IAReqJSON:   string(iaReqJSON),
		System:      system,
		UserMessage: userMsg,
		FullPayload: payloadObj,
	}, nil
}

// DryRunRaw downloads ia_req.json and logs the Bedrock prompt without calling
// the model. Implements worker.IAAnalyzer for dry-run mode.
func (a *Analyzer) DryRunRaw(ctx context.Context, s3Key string) error {
	result, err := a.DryRun(ctx, s3Key)
	if err != nil {
		return err
	}
	a.log.Info("ia_auto dry-run", "s3_key", s3Key, "chars_system", len(result.System), "chars_user", len(result.UserMessage))
	return nil
}

// IsRunning reports whether an analysis is currently active for escenaID.
func (a *Analyzer) IsRunning(escenaID uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.running[escenaID]
	return ok
}

// Analyze runs the full pipeline for escenaID:
//  1. Acquires the in-memory slot (returns ErrAlreadyRunning if taken)
//  2. Downloads ia_req.json from S3
//  3. Calls Bedrock
//  4. Upserts result in MySQL
//  5. Marks scene analysis=true
//  6. Updates posible_cosecha on the production (if Productions dep is set)
//
// produccionID is the producciones.produccion_id (not the monitoring ID) used
// to update posible_cosecha. Pass 0 to skip that step.
//
// The caller is responsible for running this in a goroutine when async behavior
// is needed (e.g., from the HTTP trigger endpoint).
func (a *Analyzer) Analyze(ctx context.Context, escenaID uint64, s3Key string, produccionID int64) (*domain.IAResultSummary, error) {
	// Acquire slot — reject if already running.
	a.mu.Lock()
	if _, busy := a.running[escenaID]; busy {
		a.mu.Unlock()
		return nil, ErrAlreadyRunning
	}
	a.running[escenaID] = struct{}{}
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		delete(a.running, escenaID)
		a.mu.Unlock()
	}()

	a.log.Info("ia analysis started", "escena_id", escenaID, "s3_key", s3Key)
	start := time.Now()

	// Download ia_req.json to a temp file.
	tmpDir := a.deps.TempDir
	if tmpDir == "" {
		tmpDir = os.TempDir()
	}
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("ia_req_%d.json", escenaID))
	defer os.Remove(tmpFile)

	if err := a.deps.S3.Download(ctx, a.deps.Bucket, s3Key, tmpFile); err != nil {
		return nil, fmt.Errorf("ia: downloading ia_req.json: %w", err)
	}

	iaReqJSON, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("ia: reading ia_req.json: %w", err)
	}

	// Invoke Bedrock.
	result, posibleCosecha, err := a.deps.Bedrock.Analyze(ctx, escenaID, iaReqJSON)
	if err != nil {
		return nil, fmt.Errorf("ia: bedrock analyze: %w", err)
	}

	// Persist result.
	if err := a.deps.Results.Upsert(ctx, result); err != nil {
		return nil, fmt.Errorf("ia: upserting result: %w", err)
	}

	// Upload result JSON to S3 and index it in escena_archivos.
	resultKey := deriveResultKey(s3Key)
	if a.deps.S3Up != nil && a.deps.Files != nil {
		if uploadErr := a.uploadResultJSON(ctx, result, escenaID, resultKey); uploadErr != nil {
			a.log.Warn("ia: could not upload result JSON", "escena_id", escenaID, "error", uploadErr)
		}
	}

	// Mark scene analysis=true.
	if a.deps.Scenes != nil {
		if err := a.deps.Scenes.SetAnalysis(ctx, escenaID, true); err != nil {
			a.log.Warn("ia: could not mark scene analysis=true", "escena_id", escenaID, "error", err)
		}
	}

	// Update posible_cosecha on the production when the model flagged it.
	if a.deps.Productions != nil && produccionID > 0 {
		if err := a.deps.Productions.UpdatePosibleCosecha(ctx, produccionID, posibleCosecha); err != nil {
			a.log.Warn("ia: could not update posible_cosecha", "produccion_id", produccionID, "error", err)
		}
		// Block monitoring when the analysis is critical.
		if result.EstadoClave == "critico" {
			if err := a.deps.Productions.SetBloqueado(ctx, produccionID, true); err != nil {
				a.log.Warn("ia: could not block production", "produccion_id", produccionID, "error", err)
			} else {
				a.log.Info("ia: production blocked due to critico analysis", "produccion_id", produccionID)
			}
		}
	}

	a.log.Info("ia analysis completed", "escena_id", escenaID,
		"estado_clave", result.EstadoClave, "riesgo", result.RiesgoNivel,
		"posible_cosecha", posibleCosecha, "elapsed", time.Since(start).Round(time.Millisecond))

	return result, nil
}
