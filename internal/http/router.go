package http

import (
	"log/slog"
	"net/http"

	"agro-sentinel-worker/internal/auth"
)

// NewRouter builds the API's http.ServeMux, wiring health, docs and all
// versioned API routes, wrapped in the logging and auth middlewares.
func NewRouter(logger *slog.Logger, h *Handlers, authH *AuthHandlers, secretKey []byte) http.Handler {
	mux := http.NewServeMux()

	// Public — no token required.
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
	mux.HandleFunc("POST /api/v1/auth/login", authH.Login)

	// Protected — require valid JWT.
	mux.HandleFunc("POST /api/v1/auth/change-password", authH.ChangePassword)

	mux.HandleFunc("GET /api/v1/producciones", h.ListProducciones)
	mux.HandleFunc("GET /api/v1/producciones/{id}", h.GetProduccion)
	mux.HandleFunc("POST /api/v1/producciones/{id}/desbloquear", h.DesbloquearProduccion)
	mux.HandleFunc("PATCH /api/v1/producciones/{id}/ia-auto", h.PatchIAAuto)
	mux.HandleFunc("PUT /api/v1/producciones/{id}/poligono", h.PutProduccionPoligono)

	mux.HandleFunc("GET /api/v1/escenas/{id}", h.GetEscena)
	mux.HandleFunc("GET /api/v1/escenas/{id}/archivos", h.ListEscenaArchivos)
	mux.HandleFunc("GET /api/v1/escenas/{id}/archivos/{tipo}/stream", h.ProxyEscenaArchivo)
	mux.HandleFunc("GET /api/v1/escenas/{id}/archivos/{tipo}", h.GetEscenaArchivo)
	mux.HandleFunc("GET /api/v1/escenas/{id}/analisis", h.GetEscenaAnalisis)
	mux.HandleFunc("POST /api/v1/escenas/{id}/analizar", h.TriggerEscenaAnalisis)

	mux.HandleFunc("GET /api/v1/sync/status", h.SyncStatus)
	mux.HandleFunc("GET /api/v1/sync/events", h.SyncEvents)
	mux.HandleFunc("POST /api/v1/sync/trigger", h.TriggerSync)

	// Worker status, control, and on-demand triggers.
	mux.HandleFunc("GET /api/v1/worker/status", WorkerStatus)
	mux.HandleFunc("GET /api/v1/worker/events", WorkerEvents)
	mux.HandleFunc("POST /api/v1/worker/cancel", CancelWorker)
	mux.HandleFunc("POST /api/v1/worker/unlock", UnlockWorker)
	mux.HandleFunc("POST /api/v1/worker/run", TriggerWorkerAll)
	mux.HandleFunc("POST /api/v1/worker/run-production/{id}", h.TriggerWorkerProduction)

	return LoggingMiddleware(logger,
		auth.Middleware(secretKey)(mux),
	)
}
