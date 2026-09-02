package gdal

import (
	"context"
	"os"

	"agro-sentinel-worker/internal/domain"
)

// VRTOpts configures a `gdalbuildvrt` invocation.
type VRTOpts struct {
	Inputs   []string
	Output   string
	Separate bool
}

// BuildVRT runs `gdalbuildvrt` to compose opts.Inputs (e.g. individual band
// files) into a single VRT at opts.Output. When Separate is true, each
// input becomes its own band in the output (-separate).
func BuildVRT(ctx context.Context, executor *Executor, opts VRTOpts) error {
	args := []string{}

	if opts.Separate {
		args = append(args, "-separate")
	}

	args = append(args, opts.Output)
	args = append(args, opts.Inputs...)

	_, stderr, err := executor.Run(ctx, "gdalbuildvrt", args)
	if err != nil {
		return err
	}

	if _, statErr := os.Stat(opts.Output); statErr != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdalbuildvrt did not produce output file: " + opts.Output + " (" + stderr + ")",
			Wrapped: statErr,
		}
	}

	return nil
}
