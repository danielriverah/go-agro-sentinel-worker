// Package ia implements the HTTP client for the external IA (image analysis)
// service. The worker calls it after generating params.json for a scene; a
// failure here is a partial failure per the spec (IA_ERROR), so the scene
// still completes without analisis.json.
package ia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/processing"
	"agro-sentinel-worker/internal/worker"
)

// Client is an HTTP client for the IA analysis service.
type Client struct {
	cfg        config.IAConfig
	httpClient *http.Client
}

// New creates a Client configured from cfg. The underlying HTTP client's
// timeout is set from cfg.TimeoutSeconds.
func New(cfg config.IAConfig) *Client {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// analyzeRequest is the JSON body POSTed to the IA service's /analyze
// endpoint.
type analyzeRequest struct {
	ProduccionID  int64              `json:"produccion_id"`
	SceneID       string             `json:"scene_id"`
	MultibandPath string             `json:"multiband_path"`
	Params        *processing.Params `json:"params,omitempty"`
}

// Analyze POSTs the given input to {ServiceURL}/analyze and parses the
// response body as a domain.AnalysisResult. If the IA service is disabled
// (cfg.Enabled == false), it returns nil, nil immediately without making a
// request. Any HTTP-level failure (non-2xx status, timeout, connection
// refused, malformed response) is wrapped as a
// domain.ProcessingError{Type: domain.ErrIA}.
func (c *Client) Analyze(ctx context.Context, input worker.IAInput) (*domain.AnalysisResult, error) {
	if !c.cfg.Enabled {
		return nil, nil
	}

	reqBody := analyzeRequest{
		ProduccionID:  input.ProduccionID,
		SceneID:       input.SceneID,
		MultibandPath: input.MultibandPath,
		Params:        input.Params,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, &domain.ProcessingError{Type: domain.ErrIA, Message: "encoding IA request", Wrapped: err}
	}

	url := fmt.Sprintf("%s/analyze", c.cfg.ServiceURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, &domain.ProcessingError{Type: domain.ErrIA, Message: "building IA request", Wrapped: err}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &domain.ProcessingError{Type: domain.ErrIA, Message: "calling IA service", Wrapped: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &domain.ProcessingError{Type: domain.ErrIA, Message: fmt.Sprintf("IA service returned status %d", resp.StatusCode)}
	}

	var result domain.AnalysisResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, &domain.ProcessingError{Type: domain.ErrIA, Message: "parsing IA response", Wrapped: err}
	}

	return &result, nil
}
