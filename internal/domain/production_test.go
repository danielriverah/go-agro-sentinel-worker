package domain

import (
	"encoding/json"
	"testing"
)

func TestBBoxValidate(t *testing.T) {
	tests := []struct {
		name    string
		bbox    BBox
		wantErr bool
	}{
		{
			name:    "valid bbox",
			bbox:    BBox{MinX: -102.35, MinY: 21.80, MaxX: -102.30, MaxY: 21.85},
			wantErr: false,
		},
		{
			name:    "minx >= maxx",
			bbox:    BBox{MinX: -102.30, MinY: 21.80, MaxX: -102.35, MaxY: 21.85},
			wantErr: true,
		},
		{
			name:    "miny >= maxy",
			bbox:    BBox{MinX: -102.35, MinY: 21.85, MaxX: -102.30, MaxY: 21.80},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.bbox.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProductionShouldProcess(t *testing.T) {
	p := Production{Monitoring: true, Bloqueado: false}
	if !p.ShouldProcess() {
		t.Error("monitoring=true, bloqueado=false should be processable")
	}

	p.Bloqueado = true
	if p.ShouldProcess() {
		t.Error("bloqueado=true should not be processable")
	}

	p.Bloqueado = false
	p.Monitoring = false
	if p.ShouldProcess() {
		t.Error("monitoring=false should not be processable")
	}
}

func TestProductionParsePBox(t *testing.T) {
	pboxDoc := map[string]interface{}{
		"pbox": []float64{-102.35, 21.80, -102.30, 21.85},
	}
	raw, _ := json.Marshal(pboxDoc)

	p := Production{ProduccionID: 1, PBoxJSON: raw}
	bbox := p.ParsePBox()
	if bbox == nil {
		t.Fatal("ParsePBox() returned nil for valid pbox")
	}
	if bbox.MinX != -102.35 || bbox.MinY != 21.80 || bbox.MaxX != -102.30 || bbox.MaxY != 21.85 {
		t.Errorf("ParsePBox() = %+v, unexpected values", bbox)
	}
}

func TestProductionParsePBoxNil(t *testing.T) {
	p := Production{ProduccionID: 1}
	if p.ParsePBox() != nil {
		t.Error("ParsePBox() should return nil when PBoxJSON is empty")
	}
}

func TestProductionValidate(t *testing.T) {
	tests := []struct {
		name    string
		prod    Production
		wantErr bool
	}{
		{
			name:    "valid",
			prod:    Production{ProduccionID: 1},
			wantErr: false,
		},
		{
			name:    "zero ProduccionID",
			prod:    Production{ProduccionID: 0},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.prod.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
