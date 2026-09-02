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

// GenerateRGB creates an 8-bit RGB PNG from multibandPath using the given
// 1-based band numbers for red, green, and blue.
func GenerateRGB(ctx context.Context, executor GDALExecutor, multibandPath string, outputPath string, redBand, greenBand, blueBand int) error {
	args := []string{
		"-b", strconv.Itoa(redBand),
		"-b", strconv.Itoa(greenBand),
		"-b", strconv.Itoa(blueBand),
		"-of", "PNG",
		"-scale",
		"-ot", "Byte",
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
