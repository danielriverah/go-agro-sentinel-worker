diff --git a/internal/infrastructure/gdal/executor.go b/internal/infrastructure/gdal/executor.go
new file mode 100644
index 0000000..a46341b
--- /dev/null
+++ b/internal/infrastructure/gdal/executor.go
@@ -0,0 +1,70 @@
+// Package gdal provides a thin wrapper around the GDAL command-line tools.
+// GDAL is always invoked as an external process (never as a Go library
+// binding) so the worker has no cgo dependency on libgdal.
+package gdal
+
+import (
+	"bytes"
+	"context"
+	"os/exec"
+	"time"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// Executor runs GDAL command-line tools with a configurable timeout.
+type Executor struct {
+	// DefaultTimeout is used when the caller's context has no deadline.
+	DefaultTimeout time.Duration
+}
+
+// NewExecutor creates an Executor whose default timeout is timeoutSeconds.
+// If timeoutSeconds is <= 0, a default of 300 seconds is used.
+func NewExecutor(timeoutSeconds int) *Executor {
+	if timeoutSeconds <= 0 {
+		timeoutSeconds = 300
+	}
+	return &Executor{DefaultTimeout: time.Duration(timeoutSeconds) * time.Second}
+}
+
+// Run executes a GDAL command (e.g. "gdalinfo") with the given args,
+// capturing stdout and stderr. If ctx has no deadline, the Executor's
+// DefaultTimeout is applied. A non-zero exit code, a timeout, or a failure
+// to start the process are all reported as *domain.ProcessingError with
+// Type domain.ErrGDAL.
+func (e *Executor) Run(ctx context.Context, command string, args []string) (string, string, error) {
+	runCtx := ctx
+	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
+		timeout := e.DefaultTimeout
+		if timeout <= 0 {
+			timeout = 300 * time.Second
+		}
+		var cancel context.CancelFunc
+		runCtx, cancel = context.WithTimeout(ctx, timeout)
+		defer cancel()
+	}
+
+	cmd := exec.CommandContext(runCtx, command, args...)
+
+	var stdout, stderr bytes.Buffer
+	cmd.Stdout = &stdout
+	cmd.Stderr = &stderr
+
+	err := cmd.Run()
+	if err != nil {
+		if runCtx.Err() == context.DeadlineExceeded {
+			return stdout.String(), stderr.String(), &domain.ProcessingError{
+				Type:    domain.ErrGDAL,
+				Message: command + " timed out",
+				Wrapped: runCtx.Err(),
+			}
+		}
+		return stdout.String(), stderr.String(), &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: command + " failed: " + stderr.String(),
+			Wrapped: err,
+		}
+	}
+
+	return stdout.String(), stderr.String(), nil
+}
diff --git a/internal/infrastructure/gdal/executor_test.go b/internal/infrastructure/gdal/executor_test.go
new file mode 100644
index 0000000..c0a1f7c
--- /dev/null
+++ b/internal/infrastructure/gdal/executor_test.go
@@ -0,0 +1,79 @@
+package gdal
+
+import (
+	"context"
+	"os/exec"
+	"runtime"
+	"strconv"
+	"strings"
+	"testing"
+	"time"
+)
+
+func TestExecutorRun(t *testing.T) {
+	if _, err := exec.LookPath("gdalinfo"); err != nil {
+		t.Skip("gdalinfo not found in PATH")
+	}
+	e := NewExecutor(300)
+	stdout, stderr, err := e.Run(context.Background(), "gdalinfo", []string{"--version"})
+	if err != nil {
+		t.Fatalf("gdalinfo --version failed: %v (stderr: %s)", err, stderr)
+	}
+	if !strings.Contains(stdout, "GDAL") {
+		t.Errorf("expected GDAL in output, got: %s", stdout)
+	}
+}
+
+func TestExecutorRunNonZeroExit(t *testing.T) {
+	if _, err := exec.LookPath("gdalinfo"); err != nil {
+		t.Skip("gdalinfo not found in PATH")
+	}
+	e := NewExecutor(300)
+	_, _, err := e.Run(context.Background(), "gdalinfo", []string{"/nonexistent/path/does-not-exist.tif"})
+	if err == nil {
+		t.Fatal("expected error for nonexistent input file")
+	}
+}
+
+func TestExecutorRunTimeout(t *testing.T) {
+	sleepCmd, sleepArgs := sleepCommand(2)
+	if _, err := exec.LookPath(sleepCmd); err != nil {
+		t.Skipf("%s not found in PATH", sleepCmd)
+	}
+
+	e := NewExecutor(1) // 1 second default timeout
+	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
+	defer cancel()
+	_, _, err := e.Run(ctx, sleepCmd, sleepArgs)
+	if err == nil {
+		t.Error("expected timeout error")
+	}
+}
+
+func TestExecutorRunDefaultTimeoutAppliedWithoutDeadline(t *testing.T) {
+	sleepCmd, sleepArgs := sleepCommand(5)
+	if _, err := exec.LookPath(sleepCmd); err != nil {
+		t.Skipf("%s not found in PATH", sleepCmd)
+	}
+
+	e := NewExecutor(1) // 1 second default timeout, no deadline on ctx
+	start := time.Now()
+	_, _, err := e.Run(context.Background(), sleepCmd, sleepArgs)
+	elapsed := time.Since(start)
+
+	if err == nil {
+		t.Error("expected timeout error from default timeout")
+	}
+	if elapsed > 4*time.Second {
+		t.Errorf("expected Run to be cut off near 1s default timeout, took %v", elapsed)
+	}
+}
+
+// sleepCommand returns a platform-appropriate command+args that sleeps for
+// roughly n seconds.
+func sleepCommand(n int) (string, []string) {
+	if runtime.GOOS == "windows" {
+		return "ping", []string{"-n", strconv.Itoa(n + 1), "127.0.0.1"}
+	}
+	return "sleep", []string{strconv.Itoa(n)}
+}
diff --git a/internal/infrastructure/gdal/info.go b/internal/infrastructure/gdal/info.go
new file mode 100644
index 0000000..1a74f2f
--- /dev/null
+++ b/internal/infrastructure/gdal/info.go
@@ -0,0 +1,77 @@
+package gdal
+
+import (
+	"context"
+	"encoding/json"
+	"os"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// GDALInfo holds the subset of `gdalinfo -json` output this worker cares about.
+type GDALInfo struct {
+	Width      int
+	Height     int
+	Bands      int
+	Projection string
+	BoundsMinX float64
+	BoundsMinY float64
+	BoundsMaxX float64
+	BoundsMaxY float64
+}
+
+// gdalInfoJSON mirrors the relevant fields of `gdalinfo -json` output.
+type gdalInfoJSON struct {
+	Size             [2]int     `json:"size"`
+	Bands            []struct{} `json:"bands"`
+	CoordinateSystem struct {
+		Wkt string `json:"wkt"`
+	} `json:"coordinateSystem"`
+	CornerCoordinates struct {
+		UpperLeft  [2]float64 `json:"upperLeft"`
+		LowerLeft  [2]float64 `json:"lowerLeft"`
+		UpperRight [2]float64 `json:"upperRight"`
+		LowerRight [2]float64 `json:"lowerRight"`
+	} `json:"cornerCoordinates"`
+}
+
+// Info runs `gdalinfo -json <inputPath>` and parses the result.
+func Info(ctx context.Context, executor *Executor, inputPath string) (*GDALInfo, error) {
+	if _, err := os.Stat(inputPath); err != nil {
+		return nil, &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "gdalinfo input file not found: " + inputPath,
+			Wrapped: err,
+		}
+	}
+
+	stdout, stderr, err := executor.Run(ctx, "gdalinfo", []string{"-json", inputPath})
+	if err != nil {
+		return nil, err
+	}
+
+	var parsed gdalInfoJSON
+	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
+		return nil, &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "failed to parse gdalinfo output: " + stderr,
+			Wrapped: err,
+		}
+	}
+
+	minX := parsed.CornerCoordinates.LowerLeft[0]
+	minY := parsed.CornerCoordinates.LowerLeft[1]
+	maxX := parsed.CornerCoordinates.UpperRight[0]
+	maxY := parsed.CornerCoordinates.UpperRight[1]
+
+	return &GDALInfo{
+		Width:      parsed.Size[0],
+		Height:     parsed.Size[1],
+		Bands:      len(parsed.Bands),
+		Projection: parsed.CoordinateSystem.Wkt,
+		BoundsMinX: minX,
+		BoundsMinY: minY,
+		BoundsMaxX: maxX,
+		BoundsMaxY: maxY,
+	}, nil
+}
diff --git a/internal/infrastructure/gdal/info_test.go b/internal/infrastructure/gdal/info_test.go
new file mode 100644
index 0000000..cbe8ff1
--- /dev/null
+++ b/internal/infrastructure/gdal/info_test.go
@@ -0,0 +1,115 @@
+package gdal
+
+import (
+	"context"
+	"encoding/json"
+	"os"
+	"os/exec"
+	"path/filepath"
+	"testing"
+)
+
+// ensureTinyTif returns the path to testdata/tiny.tif, creating it with
+// `gdal_create` if it doesn't already exist. It skips the calling test if
+// gdal_create is not available.
+func ensureTinyTif(t *testing.T) string {
+	t.Helper()
+
+	path, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "tiny.tif"))
+	if err != nil {
+		t.Fatalf("resolving testdata path: %v", err)
+	}
+
+	if _, statErr := os.Stat(path); statErr == nil {
+		return path
+	}
+
+	if _, err := exec.LookPath("gdal_create"); err != nil {
+		t.Skip("gdal_create not found in PATH; cannot create testdata/tiny.tif")
+	}
+
+	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
+		t.Fatalf("creating testdata dir: %v", err)
+	}
+
+	cmd := exec.Command("gdal_create", "-of", "GTiff", "-outsize", "10", "10", "-bands", "1", "-burn", "128", path)
+	if out, err := cmd.CombinedOutput(); err != nil {
+		t.Skipf("gdal_create failed, skipping: %v (%s)", err, out)
+	}
+
+	return path
+}
+
+func TestInfoParsesTinyTif(t *testing.T) {
+	if _, err := exec.LookPath("gdalinfo"); err != nil {
+		t.Skip("gdalinfo not found in PATH")
+	}
+
+	path := ensureTinyTif(t)
+	e := NewExecutor(300)
+
+	info, err := Info(context.Background(), e, path)
+	if err != nil {
+		t.Fatalf("Info failed: %v", err)
+	}
+
+	if info.Width != 10 || info.Height != 10 {
+		t.Errorf("expected 10x10, got %dx%d", info.Width, info.Height)
+	}
+	if info.Bands != 1 {
+		t.Errorf("expected 1 band, got %d", info.Bands)
+	}
+}
+
+func TestInfoNonexistentFile(t *testing.T) {
+	e := NewExecutor(300)
+	_, err := Info(context.Background(), e, filepath.Join(t.TempDir(), "does-not-exist.tif"))
+	if err == nil {
+		t.Fatal("expected error for nonexistent input file")
+	}
+}
+
+// TestGDALInfoJSONParsing verifies the JSON-parsing logic against a mock
+// gdalinfo -json response, without requiring GDAL to be installed.
+func TestGDALInfoJSONParsing(t *testing.T) {
+	mock := []byte(`{
+		"size": [512, 256],
+		"bands": [{}, {}, {}],
+		"coordinateSystem": {"wkt": "PROJCS[\"WGS 84 / UTM zone 33N\"]"},
+		"cornerCoordinates": {
+			"upperLeft": [499980.0, 4200000.0],
+			"lowerLeft": [499980.0, 4100000.0],
+			"upperRight": [600000.0, 4200000.0],
+			"lowerRight": [600000.0, 4100000.0]
+		}
+	}`)
+
+	var parsed gdalInfoJSON
+	if err := json.Unmarshal(mock, &parsed); err != nil {
+		t.Fatalf("unmarshal failed: %v", err)
+	}
+
+	info := &GDALInfo{
+		Width:      parsed.Size[0],
+		Height:     parsed.Size[1],
+		Bands:      len(parsed.Bands),
+		Projection: parsed.CoordinateSystem.Wkt,
+		BoundsMinX: parsed.CornerCoordinates.LowerLeft[0],
+		BoundsMinY: parsed.CornerCoordinates.LowerLeft[1],
+		BoundsMaxX: parsed.CornerCoordinates.UpperRight[0],
+		BoundsMaxY: parsed.CornerCoordinates.UpperRight[1],
+	}
+
+	if info.Width != 512 || info.Height != 256 {
+		t.Errorf("expected 512x256, got %dx%d", info.Width, info.Height)
+	}
+	if info.Bands != 3 {
+		t.Errorf("expected 3 bands, got %d", info.Bands)
+	}
+	if info.BoundsMinX != 499980.0 || info.BoundsMaxX != 600000.0 {
+		t.Errorf("unexpected X bounds: min=%v max=%v", info.BoundsMinX, info.BoundsMaxX)
+	}
+	if info.BoundsMinY != 4100000.0 || info.BoundsMaxY != 4200000.0 {
+		t.Errorf("unexpected Y bounds: min=%v max=%v", info.BoundsMinY, info.BoundsMaxY)
+	}
+}
diff --git a/internal/infrastructure/gdal/translate.go b/internal/infrastructure/gdal/translate.go
new file mode 100644
index 0000000..0eefc5e
--- /dev/null
+++ b/internal/infrastructure/gdal/translate.go
@@ -0,0 +1,58 @@
+package gdal
+
+import (
+	"context"
+	"fmt"
+	"os"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// TranslateOpts configures a `gdal_translate` invocation.
+type TranslateOpts struct {
+	Input        string
+	Output       string
+	BBox         *domain.BBox
+	OutputFormat string
+}
+
+// Translate runs `gdal_translate` on opts.Input, optionally cropping to
+// opts.BBox via -projwin, and writes opts.Output. It verifies the command
+// exited successfully and that the output file was actually created.
+func Translate(ctx context.Context, executor *Executor, opts TranslateOpts) error {
+	args := []string{}
+
+	format := opts.OutputFormat
+	if format == "" {
+		format = "GTiff"
+	}
+	args = append(args, "-of", format)
+
+	if opts.BBox != nil {
+		// -projwin ulx uly lrx lry (upper-left / lower-right corners)
+		args = append(args,
+			"-projwin",
+			fmt.Sprintf("%v", opts.BBox.MinX),
+			fmt.Sprintf("%v", opts.BBox.MaxY),
+			fmt.Sprintf("%v", opts.BBox.MaxX),
+			fmt.Sprintf("%v", opts.BBox.MinY),
+		)
+	}
+
+	args = append(args, opts.Input, opts.Output)
+
+	_, stderr, err := executor.Run(ctx, "gdal_translate", args)
+	if err != nil {
+		return err
+	}
+
+	if _, statErr := os.Stat(opts.Output); statErr != nil {
+		return &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "gdal_translate did not produce output file: " + opts.Output + " (" + stderr + ")",
+			Wrapped: statErr,
+		}
+	}
+
+	return nil
+}
diff --git a/internal/infrastructure/gdal/vrt.go b/internal/infrastructure/gdal/vrt.go
new file mode 100644
index 0000000..70bcfa3
--- /dev/null
+++ b/internal/infrastructure/gdal/vrt.go
@@ -0,0 +1,44 @@
+package gdal
+
+import (
+	"context"
+	"os"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// VRTOpts configures a `gdalbuildvrt` invocation.
+type VRTOpts struct {
+	Inputs   []string
+	Output   string
+	Separate bool
+}
+
+// BuildVRT runs `gdalbuildvrt` to compose opts.Inputs (e.g. individual band
+// files) into a single VRT at opts.Output. When Separate is true, each
+// input becomes its own band in the output (-separate).
+func BuildVRT(ctx context.Context, executor *Executor, opts VRTOpts) error {
+	args := []string{}
+
+	if opts.Separate {
+		args = append(args, "-separate")
+	}
+
+	args = append(args, opts.Output)
+	args = append(args, opts.Inputs...)
+
+	_, stderr, err := executor.Run(ctx, "gdalbuildvrt", args)
+	if err != nil {
+		return err
+	}
+
+	if _, statErr := os.Stat(opts.Output); statErr != nil {
+		return &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "gdalbuildvrt did not produce output file: " + opts.Output + " (" + stderr + ")",
+			Wrapped: statErr,
+		}
+	}
+
+	return nil
+}
diff --git a/internal/infrastructure/gdal/warp.go b/internal/infrastructure/gdal/warp.go
new file mode 100644
index 0000000..252d836
--- /dev/null
+++ b/internal/infrastructure/gdal/warp.go
@@ -0,0 +1,67 @@
+package gdal
+
+import (
+	"context"
+	"fmt"
+	"os"
+	"strconv"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// WarpOpts configures a `gdalwarp` invocation.
+type WarpOpts struct {
+	Input            string
+	Output           string
+	TargetResolution int
+	ResamplingMethod string
+	TargetSRS        string
+	BBox             *domain.BBox
+}
+
+// Warp runs `gdalwarp` on opts.Input, applying target resolution,
+// resampling method, target SRS, and an optional BBOX crop. It verifies
+// the command exited successfully and that the output file was created.
+func Warp(ctx context.Context, executor *Executor, opts WarpOpts) error {
+	args := []string{}
+
+	if opts.TargetResolution > 0 {
+		res := strconv.Itoa(opts.TargetResolution)
+		args = append(args, "-tr", res, res)
+	}
+
+	if opts.ResamplingMethod != "" {
+		args = append(args, "-r", opts.ResamplingMethod)
+	}
+
+	if opts.TargetSRS != "" {
+		args = append(args, "-t_srs", opts.TargetSRS)
+	}
+
+	if opts.BBox != nil {
+		args = append(args,
+			"-te",
+			fmt.Sprintf("%v", opts.BBox.MinX),
+			fmt.Sprintf("%v", opts.BBox.MinY),
+			fmt.Sprintf("%v", opts.BBox.MaxX),
+			fmt.Sprintf("%v", opts.BBox.MaxY),
+		)
+	}
+
+	args = append(args, opts.Input, opts.Output)
+
+	_, stderr, err := executor.Run(ctx, "gdalwarp", args)
+	if err != nil {
+		return err
+	}
+
+	if _, statErr := os.Stat(opts.Output); statErr != nil {
+		return &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "gdalwarp did not produce output file: " + opts.Output + " (" + stderr + ")",
+			Wrapped: statErr,
+		}
+	}
+
+	return nil
+}
