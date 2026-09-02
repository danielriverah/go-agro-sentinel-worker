// Package processing implements the core GDAL-driven raster processing
// pipeline: cropping/resampling individual Sentinel-2 COG bands and
// composing them into a single multiband GeoTIFF.
package processing

import "agro-sentinel-worker/internal/domain"

// TargetSRS is the projection all bands are warped into before being
// composed into the multiband output.
const TargetSRS = "EPSG:32614"

// resamplingMethodFor returns the gdalwarp resampling method to use for the
// given band. 20m bands are always resampled with bilinear so they don't
// introduce blocky artifacts when upsampled to the target resolution;
// native 10m bands use the configured default (bilinear when unset).
func resamplingMethodFor(band domain.Band, defaultMethod string) string {
	if band.Resolution() == 20 {
		return "bilinear"
	}
	if defaultMethod == "" {
		return "bilinear"
	}
	return defaultMethod
}
