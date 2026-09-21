package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/domain"
)

// BedrockClient invokes an Anthropic model via AWS Bedrock Runtime using
// credentials dedicated to the IA billing account.
type BedrockClient struct {
	client  *bedrockruntime.Client
	modelID string
	timeout time.Duration
}

// NewBedrockClient creates a BedrockClient from cfg. Returns an error when
// credentials or model ID are missing.
//
// Auth resolution (first match wins):
//   - APIKey (BEDROCK_API_KEY) as SecretAccessKey — requires AccessKeyID too
//   - AccessKeyID + SecretAccessKey (IA_AWS_ACCESS_KEY_ID / IA_AWS_SECRET_ACCESS_KEY)
func NewBedrockClient(cfg config.BedrockConfig) (*BedrockClient, error) {
	if cfg.ModelID == "" {
		return nil, fmt.Errorf("bedrock: model_id is required")
	}
	// Resolve secret: BEDROCK_API_KEY overrides IA_AWS_SECRET_ACCESS_KEY.
	secretKey := cfg.SecretAccessKey
	if cfg.APIKey != "" {
		secretKey = cfg.APIKey
	}
	if cfg.AccessKeyID == "" || secretKey == "" {
		return nil, fmt.Errorf("bedrock: credentials required — set (IA_AWS_ACCESS_KEY_ID + IA_AWS_SECRET_ACCESS_KEY) or (IA_AWS_ACCESS_KEY_ID + BEDROCK_API_KEY)")
	}
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	awsCfg := awssdk.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, secretKey, ""),
	}

	return &BedrockClient{
		client:  bedrockruntime.NewFromConfig(awsCfg),
		modelID: cfg.ModelID,
		timeout: timeout,
	}, nil
}

// bedrockRequest is the Messages API payload sent to the model.
type bedrockRequest struct {
	AnthropicVersion string             `json:"anthropic_version"`
	MaxTokens        int                `json:"max_tokens"`
	System           string             `json:"system"`
	Messages         []bedrockMessage   `json:"messages"`
}

type bedrockMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// bedrockResponse is the subset of the Anthropic response we care about.
type bedrockResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

const bedrockSystem = `Eres un asistente experto en análisis agrícola, agricultura de precisión e interpretación de índices satelitales.

Tu tarea es analizar una producción agrícola usando contexto del lote, cultivo, fechas, escena satelital, clima resumido e índices vegetativos.

Debes responder ÚNICAMENTE en formato JSON válido.
No uses Markdown.
No expliques fuera del JSON.
No incluyas texto adicional antes ni después.
No inventes datos no proporcionados.
Si un dato opcional no viene informado, usa null.
Si no hay suficiente información para afirmar algo, usa lenguaje conservador y orientado a verificación en campo.
Responde siempre en español. Todos los textos del JSON deben estar en español correcto, sin mezcla de idiomas.

Estructura del payload que recibirás:
- produccion: id, folio (identificador del lote), rancho (nombre del centro de costo), cosecha, variedades, fecha_plantacion, dias_desde_plantacion, dias_ciclo_aprox
- escena: id (scene name), fecha, cloud_cover_pct
- fecha_analisis: fecha en que se genera este análisis
- estado.cobertura: vegetacion_pct, suelo_pct, agua_pct
- estado.indices: mapa de índices disponibles (ndvi, evi, gndvi, ndre, nbr, ndmi, savi) — solo los que tienen valor válido
- historico: n_total (total de escenas históricas), recientes (últimas 3 escenas con indices y cloud_pct), tendencia (cambio promedio por índice)
- ultimo_analisis (opcional): fecha, estado_clave, estado_general, riesgo_nivel, riesgo_motivo del análisis anterior

Reglas:
1. Si la etapa fenológica no viene explícita, estímala con fecha_plantacion, fecha_escena y dias_despues_plantacion.
2. Interpreta NDVI como vigor vegetal.
3. Interpreta NDWI/NDMI como posible indicador de humedad o estrés hídrico.
4. Interpreta NDRE como indicador relacionado con clorofila/nitrógeno.
5. Interpreta SAVI especialmente en etapas tempranas o baja cobertura vegetal.
6. Compara índices actuales contra la escena anterior cuando exista.
7. Usa el histórico solo como referencia de tendencia, no como diagnóstico absoluto.
8. Usa el clima histórico y el pronóstico solo como contexto agronómico complementario.
9. Las recomendaciones deben ser prácticas, accionables y orientadas a revisión de campo.
10. No recomiendes aplicaciones químicas específicas si no hay evidencia suficiente.
11. No afirmes plagas, enfermedades o deficiencias como hechos; usa términos como "posible", "sugerido por" o "conviene verificar en campo".
12. Prioriza claridad y concisión.
13. Si por etapa fenológica, vigor, uniformidad y contexto agronómico observas que la producción podría estar en ventana de cosecha o muy próxima a ella, marca posible_cosecha=true; en caso contrario false.
14. El resultado debe ajustarse exactamente al esquema solicitado.

Devuelve SOLO este esquema JSON (nada más):
{
  "estado_clave": "optimo|bueno|alerta|critico",
  "estado_general": "descripcion breve del estado general del cultivo",
  "resumen": "resumen ejecutivo de 2-4 oraciones",
  "hallazgos": [{"tipo": "string", "zona": "string", "severidad": "baja|media|alta", "descripcion": "string"}],
  "recomendaciones": ["accion 1", "accion 2"],
  "riesgo": {"nivel": "bajo|medio|alto", "motivo": "string"},
  "posible_cosecha": false
}`

// DryRunPrompt returns the exact payload that would be sent to Bedrock,
// without making any API call. Useful for inspecting and validating prompts.
func (c *BedrockClient) DryRunPrompt(iaReqJSON []byte) (system string, userMessage string, fullPayload []byte) {
	userMsg := "Analiza el siguiente reporte de monitoreo satelital y genera el diagnostico:\n\n" + string(iaReqJSON)
	req := bedrockRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        8192,
		System:           bedrockSystem,
		Messages:         []bedrockMessage{{Role: "user", Content: userMsg}},
	}
	payload, _ := json.MarshalIndent(req, "", "  ")
	return bedrockSystem, userMsg, payload
}

// Analyze invokes the Bedrock model with the ia_req JSON payload and parses
// the structured JSON response into a domain.IAResultSummary.
// Also returns posibleCosecha so the caller can update the production flag.
func (c *BedrockClient) Analyze(ctx context.Context, escenaID uint64, iaReqJSON []byte) (*domain.IAResultSummary, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	userMsg := fmt.Sprintf("Analiza el siguiente reporte de monitoreo satelital y genera el diagnóstico:\n\n%s", string(iaReqJSON))

	reqBody, err := json.Marshal(bedrockRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        8192,
		System:           bedrockSystem,
		Messages:         []bedrockMessage{{Role: "user", Content: userMsg}},
	})
	if err != nil {
		return nil, false, fmt.Errorf("bedrock: marshaling request: %w", err)
	}

	out, err := c.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     awssdk.String(c.modelID),
		ContentType: awssdk.String("application/json"),
		Accept:      awssdk.String("application/json"),
		Body:        reqBody,
	})
	if err != nil {
		return nil, false, fmt.Errorf("bedrock: invoking model %s: %w", c.modelID, err)
	}

	var resp bedrockResponse
	if err := json.NewDecoder(bytes.NewReader(out.Body)).Decode(&resp); err != nil {
		return nil, false, fmt.Errorf("bedrock: decoding response: %w", err)
	}
	if len(resp.Content) == 0 || resp.Content[0].Text == "" {
		return nil, false, fmt.Errorf("bedrock: empty response from model")
	}

	// Parse the structured JSON the model returned.
	type hallazgo struct {
		Tipo       string `json:"tipo"`
		Zona       string `json:"zona"`
		Severidad  string `json:"severidad"`
		Descripcion string `json:"descripcion"`
	}
	type riesgoObj struct {
		Nivel  string `json:"nivel"`
		Motivo string `json:"motivo"`
	}
	type modelOutput struct {
		EstadoClave      string     `json:"estado_clave"`
		EstadoGeneral    string     `json:"estado_general"`
		Resumen          string     `json:"resumen"`
		Hallazgos        []hallazgo `json:"hallazgos"`
		Recomendaciones  []string   `json:"recomendaciones"`
		Riesgo           riesgoObj  `json:"riesgo"`
		PosibleCosecha   bool       `json:"posible_cosecha"`
	}

	text := resp.Content[0].Text
	// Strip accidental markdown fences if the model wraps the JSON.
	text = stripJSONFences(text)

	var mo modelOutput
	if err := json.Unmarshal([]byte(text), &mo); err != nil {
		return nil, false, fmt.Errorf("bedrock: parsing model JSON output: %w — raw: %s", err, text)
	}

	now := time.Now().UTC()
	// Serialize full model output as json_original for audit trail.
	jsonOrig, _ := json.Marshal(mo)

	return &domain.IAResultSummary{
		S3MonitoringEscenaID: escenaID,
		EstadoClave:          mo.EstadoClave,
		EstadoGeneral:        mo.EstadoGeneral,
		RiesgoNivel:          mo.Riesgo.Nivel,
		RiesgoMotivo:         mo.Riesgo.Motivo,
		FechaAnalisis:        &now,
		JSONOriginal:         string(jsonOrig),
	}, mo.PosibleCosecha, nil
}

// stripJSONFences removes ```json … ``` or ``` … ``` wrappers that some
// models add around their JSON output despite instructions not to.
func stripJSONFences(s string) string {
	b := bytes.TrimSpace([]byte(s))
	if bytes.HasPrefix(b, []byte("```")) {
		if i := bytes.Index(b, []byte("\n")); i >= 0 {
			b = b[i+1:]
		}
		if bytes.HasSuffix(b, []byte("```")) {
			b = b[:len(b)-3]
		}
		b = bytes.TrimSpace(b)
	}
	return string(b)
}

// joinStrings joins a slice with sep, returning "" for empty slices.
func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	var buf bytes.Buffer
	for i, s := range ss {
		if i > 0 {
			buf.WriteString(sep)
		}
		buf.WriteString(s)
	}
	return buf.String()
}
