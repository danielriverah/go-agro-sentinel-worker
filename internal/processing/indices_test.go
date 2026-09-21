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

	// Verify python3 calc_index.py call with correct NDVI formula and 2 band inputs.
	var calcCall *recordedCall
	var reliefCall *recordedCall
	var upscaleCall *recordedCall
	for i := range mock.calls {
		c := &mock.calls[i]
		switch {
		case c.command == "python3" && len(c.args) > 0 && c.args[0] == "/usr/local/bin/calc_index.py":
			calcCall = c
		case c.command == "gdaldem":
			reliefCall = c
		case c.command == "gdal_translate" && len(c.args) > 0 && c.args[0] == "-of" && c.args[1] == "PNG":
			upscaleCall = c
		}
	}

	if calcCall == nil {
		t.Fatal("expected a python3 /usr/local/bin/calc_index.py call, got none")
	}
	if reliefCall == nil {
		t.Fatal("expected a gdaldem color-relief call, got none")
	}
	if upscaleCall == nil {
		t.Fatal("expected a gdal_translate upscale call, got none")
	}

	// NDVI formula should be the safe-ratio form.
	wantFormula := "--calc=" + safeRatio("A-B", "A+B")
	foundFormula := false
	for _, a := range calcCall.args {
		if a == wantFormula {
			foundFormula = true
		}
	}
	if !foundFormula {
		t.Errorf("NDVI formula arg not found; want %q in args %v", wantFormula, calcCall.args)
	}

	// 2 band inputs (-A, -B) present.
	bandArgs := 0
	for _, a := range calcCall.args {
		if a == "-A" || a == "-B" {
			bandArgs++
		}
	}
	if bandArgs != 2 {
		t.Errorf("expected 2 band args (-A, -B), got %d", bandArgs)
	}

	// gdaldem writes to native GTiff first.
	wantNative := outputPath + ".native.tif"
	if reliefCall.args[0] != "color-relief" {
		t.Errorf("gdaldem args[0] = %s, want color-relief", reliefCall.args[0])
	}
	if reliefCall.args[3] != wantNative {
		t.Errorf("gdaldem output = %q, want %q", reliefCall.args[3], wantNative)
	}

	// gdal_translate upscale goes from native tif to final PNG.
	if upscaleCall.args[len(upscaleCall.args)-1] != outputPath {
		t.Errorf("gdal_translate final arg = %q, want %q", upscaleCall.args[len(upscaleCall.args)-1], outputPath)
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

	// Verify that the python3 calc_index.py call uses the DN-scaled EVI formula
	// (L=10000, not L=1) and references 3 extracted band temp files.
	var calcCall *recordedCall
	for i := range mock.calls {
		if mock.calls[i].command == "python3" && len(mock.calls[i].args) > 0 && mock.calls[i].args[0] == "/usr/local/bin/calc_index.py" {
			calcCall = &mock.calls[i]
			break
		}
	}
	if calcCall == nil {
		t.Fatal("expected a python3 calc_index.py call, got none")
	}

	wantFormula := "numpy.where(numpy.abs(A+6*B-7.5*C+10000)<1e-6,9999,numpy.clip(2.5*(A-B)/(A+6*B-7.5*C+10000),-1,1))"
	var gotFormula string
	for _, a := range calcCall.args {
		if len(a) > 7 && a[:7] == "--calc=" {
			gotFormula = a[7:]
		}
	}
	if gotFormula != wantFormula {
		t.Errorf("EVI formula = %q, want %q", gotFormula, wantFormula)
	}

	// Confirm 3 band inputs (-A, -B, -C) are present.
	bandArgs := 0
	for _, a := range calcCall.args {
		if a == "-A" || a == "-B" || a == "-C" {
			bandArgs++
		}
	}
	if bandArgs != 3 {
		t.Errorf("expected 3 band args (-A, -B, -C), got %d", bandArgs)
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
