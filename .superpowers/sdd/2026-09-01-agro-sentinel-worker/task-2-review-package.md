# Review Package — Task 2

## Commits
60fb6ca feat: domain entities — production, scene, band, errors

## Stat
 internal/domain/analysis.go        | 11 ++++++++
 internal/domain/band.go            | 55 ++++++++++++++++++++++++++++++++++++++
 internal/domain/band_test.go       | 34 +++++++++++++++++++++++
 internal/domain/errors.go          | 34 +++++++++++++++++++++++
 internal/domain/product.go         | 28 +++++++++++++++++++
 internal/domain/production.go      | 51 +++++++++++++++++++++++++++++++++++
 internal/domain/production_test.go | 52 +++++++++++++++++++++++++++++++++++
 internal/domain/scene.go           | 55 ++++++++++++++++++++++++++++++++++++++
 internal/domain/scene_test.go      | 26 ++++++++++++++++++
 9 files changed, 346 insertions(+)

## Diff
diff --git a/internal/domain/analysis.go b/internal/domain/analysis.go
new file mode 100644
index 0000000..c56c881
--- /dev/null
+++ b/internal/domain/analysis.go
@@ -0,0 +1,11 @@
+package domain
+
+type AnalysisResult struct {
+	EstadoGeneral   string   `json:"estado_general"`
+	VigorVegetativo string   `json:"vigor_vegetativo"`
+	EstresDetectado bool     `json:"estres_detectado"`
+	PosibleCosecha  bool     `json:"posible_cosecha"`
+	Recomendaciones []string `json:"recomendaciones"`
+	Alertas         []string `json:"alertas"`
+	Confianza       float64  `json:"confianza"`
+}
diff --git a/internal/domain/band.go b/internal/domain/band.go
new file mode 100644
index 0000000..66d1afa
--- /dev/null
+++ b/internal/domain/band.go
@@ -0,0 +1,55 @@
+package domain
+
+type Band string
+
+const (
+	BandB02 Band = "B02"
+	BandB03 Band = "B03"
+	BandB04 Band = "B04"
+	BandB05 Band = "B05"
+	BandB06 Band = "B06"
+	BandB07 Band = "B07"
+	BandB08 Band = "B08"
+	BandB8A Band = "B8A"
+	BandB11 Band = "B11"
+	BandB12 Band = "B12"
+	BandSCL Band = "SCL"
+)
+
+var bandResolutions = map[Band]int{
+	BandB02: 10,
+	BandB03: 10,
+	BandB04: 10,
+	BandB05: 20,
+	BandB06: 20,
+	BandB07: 20,
+	BandB08: 10,
+	BandB8A: 20,
+	BandB11: 20,
+	BandB12: 20,
+	BandSCL: 20,
+}
+
+func (b Band) Resolution() int {
+	return bandResolutions[b]
+}
+
+func AllSpectralBands() []Band {
+	return []Band{BandB02, BandB03, BandB04, BandB05, BandB06, BandB07, BandB08, BandB8A, BandB11, BandB12}
+}
+
+func BandsAtResolution(resolution int) []Band {
+	var result []Band
+	for _, b := range AllSpectralBands() {
+		if b.Resolution() == resolution {
+			result = append(result, b)
+		}
+	}
+	return result
+}
+
+type BandInfo struct {
+	Name       Band
+	Resolution int
+	Href       string
+}
diff --git a/internal/domain/band_test.go b/internal/domain/band_test.go
new file mode 100644
index 0000000..84593f6
--- /dev/null
+++ b/internal/domain/band_test.go
@@ -0,0 +1,34 @@
+package domain
+
+import "testing"
+
+func TestAllSpectralBands(t *testing.T) {
+	bands := AllSpectralBands()
+	if len(bands) != 10 {
+		t.Errorf("AllSpectralBands() returned %d bands, want 10", len(bands))
+	}
+}
+
+func TestBandsAtResolution(t *testing.T) {
+	bands10m := BandsAtResolution(10)
+	if len(bands10m) != 4 {
+		t.Errorf("BandsAtResolution(10) = %d bands, want 4 (B02,B03,B04,B08)", len(bands10m))
+	}
+
+	bands20m := BandsAtResolution(20)
+	if len(bands20m) != 6 {
+		t.Errorf("BandsAtResolution(20) = %d bands, want 6 (B05,B06,B07,B8A,B11,B12)", len(bands20m))
+	}
+}
+
+func TestBandResolution(t *testing.T) {
+	if BandB04.Resolution() != 10 {
+		t.Errorf("B04 resolution = %d, want 10", BandB04.Resolution())
+	}
+	if BandB05.Resolution() != 20 {
+		t.Errorf("B05 resolution = %d, want 20", BandB05.Resolution())
+	}
+	if BandSCL.Resolution() != 20 {
+		t.Errorf("SCL resolution = %d, want 20", BandSCL.Resolution())
+	}
+}
diff --git a/internal/domain/errors.go b/internal/domain/errors.go
new file mode 100644
index 0000000..49325b5
--- /dev/null
+++ b/internal/domain/errors.go
@@ -0,0 +1,34 @@
+package domain
+
+import "fmt"
+
+type ErrType string
+
+const (
+	ErrGDAL       ErrType = "GDAL_ERROR"
+	ErrS3         ErrType = "S3_ERROR"
+	ErrTimeout    ErrType = "TIMEOUT"
+	ErrValidation ErrType = "VALIDATION_ERROR"
+	ErrSTAC       ErrType = "STAC_ERROR"
+	ErrIA         ErrType = "IA_ERROR"
+	ErrMySQL      ErrType = "MYSQL_ERROR"
+	ErrDynamoDB   ErrType = "DYNAMODB_ERROR"
+	ErrDisk       ErrType = "DISK_ERROR"
+)
+
+type ProcessingError struct {
+	Type    ErrType
+	Message string
+	Wrapped error
+}
+
+func (e *ProcessingError) Error() string {
+	if e.Wrapped != nil {
+		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.Wrapped)
+	}
+	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
+}
+
+func (e *ProcessingError) Unwrap() error {
+	return e.Wrapped
+}
diff --git a/internal/domain/product.go b/internal/domain/product.go
new file mode 100644
index 0000000..97b940b
--- /dev/null
+++ b/internal/domain/product.go
@@ -0,0 +1,28 @@
+package domain
+
+type FileType string
+
+const (
+	FileMultiband  FileType = "multiband"
+	FileNatural    FileType = "natural"
+	FileFalseColor FileType = "false_color"
+	FileNDVI       FileType = "ndvi"
+	FileNDRE       FileType = "ndre"
+	FileEVI        FileType = "evi"
+	FileGNDVI      FileType = "gndvi"
+	FileNBR        FileType = "nbr"
+	FileNDMI       FileType = "ndmi"
+	FileSAVI       FileType = "savi"
+	FileRedEdge    FileType = "red_edge"
+	FileSWIR       FileType = "swir"
+	FileParams     FileType = "params"
+	FileAnalisis   FileType = "analisis"
+)
+
+func AllImageTypes() []FileType {
+	return []FileType{
+		FileNatural, FileFalseColor, FileNDVI, FileNDRE,
+		FileEVI, FileGNDVI, FileNBR, FileNDMI,
+		FileSAVI, FileRedEdge, FileSWIR,
+	}
+}
diff --git a/internal/domain/production.go b/internal/domain/production.go
new file mode 100644
index 0000000..bc29df3
--- /dev/null
+++ b/internal/domain/production.go
@@ -0,0 +1,51 @@
+package domain
+
+import (
+	"errors"
+	"time"
+)
+
+type BBox struct {
+	MinX float64
+	MinY float64
+	MaxX float64
+	MaxY float64
+}
+
+func (b BBox) Validate() error {
+	if b.MinX >= b.MaxX {
+		return errors.New("bbox: MinX must be less than MaxX")
+	}
+	if b.MinY >= b.MaxY {
+		return errors.New("bbox: MinY must be less than MaxY")
+	}
+	return nil
+}
+
+type Production struct {
+	ID                  int64
+	ProduccionID        int64
+	Cultivo             string
+	Ciclo               string
+	BBox                *BBox
+	Monitoring          bool
+	MonitoringMotivo    string
+	Bloqueado           bool
+	BloqueadoMotivo     string
+	BloqueadoAt         *time.Time
+	DesbloqueadoPor     string
+	TargetResolution    int
+	CloudCoverMax       float64
+	FechaPlantacion     *time.Time
+	DiasProduccion      int
+	FechaFinMonitoreo   *time.Time
+	TotalEscenas        int
+	TotalEscenasValidas int
+	LastSyncAt          *time.Time
+	CreatedAt           time.Time
+	UpdatedAt           time.Time
+}
+
+func (p *Production) ShouldProcess() bool {
+	return p.Monitoring && !p.Bloqueado && p.BBox != nil
+}
diff --git a/internal/domain/production_test.go b/internal/domain/production_test.go
new file mode 100644
index 0000000..758e02d
--- /dev/null
+++ b/internal/domain/production_test.go
@@ -0,0 +1,52 @@
+package domain
+
+import "testing"
+
+func TestBBoxValidate(t *testing.T) {
+	tests := []struct {
+		name    string
+		bbox    BBox
+		wantErr bool
+	}{
+		{
+			name:    "valid bbox",
+			bbox:    BBox{MinX: -102.35, MinY: 21.80, MaxX: -102.30, MaxY: 21.85},
+			wantErr: false,
+		},
+		{
+			name:    "minx >= maxx",
+			bbox:    BBox{MinX: -102.30, MinY: 21.80, MaxX: -102.35, MaxY: 21.85},
+			wantErr: true,
+		},
+		{
+			name:    "miny >= maxy",
+			bbox:    BBox{MinX: -102.35, MinY: 21.85, MaxX: -102.30, MaxY: 21.80},
+			wantErr: true,
+		},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			err := tt.bbox.Validate()
+			if (err != nil) != tt.wantErr {
+				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
+			}
+		})
+	}
+}
+
+func TestProductionShouldMonitor(t *testing.T) {
+	p := Production{
+		Monitoring: true,
+		Bloqueado:  false,
+		BBox:       &BBox{MinX: -102.35, MinY: 21.80, MaxX: -102.30, MaxY: 21.85},
+	}
+	if !p.ShouldProcess() {
+		t.Error("production with monitoring=true, not blocked, with bbox should be processable")
+	}
+
+	p.Bloqueado = true
+	if p.ShouldProcess() {
+		t.Error("blocked production should not be processable")
+	}
+}
diff --git a/internal/domain/scene.go b/internal/domain/scene.go
new file mode 100644
index 0000000..8f289ab
--- /dev/null
+++ b/internal/domain/scene.go
@@ -0,0 +1,55 @@
+package domain
+
+import "time"
+
+type JobStatus string
+
+const (
+	StatusPending    JobStatus = "PENDING"
+	StatusProcessing JobStatus = "PROCESSING"
+	StatusCompleted  JobStatus = "COMPLETED"
+	StatusFailed     JobStatus = "FAILED"
+)
+
+type Scene struct {
+	ID              int64
+	ProduccionID    int64
+	SceneID         string
+	SceneDate       time.Time
+	CloudCoverScene float64
+	CloudCoverBBox  *float64
+	PassesQuality   bool
+	HasMultiband    bool
+	HasParams       bool
+	HasRGB          bool
+	HasAnalisis     bool
+	Status          JobStatus
+	ErrorType       string
+	ErrorMessage    string
+	RetryCount      int
+	ProcessedAt     *time.Time
+	CreatedAt       time.Time
+	UpdatedAt       time.Time
+}
+
+func (s *Scene) NeedsProcessing() bool {
+	return s.Status == StatusPending
+}
+
+func (s *Scene) CanRetry(maxRetries int) bool {
+	return s.Status == StatusFailed && s.RetryCount < maxRetries
+}
+
+type SceneFile struct {
+	ID            int64
+	EscenaID      int64
+	FileType      FileType
+	FileName      string
+	S3Key         string
+	S3Bucket      string
+	FileSizeBytes int64
+	ResolutionM   int
+	WidthPx       int
+	HeightPx      int
+	CreatedAt     time.Time
+}
diff --git a/internal/domain/scene_test.go b/internal/domain/scene_test.go
new file mode 100644
index 0000000..3eefbb9
--- /dev/null
+++ b/internal/domain/scene_test.go
@@ -0,0 +1,26 @@
+package domain
+
+import "testing"
+
+func TestSceneNeedsProcessing(t *testing.T) {
+	s := Scene{Status: StatusPending}
+	if !s.NeedsProcessing() {
+		t.Error("PENDING scene should need processing")
+	}
+
+	s.Status = StatusCompleted
+	if s.NeedsProcessing() {
+		t.Error("COMPLETED scene should not need processing")
+	}
+
+	s.Status = StatusFailed
+	s.RetryCount = 2
+	if !s.CanRetry(3) {
+		t.Error("FAILED scene with retry_count=2 and max=3 should be retryable")
+	}
+
+	s.RetryCount = 3
+	if s.CanRetry(3) {
+		t.Error("FAILED scene with retry_count=3 and max=3 should not be retryable")
+	}
+}
