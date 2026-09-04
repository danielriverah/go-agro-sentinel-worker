diff --git a/internal/processing/params.go b/internal/processing/params.go
new file mode 100644
index 0000000..4ba9ceb
--- /dev/null
+++ b/internal/processing/params.go
@@ -0,0 +1,165 @@
+package processing
+
+import (
+	"time"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// dateLayout is the date-only format used for scene dates in params.json,
+// matching the spec's params.json schema.
+const dateLayout = "2006-01-02"
+
+// maxHistoricoEntries caps the historical chain carried in params.json.
+const maxHistoricoEntries = 20
+
+// ParamsInput carries the current scene's computed statistics and metadata
+// needed to build a new Params document.
+type ParamsInput struct {
+	ProduccionID     int64
+	SceneID          string
+	SceneDate        time.Time
+	FechaPlantacion  time.Time
+	CloudCoverBBox   float64
+	Indices          map[domain.FileType]IndexStats
+	BandStats        map[domain.Band]BandStats
+	Coverage         CoverageStats
+}
+
+// HistoricoEntry summarizes one previous scene within the historical chain,
+// along with the deltas from that scene to the one immediately after it.
+type HistoricoEntry struct {
+	SceneID   string             `json:"scene_id"`
+	SceneDate string             `json:"scene_date"`
+	Indices   map[string]IndexStats `json:"indices,omitempty"`
+	Delta     map[string]float64 `json:"delta,omitempty"`
+}
+
+// Params is the JSON-serializable structure written to params.json,
+// matching the spec's schema.
+type Params struct {
+	ProduccionID        int64                  `json:"produccion_id"`
+	SceneID             string                 `json:"scene_id"`
+	SceneDate           string                 `json:"scene_date"`
+	DiasDesdePlantacion int                    `json:"dias_desde_plantacion"`
+	CloudCoverBBox      float64                `json:"cloud_cover_bbox"`
+	Coverage            CoverageStats          `json:"coverage"`
+	Indices             map[string]IndexStats  `json:"indices"`
+	BandStats           map[string]BandStats   `json:"band_stats,omitempty"`
+	Historico           []HistoricoEntry       `json:"historico,omitempty"`
+}
+
+// fileTypeIndexKeys maps domain.FileType index identifiers to the lowercase
+// string keys used in params.json.
+func indexKey(t domain.FileType) string {
+	return string(t)
+}
+
+func bandKey(b domain.Band) string {
+	return string(b)
+}
+
+// BuildParams constructs the new Params document for the current scene,
+// prepending a summary of the previous scene (if any) to the historical
+// chain, with per-index mean deltas from the previous scene to the current
+// one. The historical chain is capped at maxHistoricoEntries.
+func BuildParams(current ParamsInput, previousParams *Params) *Params {
+	indices := make(map[string]IndexStats, len(current.Indices))
+	for t, stats := range current.Indices {
+		indices[indexKey(t)] = stats
+	}
+
+	var bandStats map[string]BandStats
+	if len(current.BandStats) > 0 {
+		bandStats = make(map[string]BandStats, len(current.BandStats))
+		for b, stats := range current.BandStats {
+			bandStats[bandKey(b)] = stats
+		}
+	}
+
+	params := &Params{
+		ProduccionID:        current.ProduccionID,
+		SceneID:             current.SceneID,
+		SceneDate:           current.SceneDate.Format(dateLayout),
+		DiasDesdePlantacion: daysBetween(current.FechaPlantacion, current.SceneDate),
+		CloudCoverBBox:      current.CloudCoverBBox,
+		Coverage:            current.Coverage,
+		Indices:             indices,
+		BandStats:           bandStats,
+	}
+
+	if previousParams == nil {
+		return params
+	}
+
+	historico := make([]HistoricoEntry, 0, len(previousParams.Historico)+1)
+
+	prevEntry := HistoricoEntry{
+		SceneID:   previousParams.SceneID,
+		SceneDate: previousParams.SceneDate,
+		Indices:   previousParams.Indices,
+		Delta:     computeDeltas(previousParams.Indices, indices),
+	}
+	historico = append(historico, prevEntry)
+	historico = append(historico, previousParams.Historico...)
+
+	params.Historico = TrimHistorico(historico, maxHistoricoEntries)
+
+	return params
+}
+
+// computeDeltas returns a map of "<index>_mean_change" -> current mean -
+// previous mean, for every index present in both the previous and current
+// index maps.
+func computeDeltas(previous, current map[string]IndexStats) map[string]float64 {
+	if len(previous) == 0 || len(current) == 0 {
+		return nil
+	}
+
+	deltas := make(map[string]float64)
+	for name, prevStats := range previous {
+		curStats, ok := current[name]
+		if !ok {
+			continue
+		}
+		deltas[name+"_mean_change"] = roundTo(curStats.Mean-prevStats.Mean, 6)
+	}
+	if len(deltas) == 0 {
+		return nil
+	}
+	return deltas
+}
+
+// roundTo rounds v to the given number of decimal places, avoiding
+// floating-point noise in delta comparisons (e.g. 0.72-0.60 == 0.12).
+func roundTo(v float64, decimals int) float64 {
+	mult := 1.0
+	for i := 0; i < decimals; i++ {
+		mult *= 10
+	}
+	return float64(int64(v*mult+sign(v)*0.5)) / mult
+}
+
+func sign(v float64) float64 {
+	if v < 0 {
+		return -1
+	}
+	return 1
+}
+
+// daysBetween returns the number of whole days between fechaPlantacion and
+// sceneDate. Negative if sceneDate precedes fechaPlantacion.
+func daysBetween(fechaPlantacion, sceneDate time.Time) int {
+	d := sceneDate.Sub(fechaPlantacion)
+	return int(d.Hours() / 24)
+}
+
+// TrimHistorico keeps at most maxEntries entries from historico, preferring
+// the entries at the front of the slice (the most recent scenes, since
+// BuildParams always prepends).
+func TrimHistorico(historico []HistoricoEntry, maxEntries int) []HistoricoEntry {
+	if len(historico) <= maxEntries {
+		return historico
+	}
+	return historico[:maxEntries]
+}
diff --git a/internal/processing/params_test.go b/internal/processing/params_test.go
new file mode 100644
index 0000000..667581c
--- /dev/null
+++ b/internal/processing/params_test.go
@@ -0,0 +1,136 @@
+package processing
+
+import (
+	"fmt"
+	"testing"
+	"time"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+func TestBuildParams(t *testing.T) {
+	current := ParamsInput{
+		ProduccionID:    1234,
+		SceneID:         "S2A_scene2",
+		SceneDate:       time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
+		FechaPlantacion: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
+		CloudCoverBBox:  12.5,
+		Indices: map[domain.FileType]IndexStats{
+			domain.FileNDVI: {Mean: 0.72, Std: 0.08},
+		},
+		Coverage: CoverageStats{VegetationPct: 78.5, SoilPct: 15.2},
+	}
+
+	previous := &Params{
+		ProduccionID:        1234,
+		SceneID:             "S2A_scene1",
+		SceneDate:           "2026-08-22",
+		DiasDesdePlantacion: 38,
+		Indices: map[string]IndexStats{
+			"ndvi": {Mean: 0.60},
+		},
+		Historico: nil, // first scene had no history
+	}
+
+	params := BuildParams(current, previous)
+
+	if params.DiasDesdePlantacion != 48 {
+		t.Errorf("dias = %d, want 48", params.DiasDesdePlantacion)
+	}
+	if len(params.Historico) != 1 {
+		t.Fatalf("historico length = %d, want 1", len(params.Historico))
+	}
+	if params.Historico[0].SceneID != "S2A_scene1" {
+		t.Error("historico[0] should be previous scene")
+	}
+	if params.Historico[0].Delta["ndvi_mean_change"] != 0.12 {
+		t.Errorf("ndvi delta = %v, want 0.12", params.Historico[0].Delta["ndvi_mean_change"])
+	}
+}
+
+func TestBuildParams_NoPrevious(t *testing.T) {
+	current := ParamsInput{
+		ProduccionID:    1,
+		SceneID:         "S2A_scene1",
+		SceneDate:       time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC),
+		FechaPlantacion: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
+		Indices: map[domain.FileType]IndexStats{
+			domain.FileNDVI: {Mean: 0.60},
+		},
+	}
+
+	params := BuildParams(current, nil)
+
+	if params.DiasDesdePlantacion != 38 {
+		t.Errorf("dias = %d, want 38", params.DiasDesdePlantacion)
+	}
+	if len(params.Historico) != 0 {
+		t.Errorf("historico length = %d, want 0", len(params.Historico))
+	}
+	if params.Indices["ndvi"].Mean != 0.60 {
+		t.Errorf("ndvi mean = %v, want 0.60", params.Indices["ndvi"].Mean)
+	}
+}
+
+func TestBuildParams_ChainsPreviousHistorico(t *testing.T) {
+	current := ParamsInput{
+		SceneID:   "scene3",
+		SceneDate: time.Now(),
+		Indices: map[domain.FileType]IndexStats{
+			domain.FileNDVI: {Mean: 0.5},
+		},
+	}
+	previous := &Params{
+		SceneID:   "scene2",
+		SceneDate: "2026-08-22",
+		Indices: map[string]IndexStats{
+			"ndvi": {Mean: 0.4},
+		},
+		Historico: []HistoricoEntry{
+			{SceneID: "scene1"},
+		},
+	}
+
+	params := BuildParams(current, previous)
+
+	if len(params.Historico) != 2 {
+		t.Fatalf("historico length = %d, want 2", len(params.Historico))
+	}
+	if params.Historico[0].SceneID != "scene2" {
+		t.Errorf("historico[0] = %s, want scene2", params.Historico[0].SceneID)
+	}
+	if params.Historico[1].SceneID != "scene1" {
+		t.Errorf("historico[1] = %s, want scene1", params.Historico[1].SceneID)
+	}
+}
+
+func TestTrimHistorico(t *testing.T) {
+	entries := make([]HistoricoEntry, 25)
+	for i := range entries {
+		entries[i] = HistoricoEntry{SceneID: fmt.Sprintf("scene_%d", i)}
+	}
+
+	trimmed := TrimHistorico(entries, 20)
+	if len(trimmed) != 20 {
+		t.Errorf("trimmed length = %d, want 20", len(trimmed))
+	}
+	if trimmed[0].SceneID != "scene_0" {
+		t.Errorf("trimmed[0] = %s, want scene_0", trimmed[0].SceneID)
+	}
+}
+
+func TestTrimHistorico_UnderLimit(t *testing.T) {
+	entries := []HistoricoEntry{{SceneID: "a"}, {SceneID: "b"}}
+	trimmed := TrimHistorico(entries, 20)
+	if len(trimmed) != 2 {
+		t.Errorf("trimmed length = %d, want 2", len(trimmed))
+	}
+}
+
+func TestDaysBetween(t *testing.T) {
+	fecha := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
+	scene := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
+	if got := daysBetween(fecha, scene); got != 48 {
+		t.Errorf("daysBetween = %d, want 48", got)
+	}
+}
diff --git a/internal/processing/statistics.go b/internal/processing/statistics.go
new file mode 100644
index 0000000..52aaebe
--- /dev/null
+++ b/internal/processing/statistics.go
@@ -0,0 +1,188 @@
+package processing
+
+import (
+	"context"
+	"encoding/json"
+	"path/filepath"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// BandStats holds descriptive statistics for a single spectral band or
+// index raster, as computed from gdalinfo -stats/-hist output.
+type BandStats struct {
+	Mean float64 `json:"mean"`
+	Std  float64 `json:"std"`
+	Min  float64 `json:"min"`
+	Max  float64 `json:"max"`
+	P25  float64 `json:"p25"`
+	P50  float64 `json:"p50"`
+	P75  float64 `json:"p75"`
+}
+
+// IndexStats holds descriptive statistics for a computed vegetation/moisture
+// index raster. Structurally identical to BandStats.
+type IndexStats struct {
+	Mean float64 `json:"mean"`
+	Std  float64 `json:"std"`
+	Min  float64 `json:"min"`
+	Max  float64 `json:"max"`
+	P25  float64 `json:"p25"`
+	P50  float64 `json:"p50"`
+	P75  float64 `json:"p75"`
+}
+
+// gdalInfoStatsJSON mirrors the subset of `gdalinfo -json -stats -hist`
+// output this package needs: per-band statistics and histogram buckets used
+// to approximate percentiles.
+type gdalInfoStatsJSON struct {
+	Bands []struct {
+		Band      int `json:"band"`
+		Metadata  map[string]json.RawMessage `json:"metadata"`
+		Statistics struct {
+			Minimum   float64 `json:"minimum"`
+			Maximum   float64 `json:"maximum"`
+			Mean      float64 `json:"mean"`
+			StdDev    float64 `json:"stdDev"`
+		} `json:"stats"`
+		Histogram struct {
+			Count   int       `json:"count"`
+			Min     float64   `json:"min"`
+			Max     float64   `json:"max"`
+			Buckets []int64   `json:"buckets"`
+		} `json:"histogram"`
+	} `json:"bands"`
+}
+
+// CalculateBandStatistics runs `gdalinfo -json -stats -hist` on
+// multibandPath and returns per-band descriptive statistics keyed by
+// domain.Band, using the fixed band order documented in multiband.go
+// (B02=1, B03=2, B04=3, B05=4, B06=5, B07=6, B08=7, B8A=8, B11=9, B12=10).
+func CalculateBandStatistics(ctx context.Context, executor GDALExecutor, multibandPath string) (map[domain.Band]BandStats, error) {
+	stdout, stderr, err := executor.Run(ctx, "gdalinfo", []string{"-json", "-stats", "-hist", multibandPath})
+	if err != nil {
+		return nil, err
+	}
+
+	var parsed gdalInfoStatsJSON
+	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
+		return nil, &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "failed to parse gdalinfo stats output: " + stderr,
+			Wrapped: err,
+		}
+	}
+
+	if len(parsed.Bands) == 0 {
+		return nil, &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "gdalinfo output did not contain any bands: " + stderr,
+		}
+	}
+
+	bands := domain.AllSpectralBands()
+	result := make(map[domain.Band]BandStats, len(bands))
+
+	for i, band := range bands {
+		if i >= len(parsed.Bands) {
+			break
+		}
+		b := parsed.Bands[i]
+		stats := BandStats{
+			Mean: b.Statistics.Mean,
+			Std:  b.Statistics.StdDev,
+			Min:  b.Statistics.Minimum,
+			Max:  b.Statistics.Maximum,
+		}
+		p25, p50, p75 := percentilesFromHistogram(b.Histogram.Min, b.Histogram.Max, b.Histogram.Buckets, stats.Mean)
+		stats.P25, stats.P50, stats.P75 = p25, p50, p75
+		result[band] = stats
+	}
+
+	return result, nil
+}
+
+// CalculateIndexStatistics runs gdalinfo -json -stats -hist on the raw
+// (uncolored) raster for each index computed from multibandPath and returns
+// per-index descriptive statistics. It expects the raw index rasters to
+// already exist alongside multibandPath, named "<indexType>_raw.tif", as
+// produced by GenerateIndex.
+func CalculateIndexStatistics(ctx context.Context, executor GDALExecutor, multibandPath string, indices []IndexDefinition) (map[domain.FileType]IndexStats, error) {
+	result := make(map[domain.FileType]IndexStats, len(indices))
+
+	for _, def := range indices {
+		rawPath := indexRawPath(multibandPath, def.Type)
+		stdout, stderr, err := executor.Run(ctx, "gdalinfo", []string{"-json", "-stats", "-hist", rawPath})
+		if err != nil {
+			return nil, err
+		}
+
+		var parsed gdalInfoStatsJSON
+		if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
+			return nil, &domain.ProcessingError{
+				Type:    domain.ErrGDAL,
+				Message: "failed to parse gdalinfo stats output for " + def.Name + ": " + stderr,
+				Wrapped: err,
+			}
+		}
+
+		if len(parsed.Bands) == 0 {
+			return nil, &domain.ProcessingError{
+				Type:    domain.ErrGDAL,
+				Message: "gdalinfo output for " + def.Name + " did not contain any bands: " + stderr,
+			}
+		}
+
+		b := parsed.Bands[0]
+		stats := IndexStats{
+			Mean: b.Statistics.Mean,
+			Std:  b.Statistics.StdDev,
+			Min:  b.Statistics.Minimum,
+			Max:  b.Statistics.Maximum,
+		}
+		p25, p50, p75 := percentilesFromHistogram(b.Histogram.Min, b.Histogram.Max, b.Histogram.Buckets, stats.Mean)
+		stats.P25, stats.P50, stats.P75 = p25, p50, p75
+		result[def.Type] = stats
+	}
+
+	return result, nil
+}
+
+// indexRawPath derives the path to an index's raw (uncolored) raster from
+// the multiband.tif path, matching the naming used by GenerateIndex.
+func indexRawPath(multibandPath string, indexType domain.FileType) string {
+	dir := filepath.Dir(multibandPath)
+	return filepath.Join(dir, string(indexType)+"_raw.tif")
+}
+
+// percentilesFromHistogram approximates the 25th, 50th and 75th percentiles
+// from a gdalinfo histogram. If no histogram buckets are available, it
+// falls back to the mean for all three percentiles (a conservative
+// approximation preferable to failing statistics calculation entirely).
+func percentilesFromHistogram(min, max float64, buckets []int64, fallbackMean float64) (p25, p50, p75 float64) {
+	var total int64
+	for _, c := range buckets {
+		total += c
+	}
+	if total <= 0 || len(buckets) == 0 || max <= min {
+		return fallbackMean, fallbackMean, fallbackMean
+	}
+
+	bucketWidth := (max - min) / float64(len(buckets))
+
+	percentile := func(fraction float64) float64 {
+		target := fraction * float64(total)
+		var cumulative int64
+		for i, c := range buckets {
+			cumulative += c
+			if float64(cumulative) >= target {
+				// Value at the midpoint of the bucket containing the target.
+				return min + (float64(i)+0.5)*bucketWidth
+			}
+		}
+		return max
+	}
+
+	return percentile(0.25), percentile(0.50), percentile(0.75)
+}
+
diff --git a/internal/processing/statistics_test.go b/internal/processing/statistics_test.go
new file mode 100644
index 0000000..85d33a8
--- /dev/null
+++ b/internal/processing/statistics_test.go
@@ -0,0 +1,127 @@
+package processing
+
+import (
+	"context"
+	"fmt"
+	"testing"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// statsMockExecutor returns a fixed gdalinfo -json -stats -hist response
+// for every call, regardless of the input path, so it can be reused for
+// both multiband band statistics and single-band index statistics.
+type statsMockExecutor struct {
+	stdout string
+	err    error
+	calls  int
+}
+
+func (m *statsMockExecutor) Run(ctx context.Context, command string, args []string) (string, string, error) {
+	m.calls++
+	if m.err != nil {
+		return "", "mock error", m.err
+	}
+	return m.stdout, "", nil
+}
+
+func tenBandStatsJSON() string {
+	const bandTmpl = `{"band":%d,"stats":{"minimum":0.0,"maximum":1.0,"mean":0.5,"stdDev":0.1},"histogram":{"count":10,"min":0.0,"max":1.0,"buckets":[1,1,1,1,1,1,1,1,1,1]}}`
+	out := `{"bands":[`
+	for i := 1; i <= 10; i++ {
+		if i > 1 {
+			out += ","
+		}
+		out += fmt.Sprintf(bandTmpl, i)
+	}
+	out += `]}`
+	return out
+}
+
+func TestCalculateBandStatistics(t *testing.T) {
+	mock := &statsMockExecutor{stdout: tenBandStatsJSON()}
+
+	stats, err := CalculateBandStatistics(context.Background(), mock, "/tmp/multiband.tif")
+	if err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+
+	if len(stats) != 10 {
+		t.Fatalf("got %d bands, want 10", len(stats))
+	}
+
+	ndviBand := stats[domain.BandB08]
+	if ndviBand.Mean != 0.5 {
+		t.Errorf("B08 mean = %v, want 0.5", ndviBand.Mean)
+	}
+	if ndviBand.Std != 0.1 {
+		t.Errorf("B08 std = %v, want 0.1", ndviBand.Std)
+	}
+	if ndviBand.Min != 0.0 || ndviBand.Max != 1.0 {
+		t.Errorf("B08 min/max = %v/%v, want 0.0/1.0", ndviBand.Min, ndviBand.Max)
+	}
+	if ndviBand.P50 < 0.0 || ndviBand.P50 > 1.0 {
+		t.Errorf("B08 p50 = %v, out of range", ndviBand.P50)
+	}
+
+	if mock.calls != 1 {
+		t.Errorf("expected 1 gdalinfo call, got %d", mock.calls)
+	}
+}
+
+func TestCalculateBandStatistics_ParseError(t *testing.T) {
+	mock := &statsMockExecutor{stdout: "not json"}
+
+	_, err := CalculateBandStatistics(context.Background(), mock, "/tmp/multiband.tif")
+	if err == nil {
+		t.Fatal("expected error for invalid JSON, got nil")
+	}
+	var procErr *domain.ProcessingError
+	if !isProcessingError(err, &procErr) {
+		t.Fatalf("expected *domain.ProcessingError, got %T", err)
+	}
+	if procErr.Type != domain.ErrGDAL {
+		t.Errorf("Type = %v, want %v", procErr.Type, domain.ErrGDAL)
+	}
+}
+
+func TestCalculateBandStatistics_NoBands(t *testing.T) {
+	mock := &statsMockExecutor{stdout: `{"bands":[]}`}
+
+	_, err := CalculateBandStatistics(context.Background(), mock, "/tmp/multiband.tif")
+	if err == nil {
+		t.Fatal("expected error for empty bands, got nil")
+	}
+}
+
+func TestCalculateIndexStatistics(t *testing.T) {
+	singleBand := `{"bands":[{"band":1,"stats":{"minimum":-1.0,"maximum":1.0,"mean":0.72,"stdDev":0.08},"histogram":{"count":8,"min":-1.0,"max":1.0,"buckets":[0,0,0,1,5,1,1,0]}}]}`
+	mock := &statsMockExecutor{stdout: singleBand}
+
+	indices := []IndexDefinition{
+		{Type: domain.FileNDVI, Name: "NDVI", Bands: []domain.Band{domain.BandB08, domain.BandB04}},
+	}
+
+	stats, err := CalculateIndexStatistics(context.Background(), mock, "/tmp/multiband.tif", indices)
+	if err != nil {
+		t.Fatalf("unexpected error: %v", err)
+	}
+
+	ndvi, ok := stats[domain.FileNDVI]
+	if !ok {
+		t.Fatal("expected NDVI stats present")
+	}
+	if ndvi.Mean != 0.72 {
+		t.Errorf("NDVI mean = %v, want 0.72", ndvi.Mean)
+	}
+	if mock.calls != 1 {
+		t.Errorf("expected 1 gdalinfo call, got %d", mock.calls)
+	}
+}
+
+func TestPercentilesFromHistogram_EmptyFallsBackToMean(t *testing.T) {
+	p25, p50, p75 := percentilesFromHistogram(0, 0, nil, 0.42)
+	if p25 != 0.42 || p50 != 0.42 || p75 != 0.42 {
+		t.Errorf("percentiles = %v/%v/%v, want all 0.42", p25, p50, p75)
+	}
+}
