package processing

import (
	"context"
	"os"
	"strconv"

	"agro-sentinel-worker/internal/domain"
)

// CompositionDefinition describes a 3-band RGB composition built from
// multiband.tif band numbers.
type CompositionDefinition struct {
	Type      domain.FileType
	RedBand   domain.Band
	GreenBand domain.Band
	BlueBand  domain.Band
	Name      string
}

// bandIndex maps a domain.Band to its 1-based band number in multiband.tif.
// Band order: B02(1), B03(2), B04(3), B05(4), B06(5), B07(6), B08(7),
// B8A(8), B11(9), B12(10).
var bandIndex = map[domain.Band]int{
	domain.BandB02: 1,
	domain.BandB03: 2,
	domain.BandB04: 3,
	domain.BandB05: 4,
	domain.BandB06: 5,
	domain.BandB07: 6,
	domain.BandB08: 7,
	domain.BandB8A: 8,
	domain.BandB11: 9,
	domain.BandB12: 10,
	domain.BandSCL: 11,
}

// AllCompositions returns the standard RGB band compositions.
func AllCompositions() []CompositionDefinition {
	return []CompositionDefinition{
		{Type: domain.FileNatural, RedBand: domain.BandB04, GreenBand: domain.BandB03, BlueBand: domain.BandB02, Name: "Natural Color"},
		{Type: domain.FileFalseColor, RedBand: domain.BandB08, GreenBand: domain.BandB04, BlueBand: domain.BandB03, Name: "False Color"},
		{Type: domain.FileRedEdge, RedBand: domain.BandB06, GreenBand: domain.BandB05, BlueBand: domain.BandB04, Name: "Red Edge"},
		{Type: domain.FileSWIR, RedBand: domain.BandB12, GreenBand: domain.BandB8A, BlueBand: domain.BandB04, Name: "SWIR"},
	}
}

// pngUpscaleFactor controls how much the output PNGs are enlarged relative to
// the native multiband resolution (10m/px). A 4× scale makes the images
// visually smoother without altering the underlying spectral data.
const pngUpscaleFactor = 4

// sentinel2ScaleMin / sentinel2ScaleMax define the fixed input DN range for
// Sentinel-2 L2A surface reflectance (×10000). Using a fixed range instead of
// auto-scale keeps brightness consistent across productions and dates.
const sentinel2ScaleMin = "0"
const sentinel2ScaleMax = "3000"

// GenerateRGB creates an 8-bit RGB PNG from multibandPath using the given
// 1-based band numbers for red, green, and blue. The output is upscaled by
// pngUpscaleFactor using Lanczos resampling for visual quality.
// A fixed DN scale (0–3000) is applied so all images have consistent brightness.
func GenerateRGB(ctx context.Context, executor GDALExecutor, multibandPath string, outputPath string, redBand, greenBand, blueBand int) error {
	scale := strconv.Itoa(pngUpscaleFactor * 100) + "%"
	args := []string{
		"-b", strconv.Itoa(redBand),
		"-b", strconv.Itoa(greenBand),
		"-b", strconv.Itoa(blueBand),
		"-of", "PNG",
		"-scale", sentinel2ScaleMin, sentinel2ScaleMax, "0", "255",
		"-ot", "Byte",
		"-outsize", scale, scale,
		"-r", "lanczos",
		multibandPath,
		outputPath,
	}

	_, stderr, err := executor.Run(ctx, "gdal_translate", args)
	if err != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdal_translate failed generating RGB composition: " + outputPath,
			Wrapped: err,
		}
	}

	if _, statErr := os.Stat(outputPath); statErr != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdal_translate did not produce output file: " + outputPath + " (" + stderr + ")",
			Wrapped: statErr,
		}
	}

	return nil
}
