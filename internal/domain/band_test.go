package domain

import "testing"

func TestAllSpectralBands(t *testing.T) {
	bands := AllSpectralBands()
	if len(bands) != 10 {
		t.Errorf("AllSpectralBands() returned %d bands, want 10", len(bands))
	}
}

func TestBandsAtResolution(t *testing.T) {
	bands10m := BandsAtResolution(10)
	if len(bands10m) != 4 {
		t.Errorf("BandsAtResolution(10) = %d bands, want 4 (B02,B03,B04,B08)", len(bands10m))
	}

	bands20m := BandsAtResolution(20)
	if len(bands20m) != 6 {
		t.Errorf("BandsAtResolution(20) = %d bands, want 6 (B05,B06,B07,B8A,B11,B12)", len(bands20m))
	}
}

func TestBandResolution(t *testing.T) {
	if BandB04.Resolution() != 10 {
		t.Errorf("B04 resolution = %d, want 10", BandB04.Resolution())
	}
	if BandB05.Resolution() != 20 {
		t.Errorf("B05 resolution = %d, want 20", BandB05.Resolution())
	}
	if BandSCL.Resolution() != 20 {
		t.Errorf("SCL resolution = %d, want 20", BandSCL.Resolution())
	}
}
