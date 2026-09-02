package gdal

import (
	"context"
	"encoding/json"
	"os"

	"agro-sentinel-worker/internal/domain"
)

// GDALInfo holds the subset of `gdalinfo -json` output this worker cares about.
type GDALInfo struct {
	Width      int
	Height     int
	Bands      int
	Projection string
	BoundsMinX float64
	BoundsMinY float64
	BoundsMaxX float64
	BoundsMaxY float64
}

// gdalInfoJSON mirrors the relevant fields of `gdalinfo -json` output.
type gdalInfoJSON struct {
	Size             [2]int     `json:"size"`
	Bands            []struct{} `json:"bands"`
	CoordinateSystem struct {
		Wkt string `json:"wkt"`
	} `json:"coordinateSystem"`
	CornerCoordinates struct {
		UpperLeft  [2]float64 `json:"upperLeft"`
		LowerLeft  [2]float64 `json:"lowerLeft"`
		UpperRight [2]float64 `json:"upperRight"`
		LowerRight [2]float64 `json:"lowerRight"`
	} `json:"cornerCoordinates"`
}

// Info runs `gdalinfo -json <inputPath>` and parses the result.
func Info(ctx context.Context, executor *Executor, inputPath string) (*GDALInfo, error) {
	if _, err := os.Stat(inputPath); err != nil {
		return nil, &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdalinfo input file not found: " + inputPath,
			Wrapped: err,
		}
	}

	stdout, stderr, err := executor.Run(ctx, "gdalinfo", []string{"-json", inputPath})
	if err != nil {
		return nil, err
	}

	var parsed gdalInfoJSON
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		return nil, &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "failed to parse gdalinfo output: " + stderr,
			Wrapped: err,
		}
	}

	minX := parsed.CornerCoordinates.LowerLeft[0]
	minY := parsed.CornerCoordinates.LowerLeft[1]
	maxX := parsed.CornerCoordinates.UpperRight[0]
	maxY := parsed.CornerCoordinates.UpperRight[1]

	return &GDALInfo{
		Width:      parsed.Size[0],
		Height:     parsed.Size[1],
		Bands:      len(parsed.Bands),
		Projection: parsed.CoordinateSystem.Wkt,
		BoundsMinX: minX,
		BoundsMinY: minY,
		BoundsMaxX: maxX,
		BoundsMaxY: maxY,
	}, nil
}
