package sync

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"agro-sentinel-worker/internal/domain"
)

// coordPairRe matches "x y" coordinate pairs inside a WKT geometry string.
var coordPairRe = regexp.MustCompile(`(-?\d+(?:\.\d+)?)\s+(-?\d+(?:\.\d+)?)`)

// CalculateBBoxFromWKT parses a polygon string and returns its bounding box.
// Supports two formats:
//   - WKT: "POLYGON((lon lat, lon lat, ...))" or "MULTIPOLYGON(...)"
//   - Custom pipe format used in asignaciones_zonas_producciones:
//     "lat,lon|lat,lon|lat,lon" (comma-separated pair, pipe-separated points)
func CalculateBBoxFromWKT(wkt string) (*domain.BBox, error) {
	trimmed := strings.TrimSpace(wkt)
	if trimmed == "" {
		return nil, fmt.Errorf("empty WKT string")
	}

	// Detect pipe-separated "lat,lon|lat,lon" format.
	if strings.Contains(trimmed, "|") || (strings.Contains(trimmed, ",") && !strings.Contains(strings.ToUpper(trimmed), "POLYGON")) {
		return parsePipePolygon(trimmed)
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

	return envelopeFromXY(matches)
}

// ParsePipePolygonToJSON converts "lat,lon|lat,lon|..." to a JSON array of
// [lon, lat] pairs (GeoJSON coordinate order). Returns nil on parse error.
func ParsePipePolygonToJSON(s string) json.RawMessage {
	points := strings.Split(s, "|")
	type point [2]float64 // [lon, lat]
	result := make([]point, 0, len(points))
	for _, p := range points {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		parts := strings.SplitN(p, ",", 2)
		if len(parts) != 2 {
			return nil
		}
		lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil {
			return nil
		}
		lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			return nil
		}
		result = append(result, point{lon, lat})
	}
	b, err := json.Marshal(result)
	if err != nil {
		return nil
	}
	return b
}

// parsePipePolygon parses "lat,lon|lat,lon|..." returning the bounding box.
// Each point is "lat,lon" (Y,X order).
func parsePipePolygon(s string) (*domain.BBox, error) {
	points := strings.Split(s, "|")
	if len(points) < 3 {
		return nil, fmt.Errorf("pipe polygon needs at least 3 points, got %d", len(points))
	}

	var bbox domain.BBox
	for i, p := range points {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		parts := strings.SplitN(p, ",", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid point %q: expected lat,lon", p)
		}
		lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil {
			return nil, fmt.Errorf("parsing lat in %q: %w", p, err)
		}
		lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			return nil, fmt.Errorf("parsing lon in %q: %w", p, err)
		}
		// lat = Y, lon = X
		if i == 0 {
			bbox.MinX, bbox.MaxX = lon, lon
			bbox.MinY, bbox.MaxY = lat, lat
			continue
		}
		if lon < bbox.MinX {
			bbox.MinX = lon
		}
		if lon > bbox.MaxX {
			bbox.MaxX = lon
		}
		if lat < bbox.MinY {
			bbox.MinY = lat
		}
		if lat > bbox.MaxY {
			bbox.MaxY = lat
		}
	}

	return &bbox, nil
}

// CalculateTileBBox returns a square bbox centered at (centerLat, centerLon)
// with a side length of edgeMeters. The box is expanded in degrees using
// the approximation: 1 degree latitude ≈ 111320 m, and longitude degrees
// are scaled by cos(lat).
func CalculateTileBBox(centerLat, centerLon float64, edgeMeters uint) *domain.BBox {
	half := float64(edgeMeters) / 2.0
	halfLatDeg := half / 111320.0
	halfLonDeg := half / (111320.0 * math.Cos(centerLat*math.Pi/180.0))
	return &domain.BBox{
		MinX: centerLon - halfLonDeg,
		MaxX: centerLon + halfLonDeg,
		MinY: centerLat - halfLatDeg,
		MaxY: centerLat + halfLatDeg,
	}
}

// envelopeFromXY builds a BBox from WKT coordinate pair matches (x=lon, y=lat).
func envelopeFromXY(matches [][]string) (*domain.BBox, error) {
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
