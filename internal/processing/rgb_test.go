package processing

import (
	"context"
	"path/filepath"
	"testing"

	"agro-sentinel-worker/internal/domain"
)

func TestAllCompositions(t *testing.T) {
	comps := AllCompositions()
	if len(comps) != 4 {
		t.Fatalf("expected 4 compositions, got %d", len(comps))
	}

	want := map[domain.FileType][3]domain.Band{
		domain.FileNatural:    {domain.BandB04, domain.BandB03, domain.BandB02},
		domain.FileFalseColor: {domain.BandB08, domain.BandB04, domain.BandB03},
		domain.FileRedEdge:    {domain.BandB06, domain.BandB05, domain.BandB04},
		domain.FileSWIR:       {domain.BandB12, domain.BandB8A, domain.BandB04},
	}

	seen := map[domain.FileType]bool{}
	for _, c := range comps {
		seen[c.Type] = true
		wantBands, ok := want[c.Type]
		if !ok {
			t.Fatalf("unexpected composition type: %s", c.Type)
		}
		if c.RedBand != wantBands[0] || c.GreenBand != wantBands[1] || c.BlueBand != wantBands[2] {
			t.Errorf("%s: got RGB=%s,%s,%s want %s,%s,%s", c.Type, c.RedBand, c.GreenBand, c.BlueBand, wantBands[0], wantBands[1], wantBands[2])
		}
		if bandIndex[c.RedBand] == 0 || bandIndex[c.GreenBand] == 0 || bandIndex[c.BlueBand] == 0 {
			t.Errorf("%s: band missing from bandIndex map", c.Type)
		}
	}
	for ft := range want {
		if !seen[ft] {
			t.Errorf("missing composition type: %s", ft)
		}
	}
}

func TestGenerateRGB_BuildsCorrectCommand(t *testing.T) {
	dir := t.TempDir()
	multibandPath := filepath.Join(dir, "multiband.tif")
	outputPath := filepath.Join(dir, "natural.png")

	mock := &mockExecutor{}

	if err := GenerateRGB(context.Background(), mock, multibandPath, outputPath, 3, 2, 1); err != nil {
		t.Fatalf("GenerateRGB() error = %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}

	call := mock.calls[0]
	if call.command != "gdal_translate" {
		t.Errorf("command = %s, want gdal_translate", call.command)
	}

	wantArgs := []string{
		"-b", "3", "-b", "2", "-b", "1",
		"-of", "PNG",
		"-scale", "0", "3000", "0", "255",
		"-ot", "Byte",
		"-outsize", "400%", "400%",
		"-r", "lanczos",
		multibandPath,
		outputPath,
	}

	if len(call.args) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", call.args, wantArgs)
	}
	for i, a := range wantArgs {
		if call.args[i] != a {
			t.Errorf("args[%d] = %q, want %q", i, call.args[i], a)
		}
	}
}

func TestGenerateRGB_ExecutorError(t *testing.T) {
	dir := t.TempDir()
	mock := &mockExecutor{err: context.DeadlineExceeded}

	err := GenerateRGB(context.Background(), mock, filepath.Join(dir, "multiband.tif"), filepath.Join(dir, "natural.png"), 3, 2, 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	pe, ok := err.(*domain.ProcessingError)
	if !ok {
		t.Fatalf("expected *domain.ProcessingError, got %T", err)
	}
	if pe.Type != domain.ErrGDAL {
		t.Errorf("Type = %s, want %s", pe.Type, domain.ErrGDAL)
	}
}
