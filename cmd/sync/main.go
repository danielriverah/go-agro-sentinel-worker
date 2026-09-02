package main

import (
	"fmt"
	"log"
	"os"

	"agro-sentinel-worker/internal/config"
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
	l.Info("sync starting", "name", cfg.App.Name, "interval_minutes", cfg.Sync.IntervalMinutes)
	fmt.Println("sync: no DynamoDB configured yet")
}
