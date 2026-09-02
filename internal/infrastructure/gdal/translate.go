package gdal

import (
	"context"
	"fmt"
	"os"

	"agro-sentinel-worker/internal/domain"
)

// TranslateOpts configures a `gdal_translate` invocation.
type TranslateOpts struct {
	Input        string
	Output       string
	BBox         *domain.BBox
	OutputFormat string
}

// Translate runs `gdal_translate` on opts.Input, optionally cropping to
// opts.BBox via -projwin, and writes opts.Output. It verifies the command
// exited successfully and that the output file was actually created.
func Translate(ctx context.Context, executor *Executor, opts TranslateOpts) error {
	args := []string{}

	format := opts.OutputFormat
	if format == "" {
		format = "GTiff"
	}
	args = append(args, "-of", format)

	if opts.BBox != nil {
		// -projwin ulx uly lrx lry (upper-left / lower-right corners)
		args = append(args,
			"-projwin",
			fmt.Sprintf("%v", opts.BBox.MinX),
			fmt.Sprintf("%v", opts.BBox.MaxY),
			fmt.Sprintf("%v", opts.BBox.MaxX),
			fmt.Sprintf("%v", opts.BBox.MinY),
		)
	}

	args = append(args, opts.Input, opts.Output)

	_, stderr, err := executor.Run(ctx, "gdal_translate", args)
	if err != nil {
		return err
	}

	if _, statErr := os.Stat(opts.Output); statErr != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrGDAL,
			Message: "gdal_translate did not produce output file: " + opts.Output + " (" + stderr + ")",
			Wrapped: statErr,
		}
	}

	return nil
}
