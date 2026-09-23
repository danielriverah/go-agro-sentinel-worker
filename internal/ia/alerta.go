package ia

import (
	"encoding/json"
	"strings"
	"time"

	"agro-sentinel-worker/internal/domain"
)

// maxHallazgosEnMensaje limita cuántos hallazgos se resumen en el mensaje.
// El detalle completo queda en source_json.
const maxHallazgosEnMensaje = 3

// maxAccionSugerida es el ancho de la columna action_suggested.
const maxAccionSugerida = 300

// modeloSalida es el JSON que devuelve el modelo, tal como se guarda en
// json_original.
type modeloSalida struct {
	EstadoClave     string   `json:"estado_clave"`
	EstadoGeneral   string   `json:"estado_general"`
	Resumen         string   `json:"resumen"`
	Recomendaciones []string `json:"recomendaciones"`
	Hallazgos       []struct {
		Tipo        string `json:"tipo"`
		Zona        string `json:"zona"`
		Severidad   string `json:"severidad"`
		Descripcion string `json:"descripcion"`
	} `json:"hallazgos"`
	Riesgo struct {
		Nivel  string `json:"nivel"`
		Motivo string `json:"motivo"`
	} `json:"riesgo"`
	PosibleCosecha bool `json:"posible_cosecha"`
}

// ConstruirAlerta arma la fila de monitoring_alertas a partir del análisis.
//
// Reemplaza al bloqueo automático por estado crítico: un diagnóstico grave
// genera un aviso en lugar de detener el monitoreo del lote.
func ConstruirAlerta(res *domain.IAResultSummary, produccionID int64, sceneName string, sceneDate *time.Time) *domain.Alerta {
	var m modeloSalida
	// Si el JSON no se puede leer, la alerta se emite igual con lo que hay en
	// el resumen: perder el aviso sería peor que emitirlo incompleto.
	_ = json.Unmarshal([]byte(res.JSONOriginal), &m)

	alerta := &domain.Alerta{
		ProduccionID:    produccionID,
		SceneName:       sceneName,
		SceneDate:       sceneDate,
		AlertType:       domain.AlertaTipoIA,
		Severity:        severidadDeAnalisis(m, res),
		Estado:          domain.AlertaNueva,
		Title:           tituloAlerta(res.EstadoClave, sceneName),
		Message:         mensajeAlerta(m, res),
		ActionSuggested: truncarRunas(strings.Join(m.Recomendaciones, " | "), maxAccionSugerida),
		Source:          domain.AlertaFuenteIA,
	}

	if res.JSONOriginal != "" {
		alerta.SourceJSON = json.RawMessage(res.JSONOriginal)
	}
	return alerta
}

// severidadDeAnalisis toma la severidad más alta entre TODOS los hallazgos,
// no sólo los que caben en el mensaje. Cuando no hay hallazgos, cae al nivel
// de riesgo para no reportar siempre "baja".
func severidadDeAnalisis(m modeloSalida, res *domain.IAResultSummary) string {
	if len(m.Hallazgos) == 0 {
		return severidadDesdeRiesgo(res.RiesgoNivel)
	}

	peor := domain.SeveridadBaja
	for _, h := range m.Hallazgos {
		peor = domain.SeveridadMayor(peor, strings.ToLower(h.Severidad))
	}
	return peor
}

// severidadDesdeRiesgo traduce el nivel de riesgo a severidad. Son escalas
// distintas y con distinto género gramatical: el riesgo es bajo/medio/alto y
// la severidad baja/media/alta, así que no se pueden copiar tal cual.
func severidadDesdeRiesgo(nivel string) string {
	switch strings.ToLower(strings.TrimSpace(nivel)) {
	case "alto", domain.SeveridadAlta:
		return domain.SeveridadAlta
	case "medio", domain.SeveridadMedia:
		return domain.SeveridadMedia
	default:
		return domain.SeveridadBaja
	}
}

func tituloAlerta(estadoClave, sceneName string) string {
	estado := strings.ToUpper(strings.TrimSpace(estadoClave))
	if estado == "" {
		estado = "ANALISIS"
	}
	return "IA " + estado + ": " + sceneName
}

// mensajeAlerta compone el resumen y, tras él, los hallazgos que merecen
// atención. Los de severidad baja se omiten: describen normalidad y sólo
// diluirían el aviso.
func mensajeAlerta(m modeloSalida, res *domain.IAResultSummary) string {
	resumen := m.Resumen
	if resumen == "" {
		resumen = res.EstadoGeneral
	}

	var destacados []string
	for _, h := range m.Hallazgos {
		if strings.ToLower(h.Severidad) == domain.SeveridadBaja {
			continue
		}
		destacados = append(destacados, h.Tipo+" en "+h.Zona+": "+h.Descripcion)
		if len(destacados) == maxHallazgosEnMensaje {
			break
		}
	}

	if len(destacados) == 0 {
		return resumen
	}
	return resumen + "\nHallazgos: " + strings.Join(destacados, " | ")
}

// truncarRunas corta por caracteres, no por bytes: la columna es VARCHAR(300)
// y cortar a mitad de un carácter acentuado produciría UTF-8 inválido.
func truncarRunas(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
