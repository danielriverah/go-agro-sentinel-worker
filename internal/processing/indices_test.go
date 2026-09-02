package processing

import (
	"context"
	"path/filepath"
	"testing"

	"agro-sentinel-worker/internal/domain"
)

func TestAllIndices(t *testing.T) {
	indices := AllIndices()
	if len(indices) != 7 {
		t.Fatalf("expected 7 indices, got %d", len(indices))
	}

	wantBands := map[domain.FileType][]domain.Band{
		domain.FileNDVI:  {domain.BandB08, domain.BandB04},
		domain.FileNDRE:  {domain.BandB08, domain.BandB05},
		domain.FileEVI:   {domain.BandB08, domain.BandB04, domain.BandB02},
		domain.FileGNDVI: {domain.BandB08, domain.BandB03},
		domain.FileNBR:   {domain.BandB08, domain.BandB12},
		domain.FileNDMI:  {domain.BandB08, domain.BandB11},
		domain.FileSAVI:  {domain.BandB08, domain.BandB04},
	}

	seen := map[domain.FileType]bool{}
	for _, def := range indices {
		seen[def.Type] = true
		want, ok := wantBands[def.Type]
		if !ok {
			t.Fatalf("unexpected index type: %s", def.Type)
		}
		if len(def.Bands) != len(want) {
			t.Fatalf("%s: bands = %v, want %v", def.Type, def.Bands, want)
		}
		for i, b := range want {
			if def.Bands[i] != b {
				t.Errorf("%s: bands[%d] = %s, want %s", def.Type, i, def.Bands[i], b)
			}
		}
		if def.Formula == "" {
			t.Errorf("%s: empty formula", def.Type)
		}
	}
	for ft := range wantBands {
		if !seen[ft] {
			t.Errorf("missing index type: %s", ft)
		}
	}
}

func TestGenerateIndex_NDVI_BuildsCorrectCommands(t *testing.T) {
	dir := t.TempDir()
	multibandPath := filepath.Join(dir, "multiband.tif")
	outputPath := filepath.Join(dir, "ndvi.png")

	mock := &mockExecutor{}

	if err := GenerateIndex(context.Background(), mock, multibandPath, outputPath, domain.FileNDVI); err != nil {
		t.Fatalf("GenerateIndex() error = %v", err)
	}

	if len(mock.calls) != 2 {
		t.Fatalf("expected 2 calls (gdal_calc.py, gdaldem), got %d: %+v", len(mock.calls), mock.calls)
	}

	calc := mock.calls[0]
	if calc.command != "gdal_calc.py" {
		t.Errorf("calls[0].command = %s, want gdal_calc.py", calc.command)
	}
	wantCalcArgs := []string{
		"-A", multibandPath, "--A_band=7",
		"-B", multibandPath, "--B_band=3",
		"--calc=(A-B)/(A+B)",
		"--outfile=" + filepath.Join(dir, "ndvi_raw.tif"),
		"--type=Float32",
	}
	if len(calc.args) != len(wantCalcArgs) {
		t.Fatalf("calc args = %v, want %v", calc.args, wantCalcArgs)
	}
	for i, a := range wantCalcArgs {
		if calc.args[i] != a {
			t.Errorf("calc args[%d] = %q, want %q", i, calc.args[i], a)
		}
	}

	relief := mock.calls[1]
	if relief.command != "gdaldem" {
		t.Errorf("calls[1].command = %s, want gdaldem", relief.command)
	}
	if relief.args[0] != "color-relief" {
		t.Errorf("relief args[0] = %s, want color-relief", relief.args[0])
	}
	if relief.args[3] != outputPath {
		// args: color-relief raw color outputPath -of PNG -alpha
		t.Errorf("relief args missing outputPath at expected position: %v", relief.args)
	}
}

func TestGenerateIndex_EVI_UsesThreeBands(t *testing.T) {
	dir := t.TempDir()
	multibandPath := filepath.Join(dir, "multiband.tif")
	outputPath := filepath.Join(dir, "evi.png")

	mock := &mockExecutor{}

	if err := GenerateIndex(context.Background(), mock, multibandPath, outputPath, domain.FileEVI); err != nil {
		t.Fatalf("GenerateIndex() error = %v", err)
	}

	calc := mock.calls[0]
	wantCalcArgs := []string{
		"-A", multibandPath, "--A_band=7",
		"-B", multibandPath, "--B_band=3",
		"-C", multibandPath, "--C_band=1",
		"--calc=2.5*(A-B)/(A+6*B-7.5*C+1)",
		"--outfile=" + filepath.Join(dir, "evi_raw.tif"),
		"--type=Float32",
	}
	if len(calc.args) != len(wantCalcArgs) {
		t.Fatalf("calc args = %v, want %v", calc.args, wantCalcArgs)
	}
	for i, a := range wantCalcArgs {
		if calc.args[i] != a {
			t.Errorf("calc args[%d] = %q, want %q", i, calc.args[i], a)
		}
	}
}

func TestGenerateIndex_UnknownType(t *testing.T) {
	dir := t.TempDir()
	mock := &mockExecutor{}

	err := GenerateIndex(context.Background(), mock, filepath.Join(dir, "multiband.tif"), filepath.Join(dir, "out.png"), domain.FileType("bogus"))
	if err == nil {
		t.Fatal("expected error for unknown index type")
	}
	pe, ok := err.(*domain.ProcessingError)
	if !ok {
		t.Fatalf("expected *domain.ProcessingError, got %T", err)
	}
	if pe.Type != domain.ErrValidation {
		t.Errorf("Type = %s, want %s", pe.Type, domain.ErrValidation)
	}
	if len(mock.calls) != 0 {
		t.Errorf("expected no executor calls, got %d", len(mock.calls))
	}
}

func TestGenerateIndex_CalcExecutorError(t *testing.T) {
	dir := t.TempDir()
	mock := &mockExecutor{err: context.DeadlineExceeded}

	err := GenerateIndex(context.Background(), mock, filepath.Join(dir, "multiband.tif"), filepath.Join(dir, "ndvi.png"), domain.FileNDVI)
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
