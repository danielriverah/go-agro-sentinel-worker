package domain

import (
	"errors"
	"fmt"
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

// IAResultSummary contiene el resumen de los análisis de inteligencia artificial
// para una escena de monitoreo, incluyendo estado de salud de cultivo y riesgos detectados
type IAResultSummary struct {
	ID                   int64      // Identificador único del análisis
	S3MonitoringEscenaID int64      // FK a s3_monitoring_escenas - Referencia a la escena monitorizada
	EstadoClave          string     // Estado clave: crítico|alerta|normal - Resumen del estado general
	EstadoGeneral        string     // Estado general: descripción detallada del estado del cultivo
	RiesgoNivel          string     // Nivel de riesgo: alto|medio|bajo - Clasificación de severidad
	RiesgoMotivo         string     // Motivo del riesgo: explicación de qué genera el riesgo detectado
	FechaAnalisis        *time.Time // Fecha y hora en que se realizó el análisis por IA
	JSONOriginal         string     // JSON original de la respuesta del servicio IA para auditoría
	CreatedAt            time.Time  // Timestamp de creación del registro
	UpdatedAt            time.Time  // Timestamp de última actualización
}

// Validate verifica que los campos críticos del IAResultSummary sean válidos
func (i *IAResultSummary) Validate() error {
	if i.ID <= 0 {
		return errors.New("IAResultSummary: ID debe ser mayor a 0")
	}
	if i.S3MonitoringEscenaID <= 0 {
		return errors.New("IAResultSummary: S3MonitoringEscenaID debe ser mayor a 0")
	}
	if i.EstadoClave == "" {
		return errors.New("IAResultSummary: EstadoClave es requerido")
	}
	// Validar valores permitidos para EstadoClave
	validEstadoClave := map[string]bool{"crítico": true, "alerta": true, "normal": true}
	if !validEstadoClave[i.EstadoClave] {
		return fmt.Errorf("IAResultSummary: EstadoClave inválido '%s', debe ser: crítico|alerta|normal", i.EstadoClave)
	}
	if i.RiesgoNivel == "" {
		return errors.New("IAResultSummary: RiesgoNivel es requerido")
	}
	// Validar valores permitidos para RiesgoNivel
	validRiesgoNivel := map[string]bool{"alto": true, "medio": true, "bajo": true}
	if !validRiesgoNivel[i.RiesgoNivel] {
		return fmt.Errorf("IAResultSummary: RiesgoNivel inválido '%s', debe ser: alto|medio|bajo", i.RiesgoNivel)
	}
	return nil
}

// String retorna una representación legible del IAResultSummary
func (i *IAResultSummary) String() string {
	fechaAnalisis := "no disponible"
	if i.FechaAnalisis != nil {
		fechaAnalisis = i.FechaAnalisis.Format("2006-01-02 15:04:05")
	}
	return fmt.Sprintf("IAResultSummary{ID: %d, EscenaID: %d, Estado: %s, Riesgo: %s (%s), Fecha: %s}",
		i.ID, i.S3MonitoringEscenaID, i.EstadoClave, i.RiesgoNivel, i.RiesgoMotivo, fechaAnalisis)
}
