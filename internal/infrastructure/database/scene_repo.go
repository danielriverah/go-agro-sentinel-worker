package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"agro-sentinel-worker/internal/domain"
)

// SceneRepo provides access to s3_monitoring_escenas.
type SceneRepo struct {
	db *sql.DB
	// hasImageBBox: la columna la aplica el DBA por separado, así que puede
	// faltar cuando este binario ya está desplegado. Se detecta una vez y las
	// consultas se arman en consecuencia, para no romper el pipeline entre el
	// despliegue del código y el del esquema.
	hasImageBBox bool
	upsertQ      string
	selectCols   string
}

// NewSceneRepo creates a new SceneRepo.
func NewSceneRepo(db *sql.DB) *SceneRepo {
	has := columnExists(db, "s3_monitoring_escenas", "image_bbox")
	return &SceneRepo{
		db:           db,
		hasImageBBox: has,
		upsertQ:      buildSceneUpsert(has),
		selectCols:   buildSceneSelectCols(has),
	}
}

// HasImageBBox indica si la columna image_bbox ya existe. El borrado de
// monitoreo la exige: sin ella no puede preservarse la georreferencia de las
// escenas que dependen de la producción que se va a eliminar.
func (r *SceneRepo) HasImageBBox() bool { return r.hasImageBBox }

// Upsert inserts a scene or updates sync fields if (s3_monitoring_produccion_id, scene_name) already exists.
func (r *SceneRepo) Upsert(ctx context.Context, s *domain.Scene) error {
	now := time.Now().UTC()

	status := s.Status
	if status == "" {
		status = domain.StatusPending
	}

	createdAt := s.FechaCreacion
	if createdAt.IsZero() {
		createdAt = now
	}

	args := []any{
		s.MonitoringProduccionID, s.SceneName, nullTimeVal(s.Fecha),
		nullString(s.SceneJsonKey), nullString(s.SceneJsonUri),
		nullFloat64Ptr(s.CloudCover), status,
		boolToTinyint(s.TruthTifExists), boolToTinyint(s.RenderTifExists),
		boolToTinyint(s.ParamsExists), boolToTinyint(s.IaExists),
		nullTimeVal(s.Fase2CompletaAt), nullString(s.LatestIaRiesgoNivel),
		nullTimeVal(s.LatestIaFechaAnalisis), nullTimeVal(s.UltimaSincronizacion),
		nullFloat64Ptr(s.ProductionCloud),
		boolToTinyint(s.Usable), boolToTinyint(s.Analysis),
		nullString(s.UrlsBandas), nullString(s.BaseBands),
		nullUint64Ptr(s.MultibandRefEscenaID),
	}
	args = append(args, createdAt, now)

	if _, err := r.db.ExecContext(ctx, r.upsertQ, args...); err != nil {
		return fmt.Errorf("upserting scene: %w", err)
	}

	return nil
}

// Los marcadores IMG_* se sustituyen por el fragmento de image_bbox o por nada,
// según exista la columna. Una plantilla única evita que las dos variantes se
// desincronicen.
const sceneUpsertTmpl = `
INSERT INTO s3_monitoring_escenas (
	s3_monitoring_produccion_id, scene_name, fecha, scene_json_key, scene_json_uri,
	cloud_cover, status, truth_tif_exists, render_tif_exists, params_exists, ia_exists,
	fase2_completa_at, latest_ia_riesgo_nivel, latest_ia_fecha_analisis,
	ultima_sincronizacion, production_cloud, usable, analysis, urls_bandas, base_bands,
	multiband_ref_escena_id,IMG_COL
	fecha_creacion, fecha_actualizacion
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,IMG_VAL ?, ?)
ON DUPLICATE KEY UPDATE
	fecha                    = VALUES(fecha),
	scene_json_key           = VALUES(scene_json_key),
	scene_json_uri           = VALUES(scene_json_uri),
	cloud_cover              = VALUES(cloud_cover),
	status                   = VALUES(status),
	truth_tif_exists         = VALUES(truth_tif_exists),
	render_tif_exists        = VALUES(render_tif_exists),
	params_exists            = VALUES(params_exists),
	ia_exists                = VALUES(ia_exists),
	fase2_completa_at        = VALUES(fase2_completa_at),
	latest_ia_riesgo_nivel   = VALUES(latest_ia_riesgo_nivel),
	latest_ia_fecha_analisis = VALUES(latest_ia_fecha_analisis),
	ultima_sincronizacion    = VALUES(ultima_sincronizacion),
	production_cloud         = VALUES(production_cloud),
	usable                   = VALUES(usable),
	analysis                 = VALUES(analysis),
	urls_bandas              = VALUES(urls_bandas),
	base_bands               = VALUES(base_bands),
	multiband_ref_escena_id  = VALUES(multiband_ref_escena_id),IMG_UPD
	fecha_actualizacion      = VALUES(fecha_actualizacion)
`

func buildSceneUpsert(withImageBBox bool) string {
	// image_bbox belongs to the database trigger. Omit it on INSERT/UPDATE,
	// including sync upserts with empty in-memory georeferencing.
	return strings.NewReplacer("IMG_COL", "", "IMG_VAL", "", "IMG_UPD", "").Replace(sceneUpsertTmpl)
}

const sceneSelectTmpl = `
SELECT e.s3_monitoring_escena_id, e.s3_monitoring_produccion_id,
	e.scene_name, e.fecha, e.scene_json_key, e.scene_json_uri,
	e.cloud_cover, e.status,
	e.truth_tif_exists, e.render_tif_exists, e.params_exists, e.ia_exists,
	e.fase2_completa_at, e.latest_ia_riesgo_nivel, e.latest_ia_fecha_analisis,
	e.ultima_sincronizacion, e.production_cloud, e.usable, e.analysis, e.urls_bandas, e.base_bands,
	e.multiband_ref_escena_id,IMG_SEL
	e.fecha_creacion, e.fecha_actualizacion`

func buildSceneSelectCols(withImageBBox bool) string {
	sel := ""
	if withImageBBox {
		sel = "\n\te.image_bbox,"
	}
	return strings.NewReplacer("IMG_SEL", sel).Replace(sceneSelectTmpl)
}

// GetByID fetches a scene by its primary key.
func (r *SceneRepo) GetByID(ctx context.Context, id uint64) (*domain.Scene, error) {
	q := r.selectCols + ` FROM s3_monitoring_escenas e WHERE e.s3_monitoring_escena_id = ?`

	row := r.db.QueryRowContext(ctx, q, id)
	s, err := scanScene(row, r.hasImageBBox)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting scene by id: %w", err)
	}

	return s, nil
}

// GetByProduccionAndSceneName fetches a scene by ERP produccion_id and scene name,
// joining through s3_monitoring_producciones to resolve the FK.
func (r *SceneRepo) GetByProduccionAndSceneName(ctx context.Context, produccionID int64, sceneName string) (*domain.Scene, error) {
	q := r.selectCols + `
FROM s3_monitoring_escenas e
JOIN s3_monitoring_producciones p ON e.s3_monitoring_produccion_id = p.s3_monitoring_produccion_id
WHERE p.produccion_id = ? AND e.scene_name = ?`

	row := r.db.QueryRowContext(ctx, q, produccionID, sceneName)
	s, err := scanScene(row, r.hasImageBBox)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting scene by produccion and scene_name: %w", err)
	}

	return s, nil
}

// ListByMonitoringProduccion returns all scenes for a s3_monitoring_produccion_id.
func (r *SceneRepo) ListByMonitoringProduccion(ctx context.Context, monitoringProduccionID uint) ([]*domain.Scene, error) {
	q := r.selectCols + `
FROM s3_monitoring_escenas e
WHERE e.s3_monitoring_produccion_id = ?
ORDER BY e.fecha DESC`

	rows, err := r.db.QueryContext(ctx, q, monitoringProduccionID)
	if err != nil {
		return nil, fmt.Errorf("listing scenes by monitoring produccion: %w", err)
	}
	defer rows.Close()

	var results []*domain.Scene
	for rows.Next() {
		s, err := scanScene(rows, r.hasImageBBox)
		if err != nil {
			return nil, fmt.Errorf("scanning scene row: %w", err)
		}
		results = append(results, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating scene rows: %w", err)
	}

	return results, nil
}

// UpdateExistsFlags updates the *_exists flags and the usable flag for a scene.
// Called after Phase 3 indexes S3 files so the scene reflects what is available.
func (r *SceneRepo) UpdateExistsFlags(ctx context.Context, escenaID uint64, truthTif, renderTif, params, ia bool) error {
	const q = `
UPDATE s3_monitoring_escenas
SET truth_tif_exists  = ?,
    render_tif_exists = ?,
    params_exists     = ?,
    ia_exists         = ?,
    fecha_actualizacion = ?
WHERE s3_monitoring_escena_id = ?`
	_, err := r.db.ExecContext(ctx, q,
		boolToTinyint(truthTif), boolToTinyint(renderTif),
		boolToTinyint(params), boolToTinyint(ia),
		time.Now().UTC(), escenaID,
	)
	if err != nil {
		return fmt.Errorf("updating exists flags for escena %d: %w", escenaID, err)
	}
	return nil
}

// SetAnalysis marks ia_exists and analysis=true on a scene after a successful IA run.
func (r *SceneRepo) SetAnalysis(ctx context.Context, escenaID uint64, analysis bool) error {
	const q = `
UPDATE s3_monitoring_escenas
SET ia_exists = ?, analysis = ?, fecha_actualizacion = ?
WHERE s3_monitoring_escena_id = ?`
	_, err := r.db.ExecContext(ctx, q, boolToTinyint(analysis), boolToTinyint(analysis), time.Now().UTC(), escenaID)
	if err != nil {
		return fmt.Errorf("setting analysis flag for escena %d: %w", escenaID, err)
	}
	return nil
}

// ResetProcessingToPending resets all PROCESSING scenes for a production back
// to PENDING so that a manual re-run picks them up instead of skipping them.
func (r *SceneRepo) ResetAllProcessingToPending(ctx context.Context) (int64, error) {
	const q = `UPDATE s3_monitoring_escenas SET status = 'PENDING', fecha_actualizacion = ?
               WHERE status = 'PROCESSING'`
	res, err := r.db.ExecContext(ctx, q, time.Now().UTC())
	if err != nil {
		return 0, fmt.Errorf("resetting all processing scenes to pending: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *SceneRepo) ResetProcessingToPending(ctx context.Context, monitoringProduccionID uint) (int64, error) {
	const q = `UPDATE s3_monitoring_escenas SET status = 'PENDING', fecha_actualizacion = ?
               WHERE s3_monitoring_produccion_id = ? AND status = 'PROCESSING'`
	res, err := r.db.ExecContext(ctx, q, time.Now().UTC(), monitoringProduccionID)
	if err != nil {
		return 0, fmt.Errorf("resetting processing scenes to pending: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// UpdateStatus updates the status of a scene.
func (r *SceneRepo) UpdateStatus(ctx context.Context, id uint64, status string) error {
	const q = `UPDATE s3_monitoring_escenas SET status = ?, fecha_actualizacion = ? WHERE s3_monitoring_escena_id = ?`
	_, err := r.db.ExecContext(ctx, q, status, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("updating scene status: %w", err)
	}
	return nil
}

// SetFailed marks a scene as FAILED.
func (r *SceneRepo) SetFailed(ctx context.Context, id uint64) error {
	const q = `UPDATE s3_monitoring_escenas SET status = 'FAILED', fecha_actualizacion = ? WHERE s3_monitoring_escena_id = ?`
	_, err := r.db.ExecContext(ctx, q, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("setting scene failed: %w", err)
	}
	return nil
}

// UpdateFromParams updates production_cloud, usable and analysis on a scene
// when params or IA result files are indexed by the sync service.
// Only non-nil values are applied — pass nil to leave a field unchanged.
func (r *SceneRepo) UpdateFromParams(ctx context.Context, escenaID uint64, productionCloud *float64, usable, analysis *bool) error {
	if productionCloud == nil && usable == nil && analysis == nil {
		return nil
	}

	setClauses := []string{"fecha_actualizacion = ?"}
	args := []any{time.Now().UTC()}

	if productionCloud != nil {
		setClauses = append(setClauses, "production_cloud = ?")
		args = append(args, *productionCloud)
	}
	if usable != nil {
		setClauses = append(setClauses, "usable = ?")
		args = append(args, boolToTinyint(*usable))
	}
	if analysis != nil {
		setClauses = append(setClauses, "analysis = ?")
		args = append(args, boolToTinyint(*analysis))
	}

	// Build SET clause: fecha_actualizacion always first, then the others.
	// Re-order so fecha_actualizacion goes last for readability.
	setClauses = append(setClauses[1:], setClauses[0])
	args = append(args[1:], args[0])
	args = append(args, escenaID)

	q := "UPDATE s3_monitoring_escenas SET " + strings.Join(setClauses, ", ") + " WHERE s3_monitoring_escena_id = ?"
	if _, err := r.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("updating scene from params escena %d: %w", escenaID, err)
	}
	return nil
}

// PendingStatuses are the scene states the worker treats as work to do.
// FAILED counts the same as PENDING (a failed scene is retried), and
// PROCESSING is included because ProcessAllPending resets stale PROCESSING
// rows back to PENDING before it starts.
//
// It is a SQL fragment rather than parameters so the same definition can be
// shared by the queries that must agree with each other. It is a compile-time
// constant, never user input.
const PendingStatuses = `('PENDING','FAILED','PROCESSING')`

// processableStatuses are the states ProcessAllPending actually walks: the
// pending ones plus COMPLETED, which get their params and ia_req regenerated
// when another production shares that scene_name. SKIPPED is excluded — the
// loop skips those, so counting them would inflate the total.
const processableStatuses = `('PENDING','FAILED','PROCESSING','COMPLETED')`

// firstGapFecha is the date of the earliest scene still to be processed in the
// production owning row `e` — its first gap in the historical chain.
//
// It yields NULL when that production has nothing pending, and since any
// comparison against NULL is false, a fully up-to-date production is excluded
// on its own without a separate EXISTS check.
const firstGapFecha = `
    (
      SELECT MIN(ep.fecha)
      FROM s3_monitoring_escenas ep
      WHERE ep.s3_monitoring_produccion_id = e.s3_monitoring_produccion_id
        AND ep.status IN ` + PendingStatuses + `
    )`

// sceneNeedsWork decides whether one production-scene row takes part in a run.
//
// A pending row always does. A COMPLETED row only does when it sits at or
// after its production's first gap: params carry a historical chain forward,
// so filling a gap invalidates the scenes after it, never the ones before.
// Regenerating those earlier scenes was pure waste.
//
// Keeping this in one place means CountScenesToProcess and ListAllBySceneName
// cannot drift apart — if they disagree, the progress bar never reaches 100%.
const sceneNeedsWork = `
  (
    e.status IN ` + PendingStatuses + `
    OR e.fecha >= ` + firstGapFecha + `
  )`

// CountScenesToProcess returns how many scene rows a ProcessAllPending run will
// walk, so the UI can render a determinate progress bar. It must use exactly
// the same filters as ListAllBySceneName: scenes whose scene_name has pending
// work somewhere, restricted to productions that themselves have pending work.
//
// The count is per production-scene row, not per scene_name, because
// scenes_done increments once for every row processed.
func (r *SceneRepo) CountScenesToProcess(ctx context.Context) (int, error) {
	const q = `
SELECT COUNT(*)
FROM s3_monitoring_escenas e
JOIN s3_monitoring_producciones p ON e.s3_monitoring_produccion_id = p.s3_monitoring_produccion_id
WHERE p.monitoring = 1
  AND e.status IN ` + processableStatuses + `
  AND e.scene_name IN (
    SELECT DISTINCT e2.scene_name
    FROM s3_monitoring_escenas e2
    JOIN s3_monitoring_producciones p2 ON e2.s3_monitoring_produccion_id = p2.s3_monitoring_produccion_id
    WHERE e2.status IN ` + PendingStatuses + `
      AND p2.monitoring = 1
  )
  AND` + sceneNeedsWork

	var n int
	if err := r.db.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return 0, fmt.Errorf("counting scenes to process: %w", err)
	}
	return n, nil
}

// FindOldestPendingSceneName returns the scene_name and fecha of the globally
// oldest PENDING or FAILED scene across all productions with monitoring=true.
// Returns ("", zero, nil) when there are no pending scenes.
func (r *SceneRepo) FindOldestPendingSceneName(ctx context.Context) (sceneName string, fecha time.Time, err error) {
	const q = `
SELECT e.scene_name, e.fecha
FROM s3_monitoring_escenas e
JOIN s3_monitoring_producciones p ON e.s3_monitoring_produccion_id = p.s3_monitoring_produccion_id
WHERE e.status IN ('PENDING','FAILED')
  AND p.monitoring = 1
ORDER BY e.fecha ASC
LIMIT 1`

	row := r.db.QueryRowContext(ctx, q)
	var sn string
	var f time.Time
	if scanErr := row.Scan(&sn, &f); scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return "", time.Time{}, nil
		}
		return "", time.Time{}, fmt.Errorf("finding oldest pending scene: %w", scanErr)
	}
	return sn, f, nil
}

// ListAllBySceneName returns the rows for the given scene_name that a run must
// touch, across every monitored production.
//
// A pending scene is always included. A COMPLETED one is included only when it
// falls at or after its own production's first gap, because params carry the
// historical chain forward: filling a gap invalidates what comes after it, not
// what came before. A production with no gaps is excluded entirely.
//
// With scenes A..H in time order: a production missing everything contributes
// all of them; one complete through F contributes G onward; one complete
// through D that also holds G and H contributes E and F as work plus G and H
// as regenerations; and one that is fully complete never appears.
//
// Rows are ordered so that scenes that already have a multiband.tif
// (truth_tif_exists=1) come first so later productions can reuse their multiband.
func (r *SceneRepo) ListAllBySceneName(ctx context.Context, sceneName string) ([]*domain.Scene, error) {
	q := r.selectCols + `
FROM s3_monitoring_escenas e
JOIN s3_monitoring_producciones p ON e.s3_monitoring_produccion_id = p.s3_monitoring_produccion_id
WHERE e.scene_name = ?
  AND p.monitoring = 1
  AND e.status IN ` + processableStatuses + `
  AND` + sceneNeedsWork + `
ORDER BY e.truth_tif_exists DESC, e.s3_monitoring_escena_id ASC`

	rows, err := r.db.QueryContext(ctx, q, sceneName)
	if err != nil {
		return nil, fmt.Errorf("listing scenes by scene_name: %w", err)
	}
	defer rows.Close()

	var results []*domain.Scene
	for rows.Next() {
		s, err := scanScene(rows, r.hasImageBBox)
		if err != nil {
			return nil, fmt.Errorf("scanning scene row: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// GetOldestPendingByProduccion returns the scene with the earliest fecha that
// has status PENDING or FAILED for the given s3_monitoring_produccion_id.
// Returns nil when no actionable scene exists.
func (r *SceneRepo) GetOldestPendingByProduccion(ctx context.Context, monitoringProduccionID uint) (*domain.Scene, error) {
	q := r.selectCols + `
FROM s3_monitoring_escenas e
WHERE e.s3_monitoring_produccion_id = ? AND e.status IN (?, ?)
ORDER BY e.fecha ASC
LIMIT 1`

	row := r.db.QueryRowContext(ctx, q, monitoringProduccionID, domain.StatusPending, domain.StatusFailed)
	s, err := scanScene(row, r.hasImageBBox)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting oldest pending scene: %w", err)
	}
	return s, nil
}

// ListFromDateByProduccion returns all scenes for the given monitoring
// production with fecha >= fromDate, ordered by fecha ASC. This is used to
// identify which scenes need their params/ia_req regenerated after the
// historical chain changes.
func (r *SceneRepo) ListFromDateByProduccion(ctx context.Context, monitoringProduccionID uint, fromDate time.Time) ([]*domain.Scene, error) {
	q := r.selectCols + `
FROM s3_monitoring_escenas e
WHERE e.s3_monitoring_produccion_id = ? AND e.fecha >= ?
ORDER BY e.fecha ASC`

	rows, err := r.db.QueryContext(ctx, q, monitoringProduccionID, fromDate)
	if err != nil {
		return nil, fmt.Errorf("listing scenes from date: %w", err)
	}
	defer rows.Close()

	var results []*domain.Scene
	for rows.Next() {
		s, err := scanScene(rows, r.hasImageBBox)
		if err != nil {
			return nil, fmt.Errorf("scanning scene row: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// GetPreviousUsableScene returns the most recent usable scene before beforeDate
// that also has a params.json registered (params_exists=1). Scenes without
// params are skipped so the historical chain is never broken by a gap.
func (r *SceneRepo) GetPreviousUsableScene(ctx context.Context, monitoringProduccionID uint, beforeDate time.Time) (*domain.Scene, error) {
	q := r.selectCols + `
FROM s3_monitoring_escenas e
WHERE e.s3_monitoring_produccion_id = ? AND e.fecha < ? AND e.usable = 1 AND e.params_exists = 1
ORDER BY e.fecha DESC
LIMIT 1`

	row := r.db.QueryRowContext(ctx, q, monitoringProduccionID, beforeDate)
	s, err := scanScene(row, r.hasImageBBox)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting previous usable scene: %w", err)
	}

	return s, nil
}

// withImageBBox debe coincidir con las columnas que armó buildSceneSelectCols:
// el orden de destinos del Scan tiene que reflejar el de la consulta.
func scanScene(row rowScanner, withImageBBox bool) (*domain.Scene, error) {
	var s domain.Scene
	var monitoring int
	var fecha, fase2, latestIaFecha, ultimaSync, fechaAct sql.NullTime
	var cloudCover, productionCloud sql.NullFloat64
	var sceneJsonKey, sceneJsonUri, latestIaRiesgo, urlsBandas, baseBands sql.NullString
	var truthTif, renderTif, paramsEx, iaEx, usable, analysis int
	var multibandRef sql.NullInt64
	var imageBBox sql.NullString

	dest := []any{
		&s.ID, &monitoring,
		&s.SceneName, &fecha, &sceneJsonKey, &sceneJsonUri,
		&cloudCover, &s.Status,
		&truthTif, &renderTif, &paramsEx, &iaEx,
		&fase2, &latestIaRiesgo, &latestIaFecha,
		&ultimaSync, &productionCloud, &usable, &analysis, &urlsBandas, &baseBands,
		&multibandRef,
	}
	if withImageBBox {
		dest = append(dest, &imageBBox)
	}
	dest = append(dest, &s.FechaCreacion, &fechaAct)

	if err := row.Scan(dest...); err != nil {
		return nil, err
	}

	if imageBBox.Valid && imageBBox.String != "" {
		s.ImageBBox = []byte(imageBBox.String)
	}

	s.MonitoringProduccionID = uint(monitoring)
	s.TruthTifExists = truthTif != 0
	s.RenderTifExists = renderTif != 0
	s.ParamsExists = paramsEx != 0
	s.IaExists = iaEx != 0
	s.Usable = usable != 0
	s.Analysis = analysis != 0

	if fecha.Valid {
		s.Fecha = &fecha.Time
	}
	if fase2.Valid {
		s.Fase2CompletaAt = &fase2.Time
	}
	if latestIaFecha.Valid {
		s.LatestIaFechaAnalisis = &latestIaFecha.Time
	}
	if ultimaSync.Valid {
		s.UltimaSincronizacion = &ultimaSync.Time
	}
	if fechaAct.Valid {
		s.FechaActualizacion = &fechaAct.Time
	}
	if cloudCover.Valid {
		s.CloudCover = &cloudCover.Float64
	}
	if productionCloud.Valid {
		s.ProductionCloud = &productionCloud.Float64
	}
	s.SceneJsonKey = sceneJsonKey.String
	s.SceneJsonUri = sceneJsonUri.String
	s.LatestIaRiesgoNivel = latestIaRiesgo.String
	s.UrlsBandas = urlsBandas.String
	s.BaseBands = baseBands.String
	if multibandRef.Valid {
		v := uint64(multibandRef.Int64)
		s.MultibandRefEscenaID = &v
	}

	return &s, nil
}

// FindMultibandSources returns scenes from OTHER productions that:
//   - have the same scene_name
//   - are origin multibands (multiband_ref_escena_id IS NULL)
//   - have truth_tif_exists = 1
//
// The result includes the scene's ID, and the origin production's tile_bbox and
// S3 prefix so the worker can check containment and download if it fits.
func (r *SceneRepo) FindMultibandSources(ctx context.Context, sceneName string, excludeProduccionID int64) ([]*domain.MultibandSource, error) {
	// Without this column we cannot distinguish an original from a detached
	// reused raster. Process locally instead of risking incorrect placement.
	if !r.hasImageBBox {
		return nil, nil
	}
	const q = `
SELECT e.s3_monitoring_escena_id,
       p.tile_bbox,
       p.prefix,
       e.scene_name
FROM s3_monitoring_escenas e
JOIN s3_monitoring_producciones p ON e.s3_monitoring_produccion_id = p.s3_monitoring_produccion_id
WHERE e.scene_name = ?
  AND p.produccion_id != ?
  AND e.multiband_ref_escena_id IS NULL
  AND e.image_bbox IS NULL
  AND e.truth_tif_exists = 1
  AND e.status = 'COMPLETED'
ORDER BY e.s3_monitoring_escena_id ASC`

	rows, err := r.db.QueryContext(ctx, q, sceneName, excludeProduccionID)
	if err != nil {
		return nil, fmt.Errorf("finding multiband sources: %w", err)
	}
	defer rows.Close()

	var results []*domain.MultibandSource
	for rows.Next() {
		var escenaID uint64
		var tileBBoxRaw, prefix, sname sql.NullString
		if err := rows.Scan(&escenaID, &tileBBoxRaw, &prefix, &sname); err != nil {
			return nil, fmt.Errorf("scanning multiband source row: %w", err)
		}

		if !tileBBoxRaw.Valid || prefix.String == "" {
			continue
		}

		bbox := parseTileBBoxStr(tileBBoxRaw.String)
		if bbox == nil {
			continue
		}

		key := strings.TrimRight(prefix.String, "/") + "/" + sname.String + "/multiband.tif"
		results = append(results, &domain.MultibandSource{
			EscenaID:     escenaID,
			TileBBox:     *bbox,
			MultibandKey: key,
		})
	}
	return results, rows.Err()
}

// parseTileBBoxStr parses a tile_bbox JSON string into a BBox.
func parseTileBBoxStr(raw string) *domain.BBox {
	prod := &domain.Production{TileBBoxJSON: []byte(raw)}
	return prod.ParseTileBBox()
}

// ListTimelineRows returns every scene of a production in chronological order,
// each with the content of its params.json — the source of the per-index
// statistics the timeline chart plots.
//
// params.json is read from json_content, which both the worker and the sync
// indexer populate for JSON outputs, so the whole timeline comes from a single
// query and never touches S3. The correlated subquery (rather than a join)
// guarantees one row per scene even if a scene ever ends up with more than one
// params file indexed.
func (r *SceneRepo) ListTimelineRows(ctx context.Context, monitoringProduccionID uint) ([]*domain.TimelineRow, error) {
	const q = `
SELECT e.s3_monitoring_escena_id, e.scene_name, e.fecha, e.cloud_cover,
       e.production_cloud, e.usable, e.status,
       (SELECT a.json_content
          FROM s3_monitoring_escena_archivos a
         WHERE a.s3_monitoring_escena_id = e.s3_monitoring_escena_id
           AND a.tipo = 'params'
         ORDER BY a.s3_monitoring_escena_archivo_id DESC
         LIMIT 1) AS params_json
FROM s3_monitoring_escenas e
WHERE e.s3_monitoring_produccion_id = ?
ORDER BY e.fecha ASC, e.s3_monitoring_escena_id ASC`

	rows, err := r.db.QueryContext(ctx, q, monitoringProduccionID)
	if err != nil {
		return nil, fmt.Errorf("listing timeline rows: %w", err)
	}
	defer rows.Close()

	var results []*domain.TimelineRow
	for rows.Next() {
		var (
			row        domain.TimelineRow
			fecha      sql.NullTime
			cloudCover sql.NullFloat64
			prodCloud  sql.NullFloat64
			usable     sql.NullBool
			status     sql.NullString
			paramsJSON sql.NullString
		)
		if err := rows.Scan(&row.EscenaID, &row.SceneName, &fecha, &cloudCover,
			&prodCloud, &usable, &status, &paramsJSON); err != nil {
			return nil, fmt.Errorf("scanning timeline row: %w", err)
		}

		if fecha.Valid {
			f := fecha.Time
			row.Fecha = &f
		}
		if cloudCover.Valid {
			c := cloudCover.Float64
			row.CloudCover = &c
		}
		if prodCloud.Valid {
			p := prodCloud.Float64
			row.ProductionCloud = &p
		}
		row.Usable = usable.Valid && usable.Bool
		row.Status = status.String
		row.ParamsJSON = paramsJSON.String

		results = append(results, &row)
	}

	return results, rows.Err()
}

func nullTimeVal(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}
