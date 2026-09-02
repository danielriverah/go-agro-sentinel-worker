package processing

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/domain"
)

// recordedCall captures one invocation of the mock GDALExecutor.
type recordedCall struct {
	command string
	args    []string
}

// mockExecutor records every Run call and, to simulate real GDAL behavior,
// creates an empty file at the last argument (the conventional output path
// for gdalwarp/gdalbuildvrt/gdal_translate) so downstream os.Stat checks
// succeed without ever shelling out to real GDAL.
type mockExecutor struct {
	calls []recordedCall
	err   error
}

func (m *mockExecutor) Run(ctx context.Context, command string, args []string) (string, string, error) {
	m.calls = append(m.calls, recordedCall{command: command, args: append([]string{}, args...)})

	if m.err != nil {
		return "", "mock error", m.err
	}

	outputPath := ""
	switch command {
	case "gdalbuildvrt":
		// args: [-separate] output input1 input2 ...
		if len(args) > 0 {
			if args[0] == "-separate" {
				outputPath = args[1]
			} else {
				outputPath = args[0]
			}
		}
	case "gdal_calc.py":
		// args include a "--outfile=<path>" flag.
		for _, a := range args {
			if strings.HasPrefix(a, "--outfile=") {
				outputPath = strings.TrimPrefix(a, "--outfile=")
				break
			}
		}
	case "gdaldem":
		// args: color-relief input colorfile output [flags...]
		if len(args) >= 4 {
			outputPath = args[3]
		}
	default:
		// gdalwarp/gdal_translate: args: [...flags...] input output
		if len(args) > 0 {
			outputPath = args[len(args)-1]
		}
	}

	if outputPath != "" {
		if err := os.WriteFile(outputPath, []byte("fake"), 0o644); err != nil {
			return "", "", err
		}
	}

	return "", "", nil
}

func testBands() []domain.BandInfo {
	return []domain.BandInfo{
		{Name: domain.BandB02, Resolution: 10, Href: "/vsis3/bucket/B02.tif"},
		{Name: domain.BandB03, Resolution: 10, Href: "/vsis3/bucket/B03.tif"},
		{Name: domain.BandB05, Resolution: 20, Href: "/vsis3/bucket/B05.tif"},
	}
}

func testBBox() domain.BBox {
	return domain.BBox{MinX: -100, MinY: 20, MaxX: -99, MaxY: 21}
}

func TestMultibandBuilder_Build(t *testing.T) {
	jobDir := t.TempDir()
	mock := &mockExecutor{}
	cfg := config.ProcessingConfig{ResamplingMethod: "bilinear"}

	builder := New(mock, nil, cfg, nil)

	outputPath, err := builder.Build(context.Background(), jobDir, testBBox(), testBands(), 10)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	wantOutput := filepath.Join(jobDir, "output", "multiband.tif")
	if outputPath != wantOutput {
		t.Errorf("output path = %q, want %q", outputPath, wantOutput)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Errorf("output file does not exist: %v", err)
	}

	// 3 gdalwarp calls + 1 gdalbuildvrt + 1 gdal_translate.
	if len(mock.calls) != 5 {
		t.Fatalf("expected 5 executor calls, got %d", len(mock.calls))
	}

	warpCalls := mock.calls[:3]
	for i, call := range warpCalls {
		if call.command != "gdalwarp" {
			t.Errorf("call %d: command = %q, want gdalwarp", i, call.command)
		}
		if !containsArg(call.args, "-te") {
			t.Errorf("call %d: missing -te flag: %v", i, call.args)
		}
		if !containsArg(call.args, "-tr") {
			t.Errorf("call %d: missing -tr flag: %v", i, call.args)
		}
		if !containsArgValue(call.args, "-tr", "10") {
			t.Errorf("call %d: expected -tr 10, got %v", i, call.args)
		}
	}

	// B05 is a 20m band and must always be resampled with bilinear.
	b05Call := warpCalls[2]
	if got := argAfter(b05Call.args, "-r"); got != "bilinear" {
		t.Errorf("B05 resampling method = %q, want bilinear", got)
	}

	// B02/B03 are 10m bands and use the configured default (bilinear here).
	for i := 0; i < 2; i++ {
		if got := argAfter(warpCalls[i].args, "-r"); got != "bilinear" {
			t.Errorf("call %d resampling method = %q, want bilinear", i, got)
		}
	}

	vrtCall := mock.calls[3]
	if vrtCall.command != "gdalbuildvrt" {
		t.Fatalf("call 3 command = %q, want gdalbuildvrt", vrtCall.command)
	}
	if !containsArg(vrtCall.args, "-separate") {
		t.Errorf("gdalbuildvrt missing -separate: %v", vrtCall.args)
	}
	// The VRT output is args[1] (after -separate), followed by 3 warped inputs.
	if len(vrtCall.args) != 5 {
		t.Errorf("gdalbuildvrt args = %v, want 5 (-separate, output, 3 inputs)", vrtCall.args)
	}

	translateCall := mock.calls[4]
	if translateCall.command != "gdal_translate" {
		t.Fatalf("call 4 command = %q, want gdal_translate", translateCall.command)
	}
	if translateCall.args[len(translateCall.args)-1] != wantOutput {
		t.Errorf("gdal_translate output = %q, want %q", translateCall.args[len(translateCall.args)-1], wantOutput)
	}
}

func TestMultibandBuilder_Build_NoBands(t *testing.T) {
	mock := &mockExecutor{}
	builder := New(mock, nil, config.ProcessingConfig{}, nil)

	_, err := builder.Build(context.Background(), t.TempDir(), testBBox(), nil, 10)
	if err == nil {
		t.Fatal("expected error for empty band list, got nil")
	}
}

func TestMultibandBuilder_Build_InvalidBBox(t *testing.T) {
	mock := &mockExecutor{}
	builder := New(mock, nil, config.ProcessingConfig{}, nil)

	badBBox := domain.BBox{MinX: 10, MinY: 10, MaxX: 5, MaxY: 20}
	_, err := builder.Build(context.Background(), t.TempDir(), badBBox, testBands(), 10)
	if err == nil {
		t.Fatal("expected error for invalid bbox, got nil")
	}
}

func TestMultibandBuilder_Build_ExecutorError(t *testing.T) {
	mock := &mockExecutor{err: os.ErrPermission}
	builder := New(mock, nil, config.ProcessingConfig{}, nil)

	_, err := builder.Build(context.Background(), t.TempDir(), testBBox(), testBands(), 10)
	if err == nil {
		t.Fatal("expected error when executor fails, got nil")
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func containsArgValue(args []string, flag, value string) bool {
	for i, a := range args {
		if a == flag && i+1 < len(args) && args[i+1] == value {
			return true
		}
	}
	return false
}

func argAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
