package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"agro-sentinel-worker/internal/domain"
)

// FileRepo provides CRUD access to s3_monitoring_escena_archivos.
type FileRepo struct {
	db *sql.DB
}

// NewFileRepo creates a new FileRepo.
func NewFileRepo(db *sql.DB) *FileRepo {
	return &FileRepo{db: db}
}

// Create inserts a new scene file record.
func (r *FileRepo) Create(ctx context.Context, f *domain.SceneFile) error {
	createdAt := f.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	const q = `
INSERT INTO s3_monitoring_escena_archivos (
	escena_id, file_type, file_name, s3_key, s3_bucket,
	file_size_bytes, resolution_m, width_px, height_px, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`

	res, err := r.db.ExecContext(ctx, q,
		f.EscenaID, f.FileType, f.FileName, f.S3Key, f.S3Bucket,
		f.FileSizeBytes, f.ResolutionM, f.WidthPx, f.HeightPx, createdAt,
	)
	if err != nil {
		return fmt.Errorf("creating scene file: %w", err)
	}

	if id, err := res.LastInsertId(); err == nil {
		f.ID = id
	}

	return nil
}

const fileSelectCols = `
id, escena_id, file_type, file_name, s3_key, s3_bucket,
file_size_bytes, resolution_m, width_px, height_px, created_at
`

// ListByEscena returns all files for a given scene.
func (r *FileRepo) ListByEscena(ctx context.Context, escenaID int64) ([]*domain.SceneFile, error) {
	q := "SELECT " + fileSelectCols + " FROM s3_monitoring_escena_archivos WHERE escena_id = ?"

	rows, err := r.db.QueryContext(ctx, q, escenaID)
	if err != nil {
		return nil, fmt.Errorf("listing files by escena: %w", err)
	}
	defer rows.Close()

	var results []*domain.SceneFile
	for rows.Next() {
		f, err := scanSceneFile(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning scene file row: %w", err)
		}
		results = append(results, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating scene file rows: %w", err)
	}

	return results, nil
}

// GetByType returns the file of a given type for a scene, if present.
func (r *FileRepo) GetByType(ctx context.Context, escenaID int64, fileType domain.FileType) (*domain.SceneFile, error) {
	q := "SELECT " + fileSelectCols + " FROM s3_monitoring_escena_archivos WHERE escena_id = ? AND file_type = ? LIMIT 1"

	row := r.db.QueryRowContext(ctx, q, escenaID, fileType)
	f, err := scanSceneFile(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting file by type: %w", err)
	}

	return f, nil
}

func scanSceneFile(row rowScanner) (*domain.SceneFile, error) {
	var f domain.SceneFile
	var fileType string

	err := row.Scan(
		&f.ID, &f.EscenaID, &fileType, &f.FileName, &f.S3Key, &f.S3Bucket,
		&f.FileSizeBytes, &f.ResolutionM, &f.WidthPx, &f.HeightPx, &f.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	f.FileType = domain.FileType(fileType)

	return &f, nil
}
