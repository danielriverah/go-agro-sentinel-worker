package http

import (
	"log/slog"
	"net/http"
)

// NewRouter builds the API's http.ServeMux, wiring health, docs and all
// versioned API routes, wrapped in the logging middleware.
func NewRouter(logger *slog.Logger, h *Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", HealthHandler)
	mux.HandleFunc("GET /health/dependencies", h.HealthDependencies)

	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(scalarHTML))
	})
	mux.HandleFunc("GET /api/v1/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write([]byte(openapiSpec))
	})

	mux.HandleFunc("GET /api/v1/producciones", h.ListProducciones)
	mux.HandleFunc("GET /api/v1/producciones/{id}", h.GetProduccion)
	mux.HandleFunc("POST /api/v1/producciones/{id}/desbloquear", h.DesbloquearProduccion)
	mux.HandleFunc("GET /api/v1/escenas/{id}/archivos/{tipo}", h.GetEscenaArchivo)
	mux.HandleFunc("POST /api/v1/sync/trigger", h.TriggerSync)

	return LoggingMiddleware(logger, mux)
}
