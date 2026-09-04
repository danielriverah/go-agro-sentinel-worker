package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"agro-sentinel-worker/internal/domain"
)

// ProductionRepo provides CRUD access to s3_monitoring_producciones.
type ProductionRepo struct {
	db *sql.DB
}

// NewProductionRepo creates a new ProductionRepo.
func NewProductionRepo(db *sql.DB) *ProductionRepo {
	return &ProductionRepo{db: db}
}

// Upsert inserts a production or updates it if produccion_id already exists.
func (r *ProductionRepo) Upsert(ctx context.Context, p *domain.Production) error {
	var minx, miny, maxx, maxy any
	if p.BBox != nil {
		minx, miny, maxx, maxy = p.BBox.MinX, p.BBox.MinY, p.BBox.MaxX, p.BBox.MaxY
	}

	now := time.Now().UTC()

	const q = `
INSERT INTO s3_monitoring_producciones (
	produccion_id, articulo_id, centro_costo_id, nombre_rancho, cultivo, ciclo, bbox_minx, bbox_miny, bbox_maxx, bbox_maxy,
	monitoring, monitoring_motivo, bloqueado, bloqueado_motivo, bloqueado_at,
	desbloqueado_por, target_resolution, cloud_cover_max, fecha_plantacion,
	dias_produccion, fecha_fin_monitoreo, total_escenas, total_escenas_validas,
	last_sync_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
	articulo_id = VALUES(articulo_id),
	centro_costo_id = VALUES(centro_costo_id),
	nombre_rancho = VALUES(nombre_rancho),
	cultivo = VALUES(cultivo),
	ciclo = VALUES(ciclo),
	bbox_minx = VALUES(bbox_minx),
	bbox_miny = VALUES(bbox_miny),
	bbox_maxx = VALUES(bbox_maxx),
	bbox_maxy = VALUES(bbox_maxy),
	monitoring = VALUES(monitoring),
	monitoring_motivo = VALUES(monitoring_motivo),
	bloqueado = VALUES(bloqueado),
	bloqueado_motivo = VALUES(bloqueado_motivo),
	bloqueado_at = VALUES(bloqueado_at),
	desbloqueado_por = VALUES(desbloqueado_por),
	target_resolution = VALUES(target_resolution),
	cloud_cover_max = VALUES(cloud_cover_max),
	fecha_plantacion = VALUES(fecha_plantacion),
	dias_produccion = VALUES(dias_produccion),
	fecha_fin_monitoreo = VALUES(fecha_fin_monitoreo),
	total_escenas = VALUES(total_escenas),
	total_escenas_validas = VALUES(total_escenas_validas),
	last_sync_at = VALUES(last_sync_at),
	updated_at = VALUES(updated_at)
`

	createdAt := p.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	_, err := r.db.ExecContext(ctx, q,
		p.ProduccionID, p.ArticuloID, p.CentroCostoID, p.NombreRancho, p.Cultivo, p.Ciclo, minx, miny, maxx, maxy,
		p.Monitoring, nullString(p.MonitoringMotivo), p.Bloqueado, nullString(p.BloqueadoMotivo), p.BloqueadoAt,
		nullString(p.DesbloqueadoPor), p.TargetResolution, p.CloudCoverMax, p.FechaPlantacion,
		p.DiasProduccion, p.FechaFinMonitoreo, p.TotalEscenas, p.TotalEscenasValidas,
		p.LastSyncAt, createdAt, now,
	)
	if err != nil {
		return fmt.Errorf("upserting production: %w", err)
	}

	return nil
}

// GetByProduccionID fetches a production by its external produccion_id.
func (r *ProductionRepo) GetByProduccionID(ctx context.Context, produccionID int64) (*domain.Production, error) {
	const q = `
SELECT id, produccion_id, articulo_id, centro_costo_id, nombre_rancho, cultivo, ciclo, bbox_minx, bbox_miny, bbox_maxx, bbox_maxy,
	monitoring, monitoring_motivo, bloqueado, bloqueado_motivo, bloqueado_at,
	desbloqueado_por, target_resolution, cloud_cover_max, fecha_plantacion,
	dias_produccion, fecha_fin_monitoreo, total_escenas, total_escenas_validas,
	last_sync_at, created_at, updated_at
FROM s3_monitoring_producciones
WHERE produccion_id = ?
`

	row := r.db.QueryRowContext(ctx, q, produccionID)
	p, err := scanProduction(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting production by produccion_id: %w", err)
	}

	return p, nil
}

// ListActive returns all productions with monitoring=1 and bloqueado=0.
func (r *ProductionRepo) ListActive(ctx context.Context) ([]*domain.Production, error) {
	const q = `
SELECT id, produccion_id, articulo_id, centro_costo_id, nombre_rancho, cultivo, ciclo, bbox_minx, bbox_miny, bbox_maxx, bbox_maxy,
	monitoring, monitoring_motivo, bloqueado, bloqueado_motivo, bloqueado_at,
	desbloqueado_por, target_resolution, cloud_cover_max, fecha_plantacion,
	dias_produccion, fecha_fin_monitoreo, total_escenas, total_escenas_validas,
	last_sync_at, created_at, updated_at
FROM s3_monitoring_producciones
WHERE monitoring = 1 AND bloqueado = 0
`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("listing active productions: %w", err)
	}
	defer rows.Close()

	var results []*domain.Production
	for rows.Next() {
		p, err := scanProduction(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning production row: %w", err)
		}
		results = append(results, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating production rows: %w", err)
	}

	return results, nil
}

// UpdateMonitoring updates the monitoring flag and motivo for a production.
func (r *ProductionRepo) UpdateMonitoring(ctx context.Context, produccionID int64, monitoring bool, motivo string) error {
	const q = `
UPDATE s3_monitoring_producciones
SET monitoring = ?, monitoring_motivo = ?, updated_at = ?
WHERE produccion_id = ?
`
	_, err := r.db.ExecContext(ctx, q, monitoring, nullString(motivo), time.Now().UTC(), produccionID)
	if err != nil {
		return fmt.Errorf("updating monitoring: %w", err)
	}
	return nil
}

// UpdateBBox updates the bounding box for a production.
func (r *ProductionRepo) UpdateBBox(ctx context.Context, produccionID int64, bbox domain.BBox) error {
	const q = `
UPDATE s3_monitoring_producciones
SET bbox_minx = ?, bbox_miny = ?, bbox_maxx = ?, bbox_maxy = ?, updated_at = ?
WHERE produccion_id = ?
`
	_, err := r.db.ExecContext(ctx, q, bbox.MinX, bbox.MinY, bbox.MaxX, bbox.MaxY, time.Now().UTC(), produccionID)
	if err != nil {
		return fmt.Errorf("updating bbox: %w", err)
	}
	return nil
}

// SetBloqueado marks a production as blocked with the given motivo.
func (r *ProductionRepo) SetBloqueado(ctx context.Context, produccionID int64, motivo string) error {
	now := time.Now().UTC()
	const q = `
UPDATE s3_monitoring_producciones
SET bloqueado = 1, bloqueado_motivo = ?, bloqueado_at = ?, updated_at = ?
WHERE produccion_id = ?
`
	_, err := r.db.ExecContext(ctx, q, nullString(motivo), now, now, produccionID)
	if err != nil {
		return fmt.Errorf("setting bloqueado: %w", err)
	}
	return nil
}

// Desbloquear unblocks a production, recording who unblocked it.
func (r *ProductionRepo) Desbloquear(ctx context.Context, produccionID int64, usuario string) error {
	const q = `
UPDATE s3_monitoring_producciones
SET bloqueado = 0, bloqueado_motivo = NULL, bloqueado_at = NULL, desbloqueado_por = ?, updated_at = ?
WHERE produccion_id = ?
`
	_, err := r.db.ExecContext(ctx, q, nullString(usuario), time.Now().UTC(), produccionID)
	if err != nil {
		return fmt.Errorf("desbloqueando production: %w", err)
	}
	return nil
}

// IncrementEscenas increments total_escenas, and total_escenas_validas if valid.
func (r *ProductionRepo) IncrementEscenas(ctx context.Context, produccionID int64, valid bool) error {
	q := `
UPDATE s3_monitoring_producciones
SET total_escenas = total_escenas + 1, updated_at = ?
WHERE produccion_id = ?
`
	if valid {
		q = `
UPDATE s3_monitoring_producciones
SET total_escenas = total_escenas + 1, total_escenas_validas = total_escenas_validas + 1, updated_at = ?
WHERE produccion_id = ?
`
	}

	_, err := r.db.ExecContext(ctx, q, time.Now().UTC(), produccionID)
	if err != nil {
		return fmt.Errorf("incrementing escenas: %w", err)
	}
	return nil
}

// GetByArticuloID fetches all productions for a given articulo_id.
func (r *ProductionRepo) GetByArticuloID(ctx context.Context, articuloID int64) ([]*domain.Production, error) {
	const q = `
SELECT id, produccion_id, articulo_id, centro_costo_id, nombre_rancho, cultivo, ciclo, bbox_minx, bbox_miny, bbox_maxx, bbox_maxy,
	monitoring, monitoring_motivo, bloqueado, bloqueado_motivo, bloqueado_at,
	desbloqueado_por, target_resolution, cloud_cover_max, fecha_plantacion,
	dias_produccion, fecha_fin_monitoreo, total_escenas, total_escenas_validas,
	last_sync_at, created_at, updated_at
FROM s3_monitoring_producciones
WHERE articulo_id = ?
ORDER BY created_at DESC
`

	rows, err := r.db.QueryContext(ctx, q, articuloID)
	if err != nil {
		return nil, fmt.Errorf("listing productions by articulo_id: %w", err)
	}
	defer rows.Close()

	var results []*domain.Production
	for rows.Next() {
		p, err := scanProduction(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning production row: %w", err)
		}
		results = append(results, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating production rows: %w", err)
	}

	return results, nil
}

// GetByCentroCostoID fetches all productions for a given centro_costo_id.
func (r *ProductionRepo) GetByCentroCostoID(ctx context.Context, centroCostoID int64) ([]*domain.Production, error) {
	const q = `
SELECT id, produccion_id, articulo_id, centro_costo_id, nombre_rancho, cultivo, ciclo, bbox_minx, bbox_miny, bbox_maxx, bbox_maxy,
	monitoring, monitoring_motivo, bloqueado, bloqueado_motivo, bloqueado_at,
	desbloqueado_por, target_resolution, cloud_cover_max, fecha_plantacion,
	dias_produccion, fecha_fin_monitoreo, total_escenas, total_escenas_validas,
	last_sync_at, created_at, updated_at
FROM s3_monitoring_producciones
WHERE centro_costo_id = ?
ORDER BY created_at DESC
`

	rows, err := r.db.QueryContext(ctx, q, centroCostoID)
	if err != nil {
		return nil, fmt.Errorf("listing productions by centro_costo_id: %w", err)
	}
	defer rows.Close()

	var results []*domain.Production
	for rows.Next() {
		p, err := scanProduction(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning production row: %w", err)
		}
		results = append(results, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating production rows: %w", err)
	}

	return results, nil
}

// UpdateArticuloAndCentro updates articulo_id, centro_costo_id and nombre_rancho for a production.
func (r *ProductionRepo) UpdateArticuloAndCentro(ctx context.Context, produccionID int64, articuloID int64, centroCostoID int64, nombreRancho string) error {
	const q = `
UPDATE s3_monitoring_producciones
SET articulo_id = ?, centro_costo_id = ?, nombre_rancho = ?, updated_at = ?
WHERE produccion_id = ?
`
	_, err := r.db.ExecContext(ctx, q, articuloID, centroCostoID, nombreRancho, time.Now().UTC(), produccionID)
	if err != nil {
		return fmt.Errorf("updating articulo and centro costo: %w", err)
	}
	return nil
}

// rowScanner abstracts *sql.Row / *sql.Rows for shared scan logic.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanProduction(row rowScanner) (*domain.Production, error) {
	var p domain.Production
	var minx, miny, maxx, maxy sql.NullFloat64
	var monitoringMotivo, bloqueadoMotivo, desbloqueadoPor, nombreRancho sql.NullString
	var bloqueadoAt, fechaPlantacion, fechaFinMonitoreo, lastSyncAt sql.NullTime
	var diasProduccion sql.NullInt64
	var articuloID, centroCostoID sql.NullInt64

	err := row.Scan(
		&p.ID, &p.ProduccionID, &articuloID, &centroCostoID, &nombreRancho, &p.Cultivo, &p.Ciclo, &minx, &miny, &maxx, &maxy,
		&p.Monitoring, &monitoringMotivo, &p.Bloqueado, &bloqueadoMotivo, &bloqueadoAt,
		&desbloqueadoPor, &p.TargetResolution, &p.CloudCoverMax, &fechaPlantacion,
		&diasProduccion, &fechaFinMonitoreo, &p.TotalEscenas, &p.TotalEscenasValidas,
		&lastSyncAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if minx.Valid && miny.Valid && maxx.Valid && maxy.Valid {
		p.BBox = &domain.BBox{MinX: minx.Float64, MinY: miny.Float64, MaxX: maxx.Float64, MaxY: maxy.Float64}
	}
	if articuloID.Valid {
		p.ArticuloID = articuloID.Int64
	}
	if centroCostoID.Valid {
		p.CentroCostoID = centroCostoID.Int64
	}
	p.NombreRancho = nombreRancho.String
	p.MonitoringMotivo = monitoringMotivo.String
	p.BloqueadoMotivo = bloqueadoMotivo.String
	p.DesbloqueadoPor = desbloqueadoPor.String
	if bloqueadoAt.Valid {
		p.BloqueadoAt = &bloqueadoAt.Time
	}
	if fechaPlantacion.Valid {
		p.FechaPlantacion = &fechaPlantacion.Time
	}
	if fechaFinMonitoreo.Valid {
		p.FechaFinMonitoreo = &fechaFinMonitoreo.Time
	}
	if lastSyncAt.Valid {
		p.LastSyncAt = &lastSyncAt.Time
	}
	p.DiasProduccion = int(diasProduccion.Int64)

	return &p, nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
