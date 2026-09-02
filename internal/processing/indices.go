package processing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"agro-sentinel-worker/internal/domain"
)

// IndexDefinition describes a vegetation/moisture index computed from
// multiband.tif band numbers.
type IndexDefinition struct {
	Type    domain.FileType
	Formula string
	Bands   []domain.Band
	Name    string
}

// moistureIndices identifies the indices that use the blue-white-brown
// moisture color ramp instead of the green-yellow-red vegetation ramp.
var moistureIndices = map[domain.FileType]bool{
	domain.FileNBR:  true,
	domain.FileNDMI: true,
}

// AllIndices returns the 7 vegetation/moisture indices defined by the spec.
// Formulas reference gdal_calc.py letter placeholders A, B, C in the order
// of the Bands slice.
func AllIndices() []IndexDefinition {
	return []IndexDefinition{
		{
			Type:    domain.FileNDVI,
			Formula: "(A-B)/(A+B)",
			Bands:   []domain.Band{domain.BandB08, domain.BandB04},
			Name:    "NDVI",
		},
		{
			Type:    domain.FileNDRE,
			Formula: "(A-B)/(A+B)",
			Bands:   []domain.Band{domain.BandB08, domain.BandB05},
			Name:    "NDRE",
		},
		{
			Type:    domain.FileEVI,
			Formula: "2.5*(A-B)/(A+6*B-7.5*C+1)",
			Bands:   []domain.Band{domain.BandB08, domain.BandB04, domain.BandB02},
			Name:    "EVI",
		},
		{
			Type:    domain.FileGNDVI,
			Formula: "(A-B)/(A+B)",
			Bands:   []domain.Band{domain.BandB08, domain.BandB03},
			Name:    "GNDVI",
		},
		{
			Type:    domain.FileNBR,
			Formula: "(A-B)/(A+B)",
			Bands:   []domain.Band{domain.BandB08, domain.BandB12},
			Name:    "NBR",
		},
		{
			Type:    domain.FileNDMI,
			Formula: "(A-B)/(A+B)",
			Bands:   []domain.Band{domain.BandB08, domain.BandB11},
			Name:    "NDMI",
		},
		{
			Type:    domain.FileSAVI,
			Formula: "1.5*(A-B)/(A+B+0.5)",
			Bands:   []domain.Band{domain.BandB08, domain.BandB04},
			Name:    "SAVI",
		},
	}
}

// calcLetters are the gdal_calc.py input letters, in order, used to name
// each band argument (-A, -B, -C, ...).
var calcLetters = []string{"A", "B", "C", "D"}

// vegetationColorRamp is a gdaldem color-relief ramp (green-yellow-red)
// for vegetation indices, keyed on values in the [-1, 1] index range.
const vegetationColorRamp = `-1.0 165 0 38
-0.2 215 48 39
0.0 254 224 139
0.3 166 217 106
0.6 26 152 80
1.0 0 68 27
`

// moistureColorRamp is a gdaldem color-relief ramp (blue-white-brown) for
// moisture-related indices, keyed on values in the [-1, 1] index range.
const moistureColorRamp = `-1.0 140 81 10
-0.2 223 194 125
0.0 245 245 245
0.2 128 205 193
1.0 1 102 94
`

// GenerateIndex computes the vegetation/moisture index identified by
// indexType from multibandPath and writes a color-mapped PNG to
// outputPath. It uses gdal_calc.py to compute the raw index values into a
// temporary GeoTIFF, then gdaldem color-relief to render the PNG.
func GenerateIndex(ctx context.Context, executor GDALExecutor, multibandPath string, outputPath string, indexType domain.FileType) error {
	def, ok := findIndexDefinition(indexType)
	if !ok {
		return &domain.ProcessingError{
			Type:    domain.ErrValidation,
			Message: "unknown index type: " + string(indexType),
		}
	}

	outDir := filepath.Dir(outputPath)
	rawPath := filepath.Join(outDir, string(indexType)+"_raw.tif")

	if err := runIndexCalc(ctx, executor, def, multibandPath, rawPath); err != nil {
		return err
	}

	colorFilePath := filepath.Join(outDir, string(indexType)+".clr")
	ramp := vegetationColorRamp
	if moistureIndices[indexType] {
		ramp = moistureColorRamp
	}
	if err := os.WriteFile(colorFilePath, []byte(ramp), 0o644); err != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrDisk,
			Message: "writing color ramp file: " + colorFilePath,
			Wrapped: err,
		}
	}

	return runColorRelief(ctx, executor, rawPath, colorFilePath, outputPath)
}

func findIndexDefinition(indexType domain.FileType) (IndexDefinition, bool) {
	for _, def := range AllIndices() {
		if def.Type == indexType {
			return def, true
		}
	}
	return IndexDefinition{}, false
}

func runIndexCalc(ctx context.Context, executor GDALExecutor, def IndexDefinition, multibandPath, rawPath string) error {
	args := []string{}
	for i, band := range def.Bands {
		letter := calcLetters[i]
		args = append(args,
			"-"+letter, multibandPath,
			fmt.Sprintf("--%s_band=%d", letter, bandIndex[band]),
		)
	}
	args = append(args,
		"--calc="+def.Formula,
		"--outfile="+rawPath,
		"--type=Float32",
	)

	_, stderr, err := executor.Run(ctx, "gdal_calc.py", args)
	if err != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdal_calc.py failed computing " + def.Name,
			Wrapped: err,
		}
	}

	if _, statErr := os.Stat(rawPath); statErr != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdal_calc.py did not produce output file: " + rawPath + " (" + stderr + ")",
			Wrapped: statErr,
		}
	}

	return nil
}

func runColorRelief(ctx context.Context, executor GDALExecutor, rawPath, colorFilePath, outputPath string) error {
	args := []string{
		"color-relief",
		rawPath,
		colorFilePath,
		outputPath,
		"-of", "PNG",
		"-alpha",
	}

	_, stderr, err := executor.Run(ctx, "gdaldem", args)
	if err != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdaldem color-relief failed: " + outputPath,
			Wrapped: err,
		}
	}

	if _, statErr := os.Stat(outputPath); statErr != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdaldem did not produce output file: " + outputPath + " (" + stderr + ")",
			Wrapped: statErr,
		}
	}

	return nil
}
