package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"agro-sentinel-worker/internal/domain"
)

// PolygonRepo looks up the monitored polygon assigned to a production and
// extracts its bounding box. It is the concrete implementation of
// sync.PolygonRepository.
type PolygonRepo struct {
	db          *sql.DB
	bboxFromWKT func(wkt string) (*domain.BBox, error)
}

// NewPolygonRepo creates a PolygonRepo. bboxFromWKT parses the WKT polygon
// text into a domain.BBox (typically sync.CalculateBBoxFromWKT) — it is
// injected here so the database package does not need to depend on sync.
func NewPolygonRepo(db *sql.DB, bboxFromWKT func(wkt string) (*domain.BBox, error)) *PolygonRepo {
	return &PolygonRepo{db: db, bboxFromWKT: bboxFromWKT}
}

// GetPolygonBBox fetches the monitored polygon for produccionID from
// asignaciones_zonas_producciones and returns its bounding box. It returns
// (nil, nil) when the production has no assigned polygon.
func (r *PolygonRepo) GetPolygonBBox(ctx context.Context, produccionID int64) (*domain.BBox, error) {
	const q = `
SELECT ST_AsText(poligono)
FROM asignaciones_zonas_producciones
WHERE produccion_id = ?
LIMIT 1
`

	var wkt string
	err := r.db.QueryRowContext(ctx, q, produccionID).Scan(&wkt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting polygon for produccion %d: %w", produccionID, err)
	}

	bbox, err := r.bboxFromWKT(wkt)
	if err != nil {
		return nil, fmt.Errorf("parsing polygon for produccion %d: %w", produccionID, err)
	}

	return bbox, nil
}
