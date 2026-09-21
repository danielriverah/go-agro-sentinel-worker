package processing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"agro-sentinel-worker/internal/domain"
)

// WritePolygonGeoJSON writes the production polygon to a temporary GeoJSON
// file suitable for use as a GDAL cutline (-cutline flag in gdalwarp).
//
// The polygon is stored in PoligonoJSON as [[lon,lat],...] (GeoJSON order).
// The output is a FeatureCollection with a single Polygon feature that
// carries produccion_id as a property so the file is identifiable.
//
// Returns the file path. The caller is responsible for removing it.
func WritePolygonGeoJSON(prod *domain.Production, dir string) (string, error) {
	if len(prod.PoligonoJSON) == 0 {
		return "", &domain.ProcessingError{
			Type:    domain.ErrValidation,
			Message: fmt.Sprintf("production %d has no polygon (poligono is empty)", prod.ProduccionID),
		}
	}

	// PoligonoJSON is [[lon,lat],...] — wrap it as a GeoJSON Polygon ring.
	// GeoJSON requires the ring to be closed (first == last point).
	var pts [][2]float64
	if err := json.Unmarshal(prod.PoligonoJSON, &pts); err != nil {
		return "", &domain.ProcessingError{
			Type:    domain.ErrValidation,
			Message: fmt.Sprintf("production %d: cannot parse poligono JSON", prod.ProduccionID),
			Wrapped: err,
		}
	}
	if len(pts) < 3 {
		return "", &domain.ProcessingError{
			Type:    domain.ErrValidation,
			Message: fmt.Sprintf("production %d: polygon has fewer than 3 points", prod.ProduccionID),
		}
	}

	// Close the ring if not already closed.
	ring := make([][2]float64, len(pts))
	copy(ring, pts)
	if ring[0] != ring[len(ring)-1] {
		ring = append(ring, ring[0])
	}

	// Build GeoJSON coordinates: [[[lon,lat],...]]
	coords := make([][][2]float64, 1)
	coords[0] = ring

	fc := map[string]any{
		"type": "FeatureCollection",
		"features": []map[string]any{
			{
				"type": "Feature",
				"properties": map[string]any{
					"produccion_id": prod.ProduccionID,
				},
				"geometry": map[string]any{
					"type":        "Polygon",
					"coordinates": coords,
				},
			},
		},
	}

	data, err := json.Marshal(fc)
	if err != nil {
		return "", &domain.ProcessingError{
			Type:    domain.ErrDisk,
			Message: "marshalling polygon GeoJSON",
			Wrapped: err,
		}
	}

	path := fmt.Sprintf("%s/polygon_%d.geojson", dir, prod.ProduccionID)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", &domain.ProcessingError{
			Type:    domain.ErrDisk,
			Message: "writing polygon GeoJSON",
			Wrapped: err,
		}
	}

	return path, nil
}

// overlayScriptPath is the location of the polygon overlay Python script
// installed inside the container.
const overlayScriptPath = "/usr/local/bin/overlay_polygon.py"

// overlayColor is the RGBA color used for the polygon outline on all images
// (yellow — visible on both light and dark backgrounds).
const overlayR, overlayG, overlayB, overlayThickness = "255", "255", "0", "1"

// OverlayPolygon draws the production polygon outline on pngPath and writes
// the result to outputPath (may be the same file to overwrite in place).
// Color is yellow by default; thickness is 4 pixels at upscaled resolution.
// Silently skips when the overlay script is not present in the container.
func OverlayPolygon(ctx context.Context, executor GDALExecutor, pngPath, polygonPath, outputPath string) error {
	if _, err := os.Stat(overlayScriptPath); err != nil {
		return nil // script not installed, skip silently
	}

	args := []string{
		overlayScriptPath,
		pngPath,
		polygonPath,
		outputPath,
		overlayR, overlayG, overlayB,
		overlayThickness,
	}

	_, stderr, err := executor.Run(ctx, "python3", args)
	if err != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "overlay_polygon.py failed on " + filepath.Base(pngPath) + ": " + stderr,
			Wrapped: err,
		}
	}

	return nil
}

// MaskMultiband creates a copy of multibandPath with pixels outside the
// polygon set to nodata (0). The masked file is written to workDir and its
// path is returned. It is used to compute band/index statistics only over
// pixels that fall inside the production polygon.
func MaskMultiband(ctx context.Context, executor GDALExecutor, multibandPath, polygonPath, workDir string) (string, error) {
	maskedPath := filepath.Join(workDir, "multiband_masked.tif")

	args := []string{
		"-cutline", polygonPath,
		"-crop_to_cutline",
		"-dstnodata", "0",
		multibandPath,
		maskedPath,
	}

	_, stderr, err := executor.Run(ctx, "gdalwarp", args)
	if err != nil {
		return "", err
	}

	if _, statErr := os.Stat(maskedPath); statErr != nil {
		return "", &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdalwarp did not produce masked multiband: " + stderr,
			Wrapped: statErr,
		}
	}

	return maskedPath, nil
}
