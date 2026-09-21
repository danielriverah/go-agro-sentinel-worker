package database

import (
	"context"
	"crypto/md5"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"agro-sentinel-worker/internal/domain"
)

// FileRepo provides access to s3_monitoring_escena_archivos.
type FileRepo struct {
	db *sql.DB
}

// NewFileRepo creates a new FileRepo.
func NewFileRepo(db *sql.DB) *FileRepo {
	return &FileRepo{db: db}
}

// Create inserts a new scene file record.
// S3KeyHash is auto-computed from S3Key when not set.
// Existe defaults to true for new files.
func (r *FileRepo) Create(ctx context.Context, f *domain.SceneFile) error {
	now := time.Now().UTC()

	hash := f.S3KeyHash
	if hash == "" {
		hash = fmt.Sprintf("%x", md5.Sum([]byte(f.S3Key)))
	}

	const q = `
INSERT INTO s3_monitoring_escena_archivos (
	s3_monitoring_escena_id, tipo, s3_key, s3_key_hash, s3_uri,
	extension, size_bytes, last_modified, existe, json_content,
	fecha_creacion, fecha_actualizacion
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
	size_bytes         = VALUES(size_bytes),
	last_modified      = VALUES(last_modified),
	existe             = VALUES(existe),
	json_content       = VALUES(json_content),
	fecha_actualizacion = VALUES(fecha_actualizacion)
`

	createdAt := f.FechaCreacion
	if createdAt.IsZero() {
		createdAt = now
	}

	res, err := r.db.ExecContext(ctx, q,
		f.EscenaID, f.Tipo, f.S3Key, hash, f.S3Uri,
		nullString(f.Extension), f.SizeBytes, nullTime(f.LastModified),
		boolToTinyint(f.Existe || true),
		nullString(f.JsonContent),
		createdAt, now,
	)
	if err != nil {
		return fmt.Errorf("creating scene file: %w", err)
	}

	if id, err := res.LastInsertId(); err == nil {
		f.ID = uint64(id)
	}
	f.S3KeyHash = hash

	return nil
}

const fileSelectCols = `
s3_monitoring_escena_archivo_id, s3_monitoring_escena_id, tipo, s3_key, s3_key_hash,
s3_uri, extension, size_bytes, last_modified, existe, json_content, fecha_creacion`

// ListByEscena returns all files for a given s3_monitoring_escena_id.
func (r *FileRepo) ListByEscena(ctx context.Context, escenaID uint64) ([]*domain.SceneFile, error) {
	q := "SELECT " + fileSelectCols + " FROM s3_monitoring_escena_archivos WHERE s3_monitoring_escena_id = ?"

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

// ExistsByKeyHash reports whether a file with the given s3_key_hash is already
// indexed for escenaID.
func (r *FileRepo) ExistsByKeyHash(ctx context.Context, escenaID uint64, hash string) (bool, error) {
	const q = `SELECT 1 FROM s3_monitoring_escena_archivos WHERE s3_monitoring_escena_id = ? AND s3_key_hash = ? LIMIT 1`
	var dummy int
	err := r.db.QueryRowContext(ctx, q, escenaID, hash).Scan(&dummy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("checking file by key hash: %w", err)
	}
	return true, nil
}

// GetByTipo returns the file of a given tipo for a scene, if present.
func (r *FileRepo) GetByTipo(ctx context.Context, escenaID uint64, tipo string) (*domain.SceneFile, error) {
	q := "SELECT " + fileSelectCols + " FROM s3_monitoring_escena_archivos WHERE s3_monitoring_escena_id = ? AND tipo = ? LIMIT 1"

	row := r.db.QueryRowContext(ctx, q, escenaID, tipo)
	f, err := scanSceneFile(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting file by tipo: %w", err)
	}

	return f, nil
}

func scanSceneFile(row rowScanner) (*domain.SceneFile, error) {
	var f domain.SceneFile
	var extension, jsonContent sql.NullString
	var lastModified sql.NullTime
	var existe int

	err := row.Scan(
		&f.ID, &f.EscenaID, &f.Tipo, &f.S3Key, &f.S3KeyHash,
		&f.S3Uri, &extension, &f.SizeBytes, &lastModified, &existe,
		&jsonContent, &f.FechaCreacion,
	)
	if err != nil {
		return nil, err
	}

	f.Extension = extension.String
	f.JsonContent = jsonContent.String
	f.Existe = existe != 0
	if lastModified.Valid {
		f.LastModified = &lastModified.Time
	}

	return &f, nil
}
