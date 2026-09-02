package domain

type AnalysisResult struct {
	EstadoGeneral   string   `json:"estado_general"`
	VigorVegetativo string   `json:"vigor_vegetativo"`
	EstresDetectado bool     `json:"estres_detectado"`
	PosibleCosecha  bool     `json:"posible_cosecha"`
	Recomendaciones []string `json:"recomendaciones"`
	Alertas         []string `json:"alertas"`
	Confianza       float64  `json:"confianza"`
}
