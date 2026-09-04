diff --git a/internal/processing/bands.go b/internal/processing/bands.go
new file mode 100644
index 0000000..b0d976d
--- /dev/null
+++ b/internal/processing/bands.go
@@ -0,0 +1,24 @@
+// Package processing implements the core GDAL-driven raster processing
+// pipeline: cropping/resampling individual Sentinel-2 COG bands and
+// composing them into a single multiband GeoTIFF.
+package processing
+
+import "agro-sentinel-worker/internal/domain"
+
+// TargetSRS is the projection all bands are warped into before being
+// composed into the multiband output.
+const TargetSRS = "EPSG:32614"
+
+// resamplingMethodFor returns the gdalwarp resampling method to use for the
+// given band. 20m bands are always resampled with bilinear so they don't
+// introduce blocky artifacts when upsampled to the target resolution;
+// native 10m bands use the configured default (bilinear when unset).
+func resamplingMethodFor(band domain.Band, defaultMethod string) string {
+	if band.Resolution() == 20 {
+		return "bilinear"
+	}
+	if defaultMethod == "" {
+		return "bilinear"
+	}
+	return defaultMethod
+}
diff --git a/internal/processing/multiband.go b/internal/processing/multiband.go
new file mode 100644
index 0000000..cbae2f9
--- /dev/null
+++ b/internal/processing/multiband.go
@@ -0,0 +1,169 @@
+package processing
+
+import (
+	"context"
+	"fmt"
+	"log/slog"
+	"os"
+	"path/filepath"
+
+	"agro-sentinel-worker/internal/config"
+	"agro-sentinel-worker/internal/domain"
+)
+
+// GDALExecutor is the subset of gdal.Executor's behavior that the
+// processing package depends on. It is satisfied by *gdal.Executor and by
+// test doubles that record invocations without running real GDAL.
+type GDALExecutor interface {
+	Run(ctx context.Context, command string, args []string) (stdout string, stderr string, err error)
+}
+
+// S3Uploader is the subset of aws.S3Client's behavior the processing
+// package depends on when persisting build artifacts.
+type S3Uploader interface {
+	Upload(ctx context.Context, bucket, key, filePath string) error
+	HeadObject(ctx context.Context, bucket, key string) (bool, int64, error)
+}
+
+// MultibandBuilder builds a single multiband.tif from a set of Sentinel-2
+// COG bands by warping each band into a common grid/CRS, stacking them into
+// a VRT, and translating the VRT to GeoTIFF.
+type MultibandBuilder struct {
+	executor GDALExecutor
+	s3       S3Uploader
+	cfg      config.ProcessingConfig
+	logger   *slog.Logger
+}
+
+// New creates a MultibandBuilder.
+func New(executor GDALExecutor, s3 S3Uploader, cfg config.ProcessingConfig, logger *slog.Logger) *MultibandBuilder {
+	if logger == nil {
+		logger = slog.Default()
+	}
+	return &MultibandBuilder{executor: executor, s3: s3, cfg: cfg, logger: logger}
+}
+
+// Build warps each band in bands to bbox/targetResolution in jobDir's work
+// subdirectory, stacks them into a VRT, and translates the VRT into
+// multiband.tif in jobDir's output subdirectory. It returns the path to the
+// generated multiband.tif.
+func (b *MultibandBuilder) Build(ctx context.Context, jobDir string, bbox domain.BBox, bands []domain.BandInfo, targetResolution int) (string, error) {
+	if len(bands) == 0 {
+		return "", &domain.ProcessingError{
+			Type:    domain.ErrValidation,
+			Message: "no bands provided to build multiband.tif",
+		}
+	}
+
+	if err := bbox.Validate(); err != nil {
+		return "", &domain.ProcessingError{
+			Type:    domain.ErrValidation,
+			Message: "invalid bbox",
+			Wrapped: err,
+		}
+	}
+
+	workDir := filepath.Join(jobDir, "work")
+	outputDir := filepath.Join(jobDir, "output")
+	if err := os.MkdirAll(workDir, 0o755); err != nil {
+		return "", &domain.ProcessingError{Type: domain.ErrDisk, Message: "creating work dir", Wrapped: err}
+	}
+	if err := os.MkdirAll(outputDir, 0o755); err != nil {
+		return "", &domain.ProcessingError{Type: domain.ErrDisk, Message: "creating output dir", Wrapped: err}
+	}
+
+	warpedPaths := make([]string, 0, len(bands))
+	for _, band := range bands {
+		warpedPath := filepath.Join(workDir, fmt.Sprintf("%s.tif", band.Name))
+		if err := b.warpBand(ctx, band, bbox, targetResolution, warpedPath); err != nil {
+			return "", err
+		}
+		warpedPaths = append(warpedPaths, warpedPath)
+	}
+
+	vrtPath := filepath.Join(workDir, "composite.vrt")
+	if err := b.buildVRT(ctx, warpedPaths, vrtPath); err != nil {
+		return "", err
+	}
+
+	outputPath := filepath.Join(outputDir, "multiband.tif")
+	if err := b.translate(ctx, vrtPath, outputPath); err != nil {
+		return "", err
+	}
+
+	return outputPath, nil
+}
+
+func (b *MultibandBuilder) warpBand(ctx context.Context, band domain.BandInfo, bbox domain.BBox, targetResolution int, outputPath string) error {
+	resamplingMethod := resamplingMethodFor(band.Name, b.cfg.ResamplingMethod)
+	res := fmt.Sprintf("%d", targetResolution)
+
+	args := []string{
+		"-t_srs", TargetSRS,
+		"-te", fmt.Sprintf("%v", bbox.MinX), fmt.Sprintf("%v", bbox.MinY), fmt.Sprintf("%v", bbox.MaxX), fmt.Sprintf("%v", bbox.MaxY),
+		"-tr", res, res,
+		"-r", resamplingMethod,
+		band.Href,
+		outputPath,
+	}
+
+	b.logger.Debug("warping band", "band", band.Name, "resampling", resamplingMethod, "output", outputPath)
+
+	_, stderr, err := b.executor.Run(ctx, "gdalwarp", args)
+	if err != nil {
+		return err
+	}
+
+	if _, statErr := os.Stat(outputPath); statErr != nil {
+		return &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "gdalwarp did not produce output file: " + outputPath + " (" + stderr + ")",
+			Wrapped: statErr,
+		}
+	}
+
+	return nil
+}
+
+func (b *MultibandBuilder) buildVRT(ctx context.Context, inputs []string, vrtPath string) error {
+	args := []string{"-separate", vrtPath}
+	args = append(args, inputs...)
+
+	b.logger.Debug("building VRT", "inputs", len(inputs), "output", vrtPath)
+
+	_, stderr, err := b.executor.Run(ctx, "gdalbuildvrt", args)
+	if err != nil {
+		return err
+	}
+
+	if _, statErr := os.Stat(vrtPath); statErr != nil {
+		return &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "gdalbuildvrt did not produce output file: " + vrtPath + " (" + stderr + ")",
+			Wrapped: statErr,
+		}
+	}
+
+	return nil
+}
+
+func (b *MultibandBuilder) translate(ctx context.Context, vrtPath, outputPath string) error {
+	args := []string{"-of", "GTiff", vrtPath, outputPath}
+
+	b.logger.Debug("translating VRT to GeoTIFF", "output", outputPath)
+
+	_, stderr, err := b.executor.Run(ctx, "gdal_translate", args)
+	if err != nil {
+		return err
+	}
+
+	if _, statErr := os.Stat(outputPath); statErr != nil {
+		return &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "gdal_translate did not produce output file: " + outputPath + " (" + stderr + ")",
+			Wrapped: statErr,
+		}
+	}
+
+	return nil
+}
diff --git a/internal/processing/multiband_test.go b/internal/processing/multiband_test.go
new file mode 100644
index 0000000..01f2ae9
--- /dev/null
+++ b/internal/processing/multiband_test.go
@@ -0,0 +1,205 @@
+package processing
+
+import (
+	"context"
+	"os"
+	"path/filepath"
+	"testing"
+
+	"agro-sentinel-worker/internal/config"
+	"agro-sentinel-worker/internal/domain"
+)
+
+// recordedCall captures one invocation of the mock GDALExecutor.
+type recordedCall struct {
+	command string
+	args    []string
+}
+
+// mockExecutor records every Run call and, to simulate real GDAL behavior,
+// creates an empty file at the last argument (the conventional output path
+// for gdalwarp/gdalbuildvrt/gdal_translate) so downstream os.Stat checks
+// succeed without ever shelling out to real GDAL.
+type mockExecutor struct {
+	calls []recordedCall
+	err   error
+}
+
+func (m *mockExecutor) Run(ctx context.Context, command string, args []string) (string, string, error) {
+	m.calls = append(m.calls, recordedCall{command: command, args: append([]string{}, args...)})
+
+	if m.err != nil {
+		return "", "mock error", m.err
+	}
+
+	outputPath := ""
+	switch command {
+	case "gdalbuildvrt":
+		// args: [-separate] output input1 input2 ...
+		if len(args) > 0 {
+			if args[0] == "-separate" {
+				outputPath = args[1]
+			} else {
+				outputPath = args[0]
+			}
+		}
+	default:
+		// gdalwarp/gdal_translate: args: [...flags...] input output
+		if len(args) > 0 {
+			outputPath = args[len(args)-1]
+		}
+	}
+
+	if outputPath != "" {
+		if err := os.WriteFile(outputPath, []byte("fake"), 0o644); err != nil {
+			return "", "", err
+		}
+	}
+
+	return "", "", nil
+}
+
+func testBands() []domain.BandInfo {
+	return []domain.BandInfo{
+		{Name: domain.BandB02, Resolution: 10, Href: "/vsis3/bucket/B02.tif"},
+		{Name: domain.BandB03, Resolution: 10, Href: "/vsis3/bucket/B03.tif"},
+		{Name: domain.BandB05, Resolution: 20, Href: "/vsis3/bucket/B05.tif"},
+	}
+}
+
+func testBBox() domain.BBox {
+	return domain.BBox{MinX: -100, MinY: 20, MaxX: -99, MaxY: 21}
+}
+
+func TestMultibandBuilder_Build(t *testing.T) {
+	jobDir := t.TempDir()
+	mock := &mockExecutor{}
+	cfg := config.ProcessingConfig{ResamplingMethod: "bilinear"}
+
+	builder := New(mock, nil, cfg, nil)
+
+	outputPath, err := builder.Build(context.Background(), jobDir, testBBox(), testBands(), 10)
+	if err != nil {
+		t.Fatalf("Build failed: %v", err)
+	}
+
+	wantOutput := filepath.Join(jobDir, "output", "multiband.tif")
+	if outputPath != wantOutput {
+		t.Errorf("output path = %q, want %q", outputPath, wantOutput)
+	}
+	if _, err := os.Stat(outputPath); err != nil {
+		t.Errorf("output file does not exist: %v", err)
+	}
+
+	// 3 gdalwarp calls + 1 gdalbuildvrt + 1 gdal_translate.
+	if len(mock.calls) != 5 {
+		t.Fatalf("expected 5 executor calls, got %d", len(mock.calls))
+	}
+
+	warpCalls := mock.calls[:3]
+	for i, call := range warpCalls {
+		if call.command != "gdalwarp" {
+			t.Errorf("call %d: command = %q, want gdalwarp", i, call.command)
+		}
+		if !containsArg(call.args, "-te") {
+			t.Errorf("call %d: missing -te flag: %v", i, call.args)
+		}
+		if !containsArg(call.args, "-tr") {
+			t.Errorf("call %d: missing -tr flag: %v", i, call.args)
+		}
+		if !containsArgValue(call.args, "-tr", "10") {
+			t.Errorf("call %d: expected -tr 10, got %v", i, call.args)
+		}
+	}
+
+	// B05 is a 20m band and must always be resampled with bilinear.
+	b05Call := warpCalls[2]
+	if got := argAfter(b05Call.args, "-r"); got != "bilinear" {
+		t.Errorf("B05 resampling method = %q, want bilinear", got)
+	}
+
+	// B02/B03 are 10m bands and use the configured default (bilinear here).
+	for i := 0; i < 2; i++ {
+		if got := argAfter(warpCalls[i].args, "-r"); got != "bilinear" {
+			t.Errorf("call %d resampling method = %q, want bilinear", i, got)
+		}
+	}
+
+	vrtCall := mock.calls[3]
+	if vrtCall.command != "gdalbuildvrt" {
+		t.Fatalf("call 3 command = %q, want gdalbuildvrt", vrtCall.command)
+	}
+	if !containsArg(vrtCall.args, "-separate") {
+		t.Errorf("gdalbuildvrt missing -separate: %v", vrtCall.args)
+	}
+	// The VRT output is args[1] (after -separate), followed by 3 warped inputs.
+	if len(vrtCall.args) != 5 {
+		t.Errorf("gdalbuildvrt args = %v, want 5 (-separate, output, 3 inputs)", vrtCall.args)
+	}
+
+	translateCall := mock.calls[4]
+	if translateCall.command != "gdal_translate" {
+		t.Fatalf("call 4 command = %q, want gdal_translate", translateCall.command)
+	}
+	if translateCall.args[len(translateCall.args)-1] != wantOutput {
+		t.Errorf("gdal_translate output = %q, want %q", translateCall.args[len(translateCall.args)-1], wantOutput)
+	}
+}
+
+func TestMultibandBuilder_Build_NoBands(t *testing.T) {
+	mock := &mockExecutor{}
+	builder := New(mock, nil, config.ProcessingConfig{}, nil)
+
+	_, err := builder.Build(context.Background(), t.TempDir(), testBBox(), nil, 10)
+	if err == nil {
+		t.Fatal("expected error for empty band list, got nil")
+	}
+}
+
+func TestMultibandBuilder_Build_InvalidBBox(t *testing.T) {
+	mock := &mockExecutor{}
+	builder := New(mock, nil, config.ProcessingConfig{}, nil)
+
+	badBBox := domain.BBox{MinX: 10, MinY: 10, MaxX: 5, MaxY: 20}
+	_, err := builder.Build(context.Background(), t.TempDir(), badBBox, testBands(), 10)
+	if err == nil {
+		t.Fatal("expected error for invalid bbox, got nil")
+	}
+}
+
+func TestMultibandBuilder_Build_ExecutorError(t *testing.T) {
+	mock := &mockExecutor{err: os.ErrPermission}
+	builder := New(mock, nil, config.ProcessingConfig{}, nil)
+
+	_, err := builder.Build(context.Background(), t.TempDir(), testBBox(), testBands(), 10)
+	if err == nil {
+		t.Fatal("expected error when executor fails, got nil")
+	}
+}
+
+func containsArg(args []string, want string) bool {
+	for _, a := range args {
+		if a == want {
+			return true
+		}
+	}
+	return false
+}
+
+func containsArgValue(args []string, flag, value string) bool {
+	for i, a := range args {
+		if a == flag && i+1 < len(args) && args[i+1] == value {
+			return true
+		}
+	}
+	return false
+}
+
+func argAfter(args []string, flag string) string {
+	for i, a := range args {
+		if a == flag && i+1 < len(args) {
+			return args[i+1]
+		}
+	}
+	return ""
+}
diff --git a/internal/storage/filesystem.go b/internal/storage/filesystem.go
new file mode 100644
index 0000000..50cbdfa
--- /dev/null
+++ b/internal/storage/filesystem.go
@@ -0,0 +1,57 @@
+// Package storage provides filesystem layout helpers for job processing.
+package storage
+
+import (
+	"os"
+	"path/filepath"
+)
+
+// JobDir represents the on-disk directory layout for a single job:
+//
+//	{baseDir}/jobs/{jobID}/input/
+//	{baseDir}/jobs/{jobID}/work/
+//	{baseDir}/jobs/{jobID}/output/
+type JobDir struct {
+	baseDir string
+	jobID   string
+}
+
+// New creates a JobDir rooted at baseDir for the given jobID.
+func New(baseDir string, jobID string) *JobDir {
+	return &JobDir{baseDir: baseDir, jobID: jobID}
+}
+
+// root returns {baseDir}/jobs/{jobID}.
+func (j *JobDir) root() string {
+	return filepath.Join(j.baseDir, "jobs", j.jobID)
+}
+
+// Input returns {baseDir}/jobs/{jobID}/input/.
+func (j *JobDir) Input() string {
+	return filepath.Join(j.root(), "input")
+}
+
+// Work returns {baseDir}/jobs/{jobID}/work/.
+func (j *JobDir) Work() string {
+	return filepath.Join(j.root(), "work")
+}
+
+// Output returns {baseDir}/jobs/{jobID}/output/.
+func (j *JobDir) Output() string {
+	return filepath.Join(j.root(), "output")
+}
+
+// Create creates the input, work, and output subdirectories.
+func (j *JobDir) Create() error {
+	for _, dir := range []string{j.Input(), j.Work(), j.Output()} {
+		if err := os.MkdirAll(dir, 0o755); err != nil {
+			return err
+		}
+	}
+	return nil
+}
+
+// Cleanup removes the entire job directory tree.
+func (j *JobDir) Cleanup() error {
+	return os.RemoveAll(j.root())
+}
diff --git a/internal/storage/filesystem_test.go b/internal/storage/filesystem_test.go
new file mode 100644
index 0000000..b55bf74
--- /dev/null
+++ b/internal/storage/filesystem_test.go
@@ -0,0 +1,29 @@
+package storage
+
+import (
+	"os"
+	"testing"
+)
+
+func TestJobDir(t *testing.T) {
+	base := t.TempDir()
+	jd := New(base, "test-job-123")
+
+	if err := jd.Create(); err != nil {
+		t.Fatalf("Create failed: %v", err)
+	}
+
+	for _, dir := range []string{jd.Input(), jd.Work(), jd.Output()} {
+		if _, err := os.Stat(dir); os.IsNotExist(err) {
+			t.Errorf("directory %s should exist", dir)
+		}
+	}
+
+	if err := jd.Cleanup(); err != nil {
+		t.Fatalf("Cleanup failed: %v", err)
+	}
+
+	if _, err := os.Stat(jd.Input()); !os.IsNotExist(err) {
+		t.Error("job directory should be removed after cleanup")
+	}
+}
