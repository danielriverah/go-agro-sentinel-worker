package sync

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"agro-sentinel-worker/internal/domain"
)

// coordPairRe matches "x y" coordinate pairs inside a WKT geometry string.
var coordPairRe = regexp.MustCompile(`(-?\d+(?:\.\d+)?)\s+(-?\d+(?:\.\d+)?)`)

// CalculateBBoxFromWKT parses a WKT POLYGON (or MULTIPOLYGON) string, as
// returned by MySQL spatial columns via ST_AsText, and returns the envelope
// (bounding box) of all its coordinates. It does not depend on GDAL — this
// project uses pure Go WKT parsing for BBOX extraction.
func CalculateBBoxFromWKT(wkt string) (*domain.BBox, error) {
	trimmed := strings.TrimSpace(wkt)
	if trimmed == "" {
		return nil, fmt.Errorf("empty WKT string")
	}

	// Strip an optional "SRID=4326;" prefix.
	if idx := strings.Index(trimmed, ";"); idx != -1 {
		trimmed = trimmed[idx+1:]
	}

	upper := strings.ToUpper(trimmed)
	if !strings.HasPrefix(upper, "POLYGON") && !strings.HasPrefix(upper, "MULTIPOLYGON") {
		return nil, fmt.Errorf("unsupported WKT geometry type: %s", trimmed)
	}

	matches := coordPairRe.FindAllStringSubmatch(trimmed, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no coordinates found in WKT: %s", trimmed)
	}

	var bbox domain.BBox
	for i, m := range matches {
		x, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			return nil, fmt.Errorf("parsing x coordinate %q: %w", m[1], err)
		}
		y, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			return nil, fmt.Errorf("parsing y coordinate %q: %w", m[2], err)
		}

		if i == 0 {
			bbox.MinX, bbox.MaxX = x, x
			bbox.MinY, bbox.MaxY = y, y
			continue
		}

		if x < bbox.MinX {
			bbox.MinX = x
		}
		if x > bbox.MaxX {
			bbox.MaxX = x
		}
		if y < bbox.MinY {
			bbox.MinY = y
		}
		if y > bbox.MaxY {
			bbox.MaxY = y
		}
	}

	return &bbox, nil
}
