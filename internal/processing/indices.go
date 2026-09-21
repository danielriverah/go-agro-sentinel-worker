package processing

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

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

// safeRatio builds a division-safe normalized-difference formula for gdal_calc.py.
// When the denominator is ~0 (both bands are zero/nodata) the pixel is set to
// the nodata value (9999) instead of ±Inf or NaN, so gdalinfo statistics and
// the ia_req.json payload are not polluted by spurious extreme values.
// The result is also clamped to [-1, 1] to catch any remaining float rounding.
// safeRatio builds a division-safe normalized-difference formula for gdal_calc.py.
// Invalid pixels (denominator ~0) are set to nodata (9999) AFTER clipping valid
// ones to [-1,1], so the clip never converts 9999 into a boundary value.
func safeRatio(num, den string) string {
	return "numpy.where(numpy.abs(" + den + ")<1e-6,9999,numpy.clip((" + num + ")/(" + den + "),-1,1))"
}

// AllIndices returns the 7 vegetation/moisture indices defined by the spec.
// Formulas reference gdal_calc.py letter placeholders A, B, C in the order
// of the Bands slice. All formulas are division-safe and clamped to [-1, 1].
func AllIndices() []IndexDefinition {
	return []IndexDefinition{
		{
			Type:    domain.FileNDVI,
			Formula: safeRatio("A-B", "A+B"),
			Bands:   []domain.Band{domain.BandB08, domain.BandB04},
			Name:    "NDVI",
		},
		{
			Type:    domain.FileNDRE,
			Formula: safeRatio("A-B", "A+B"),
			Bands:   []domain.Band{domain.BandB08, domain.BandB05},
			Name:    "NDRE",
		},
		{
			Type:    domain.FileEVI,
			// L=10000 because Sentinel-2 L2A DN values are scaled by 10000
			// (reflectance 0.0–1.0 → DN 0–10000). Standard EVI uses L=1 for
			// reflectance inputs; using L=1 with DN inflates EVI ~5× (e.g. 0.77
			// instead of 0.14 for sparse maize), which confuses the IA model.
			Formula: "numpy.where(numpy.abs(A+6*B-7.5*C+10000)<1e-6,9999,numpy.clip(2.5*(A-B)/(A+6*B-7.5*C+10000),-1,1))",
			Bands:   []domain.Band{domain.BandB08, domain.BandB04, domain.BandB02},
			Name:    "EVI",
		},
		{
			Type:    domain.FileGNDVI,
			Formula: safeRatio("A-B", "A+B"),
			Bands:   []domain.Band{domain.BandB08, domain.BandB03},
			Name:    "GNDVI",
		},
		{
			Type:    domain.FileNBR,
			Formula: safeRatio("A-B", "A+B"),
			Bands:   []domain.Band{domain.BandB08, domain.BandB12},
			Name:    "NBR",
		},
		{
			Type:    domain.FileNDMI,
			Formula: safeRatio("A-B", "A+B"),
			Bands:   []domain.Band{domain.BandB08, domain.BandB11},
			Name:    "NDMI",
		},
		{
			Type:    domain.FileSAVI,
			// L=5000 = 0.5 reflectance in Sentinel-2 DN scale (×10000).
			Formula: "numpy.clip(1.5*(A-B)/(A+B+5000),-1,1)",
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
	outDir := filepath.Dir(rawPath)

	// One-time diagnostic: for NBR/NDMI check source bands 9+10 directly.
	if def.Name == "NBR" || def.Name == "NDMI" {
		srcCheckScript := `
import sys
from osgeo import gdal
ds = gdal.Open(sys.argv[1])
print(f"src_bands={ds.RasterCount}")
for i in [7, 9, 10]:
    if i <= ds.RasterCount:
        b = ds.GetRasterBand(i)
        nd = b.GetNoDataValue()
        st = b.GetStatistics(0, 1)
        print(f"  srcband{i}: min={st[0]:.1f} max={st[1]:.1f} mean={st[2]:.1f} nodata={nd}")
`
		if out, _, err := executor.Run(ctx, "python3", []string{"-c", srcCheckScript, multibandPath}); err == nil {
			slog.Info("source TIF band check", "index", def.Name, "path", multibandPath, "result", out)
		} else {
			slog.Warn("source TIF band check failed", "index", def.Name, "err", err)
		}
	}

	// Extract each required band to a temp single-band GeoTIFF so gdal_calc
	// receives distinct files with self-contained pixel data. This avoids any
	// VRT relative-path resolution issues and dataset-caching behaviour that
	// can occur when the same source file is opened multiple times via VRTs.
	args := []string{}
	var tmpPaths []string
	for i, band := range def.Bands {
		letter := calcLetters[i]
		bandNum := bandIndex[band]
		tmpPath := filepath.Join(outDir, fmt.Sprintf("%s_band%s_tmp.tif", string(def.Type), letter))
		tmpPaths = append(tmpPaths, tmpPath)

		_, extractStderr, extractErr := executor.Run(ctx, "gdal_translate",
			[]string{"-of", "GTiff", "-b", strconv.Itoa(bandNum), multibandPath, tmpPath},
		)
		if extractErr != nil {
			for _, p := range tmpPaths {
				os.Remove(p)
			}
			return &domain.ProcessingError{
				Type:    domain.ErrGDAL,
				Message: fmt.Sprintf("extracting band %d (%s) to temp TIF for %s", bandNum, letter, def.Name),
				Wrapped: extractErr,
			}
		}
		if extractStderr != "" {
			slog.Warn("gdal_translate band extract stderr", "index", def.Name, "band", band, "num", bandNum, "stderr", extractStderr)
		}
		// Diagnostic: log band min/max to confirm extraction is correct.
		pyScript := `
import sys
from osgeo import gdal
ds=gdal.Open(sys.argv[1])
b=ds.GetRasterBand(1)
nd=b.GetNoDataValue()
st=b.GetStatistics(0,1)
print(f"min={st[0]:.1f} max={st[1]:.1f} mean={st[2]:.1f} nodata={nd}")
`
		if statOut, _, statErr := executor.Run(ctx, "python3", []string{"-c", pyScript, tmpPath}); statErr == nil {
			slog.Info("band extract check", "index", def.Name, "band", band, "num", bandNum, "stats", statOut)
		} else {
			slog.Warn("band extract check failed", "index", def.Name, "band", band, "num", bandNum)
		}
		args = append(args, "-"+letter, tmpPath)
	}
	defer func() {
		for _, p := range tmpPaths {
			os.Remove(p)
		}
	}()

	args = append(args,
		"--calc="+def.Formula,
		"--outfile="+rawPath,
		"--type=Float32",
		"--NoDataValue=9999",
		"--overwrite",
	)

	// Use calc_index.py instead of gdal_calc.py to bypass gdal_calc's
	// dataset-caching behaviour that causes B11/B12 to read as zeros.
	calcArgs := append([]string{"/usr/local/bin/calc_index.py"}, args...)
	_, stderr, err := executor.Run(ctx, "python3", calcArgs)
	if stderr != "" {
		slog.Info("calc_index.py stderr", "index", def.Name, "stderr", stderr)
	}
	if err != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "calc_index.py failed computing " + def.Name,
			Wrapped: err,
		}
	}

	if _, statErr := os.Stat(rawPath); statErr != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "calc_index.py did not produce output file: " + rawPath + " (" + stderr + ")",
			Wrapped: statErr,
		}
	}

	return nil
}

func runColorRelief(ctx context.Context, executor GDALExecutor, rawPath, colorFilePath, outputPath string) error {
	// gdaldem color-relief does not support -outsize directly, so we generate
	// at native resolution first, then upscale with gdal_translate + lanczos.
	nativeOutput := outputPath + ".native.tif"

	reliefArgs := []string{
		"color-relief",
		rawPath,
		colorFilePath,
		nativeOutput,
		"-of", "GTiff",
		"-alpha",
	}

	_, stderr, err := executor.Run(ctx, "gdaldem", reliefArgs)
	if err != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdaldem color-relief failed: " + outputPath,
			Wrapped: err,
		}
	}

	if _, statErr := os.Stat(nativeOutput); statErr != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdaldem did not produce output file: " + nativeOutput + " (" + stderr + ")",
			Wrapped: statErr,
		}
	}
	defer os.Remove(nativeOutput)

	scale := strconv.Itoa(pngUpscaleFactor*100) + "%"
	scaleArgs := []string{
		"-of", "PNG",
		"-outsize", scale, scale,
		"-r", "lanczos",
		nativeOutput,
		outputPath,
	}

	_, stderr, err = executor.Run(ctx, "gdal_translate", scaleArgs)
	if err != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdal_translate upscale failed: " + outputPath,
			Wrapped: err,
		}
	}

	if _, statErr := os.Stat(outputPath); statErr != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdal_translate did not produce upscaled PNG: " + outputPath + " (" + stderr + ")",
			Wrapped: statErr,
		}
	}

	return nil
}
