package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"agro-sentinel-worker/internal/domain"
)

// SceneRepo provides CRUD access to s3_monitoring_escenas.
type SceneRepo struct {
	db *sql.DB
}

// NewSceneRepo creates a new SceneRepo.
func NewSceneRepo(db *sql.DB) *SceneRepo {
	return &SceneRepo{db: db}
}

// Upsert inserts a scene, or updates it if (produccion_id, scene_id) already exists.
func (r *SceneRepo) Upsert(ctx context.Context, s *domain.Scene) error {
	now := time.Now().UTC()
	createdAt := s.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	var cloudCoverBBox any
	if s.CloudCoverBBox != nil {
		cloudCoverBBox = *s.CloudCoverBBox
	}

	status := s.Status
	if status == "" {
		status = domain.StatusPending
	}

	const q = `
INSERT INTO s3_monitoring_escenas (
	produccion_id, scene_id, scene_date, cloud_cover_scene, cloud_cover_bbox,
	passes_quality, has_multiband, has_params, has_rgb, has_analisis,
	status, error_type, error_message, retry_count, processed_at,
	created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
	scene_date = VALUES(scene_date),
	cloud_cover_scene = VALUES(cloud_cover_scene),
	cloud_cover_bbox = VALUES(cloud_cover_bbox),
	passes_quality = VALUES(passes_quality),
	has_multiband = VALUES(has_multiband),
	has_params = VALUES(has_params),
	has_rgb = VALUES(has_rgb),
	has_analisis = VALUES(has_analisis),
	status = VALUES(status),
	error_type = VALUES(error_type),
	error_message = VALUES(error_message),
	retry_count = VALUES(retry_count),
	processed_at = VALUES(processed_at),
	updated_at = VALUES(updated_at)
`

	_, err := r.db.ExecContext(ctx, q,
		s.ProduccionID, s.SceneID, s.SceneDate, s.CloudCoverScene, cloudCoverBBox,
		s.PassesQuality, s.HasMultiband, s.HasParams, s.HasRGB, s.HasAnalisis,
		status, nullString(s.ErrorType), nullString(s.ErrorMessage), s.RetryCount, s.ProcessedAt,
		createdAt, now,
	)
	if err != nil {
		return fmt.Errorf("upserting scene: %w", err)
	}

	return nil
}

const sceneSelectCols = `
id, produccion_id, scene_id, scene_date, cloud_cover_scene, cloud_cover_bbox,
passes_quality, has_multiband, has_params, has_rgb, has_analisis,
status, error_type, error_message, retry_count, processed_at,
created_at, updated_at
`

// GetByID fetches a scene by its primary key.
func (r *SceneRepo) GetByID(ctx context.Context, id int64) (*domain.Scene, error) {
	q := "SELECT " + sceneSelectCols + " FROM s3_monitoring_escenas WHERE id = ?"

	row := r.db.QueryRowContext(ctx, q, id)
	s, err := scanScene(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting scene by id: %w", err)
	}

	return s, nil
}

// GetByProduccionAndSceneID fetches a scene by its production and external scene id.
func (r *SceneRepo) GetByProduccionAndSceneID(ctx context.Context, produccionID int64, sceneID string) (*domain.Scene, error) {
	q := "SELECT " + sceneSelectCols + " FROM s3_monitoring_escenas WHERE produccion_id = ? AND scene_id = ?"

	row := r.db.QueryRowContext(ctx, q, produccionID, sceneID)
	s, err := scanScene(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting scene by produccion and scene_id: %w", err)
	}

	return s, nil
}

// ListByProduccion returns all scenes for a production, ordered by scene_date descending.
func (r *SceneRepo) ListByProduccion(ctx context.Context, produccionID int64) ([]*domain.Scene, error) {
	q := "SELECT " + sceneSelectCols + " FROM s3_monitoring_escenas WHERE produccion_id = ? ORDER BY scene_date DESC"

	rows, err := r.db.QueryContext(ctx, q, produccionID)
	if err != nil {
		return nil, fmt.Errorf("listing scenes by produccion: %w", err)
	}
	defer rows.Close()

	var results []*domain.Scene
	for rows.Next() {
		s, err := scanScene(rows)
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

// UpdateStatus updates the status field of a scene.
func (r *SceneRepo) UpdateStatus(ctx context.Context, id int64, status domain.JobStatus) error {
	const q = `UPDATE s3_monitoring_escenas SET status = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, q, status, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("updating scene status: %w", err)
	}
	return nil
}

// SetError marks a scene as failed with the given error type and message, incrementing retry_count.
func (r *SceneRepo) SetError(ctx context.Context, id int64, errType string, errMsg string) error {
	const q = `
UPDATE s3_monitoring_escenas
SET status = ?, error_type = ?, error_message = ?, retry_count = retry_count + 1, updated_at = ?
WHERE id = ?
`
	_, err := r.db.ExecContext(ctx, q, domain.StatusFailed, nullString(errType), nullString(errMsg), time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("setting scene error: %w", err)
	}
	return nil
}

// SetCompleted marks a scene as completed, recording quality and cloud cover over the bbox.
func (r *SceneRepo) SetCompleted(ctx context.Context, id int64, passesQuality bool, cloudCoverBBox float64) error {
	now := time.Now().UTC()
	const q = `
UPDATE s3_monitoring_escenas
SET status = ?, passes_quality = ?, cloud_cover_bbox = ?, processed_at = ?, updated_at = ?
WHERE id = ?
`
	_, err := r.db.ExecContext(ctx, q, domain.StatusCompleted, passesQuality, cloudCoverBBox, now, now, id)
	if err != nil {
		return fmt.Errorf("setting scene completed: %w", err)
	}
	return nil
}

// GetPreviousValidScene returns the most recent scene before beforeDate that passes quality.
func (r *SceneRepo) GetPreviousValidScene(ctx context.Context, produccionID int64, beforeDate time.Time) (*domain.Scene, error) {
	q := "SELECT " + sceneSelectCols + ` FROM s3_monitoring_escenas
WHERE produccion_id = ? AND scene_date < ? AND passes_quality = 1
ORDER BY scene_date DESC
LIMIT 1`

	row := r.db.QueryRowContext(ctx, q, produccionID, beforeDate)
	s, err := scanScene(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting previous valid scene: %w", err)
	}

	return s, nil
}

func scanScene(row rowScanner) (*domain.Scene, error) {
	var s domain.Scene
	var cloudCoverBBox sql.NullFloat64
	var errorType, errorMessage sql.NullString
	var processedAt sql.NullTime
	var status string

	err := row.Scan(
		&s.ID, &s.ProduccionID, &s.SceneID, &s.SceneDate, &s.CloudCoverScene, &cloudCoverBBox,
		&s.PassesQuality, &s.HasMultiband, &s.HasParams, &s.HasRGB, &s.HasAnalisis,
		&status, &errorType, &errorMessage, &s.RetryCount, &processedAt,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	s.Status = domain.JobStatus(status)
	if cloudCoverBBox.Valid {
		v := cloudCoverBBox.Float64
		s.CloudCoverBBox = &v
	}
	s.ErrorType = errorType.String
	s.ErrorMessage = errorMessage.String
	if processedAt.Valid {
		s.ProcessedAt = &processedAt.Time
	}

	return &s, nil
}
