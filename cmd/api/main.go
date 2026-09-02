package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"agro-sentinel-worker/internal/config"
	apphttp "agro-sentinel-worker/internal/http"
	"agro-sentinel-worker/internal/logger"
)

func main() {
	cfgPath := "configs/config.yaml"
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		cfgPath = p
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	l := logger.New(cfg.Logging)
	router := apphttp.NewRouter(l)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	l.Info("API server starting", "addr", addr)

	if err := http.ListenAndServe(addr, router); err != nil {
		l.Error("server failed", "error", err)
		os.Exit(1)
	}
}
