package domain

import (
	"strings"
	"testing"
	"time"
)

func TestIAResultSummaryValidate(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name    string
		result  IAResultSummary
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid ia result",
			result: IAResultSummary{
				ID:                   1,
				S3MonitoringEscenaID: 100,
				EstadoClave:          "normal",
				RiesgoNivel:          "bajo",
				FechaAnalisis:        &now,
			},
			wantErr: false,
		},
		{
			name: "valid with alert state",
			result: IAResultSummary{
				ID:                   1,
				S3MonitoringEscenaID: 100,
				EstadoClave:          "alerta",
				EstadoGeneral:        "Estrés detectado",
				RiesgoNivel:          "medio",
				RiesgoMotivo:         "Sequía detectada",
			},
			wantErr: false,
		},
		{
			name: "valid with critical state",
			result: IAResultSummary{
				ID:                   1,
				S3MonitoringEscenaID: 100,
				EstadoClave:          "crítico",
				EstadoGeneral:        "Cultivo en riesgo severo",
				RiesgoNivel:          "alto",
				RiesgoMotivo:         "Plagas detectadas",
			},
			wantErr: false,
		},
		{
			name: "invalid: S3MonitoringEscenaID is 0",
			result: IAResultSummary{
				ID:                   1,
				S3MonitoringEscenaID: 0,
				EstadoClave:          "normal",
				RiesgoNivel:          "bajo",
			},
			wantErr: true,
			errMsg:  "S3MonitoringEscenaID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.result.Validate()
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

func TestIAResultSummaryFields(t *testing.T) {
	now := time.Now().UTC()
	r := IAResultSummary{
		ID:                   1,
		S3MonitoringEscenaID: 100,
		EstadoClave:          "normal",
		RiesgoNivel:          "bajo",
		RiesgoMotivo:         "test motivo",
		FechaAnalisis:        &now,
	}
	if r.ID != 1 {
		t.Errorf("ID = %d, want 1", r.ID)
	}
	if r.S3MonitoringEscenaID != 100 {
		t.Errorf("S3MonitoringEscenaID = %d, want 100", r.S3MonitoringEscenaID)
	}
	if r.EstadoClave != "normal" {
		t.Errorf("EstadoClave = %q, want %q", r.EstadoClave, "normal")
	}
	if r.RiesgoNivel != "bajo" {
		t.Errorf("RiesgoNivel = %q, want %q", r.RiesgoNivel, "bajo")
	}
	if r.FechaAnalisis == nil {
		t.Error("FechaAnalisis should not be nil")
	}

	r2 := IAResultSummary{
		ID:            2,
		EstadoClave:   "alerta",
		RiesgoNivel:   "medio",
		FechaAnalisis: nil,
	}
	if r2.FechaAnalisis != nil {
		t.Error("FechaAnalisis should be nil")
	}
}
