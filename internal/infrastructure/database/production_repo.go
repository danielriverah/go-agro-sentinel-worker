package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"agro-sentinel-worker/internal/domain"
)

// ProductionRepo provides access to s3_monitoring_producciones.
type ProductionRepo struct {
	db *sql.DB
}

// NewProductionRepo creates a new ProductionRepo.
func NewProductionRepo(db *sql.DB) *ProductionRepo {
	return &ProductionRepo{db: db}
}

// Upsert inserts a production or updates sync fields if produccion_id already exists.
// cosecha, vaiedades and prefix are NOT NULL — they are set from ERP data when inserting.
// Upsert inserts a new production row if it does not yet exist (INSERT IGNORE).
// Existing rows are never modified — use UpdateMonitoring / UpdateUltimaSincronizacion
// for fields the sync process controls. The DBA is the sole owner of all other schema changes.
func (r *ProductionRepo) Upsert(ctx context.Context, p *domain.Production) error {
	now := time.Now().UTC()

	cosecha := p.Cosecha
	if cosecha == "" {
		cosecha = "Desconocido"
	}
	vaiedades := p.Vaiedades
	if vaiedades == "" {
		vaiedades = cosecha
	}
	prefix := p.Prefix
	if prefix == "" {
		prefix = fmt.Sprintf("produccion/%d", p.ProduccionID)
	}

	tileEdge := p.TileEdgeMeters
	if tileEdge == 0 {
		tileEdge = 2000
	}

	createdAt := p.FechaCreacion
	if createdAt.IsZero() {
		createdAt = now
	}

	const q = `
INSERT INTO s3_monitoring_producciones (
	produccion_id, cosecha, vaiedades, folio, rancho, prefix, monitoring, max_dias_monitoring,
	fecha_fin, fecha_plantacion, pbox, polygon_bbox, tile_bbox,
	tile_center_lat, tile_center_lon, tile_edge_meters,
	fase2_completa_at, poligono, tif_complete_at, ia_complete_at,
	posible_cosecha, bloqueado, ultima_sincronizacion, fecha_creacion, fecha_actualizacion
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
	folio = COALESCE(folio, VALUES(folio)),
	rancho = COALESCE(rancho, VALUES(rancho)),
	fecha_actualizacion = VALUES(fecha_actualizacion)`

	_, err := r.db.ExecContext(ctx, q,
		p.ProduccionID, cosecha, vaiedades, nullString(p.Folio), nullString(p.Rancho), prefix,
		boolToTinyint(p.Monitoring), p.MaxDiasMonitoring,
		nullTime(p.FechaFin), nullTime(p.FechaPlantacion),
		nullJSON(p.PBoxJSON), nullJSON(p.PolygonBBoxJSON), nullJSON(p.TileBBoxJSON),
		nullFloat64Ptr(p.TileCenterLat), nullFloat64Ptr(p.TileCenterLon), tileEdge,
		nullTime(p.Fase2CompletaAt), nullJSON(p.PoligonoJSON),
		nullTime(p.TifCompleteAt), nullTime(p.IaCompleteAt),
		boolToTinyint(p.PosibleCosecha), boolToTinyint(p.Bloqueado), nullTime(p.UltimaSincronizacion),
		createdAt, now,
	)
	if err != nil {
		return fmt.Errorf("upserting production: %w", err)
	}

	return nil
}

// GetByProduccionID fetches a production by its external produccion_id.
func (r *ProductionRepo) GetByProduccionID(ctx context.Context, produccionID int64) (*domain.Production, error) {
	const q = prodSelectCols + ` FROM s3_monitoring_producciones WHERE produccion_id = ?`

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

// GetByID fetches a production by its s3_monitoring_produccion_id (PK).
func (r *ProductionRepo) GetByID(ctx context.Context, id uint) (*domain.Production, error) {
	const q = prodSelectCols + ` FROM s3_monitoring_producciones WHERE s3_monitoring_produccion_id = ?`

	row := r.db.QueryRowContext(ctx, q, id)
	p, err := scanProduction(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting production by id: %w", err)
	}

	return p, nil
}

// GetByMonitoringID is an alias for GetByID — kept for worker compatibility.
func (r *ProductionRepo) GetByMonitoringID(ctx context.Context, monitoringID uint) (*domain.Production, error) {
	return r.GetByID(ctx, monitoringID)
}

// ListActive returns all productions with monitoring=1.
func (r *ProductionRepo) ListActive(ctx context.Context) ([]*domain.Production, error) {
	const q = prodSelectCols + ` FROM s3_monitoring_producciones WHERE monitoring = 1`

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

// GetStatsForActive returns scene-level stats for all productions with monitoring=1.
// Returns a map keyed by s3_monitoring_produccion_id.
//
// "pendientes" uses the same definition as the worker (PendingStatuses): a
// FAILED scene is work still to do, since the worker retries it, and PROCESSING
// is in flight. Counting only PENDING made a production with failed scenes look
// up to date in the cards, the grouped headers and the pipeline indicator.
func (r *ProductionRepo) GetStatsForActive(ctx context.Context) (map[uint]*domain.ProductionStats, error) {
	const q = `
SELECT
    s3_monitoring_produccion_id,
    COUNT(*)                                                     AS total,
    SUM(usable)                                                  AS usables,
    SUM(status = 'COMPLETED')                                    AS completadas,
    SUM(status IN ` + PendingStatuses + `)                        AS pendientes,
    SUM(truth_tif_exists)                                        AS tif_ok,
    SUM(ia_exists)                                               AS ia_ok,
    MAX(CASE WHEN truth_tif_exists = 1 THEN fecha END)           AS last_tif_fecha,
    MAX(latest_ia_fecha_analisis)                                AS last_ia_fecha
FROM s3_monitoring_escenas
WHERE s3_monitoring_produccion_id IN (
    SELECT s3_monitoring_produccion_id FROM s3_monitoring_producciones WHERE monitoring = 1
)
GROUP BY s3_monitoring_produccion_id`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("getting production stats: %w", err)
	}
	defer rows.Close()

	result := make(map[uint]*domain.ProductionStats)
	for rows.Next() {
		var s domain.ProductionStats
		var lastTif, lastIa sql.NullTime
		if err := rows.Scan(
			&s.MonitoringProduccionID,
			&s.ScenasTotal, &s.ScenasUsables, &s.ScenasCompletadas, &s.ScenasPendientes,
			&s.ScenasTifOk, &s.ScenasIaOk,
			&lastTif, &lastIa,
		); err != nil {
			return nil, fmt.Errorf("scanning production stats: %w", err)
		}
		if lastTif.Valid { s.LastTifFecha = &lastTif.Time }
		if lastIa.Valid  { s.LastIaFecha  = &lastIa.Time }
		result[s.MonitoringProduccionID] = &s
	}
	return result, rows.Err()
}

// UpdateIAuto enables or disables automatic IA analysis for a production.
func (r *ProductionRepo) UpdateIAuto(ctx context.Context, produccionID int64, iaAuto bool) error {
	const q = `UPDATE s3_monitoring_producciones SET ia_auto = ?, fecha_actualizacion = ? WHERE produccion_id = ?`
	_, err := r.db.ExecContext(ctx, q, boolToTinyint(iaAuto), time.Now().UTC(), produccionID)
	if err != nil {
		return fmt.Errorf("updating ia_auto: %w", err)
	}
	return nil
}

// UpdateMonitoring updates the monitoring flag for a production.
func (r *ProductionRepo) UpdateMonitoring(ctx context.Context, produccionID int64, monitoring bool) error {
	const q = `
UPDATE s3_monitoring_producciones
SET monitoring = ?, fecha_actualizacion = ?
WHERE produccion_id = ?
`
	_, err := r.db.ExecContext(ctx, q, boolToTinyint(monitoring), time.Now().UTC(), produccionID)
	if err != nil {
		return fmt.Errorf("updating monitoring: %w", err)
	}
	return nil
}

// UpdatePosibleCosecha flags a production as having a possible harvest detected.
func (r *ProductionRepo) UpdatePosibleCosecha(ctx context.Context, produccionID int64, posible bool) error {
	const q = `
UPDATE s3_monitoring_producciones
SET posible_cosecha = ?, fecha_actualizacion = ?
WHERE produccion_id = ?
`
	_, err := r.db.ExecContext(ctx, q, boolToTinyint(posible), time.Now().UTC(), produccionID)
	if err != nil {
		return fmt.Errorf("updating posible_cosecha: %w", err)
	}
	return nil
}

// UpdateUltimaSincronizacion records the last sync timestamp.
func (r *ProductionRepo) UpdateUltimaSincronizacion(ctx context.Context, produccionID int64, t time.Time) error {
	const q = `
UPDATE s3_monitoring_producciones
SET ultima_sincronizacion = ?, fecha_actualizacion = ?
WHERE produccion_id = ?
`
	_, err := r.db.ExecContext(ctx, q, t, time.Now().UTC(), produccionID)
	if err != nil {
		return fmt.Errorf("updating ultima_sincronizacion: %w", err)
	}
	return nil
}

// UpdateSyncState updates only the fields the sync process controls on an existing row.
// All other columns are left untouched (DBA ownership).
func (r *ProductionRepo) UpdateSyncState(ctx context.Context, p *domain.Production) error {
	const q = `
UPDATE s3_monitoring_producciones
SET monitoring = ?, max_dias_monitoring = ?, fecha_fin = ?,
    pbox = ?, tile_bbox = ?, tile_center_lat = ?, tile_center_lon = ?,
    poligono = ?, vaiedades = ?,
    folio = COALESCE(folio, ?), rancho = COALESCE(rancho, ?),
    ultima_sincronizacion = ?, fecha_actualizacion = ?
WHERE produccion_id = ?`
	_, err := r.db.ExecContext(ctx, q,
		boolToTinyint(p.Monitoring), p.MaxDiasMonitoring,
		nullTime(p.FechaFin),
		nullJSON(p.PBoxJSON), nullJSON(p.TileBBoxJSON),
		nullFloat64Ptr(p.TileCenterLat), nullFloat64Ptr(p.TileCenterLon),
		nullJSON(p.PoligonoJSON), nullString(p.Vaiedades),
		nullString(p.Folio), nullString(p.Rancho),
		nullTime(p.UltimaSincronizacion), time.Now().UTC(),
		p.ProduccionID,
	)
	if err != nil {
		return fmt.Errorf("updating sync state: %w", err)
	}
	return nil
}

// Nota: no existe SetBloqueado a propósito. La columna bloqueado la mantiene
// un trigger de s3_monitoring_producciones a partir de posible_cosecha:
//
//	IF new.posible_cosecha <> old.posible_cosecha THEN
//	    SET new.bloqueado = new.posible_cosecha;
//	END IF;
//
// Escribir bloqueado directamente dejaría ambos campos desincronizados, porque
// el trigger sólo reacciona a cambios de posible_cosecha. Para bloquear o
// desbloquear, usar UpdatePosibleCosecha.

// ERPProduccion holds fields enriched from ERP tables.
type ERPProduccion struct {
	Folio         string
	ArticuloID    int64
	CentroCostoID int64
	Cultivo       string // articulos.nombre
	NombreRancho  string // centros_costos.nombre
}

// GetVariedades returns a comma-separated list of variety names assigned to the
// production via doctos_producciones_plantaciones_app. Returns ("", nil) when
// no varieties are found.
func (r *ProductionRepo) GetVariedades(ctx context.Context, produccionID int64) (string, error) {
	const q = `
SELECT GROUP_CONCAT(v.nombre ORDER BY v.nombre SEPARATOR ', ')
FROM doctos_producciones dp
JOIN doctos_producciones_plantaciones_app dpp ON dpp.docto_produccion_id = dp.docto_produccion_id
JOIN variedades v ON v.variedad_id = dpp.variedad_id
WHERE dp.estatus = 'N'
  AND dp.produccion_id = ?
GROUP BY dp.produccion_id
`
	var val sql.NullString
	err := r.db.QueryRowContext(ctx, q, produccionID).Scan(&val)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("getting variedades for produccion %d: %w", produccionID, err)
	}
	return val.String, nil
}

// GetERPFolioRancho returns (folio, rancho, nil) for a production.
// Satisfies the worker.ProductionRepository interface.
// Returns ("", "", nil) when the production has no ERP record.
func (r *ProductionRepo) GetERPFolioRancho(ctx context.Context, produccionID int64) (folio, rancho string, err error) {
	erp, err := r.GetERPDetails(ctx, produccionID)
	if err != nil {
		return "", "", err
	}
	if erp == nil {
		return "", "", nil
	}
	return erp.Folio, erp.NombreRancho, nil
}

// UpdatePolygon overwrites only the polygon and its tight bounding box for one
// monitoring production, addressed by its PK.
//
// tile_bbox and tile_center are deliberately left alone: the download tile must
// keep its position so the multiband rasters already generated stay aligned.
// The ERP tables (producciones, asignaciones_zonas_producciones) are never
// touched either, so the original polygon remains recoverable from there.
func (r *ProductionRepo) UpdatePolygon(ctx context.Context, monitoringID uint, poligono, pbox []byte) error {
	const q = `
UPDATE s3_monitoring_producciones
SET poligono = ?, pbox = ?, polygon_bbox = ?, fecha_actualizacion = ?
WHERE s3_monitoring_produccion_id = ?`

	res, err := r.db.ExecContext(ctx, q, poligono, pbox, pbox, time.Now().UTC(), monitoringID)
	if err != nil {
		return fmt.Errorf("updating polygon for monitoring production %d: %w", monitoringID, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("monitoring production %d not found", monitoringID)
	}
	return nil
}

// ExistsInERP reports whether produccionID exists in the ERP producciones table.
// s3_monitoring_producciones has a foreign key to it, so inserting a row for a
// produccion_id absent from the ERP fails with MySQL error 1452.
func (r *ProductionRepo) ExistsInERP(ctx context.Context, produccionID int64) (bool, error) {
	const q = `SELECT 1 FROM producciones WHERE produccion_id = ? LIMIT 1`

	var one int
	err := r.db.QueryRowContext(ctx, q, produccionID).Scan(&one)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("checking erp production %d: %w", produccionID, err)
	}
	return true, nil
}

// GetERPDetails fetches cultivo, rancho and FK ids by joining ERP tables.
// Returns nil when the production does not exist in the ERP.
func (r *ProductionRepo) GetERPDetails(ctx context.Context, produccionID int64) (*ERPProduccion, error) {
	const q = `
SELECT p.folio, p.articulo_id, p.centro_costo_id,
       a.nombre AS cultivo, c.nombre AS nombre_rancho
FROM producciones p
JOIN articulos a ON p.articulo_id = a.articulo_id
JOIN centros_costos c ON p.centro_costo_id = c.centro_costo_id
WHERE p.produccion_id = ?
`
	row := r.db.QueryRowContext(ctx, q, produccionID)
	var e ERPProduccion
	err := row.Scan(&e.Folio, &e.ArticuloID, &e.CentroCostoID, &e.Cultivo, &e.NombreRancho)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting erp details for produccion %d: %w", produccionID, err)
	}
	return &e, nil
}

// GetCentroCostoByMonitoringID resolves a monitoring production ID to its
// centro_costo_id via the ERP tables. Returns nil when not found.
func (r *ProductionRepo) GetCentroCostoByMonitoringID(ctx context.Context, monitoringID uint) (*int64, error) {
	const q = `
SELECT p.centro_costo_id
FROM s3_monitoring_producciones mp
JOIN producciones p ON mp.produccion_id = p.produccion_id
WHERE mp.s3_monitoring_produccion_id = ?`

	var ccID int64
	err := r.db.QueryRowContext(ctx, q, monitoringID).Scan(&ccID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("resolving centro_costo for monitoring %d: %w", monitoringID, err)
	}
	return &ccID, nil
}

const prodSelectCols = `
SELECT s3_monitoring_produccion_id, produccion_id, cosecha, vaiedades, folio, rancho, prefix,
	monitoring, max_dias_monitoring, fecha_fin, fecha_plantacion,
	pbox, polygon_bbox, tile_bbox, tile_center_lat, tile_center_lon, tile_edge_meters,
	fase2_completa_at, poligono, tif_complete_at, ia_complete_at, ia_auto,
	posible_cosecha, bloqueado, ultima_sincronizacion, fecha_creacion, fecha_actualizacion`

// rowScanner abstracts *sql.Row / *sql.Rows for shared scan logic.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanProduction(row rowScanner) (*domain.Production, error) {
	var p domain.Production
	var monitoring, posibleCosecha, bloqueado, iaAuto int
	var maxDias sql.NullInt64
	var fechaFin, fechaPlantacion, fase2, tifAt, iaAt, ultimaSync sql.NullTime
	var lat, lon sql.NullFloat64
	var tileEdge sql.NullInt32
	var pbox, polygonBbox, tileBbox, poligono []byte
	var fechaAct sql.NullTime
	var folio, rancho sql.NullString

	err := row.Scan(
		&p.ID, &p.ProduccionID, &p.Cosecha, &p.Vaiedades, &folio, &rancho, &p.Prefix,
		&monitoring, &maxDias, &fechaFin, &fechaPlantacion,
		&pbox, &polygonBbox, &tileBbox, &lat, &lon, &tileEdge,
		&fase2, &poligono, &tifAt, &iaAt, &iaAuto,
		&posibleCosecha, &bloqueado, &ultimaSync, &p.FechaCreacion, &fechaAct,
	)
	if err != nil {
		return nil, err
	}

	p.Monitoring = monitoring != 0
	p.PosibleCosecha = posibleCosecha != 0
	p.Bloqueado = bloqueado != 0
	p.IAuto = iaAuto != 0
	p.Folio = folio.String
	p.Rancho = rancho.String

	if maxDias.Valid {
		p.MaxDiasMonitoring = int(maxDias.Int64)
	}
	if fechaFin.Valid {
		p.FechaFin = &fechaFin.Time
	}
	if fechaPlantacion.Valid {
		p.FechaPlantacion = &fechaPlantacion.Time
	}
	if fase2.Valid {
		p.Fase2CompletaAt = &fase2.Time
	}
	if tifAt.Valid {
		p.TifCompleteAt = &tifAt.Time
	}
	if iaAt.Valid {
		p.IaCompleteAt = &iaAt.Time
	}
	if ultimaSync.Valid {
		p.UltimaSincronizacion = &ultimaSync.Time
	}
	if lat.Valid {
		p.TileCenterLat = &lat.Float64
	}
	if lon.Valid {
		p.TileCenterLon = &lon.Float64
	}
	if tileEdge.Valid {
		p.TileEdgeMeters = uint(tileEdge.Int32)
	}
	if fechaAct.Valid {
		p.FechaActualizacion = &fechaAct.Time
	}

	p.PBoxJSON = jsonOrNil(pbox)
	p.PolygonBBoxJSON = jsonOrNil(polygonBbox)
	p.TileBBoxJSON = jsonOrNil(tileBbox)
	p.PoligonoJSON = jsonOrNil(poligono)

	return &p, nil
}

// helpers

func boolToTinyint(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

func nullFloat64Ptr(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func nullUint64Ptr(v *uint64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

func jsonOrNil(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
