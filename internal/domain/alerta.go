package domain

import (
	"encoding/json"
	"time"
)

// Estados del ciclo de vida de una alerta.
const (
	AlertaNueva    = "nueva"
	AlertaVista    = "vista"
	AlertaResuelta = "resuelta"
)

// Severidades, de menor a mayor.
const (
	SeveridadBaja  = "baja"
	SeveridadMedia = "media"
	SeveridadAlta  = "alta"
)

// AlertType y Source conocidos.
const (
	AlertaTipoIA   = "ia_analysis"
	AlertaFuenteIA = "ia"
)

// Alerta mapea monitoring_alertas.
//
// Sustituye al bloqueo automático por estado crítico: un análisis preocupante
// avisa en lugar de detener el monitoreo del lote. El bloqueo queda reservado
// a posible_cosecha, que es lo que el trigger de la tabla de producciones
// interpreta.
type Alerta struct {
	ID              uint64          `json:"monitoring_alerta_id"`
	ProduccionID    int64           `json:"produccion_id"`
	SceneName       string          `json:"scene_name,omitempty"`
	SceneDate       *time.Time      `json:"scene_date,omitempty"`
	AlertType       string          `json:"alert_type"`
	Severity        string          `json:"severity"`
	Estado          string          `json:"estado"`
	Title           string          `json:"title"`
	Message         string          `json:"message"`
	ActionSuggested string          `json:"action_suggested,omitempty"`
	Source          string          `json:"source"`
	SourceJSON      json.RawMessage `json:"source_json,omitempty"`
	NotifyEmail     bool            `json:"notify_email"`
	EmailTo         string          `json:"email_to,omitempty"`
	EmailedAt       *time.Time      `json:"emailed_at,omitempty"`
	SeenAt          *time.Time      `json:"seen_at,omitempty"`
	SeenBy          string          `json:"seen_by,omitempty"`
	ResolvedAt      *time.Time      `json:"resolved_at,omitempty"`
	ResolvedBy      string          `json:"resolved_by,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       *time.Time      `json:"updated_at,omitempty"`

	// Enriquecidos por JOIN con el ERP para poder agrupar en la vista.
	// No son columnas de monitoring_alertas.
	Folio   string `json:"folio,omitempty"`
	Rancho  string `json:"rancho,omitempty"`
	Cultivo string `json:"cultivo,omitempty"`
}

// severidadOrden permite comparar severidades sin repartir el criterio.
var severidadOrden = map[string]int{
	SeveridadBaja:  1,
	SeveridadMedia: 2,
	SeveridadAlta:  3,
}

// SeveridadMayor devuelve la más grave de dos severidades.
func SeveridadMayor(a, b string) string {
	if severidadOrden[b] > severidadOrden[a] {
		return b
	}
	return a
}
