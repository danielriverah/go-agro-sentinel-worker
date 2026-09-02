package ia

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/worker"
)

func TestAnalyze(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/analyze" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(domain.AnalysisResult{
			EstadoGeneral:  "bueno",
			PosibleCosecha: false,
			Confianza:      0.85,
		}); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}))
	defer server.Close()

	client := New(config.IAConfig{Enabled: true, ServiceURL: server.URL, TimeoutSeconds: 10})
	result, err := client.Analyze(context.Background(), worker.IAInput{
		ProduccionID:  1,
		SceneID:       "scene-1",
		MultibandPath: "/tmp/multiband.tif",
	})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.EstadoGeneral != "bueno" {
		t.Errorf("estado = %q, want bueno", result.EstadoGeneral)
	}
}

func TestAnalyzeDisabled(t *testing.T) {
	client := New(config.IAConfig{Enabled: false})
	result, err := client.Analyze(context.Background(), worker.IAInput{})
	if err != nil || result != nil {
		t.Error("disabled IA should return nil, nil")
	}
}

func TestAnalyzePosibleCosecha(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(domain.AnalysisResult{
			EstadoGeneral:  "maduro",
			PosibleCosecha: true,
			Confianza:      0.92,
		}); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}))
	defer server.Close()

	client := New(config.IAConfig{Enabled: true, ServiceURL: server.URL, TimeoutSeconds: 10})
	result, err := client.Analyze(context.Background(), worker.IAInput{ProduccionID: 1, SceneID: "scene-1"})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if !result.PosibleCosecha {
		t.Error("expected PosibleCosecha = true")
	}
}

func TestAnalyzeHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := New(config.IAConfig{Enabled: true, ServiceURL: server.URL, TimeoutSeconds: 10})
	_, err := client.Analyze(context.Background(), worker.IAInput{ProduccionID: 1, SceneID: "scene-1"})
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}

	var procErr *domain.ProcessingError
	if !errors.As(err, &procErr) {
		t.Fatalf("expected *domain.ProcessingError, got %T: %v", err, err)
	}
	if procErr.Type != domain.ErrIA {
		t.Errorf("Type = %q, want %q", procErr.Type, domain.ErrIA)
	}
}

func TestAnalyzeConnectionRefused(t *testing.T) {
	client := New(config.IAConfig{Enabled: true, ServiceURL: "http://127.0.0.1:1", TimeoutSeconds: 1})
	_, err := client.Analyze(context.Background(), worker.IAInput{ProduccionID: 1, SceneID: "scene-1"})
	if err == nil {
		t.Fatal("expected error for connection refused")
	}

	var procErr *domain.ProcessingError
	if !errors.As(err, &procErr) {
		t.Fatalf("expected *domain.ProcessingError, got %T: %v", err, err)
	}
	if procErr.Type != domain.ErrIA {
		t.Errorf("Type = %q, want %q", procErr.Type, domain.ErrIA)
	}
}
