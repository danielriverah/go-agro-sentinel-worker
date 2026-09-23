package ia

import (
	"strings"
	"testing"

	"agro-sentinel-worker/internal/domain"
)

// Réplica reducida del registro 17 real: estado óptimo, riesgo bajo, pero con
// un hallazgo de severidad media entre cuatro.
const jsonOptimo = `{
  "estado_clave": "optimo",
  "estado_general": "Cultivo en etapa de llenado de grano con vigor excelente.",
  "resumen": "Maíz blanco a 83 días post-plantación presenta NDVI de 0.7705.",
  "hallazgos": [
    {"tipo":"Vigor vegetal","zona":"Lote completo","severidad":"baja","descripcion":"NDVI de 0.7705 con 99.64% en zona alta."},
    {"tipo":"Humedad foliar","zona":"Lote completo","severidad":"media","descripcion":"NDWI de -0.7157 es esperado en etapa R4-R5."},
    {"tipo":"Contenido de clorofila","zona":"Lote completo","severidad":"baja","descripcion":"NDRE de 0.5739 superior al histórico."},
    {"tipo":"Uniformidad","zona":"Lote completo","severidad":"baja","descripcion":"Sin zonas de baja cobertura."}
  ],
  "recomendaciones": ["Verificar madurez fisiológica.","Monitorear 2-3 semanas."],
  "riesgo": {"nivel":"bajo","motivo":"Índices óptimos."},
  "posible_cosecha": true
}`

// Réplica reducida del registro 20: crítico con cuatro hallazgos, dos altos
// y dos medios.
const jsonCritico = `{
  "estado_clave": "critico",
  "estado_general": "Deterioro severo del cultivo.",
  "resumen": "A los 109 días se observa colapso de índices vegetativos.",
  "hallazgos": [
    {"tipo":"Vigor vegetal crítico","zona":"Lote completo","severidad":"alta","descripcion":"NDVI 0.1283 vs histórico 0.4499."},
    {"tipo":"Estrés hídrico extremo","zona":"Lote completo","severidad":"alta","descripcion":"NDWI -0.294 con 97.86% en estrés."},
    {"tipo":"Deficiencia de clorofila","zona":"Lote completo","severidad":"media","descripcion":"NDRE cayó de 0.4659 a 0.0637."},
    {"tipo":"Baja uniformidad","zona":"Lote completo","severidad":"media","descripcion":"87.94% en zona media."}
  ],
  "recomendaciones": ["Verificar plagas.","Revisar riego.","Evaluar drenaje."],
  "riesgo": {"nivel":"alto","motivo":"Caída crítica de vigor."},
  "posible_cosecha": false
}`

func TestConstruirAlertaOptimoTomaSeveridadDelPeorHallazgo(t *testing.T) {
	res := &domain.IAResultSummary{EstadoClave: "optimo", RiesgoNivel: "bajo", JSONOriginal: jsonOptimo}

	a := ConstruirAlerta(res, 2045, "S2C_14QLJ_20260722_0_L2A", nil)

	if a.Title != "IA OPTIMO: S2C_14QLJ_20260722_0_L2A" {
		t.Errorf("title = %q", a.Title)
	}
	// El riesgo es bajo, pero un hallazgo es medio: la severidad de la alerta
	// sale del peor hallazgo, no del nivel de riesgo.
	if a.Severity != domain.SeveridadMedia {
		t.Errorf("severity = %q, want media", a.Severity)
	}
	if a.Estado != domain.AlertaNueva {
		t.Errorf("estado = %q, want nueva", a.Estado)
	}
	if a.AlertType != domain.AlertaTipoIA || a.Source != domain.AlertaFuenteIA {
		t.Errorf("alert_type = %q, source = %q", a.AlertType, a.Source)
	}
	if a.ProduccionID != 2045 {
		t.Errorf("produccion_id = %d, want 2045", a.ProduccionID)
	}

	// Sólo el hallazgo de severidad media entra en el mensaje.
	if !strings.Contains(a.Message, "Humedad foliar en Lote completo:") {
		t.Errorf("falta el hallazgo medio en el mensaje:\n%s", a.Message)
	}
	if strings.Contains(a.Message, "Vigor vegetal en") || strings.Contains(a.Message, "Uniformidad en") {
		t.Errorf("los hallazgos de severidad baja no deben aparecer:\n%s", a.Message)
	}
	if !strings.HasPrefix(a.Message, "Maíz blanco a 83 días") {
		t.Errorf("el mensaje debe empezar por el resumen:\n%s", a.Message)
	}
}

func TestConstruirAlertaCriticoLimitaTresHallazgos(t *testing.T) {
	res := &domain.IAResultSummary{EstadoClave: "critico", RiesgoNivel: "alto", JSONOriginal: jsonCritico}

	a := ConstruirAlerta(res, 2059, "S2C_14QLJ_20260831_0_L2A", nil)

	if a.Severity != domain.SeveridadAlta {
		t.Errorf("severity = %q, want alta", a.Severity)
	}
	if a.Title != "IA CRITICO: S2C_14QLJ_20260831_0_L2A" {
		t.Errorf("title = %q", a.Title)
	}

	// Cuatro hallazgos no bajos, pero sólo caben tres en el mensaje.
	if n := strings.Count(a.Message, " | "); n != 2 {
		t.Errorf("separadores = %d, want 2 (tres hallazgos):\n%s", n, a.Message)
	}
	if strings.Contains(a.Message, "Baja uniformidad") {
		t.Error("el cuarto hallazgo no debería entrar en el mensaje")
	}
	if len(a.SourceJSON) == 0 {
		t.Error("source_json debe conservar el análisis completo")
	}
}

func TestConstruirAlertaTruncaAccionSugerida(t *testing.T) {
	larga := strings.Repeat("á", 500) // acentuada a propósito: 2 bytes por carácter
	res := &domain.IAResultSummary{
		EstadoClave:  "alerta",
		JSONOriginal: `{"recomendaciones":["` + larga + `"]}`,
	}

	a := ConstruirAlerta(res, 1, "S2A_X", nil)

	// La columna es VARCHAR(300): el corte va por caracteres, no por bytes,
	// o produciría UTF-8 inválido a mitad de un acento.
	if n := len([]rune(a.ActionSuggested)); n != maxAccionSugerida {
		t.Errorf("caracteres = %d, want %d", n, maxAccionSugerida)
	}
	if !strings.ContainsRune(a.ActionSuggested, 'á') {
		t.Error("el truncado corrompió el texto")
	}
}

// Un JSON ilegible no debe hacer perder el aviso.
func TestConstruirAlertaConJSONCorrupto(t *testing.T) {
	res := &domain.IAResultSummary{
		EstadoClave:   "alerta",
		EstadoGeneral: "Estado general de respaldo",
		RiesgoNivel:   "medio",
		JSONOriginal:  "{roto",
	}

	a := ConstruirAlerta(res, 5, "S2B_Y", nil)

	if a.Message != "Estado general de respaldo" {
		t.Errorf("message = %q, want el estado general como respaldo", a.Message)
	}
	// Sin hallazgos legibles, la severidad cae al nivel de riesgo.
	if a.Severity != domain.SeveridadMedia {
		t.Errorf("severity = %q, want media", a.Severity)
	}
}
