package domain

import (
	"strings"
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

func TestProductionShouldMonitor(t *testing.T) {
	p := Production{
		Monitoring: true,
		Bloqueado:  false,
		BBox:       &BBox{MinX: -102.35, MinY: 21.80, MaxX: -102.30, MaxY: 21.85},
	}
	if !p.ShouldProcess() {
		t.Error("production with monitoring=true, not blocked, with bbox should be processable")
	}

	p.Bloqueado = true
	if p.ShouldProcess() {
		t.Error("blocked production should not be processable")
	}
}

func TestProductionValidate(t *testing.T) {
	tests := []struct {
		name    string
		prod    Production
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid production",
			prod: Production{
				ProduccionID: 1,
				Cultivo:      "Maiz",
				Ciclo:        "2026-A",
				BBox:         &BBox{MinX: -102.35, MinY: 21.80, MaxX: -102.30, MaxY: 21.85},
			},
			wantErr: false,
		},
		{
			name: "valid production with articulo and centro",
			prod: Production{
				ProduccionID:  1,
				ArticuloID:    100,
				CentroCostoID: 200,
				Cultivo:       "Soja",
				Ciclo:         "2026-B",
			},
			wantErr: false,
		},
		{
			name: "invalid: ProduccionID is 0",
			prod: Production{
				ProduccionID: 0,
				Cultivo:      "Maiz",
				Ciclo:        "2026-A",
			},
			wantErr: true,
			errMsg:  "ProduccionID must be greater than 0",
		},
		{
			name: "invalid: Cultivo is empty",
			prod: Production{
				ProduccionID: 1,
				Cultivo:      "",
				Ciclo:        "2026-A",
			},
			wantErr: true,
			errMsg:  "Cultivo is required",
		},
		{
			name: "invalid: Ciclo is empty",
			prod: Production{
				ProduccionID: 1,
				Cultivo:      "Maiz",
				Ciclo:        "",
			},
			wantErr: true,
			errMsg:  "Ciclo is required",
		},
		{
			name: "invalid: BBox is invalid",
			prod: Production{
				ProduccionID: 1,
				Cultivo:      "Maiz",
				Ciclo:        "2026-A",
				BBox:         &BBox{MinX: -102.30, MinY: 21.80, MaxX: -102.35, MaxY: 21.85},
			},
			wantErr: true,
			errMsg:  "MinX must be less than MaxX",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.prod.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error message = %q, want to contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}
