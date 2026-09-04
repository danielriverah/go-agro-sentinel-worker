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
			name: "invalid: ID is 0",
			result: IAResultSummary{
				ID:                   0,
				S3MonitoringEscenaID: 100,
				EstadoClave:          "normal",
				RiesgoNivel:          "bajo",
			},
			wantErr: true,
			errMsg:  "ID debe ser mayor a 0",
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
			errMsg:  "S3MonitoringEscenaID debe ser mayor a 0",
		},
		{
			name: "invalid: EstadoClave is empty",
			result: IAResultSummary{
				ID:                   1,
				S3MonitoringEscenaID: 100,
				EstadoClave:          "",
				RiesgoNivel:          "bajo",
			},
			wantErr: true,
			errMsg:  "EstadoClave es requerido",
		},
		{
			name: "invalid: EstadoClave value",
			result: IAResultSummary{
				ID:                   1,
				S3MonitoringEscenaID: 100,
				EstadoClave:          "invalido",
				RiesgoNivel:          "bajo",
			},
			wantErr: true,
			errMsg:  "EstadoClave inválido",
		},
		{
			name: "invalid: RiesgoNivel is empty",
			result: IAResultSummary{
				ID:                   1,
				S3MonitoringEscenaID: 100,
				EstadoClave:          "normal",
				RiesgoNivel:          "",
			},
			wantErr: true,
			errMsg:  "RiesgoNivel es requerido",
		},
		{
			name: "invalid: RiesgoNivel value",
			result: IAResultSummary{
				ID:                   1,
				S3MonitoringEscenaID: 100,
				EstadoClave:          "normal",
				RiesgoNivel:          "extremo",
			},
			wantErr: true,
			errMsg:  "RiesgoNivel inválido",
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

func TestIAResultSummaryString(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name     string
		result   IAResultSummary
		contains []string
	}{
		{
			name: "string representation",
			result: IAResultSummary{
				ID:                   1,
				S3MonitoringEscenaID: 100,
				EstadoClave:          "normal",
				RiesgoNivel:          "bajo",
				RiesgoMotivo:         "test motivo",
				FechaAnalisis:        &now,
			},
			contains: []string{"1", "100", "normal", "bajo", "test motivo"},
		},
		{
			name: "string without fecha",
			result: IAResultSummary{
				ID:                   2,
				S3MonitoringEscenaID: 200,
				EstadoClave:          "alerta",
				RiesgoNivel:          "medio",
				RiesgoMotivo:         "motivo prueba",
				FechaAnalisis:        nil,
			},
			contains: []string{"2", "200", "alerta", "medio", "no disponible"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			str := tt.result.String()
			for _, substring := range tt.contains {
				if !strings.Contains(str, substring) {
					t.Errorf("String() = %q, should contain %q", str, substring)
				}
			}
		})
	}
}
