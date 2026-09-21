package processing

import (
	"context"
	"fmt"
	"os"

	"agro-sentinel-worker/internal/domain"
)

// VisualizationSRS is the projection the PNGs meant for map display are
// reprojected into, before being turned into images.
//
// Processing and statistics stay in TargetSRS (UTM), which is what measuring
// areas and distances requires. But Leaflet places a PNG with L.imageOverlay
// by stretching it over a lat/lon rectangle — it never reprojects or rotates.
// A UTM raster shown that way is off by the meridian convergence: around 0.68°
// of rotation at 100.9°W, which is ~12 m at the corners of a 2 km tile, more
// than one Sentinel-2 pixel. Emitting the images already in EPSG:4326, clipped
// to the exact tile_bbox the map uses as bounds, makes the overlay correct by
// construction.
const VisualizationSRS = "EPSG:4326"

// WarpToWGS84 reprojects srcPath into EPSG:4326 clipped exactly to bbox.
//
// Nearest-neighbour resampling is deliberate: it reassigns pixels without
// blending them, so the DN values stay exact (index images come out the same as
// if they had been computed in UTM) and the categorical SCL band is not
// corrupted. Visual smoothing comes later, from the Lanczos upscale in
// GenerateRGB.
func WarpToWGS84(ctx context.Context, executor GDALExecutor, srcPath, dstPath string, bbox domain.BBox) error {
	args := []string{
		"-t_srs", VisualizationSRS,
		"-te", fmt.Sprintf("%v", bbox.MinX), fmt.Sprintf("%v", bbox.MinY),
		fmt.Sprintf("%v", bbox.MaxX), fmt.Sprintf("%v", bbox.MaxY),
		"-te_srs", VisualizationSRS,
		"-r", "near",
		"-overwrite",
		srcPath,
		dstPath,
	}

	_, stderr, err := executor.Run(ctx, "gdalwarp", args)
	if err != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdalwarp failed reprojecting to " + VisualizationSRS + ": " + stderr,
			Wrapped: err,
		}
	}

	if _, statErr := os.Stat(dstPath); statErr != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdalwarp did not produce " + dstPath + " (" + stderr + ")",
			Wrapped: statErr,
		}
	}

	return nil
}
