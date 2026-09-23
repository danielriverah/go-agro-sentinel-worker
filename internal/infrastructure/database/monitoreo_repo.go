package database

import (
	"context"
	"database/sql"
	"fmt"
)

// MonitoreoRepo implementa el borrado del monitoreo de una producción en MySQL.
type MonitoreoRepo struct {
	db           *sql.DB
	hasImageBBox bool
}

// NewMonitoreoRepo crea el repo y detecta si image_bbox está disponible.
func NewMonitoreoRepo(db *sql.DB) *MonitoreoRepo {
	return &MonitoreoRepo{
		db:           db,
		hasImageBBox: columnExists(db, "s3_monitoring_escenas", "image_bbox"),
	}
}

// HasImageBBox indica si la columna existe. El borrado la exige: sin ella no
// hay dónde preservar la georreferencia de las escenas dependientes.
func (r *MonitoreoRepo) HasImageBBox() bool { return r.hasImageBBox }

// ResumenBorradoMySQL cuenta lo eliminado en cada tabla.
type ResumenBorradoMySQL struct {
	Dependientes int64 `json:"escenas_dependientes_desvinculadas"`
	Archivos     int64 `json:"archivos"`
	Analisis     int64 `json:"analisis_ia"`
	Escenas      int64 `json:"escenas"`
	Monitoreo    int64 `json:"filas_monitoreo"`
	ERPApagado   bool  `json:"monitoring_erp_apagado"`
}

// EliminarMonitoreo borra en una sola transacción todo el monitoreo de una
// producción y apaga producciones.monitoring en la maestra del ERP.
//
// NO toca la fila de producciones (salvo ese campo) ni las fases de cultivo:
// ambas deben sobrevivir para servir de base a ciclos futuros.
//
// Va al final de la operación completa, después de S3 y DynamoDB: si algo
// falla antes, estos registros siguen visibles y el borrado puede reintentarse.
// Al revés quedarían archivos huérfanos imposibles de localizar.
func (r *MonitoreoRepo) EliminarMonitoreo(ctx context.Context, monitoringID uint, produccionID int64) (ResumenBorradoMySQL, error) {
	var res ResumenBorradoMySQL

	if !r.hasImageBBox {
		return res, fmt.Errorf("falta la columna image_bbox: aplica scripts/phase3-add-image-bbox.sql antes de borrar")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return res, fmt.Errorf("abriendo transacción de borrado: %w", err)
	}
	defer tx.Rollback()

	// 1. Preservar la georreferencia de las escenas de OTRAS producciones que
	//    reutilizaron un multiband de ésta. Debe ir ANTES de romper el vínculo:
	//    después ya no habría forma de saber de quién heredaban el bbox.
	const preservar = `
UPDATE s3_monitoring_escenas dep
JOIN s3_monitoring_escenas origen
  ON origen.s3_monitoring_escena_id = dep.multiband_ref_escena_id
JOIN s3_monitoring_producciones po
  ON po.s3_monitoring_produccion_id = origen.s3_monitoring_produccion_id
SET dep.image_bbox = po.tile_bbox
WHERE origen.s3_monitoring_produccion_id = ?
  AND dep.image_bbox IS NULL
  AND po.tile_bbox IS NOT NULL`

	if _, err := tx.ExecContext(ctx, preservar, monitoringID); err != nil {
		return res, fmt.Errorf("preservando bbox de escenas dependientes: %w", err)
	}

	// 2. Romper el vínculo: sin esto quedarían apuntando a escenas borradas.
	const desvincular = `
UPDATE s3_monitoring_escenas dep
JOIN s3_monitoring_escenas origen
  ON origen.s3_monitoring_escena_id = dep.multiband_ref_escena_id
SET dep.multiband_ref_escena_id = NULL
WHERE origen.s3_monitoring_produccion_id = ?`

	if res.Dependientes, err = execAffected(ctx, tx, desvincular, monitoringID); err != nil {
		return res, fmt.Errorf("desvinculando escenas dependientes: %w", err)
	}

	// 3. Hijos antes que padres.
	const borrarArchivos = `
DELETE a FROM s3_monitoring_escena_archivos a
JOIN s3_monitoring_escenas e ON e.s3_monitoring_escena_id = a.s3_monitoring_escena_id
WHERE e.s3_monitoring_produccion_id = ?`

	if res.Archivos, err = execAffected(ctx, tx, borrarArchivos, monitoringID); err != nil {
		return res, fmt.Errorf("borrando archivos de escenas: %w", err)
	}

	const borrarIA = `
DELETE ir FROM s3_monitoring_escena_ia_resumen ir
JOIN s3_monitoring_escenas e ON e.s3_monitoring_escena_id = ir.s3_monitoring_escena_id
WHERE e.s3_monitoring_produccion_id = ?`

	if res.Analisis, err = execAffected(ctx, tx, borrarIA, monitoringID); err != nil {
		return res, fmt.Errorf("borrando análisis IA: %w", err)
	}

	if res.Escenas, err = execAffected(ctx,
		tx, `DELETE FROM s3_monitoring_escenas WHERE s3_monitoring_produccion_id = ?`, monitoringID); err != nil {
		return res, fmt.Errorf("borrando escenas: %w", err)
	}

	if res.Monitoreo, err = execAffected(ctx,
		tx, `DELETE FROM s3_monitoring_producciones WHERE s3_monitoring_produccion_id = ?`, monitoringID); err != nil {
		return res, fmt.Errorf("borrando fila de monitoreo: %w", err)
	}

	// 4. Apagar el monitoreo en la maestra del ERP. Sin esto, el sync volvería
	//    a dar de alta la producción en el siguiente ciclo y se descargaría
	//    todo otra vez. Es la única escritura del servicio sobre el ERP.
	if _, err := tx.ExecContext(ctx,
		`UPDATE producciones SET monitoring = 0 WHERE produccion_id = ?`, produccionID); err != nil {
		return res, fmt.Errorf("apagando monitoring en producciones: %w", err)
	}
	res.ERPApagado = true

	if err := tx.Commit(); err != nil {
		return res, fmt.Errorf("confirmando borrado: %w", err)
	}

	return res, nil
}

func execAffected(ctx context.Context, tx *sql.Tx, query string, args ...any) (int64, error) {
	out, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	n, err := out.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}
