package processing

import (
	"context"
	"fmt"
	"testing"

	"agro-sentinel-worker/internal/domain"
)

// statsMockExecutor returns a fixed gdalinfo -json -stats -hist response
// for every call, regardless of the input path, so it can be reused for
// both multiband band statistics and single-band index statistics.
type statsMockExecutor struct {
	stdout string
	err    error
	calls  int
}

func (m *statsMockExecutor) Run(ctx context.Context, command string, args []string) (string, string, error) {
	m.calls++
	if m.err != nil {
		return "", "mock error", m.err
	}
	return m.stdout, "", nil
}

func tenBandStatsJSON() string {
	const bandTmpl = `{"band":%d,"minimum":0.0,"maximum":1.0,"mean":0.5,"stdDev":0.1,"histogram":{"count":10,"min":0.0,"max":1.0,"buckets":[1,1,1,1,1,1,1,1,1,1]}}`
	out := `{"bands":[`
	for i := 1; i <= 10; i++ {
		if i > 1 {
			out += ","
		}
		out += fmt.Sprintf(bandTmpl, i)
	}
	out += `]}`
	return out
}

func TestCalculateBandStatistics(t *testing.T) {
	mock := &statsMockExecutor{stdout: tenBandStatsJSON()}

	stats, err := CalculateBandStatistics(context.Background(), mock, "/tmp/multiband.tif")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(stats) != 10 {
		t.Fatalf("got %d bands, want 10", len(stats))
	}

	ndviBand := stats[domain.BandB08]
	if ndviBand.Mean != 0.5 {
		t.Errorf("B08 mean = %v, want 0.5", ndviBand.Mean)
	}
	if ndviBand.Std != 0.1 {
		t.Errorf("B08 std = %v, want 0.1", ndviBand.Std)
	}
	if ndviBand.Min != 0.0 || ndviBand.Max != 1.0 {
		t.Errorf("B08 min/max = %v/%v, want 0.0/1.0", ndviBand.Min, ndviBand.Max)
	}
	if ndviBand.P50 < 0.0 || ndviBand.P50 > 1.0 {
		t.Errorf("B08 p50 = %v, out of range", ndviBand.P50)
	}

	if mock.calls != 1 {
		t.Errorf("expected 1 gdalinfo call, got %d", mock.calls)
	}
}

func TestCalculateBandStatistics_ParseError(t *testing.T) {
	mock := &statsMockExecutor{stdout: "not json"}

	_, err := CalculateBandStatistics(context.Background(), mock, "/tmp/multiband.tif")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	var procErr *domain.ProcessingError
	if !isProcessingError(err, &procErr) {
		t.Fatalf("expected *domain.ProcessingError, got %T", err)
	}
	if procErr.Type != domain.ErrGDAL {
		t.Errorf("Type = %v, want %v", procErr.Type, domain.ErrGDAL)
	}
}

func TestCalculateBandStatistics_NoBands(t *testing.T) {
	mock := &statsMockExecutor{stdout: `{"bands":[]}`}

	_, err := CalculateBandStatistics(context.Background(), mock, "/tmp/multiband.tif")
	if err == nil {
		t.Fatal("expected error for empty bands, got nil")
	}
}

func TestCalculateIndexStatistics(t *testing.T) {
	singleBand := `{"bands":[{"band":1,"minimum":-1.0,"maximum":1.0,"mean":0.72,"stdDev":0.08,"histogram":{"count":8,"min":-1.0,"max":1.0,"buckets":[0,0,0,1,5,1,1,0]}}]}`
	mock := &statsMockExecutor{stdout: singleBand}

	indices := []IndexDefinition{
		{Type: domain.FileNDVI, Name: "NDVI", Bands: []domain.Band{domain.BandB08, domain.BandB04}},
	}

	stats, err := CalculateIndexStatistics(context.Background(), mock, "/tmp/multiband.tif", indices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ndvi, ok := stats[domain.FileNDVI]
	if !ok {
		t.Fatal("expected NDVI stats present")
	}
	if ndvi.Mean != 0.72 {
		t.Errorf("NDVI mean = %v, want 0.72", ndvi.Mean)
	}
	if mock.calls != 1 {
		t.Errorf("expected 1 gdalinfo call, got %d", mock.calls)
	}
}

func TestPercentilesFromHistogram_EmptyFallsBackToMean(t *testing.T) {
	p25, p50, p75 := percentilesFromHistogram(0, 0, nil, 0.42)
	if p25 != 0.42 || p50 != 0.42 || p75 != 0.42 {
		t.Errorf("percentiles = %v/%v/%v, want all 0.42", p25, p50, p75)
	}
}
