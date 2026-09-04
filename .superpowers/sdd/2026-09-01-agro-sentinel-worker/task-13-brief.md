### Task 13: IA service client and integration

**Files:**
- Create: `internal/infrastructure/ia/client.go`
- Test: `internal/infrastructure/ia/client_test.go`

**Interfaces:**
- Consumes: `config.IAConfig`, `domain.AnalysisResult`, params.json content
- Produces:
  - `ia.Client` struct with:
    - `New(cfg config.IAConfig) *Client`
    - `Analyze(ctx context.Context, params json.RawMessage) (*domain.AnalysisResult, error)` — POST to IA service, parse response
  - Wraps HTTP errors in `domain.ProcessingError{Type: domain.ErrIA}`
  - Returns `nil, nil` when `cfg.Enabled == false` (IA disabled)

- [ ] **Step 1: Write client test with httptest server**

```go
func TestAnalyze(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method != "POST" || r.URL.Path != "/analyze" {
            t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
        }
        json.NewEncoder(w).Encode(domain.AnalysisResult{
            EstadoGeneral:   "bueno",
            PosibleCosecha:  false,
            Confianza:       0.85,
        })
    }))
    defer server.Close()

    client := New(config.IAConfig{Enabled: true, ServiceURL: server.URL, TimeoutSeconds: 10})
    result, err := client.Analyze(context.Background(), json.RawMessage(`{"test":true}`))
    if err != nil {
        t.Fatalf("Analyze failed: %v", err)
    }
    if result.EstadoGeneral != "bueno" {
        t.Errorf("estado = %q, want bueno", result.EstadoGeneral)
    }
}

func TestAnalyzeDisabled(t *testing.T) {
    client := New(config.IAConfig{Enabled: false})
    result, err := client.Analyze(context.Background(), nil)
    if err != nil || result != nil {
        t.Error("disabled IA should return nil, nil")
    }
}
```

- [ ] **Step 2: Implement client.go**

- [ ] **Step 3: Update worker.go to use IA client**

After generating params.json, call `ia.Analyze`. If `posible_cosecha == true`, call `productionRepo.SetBloqueado`. If IA fails, log warning but mark scene COMPLETED anyway (spec: IA_ERROR is partial retry).

- [ ] **Step 4: Run tests and commit**

```bash
go test ./internal/infrastructure/ia/ -v
git add .
git commit -m "feat: IA service client with harvest detection and blocking"
```

---

