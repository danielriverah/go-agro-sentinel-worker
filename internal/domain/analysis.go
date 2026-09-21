package domain

import (
	"errors"
	"time"
)

type AnalysisResult struct {
	EstadoGeneral   string   `json:"estado_general"`
	VigorVegetativo string   `json:"vigor_vegetativo"`
	EstresDetectado bool     `json:"estres_detectado"`
	PosibleCosecha  bool     `json:"posible_cosecha"`
	Recomendaciones []string `json:"recomendaciones"`
	Alertas         []string `json:"alertas"`
	Confianza       float64  `json:"confianza"`
}

// IAResultSummary maps to s3_monitoring_escena_ia_resumen.
type IAResultSummary struct {
	ID                   uint64     `json:"id"`
	S3MonitoringEscenaID uint64     `json:"escena_id"`
	EstadoClave          string     `json:"estado_clave"`
	EstadoGeneral        string     `json:"estado_general"`
	RiesgoNivel          string     `json:"riesgo_nivel"`
	RiesgoMotivo         string     `json:"riesgo_motivo"`
	FechaAnalisis        *time.Time `json:"fecha_analisis"`
	JSONOriginal         string     `json:"json_original"`
	FechaCreacion        time.Time  `json:"fecha_creacion"`
	FechaActualizacion   *time.Time `json:"fecha_actualizacion,omitempty"`
}

// validEstadoClaves lista los valores permitidos para EstadoClave.
var validEstadoClaves = map[string]bool{
	"normal": true, "anomalia": true, "sin_datos": true,
	"alerta": true, "crítico": true,
}

// Validate verifica que los campos críticos del IAResultSummary sean válidos.
func (i *IAResultSummary) Validate() error {
	if i.ID == 0 {
		return errors.New("IAResultSummary: ID debe ser mayor a 0")
	}
	if i.S3MonitoringEscenaID == 0 {
		return errors.New("IAResultSummary: S3MonitoringEscenaID debe ser mayor a 0")
	}
	if i.EstadoClave != "" && !validEstadoClaves[i.EstadoClave] {
		return errors.New("IAResultSummary: EstadoClave inválido: " + i.EstadoClave)
	}
	return nil
}
