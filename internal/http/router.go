package http

import (
	"log/slog"
	"net/http"
)

func NewRouter(logger *slog.Logger) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", HealthHandler)
	return mux
}
