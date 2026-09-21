package processing

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/domain"
)

// GDALExecutor is the subset of gdal.Executor's behavior that the
// processing package depends on. It is satisfied by *gdal.Executor and by
// test doubles that record invocations without running real GDAL.
type GDALExecutor interface {
	Run(ctx context.Context, command string, args []string) (stdout string, stderr string, err error)
}

// S3Uploader is the subset of aws.S3Client's behavior the processing
// package depends on when persisting build artifacts.
type S3Uploader interface {
	Upload(ctx context.Context, bucket, key, filePath string) error
	HeadObject(ctx context.Context, bucket, key string) (bool, int64, error)
}

// MultibandBuilder builds a single multiband.tif from a set of Sentinel-2
// COG bands by warping each band into a common grid/CRS, stacking them into
// a VRT, and translating the VRT to GeoTIFF.
type MultibandBuilder struct {
	executor GDALExecutor
	s3       S3Uploader
	cfg      config.ProcessingConfig
	logger   *slog.Logger
}

// New creates a MultibandBuilder.
func New(executor GDALExecutor, s3 S3Uploader, cfg config.ProcessingConfig, logger *slog.Logger) *MultibandBuilder {
	if logger == nil {
		logger = slog.Default()
	}
	return &MultibandBuilder{executor: executor, s3: s3, cfg: cfg, logger: logger}
}

// Build warps each band in bands to bbox/targetResolution in jobDir's work
// subdirectory, stacks them into a VRT, and translates the VRT into
// multiband.tif in jobDir's output subdirectory. It returns the path to the
// generated multiband.tif.
func (b *MultibandBuilder) Build(ctx context.Context, jobDir string, bbox domain.BBox, bands []domain.BandInfo, targetResolution int) (string, error) {
	if len(bands) == 0 {
		return "", &domain.ProcessingError{
			Type:    domain.ErrValidation,
			Message: "no bands provided to build multiband.tif",
		}
	}

	if err := bbox.Validate(); err != nil {
		return "", &domain.ProcessingError{
			Type:    domain.ErrValidation,
			Message: "invalid bbox",
			Wrapped: err,
		}
	}

	workDir := filepath.Join(jobDir, "work")
	outputDir := filepath.Join(jobDir, "output")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", &domain.ProcessingError{Type: domain.ErrDisk, Message: "creating work dir", Wrapped: err}
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", &domain.ProcessingError{Type: domain.ErrDisk, Message: "creating output dir", Wrapped: err}
	}

	warpedPaths := make([]string, 0, len(bands))
	for _, band := range bands {
		warpedPath := filepath.Join(workDir, fmt.Sprintf("%s.tif", band.Name))
		if err := b.warpBand(ctx, band, bbox, targetResolution, warpedPath); err != nil {
			return "", err
		}
		warpedPaths = append(warpedPaths, warpedPath)
	}

	vrtPath := filepath.Join(workDir, "composite.vrt")
	if err := b.buildVRT(ctx, warpedPaths, vrtPath); err != nil {
		return "", err
	}

	outputPath := filepath.Join(outputDir, "multiband.tif")
	if err := b.translate(ctx, vrtPath, outputPath); err != nil {
		return "", err
	}

	return outputPath, nil
}

func (b *MultibandBuilder) warpBand(ctx context.Context, band domain.BandInfo, bbox domain.BBox, targetResolution int, outputPath string) error {
	resamplingMethod := resamplingMethodFor(band.Name, b.cfg.ResamplingMethod)
	res := fmt.Sprintf("%d", targetResolution)

	args := []string{
		"-t_srs", TargetSRS,
		"-te", fmt.Sprintf("%v", bbox.MinX), fmt.Sprintf("%v", bbox.MinY), fmt.Sprintf("%v", bbox.MaxX), fmt.Sprintf("%v", bbox.MaxY),
		"-te_srs", "EPSG:4326", // tile_bbox is stored in WGS84 (lon/lat degrees)
		"-tr", res, res,
		"-r", resamplingMethod,
		band.Href,
		outputPath,
	}

	b.logger.Debug("warping band", "band", band.Name, "resampling", resamplingMethod, "output", outputPath)

	_, stderr, err := b.executor.Run(ctx, "gdalwarp", args)
	if err != nil {
		return err
	}

	if _, statErr := os.Stat(outputPath); statErr != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdalwarp did not produce output file: " + outputPath + " (" + stderr + ")",
			Wrapped: statErr,
		}
	}

	return nil
}

func (b *MultibandBuilder) buildVRT(ctx context.Context, inputs []string, vrtPath string) error {
	args := []string{"-separate", vrtPath}
	args = append(args, inputs...)

	b.logger.Debug("building VRT", "inputs", len(inputs), "output", vrtPath)

	_, stderr, err := b.executor.Run(ctx, "gdalbuildvrt", args)
	if err != nil {
		return err
	}

	if _, statErr := os.Stat(vrtPath); statErr != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdalbuildvrt did not produce output file: " + vrtPath + " (" + stderr + ")",
			Wrapped: statErr,
		}
	}

	return nil
}

func (b *MultibandBuilder) translate(ctx context.Context, vrtPath, outputPath string) error {
	args := []string{"-of", "GTiff", vrtPath, outputPath}

	b.logger.Debug("translating VRT to GeoTIFF", "output", outputPath)

	_, stderr, err := b.executor.Run(ctx, "gdal_translate", args)
	if err != nil {
		return err
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
