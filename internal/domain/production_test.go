package domain

import "testing"

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
