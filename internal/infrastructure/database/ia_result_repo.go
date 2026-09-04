package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"agro-sentinel-worker/internal/domain"
)

// IAResultRepository provides CRUD access to s3_monitoring_escena_ia_resumen.
type IAResultRepository struct {
	db *sql.DB
}

// NewIAResultRepository creates a new IAResultRepository.
func NewIAResultRepository(db *sql.DB) *IAResultRepository {
	return &IAResultRepository{db: db}
}

// Upsert inserts an IA result or updates it if s3_monitoring_escena_id already exists.
func (r *IAResultRepository) Upsert(ctx context.Context, result *domain.IAResultSummary) error {
	now := time.Now().UTC()

	createdAt := result.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	const q = `
INSERT INTO s3_monitoring_escena_ia_resumen (
	s3_monitoring_escena_id, estado_clave, estado_general, riesgo_nivel, riesgo_motivo,
	fecha_analisis, json_original, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
	estado_clave = VALUES(estado_clave),
	estado_general = VALUES(estado_general),
	riesgo_nivel = VALUES(riesgo_nivel),
	riesgo_motivo = VALUES(riesgo_motivo),
	fecha_analisis = VALUES(fecha_analisis),
	json_original = VALUES(json_original),
	updated_at = VALUES(updated_at)
`

	_, err := r.db.ExecContext(ctx, q,
		result.S3MonitoringEscenaID, result.EstadoClave, result.EstadoGeneral, result.RiesgoNivel, nullString(result.RiesgoMotivo),
		result.FechaAnalisis, nullString(result.JSONOriginal), createdAt, now,
	)
	if err != nil {
		return fmt.Errorf("upserting ia result: %w", err)
	}

	return nil
}

// GetByEscenaID fetches an IA result by its escena_id.
func (r *IAResultRepository) GetByEscenaID(ctx context.Context, escenaID int64) (*domain.IAResultSummary, error) {
	const q = `
SELECT id, s3_monitoring_escena_id, estado_clave, estado_general, riesgo_nivel, riesgo_motivo,
	fecha_analisis, json_original, created_at, updated_at
FROM s3_monitoring_escena_ia_resumen
WHERE s3_monitoring_escena_id = ?
`

	row := r.db.QueryRowContext(ctx, q, escenaID)
	result, err := scanIAResult(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting ia result by escena_id: %w", err)
	}

	return result, nil
}

// ListByProduccion returns all IA results for a given produccion_id (via escenas).
func (r *IAResultRepository) ListByProduccion(ctx context.Context, produccionID int64) ([]*domain.IAResultSummary, error) {
	const q = `
SELECT ir.id, ir.s3_monitoring_escena_id, ir.estado_clave, ir.estado_general, ir.riesgo_nivel, ir.riesgo_motivo,
	ir.fecha_analisis, ir.json_original, ir.created_at, ir.updated_at
FROM s3_monitoring_escena_ia_resumen ir
INNER JOIN s3_monitoring_escenas e ON ir.s3_monitoring_escena_id = e.id
WHERE e.produccion_id = ?
ORDER BY ir.created_at DESC
`

	rows, err := r.db.QueryContext(ctx, q, produccionID)
	if err != nil {
		return nil, fmt.Errorf("listing ia results by produccion: %w", err)
	}
	defer rows.Close()

	var results []*domain.IAResultSummary
	for rows.Next() {
		result, err := scanIAResult(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning ia result row: %w", err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating ia result rows: %w", err)
	}

	return results, nil
}

// DeleteByEscenaID deletes an IA result by its escena_id.
func (r *IAResultRepository) DeleteByEscenaID(ctx context.Context, escenaID int64) error {
	const q = `DELETE FROM s3_monitoring_escena_ia_resumen WHERE s3_monitoring_escena_id = ?`

	result, err := r.db.ExecContext(ctx, q, escenaID)
	if err != nil {
		return fmt.Errorf("deleting ia result by escena_id: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		// No rows deleted is not an error, just log it
		return nil
	}

	return nil
}

// scanIAResult maps database row to IAResultSummary struct.
func scanIAResult(row rowScanner) (*domain.IAResultSummary, error) {
	var result domain.IAResultSummary
	var riesgoMotivo, jsonOriginal sql.NullString
	var fechaAnalisis sql.NullTime

	err := row.Scan(
		&result.ID, &result.S3MonitoringEscenaID, &result.EstadoClave, &result.EstadoGeneral,
		&result.RiesgoNivel, &riesgoMotivo, &fechaAnalisis, &jsonOriginal,
		&result.CreatedAt, &result.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	result.RiesgoMotivo = riesgoMotivo.String
	result.JSONOriginal = jsonOriginal.String
	if fechaAnalisis.Valid {
		result.FechaAnalisis = &fechaAnalisis.Time
	}

	return &result, nil
}
