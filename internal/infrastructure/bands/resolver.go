// Package bands provides a BandResolver that builds Sentinel-2 band URLs
// directly from the base_bands URL stored on each scene record.
package bands

import (
	"context"
	"fmt"
	"path"
	"strings"

	"agro-sentinel-worker/internal/domain"
)

// Resolver implements worker.BandResolver using the base_bands field on the
// scene to construct each band's COG URL.
//
// base_bands stores the full URL of a reference band (e.g. B02):
//
//	https://sentinel-cogs.s3.us-west-2.amazonaws.com/.../B02.tif
//	https://sentinel-2-l2a.s3.amazonaws.com/tiles/.../B02.jp2
//
// The resolver derives all other band URLs by replacing the reference band
// name (without extension) with each target band, preserving the extension
// and base path automatically.
type Resolver struct {
	scenes SceneReader
}

// SceneReader is the subset of the scene repository Resolver needs.
type SceneReader interface {
	GetByProduccionAndSceneName(ctx context.Context, produccionID int64, sceneName string) (*domain.Scene, error)
}

// New returns a Resolver backed by scenes.
func New(scenes SceneReader) *Resolver {
	return &Resolver{scenes: scenes}
}

// ResolveBands fetches the scene's base_bands reference URL and constructs a
// BandInfo for every spectral band (B02..B12) plus the SCL band. The SCL
// href is returned separately for cloud-cover analysis.
func (r *Resolver) ResolveBands(ctx context.Context, produccionID int64, sceneName string) ([]domain.BandInfo, string, error) {
	scene, err := r.scenes.GetByProduccionAndSceneName(ctx, produccionID, sceneName)
	if err != nil {
		return nil, "", &domain.ProcessingError{
			Type:    domain.ErrMySQL,
			Message: fmt.Sprintf("fetching scene %s for production %d", sceneName, produccionID),
			Wrapped: err,
		}
	}
	if scene == nil {
		return nil, "", &domain.ProcessingError{
			Type:    domain.ErrValidation,
			Message: fmt.Sprintf("scene %s not found for production %d", sceneName, produccionID),
		}
	}

	refURL := strings.TrimSpace(scene.BaseBands)
	if refURL == "" {
		return nil, "", &domain.ProcessingError{
			Type:    domain.ErrValidation,
			Message: fmt.Sprintf("scene %s has no base_bands URL — run sync first", sceneName),
		}
	}

	bandURLFor, err := makeBandURLFunc(refURL)
	if err != nil {
		return nil, "", &domain.ProcessingError{
			Type:    domain.ErrValidation,
			Message: fmt.Sprintf("scene %s: cannot parse base_bands URL %q", sceneName, refURL),
			Wrapped: err,
		}
	}

	spectral := domain.AllSpectralBands()
	infos := make([]domain.BandInfo, 0, len(spectral))
	for _, b := range spectral {
		infos = append(infos, domain.BandInfo{
			Name:       b,
			Resolution: b.Resolution(),
			Href:       bandURLFor(string(b)),
		})
	}

	sclHref := bandURLFor(string(domain.BandSCL))

	return infos, sclHref, nil
}

// makeBandURLFunc parses a reference band URL (e.g. "https://.../B02.tif")
// and returns a function that builds the URL for any other band by replacing
// the reference band name while preserving directory and extension.
//
// The reference band is identified as the last path segment without its
// extension (e.g. "B02"). If no known band name is found, an error is
// returned so the caller can surface it clearly.
func makeBandURLFunc(refURL string) (func(band string) string, error) {
	filename := path.Base(refURL)           // e.g. "B02.tif"
	ext := path.Ext(filename)               // e.g. ".tif"
	refBand := strings.TrimSuffix(filename, ext) // e.g. "B02"

	if refBand == "" || ext == "" {
		return nil, fmt.Errorf("expected a filename with extension, got %q", filename)
	}

	dir := strings.TrimSuffix(refURL, filename) // base path including trailing "/"

	return func(band string) string {
		return dir + band + ext
	}, nil
}
