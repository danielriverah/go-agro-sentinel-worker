package gdal

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"agro-sentinel-worker/internal/domain"
)

// WarpOpts configures a `gdalwarp` invocation.
type WarpOpts struct {
	Input            string
	Output           string
	TargetResolution int
	ResamplingMethod string
	TargetSRS        string
	BBox             *domain.BBox
}

// Warp runs `gdalwarp` on opts.Input, applying target resolution,
// resampling method, target SRS, and an optional BBOX crop. It verifies
// the command exited successfully and that the output file was created.
func Warp(ctx context.Context, executor *Executor, opts WarpOpts) error {
	args := []string{}

	if opts.TargetResolution > 0 {
		res := strconv.Itoa(opts.TargetResolution)
		args = append(args, "-tr", res, res)
	}

	if opts.ResamplingMethod != "" {
		args = append(args, "-r", opts.ResamplingMethod)
	}

	if opts.TargetSRS != "" {
		args = append(args, "-t_srs", opts.TargetSRS)
	}

	if opts.BBox != nil {
		args = append(args,
			"-te",
			fmt.Sprintf("%v", opts.BBox.MinX),
			fmt.Sprintf("%v", opts.BBox.MinY),
			fmt.Sprintf("%v", opts.BBox.MaxX),
			fmt.Sprintf("%v", opts.BBox.MaxY),
		)
	}

	args = append(args, opts.Input, opts.Output)

	_, stderr, err := executor.Run(ctx, "gdalwarp", args)
	if err != nil {
		return err
	}

	if _, statErr := os.Stat(opts.Output); statErr != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdalwarp did not produce output file: " + opts.Output + " (" + stderr + ")",
			Wrapped: statErr,
		}
	}

	return nil
}
