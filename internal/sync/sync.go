// Package sync implements the periodic sync cycle that pulls active
// producciones and their escenas from DynamoDB and reconciles them into
// MySQL, deciding which producciones should be actively monitored.
package sync

import (
	"context"
	"log/slog"
	"time"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/infrastructure/aws"
)

// Monitoring motivo codes recorded on s3_monitoring_producciones.
const (
	MotivoOK                 = "OK"
	MotivoSinFechaPlantacion = "SIN_FECHA_PLANTACION"
	MotivoFinMonitoreo       = "FIN_MONITOREO_ALCANZADO"
	MotivoSinPoligono        = "SIN_POLIGONO"
)

// DynamoReader is the subset of aws.DynamoDBClient the sync service needs.
type DynamoReader interface {
	ListActiveProducciones(ctx context.Context, tableName string) ([]aws.DynamoProduction, error)
	ListEscenas(ctx context.Context, tableName string, produccionID int64) ([]aws.DynamoScene, error)
}

// ProductionRepository is the subset of database.ProductionRepo the sync
// service needs.
type ProductionRepository interface {
	Upsert(ctx context.Context, p *domain.Production) error
	GetByProduccionID(ctx context.Context, produccionID int64) (*domain.Production, error)
	ListActive(ctx context.Context) ([]*domain.Production, error)
	UpdateMonitoring(ctx context.Context, produccionID int64, monitoring bool, motivo string) error
	UpdateBBox(ctx context.Context, produccionID int64, bbox domain.BBox) error
	SetBloqueado(ctx context.Context, produccionID int64, motivo string) error
	Desbloquear(ctx context.Context, produccionID int64, usuario string) error
	IncrementEscenas(ctx context.Context, produccionID int64, valid bool) error
}

// SceneRepository is the subset of database.SceneRepo the sync service needs.
type SceneRepository interface {
	Upsert(ctx context.Context, s *domain.Scene) error
	GetByProduccionAndSceneID(ctx context.Context, produccionID int64, sceneID string) (*domain.Scene, error)
}

// PolygonRepository looks up the monitored polygon for a produccion and
// returns its bounding box. The concrete implementation queries the
// asignaciones_zonas_producciones table and extracts the BBOX from the
// polygon column using CalculateBBoxFromWKT.
type PolygonRepository interface {
	GetPolygonBBox(ctx context.Context, produccionID int64) (*domain.BBox, error)
}

// Service runs the DynamoDB -> MySQL sync cycle.
type Service struct {
	dynamo      DynamoReader
	prodRepo    ProductionRepository
	sceneRepo   SceneRepository
	polygonRepo PolygonRepository

	cfg      config.SyncConfig
	sentinel config.SentinelConfig

	tableProducciones string
	tableEscenas      string

	logger *slog.Logger

	// now is overridable in tests.
	now func() time.Time
}

// New builds a Service with the given dependencies.
//
// tableProducciones and tableEscenas name the DynamoDB tables scanned each
// cycle (config.DynamoDBConfig.TableProducciones / TableEscenas).
func New(
	dynamo DynamoReader,
	prodRepo ProductionRepository,
	sceneRepo SceneRepository,
	polygonRepo PolygonRepository,
	cfg config.SyncConfig,
	sentinel config.SentinelConfig,
	tableProducciones string,
	tableEscenas string,
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
		cfg:               cfg,
		sentinel:          sentinel,
		tableProducciones: tableProducciones,
		tableEscenas:      tableEscenas,
		logger:            logger,
		now:               time.Now,
	}
}

// CalculateFinMonitoreo returns the date monitoring should stop for a
// production planted on plantacion, given its production cycle length and
// the configured monitoring margin (both in days).
func CalculateFinMonitoreo(plantacion time.Time, diasProduccion, diasMargen int) time.Time {
	return plantacion.AddDate(0, 0, diasProduccion+diasMargen)
}

// RunOnce executes a single sync cycle: it fetches active producciones from
// DynamoDB, reconciles each into MySQL (deciding whether it should be
// monitored), and — for producciones that end up monitored — fetches and
// upserts their new escenas.
func (s *Service) RunOnce(ctx context.Context) error {
	producciones, err := s.dynamo.ListActiveProducciones(ctx, s.tableProducciones)
	if err != nil {
		return &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: "listing active producciones", Wrapped: err}
	}

	for _, dp := range producciones {
		if err := s.syncProduccion(ctx, dp); err != nil {
			s.logger.Error("syncing produccion failed", "produccion_id", dp.ProduccionID, "error", err)
		}
	}

	return nil
}

// RunLoop calls RunOnce immediately and then every cfg.IntervalMinutes,
// until ctx is cancelled.
func (s *Service) RunLoop(ctx context.Context) error {
	interval := time.Duration(s.cfg.IntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = time.Minute
	}

	if err := s.RunOnce(ctx); err != nil {
		s.logger.Error("sync cycle failed", "error", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.RunOnce(ctx); err != nil {
				s.logger.Error("sync cycle failed", "error", err)
			}
		}
	}
}

func (s *Service) syncProduccion(ctx context.Context, dp aws.DynamoProduction) error {
	existing, err := s.prodRepo.GetByProduccionID(ctx, dp.ProduccionID)
	if err != nil {
		return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "getting production", Wrapped: err}
	}

	if existing != nil && existing.Bloqueado {
		s.logger.Info("skipping blocked produccion", "produccion_id", dp.ProduccionID)
		return nil
	}

	prod := buildProduction(dp, existing)

	monitoring, motivo := s.evaluateMonitoring(ctx, &prod)
	prod.Monitoring = monitoring
	prod.MonitoringMotivo = motivo

	now := s.now()
	prod.LastSyncAt = &now

	if err := s.prodRepo.Upsert(ctx, &prod); err != nil {
		return &domain.ProcessingError{Type: domain.ErrMySQL, Message: "upserting production", Wrapped: err}
	}

	if !monitoring {
		return nil
	}

	return s.syncEscenas(ctx, prod.ProduccionID)
}

// buildProduction merges a DynamoDB production item onto the existing MySQL
// row (if any), preserving fields DynamoDB doesn't own.
func buildProduction(dp aws.DynamoProduction, existing *domain.Production) domain.Production {
	var prod domain.Production
	if existing != nil {
		prod = *existing
	}

	prod.ProduccionID = dp.ProduccionID
	prod.Cultivo = dp.Cultivo
	prod.Ciclo = dp.Ciclo
	prod.DiasProduccion = dp.DiasProduccion

	if dp.FechaPlantacion != "" {
		if t, err := time.Parse("2006-01-02", dp.FechaPlantacion); err == nil {
			prod.FechaPlantacion = &t
		}
	}

	return prod
}

// evaluateMonitoring decides whether prod should be actively monitored,
// returning the monitoring flag and the motivo to record. On success it also
// populates prod.BBox and prod.FechaFinMonitoreo.
func (s *Service) evaluateMonitoring(ctx context.Context, prod *domain.Production) (bool, string) {
	if prod.FechaPlantacion == nil {
		return false, MotivoSinFechaPlantacion
	}

	fin := CalculateFinMonitoreo(*prod.FechaPlantacion, prod.DiasProduccion, s.cfg.DiasMargenMonitoreo)
	prod.FechaFinMonitoreo = &fin

	if s.now().After(fin) {
		return false, MotivoFinMonitoreo
	}

	bbox, err := s.polygonRepo.GetPolygonBBox(ctx, prod.ProduccionID)
	if err != nil {
		s.logger.Error("getting polygon bbox failed", "produccion_id", prod.ProduccionID, "error", err)
		return false, MotivoSinPoligono
	}
	if bbox == nil {
		return false, MotivoSinPoligono
	}

	prod.BBox = bbox

	return true, MotivoOK
}

func (s *Service) syncEscenas(ctx context.Context, produccionID int64) error {
	escenas, err := s.dynamo.ListEscenas(ctx, s.tableEscenas, produccionID)
	if err != nil {
		return &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: "listing escenas", Wrapped: err}
	}

	for _, de := range escenas {
		if de.CloudCover > s.sentinel.CloudCoverSceneMax {
			continue
		}

		existing, err := s.sceneRepo.GetByProduccionAndSceneID(ctx, produccionID, de.SceneID)
		if err != nil {
			s.logger.Error("getting scene failed", "produccion_id", produccionID, "scene_id", de.SceneID, "error", err)
			continue
		}
		if existing != nil {
			continue
		}

		sceneDate, err := parseSceneDate(de.Date)
		if err != nil {
			s.logger.Error("parsing scene date failed", "scene_id", de.SceneID, "date", de.Date, "error", err)
			continue
		}

		scene := &domain.Scene{
			ProduccionID:    produccionID,
			SceneID:         de.SceneID,
			SceneDate:       sceneDate,
			CloudCoverScene: de.CloudCover,
			Status:          domain.StatusPending,
		}

		if err := s.sceneRepo.Upsert(ctx, scene); err != nil {
			s.logger.Error("upserting scene failed", "produccion_id", produccionID, "scene_id", de.SceneID, "error", err)
			continue
		}
	}

	return nil
}

func parseSceneDate(date string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", date); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, date)
}
