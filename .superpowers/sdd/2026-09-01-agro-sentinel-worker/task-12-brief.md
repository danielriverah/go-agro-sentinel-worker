### Task 12: REST API with Scalar

**Files:**
- Modify: `internal/http/router.go` — add all API routes
- Modify: `internal/http/handlers.go` — implement all handlers
- Create: `internal/http/middleware.go` — logging middleware
- Create: `internal/http/responses.go` — JSON response helpers
- Modify: `cmd/api/main.go` — wire dependencies
- Test: `internal/http/handlers_test.go` — expand with all endpoint tests

**Interfaces:**
- Consumes: `database.ProductionRepo`, `database.SceneRepo`, `database.FileRepo`, `aws.S3Client.PresignGetObject`
- Produces:
  - All REST endpoints from the spec
  - Scalar documentation at `/docs`
  - `http.LoggingMiddleware(logger, next) http.Handler`
  - `http.JSON(w, status int, data any)` helper
  - `http.Error(w, status int, message string)` helper

- [ ] **Step 1: Write handler tests for each endpoint**

Test each endpoint with mock repos:
- `GET /api/v1/producciones` → returns list
- `GET /api/v1/producciones/{id}` → returns production with escenas
- `POST /api/v1/producciones/{id}/desbloquear` → calls Desbloquear, returns updated production
- `GET /api/v1/escenas/{id}/archivos/{tipo}` → returns presigned URL
- `POST /api/v1/sync/trigger` → triggers sync, returns 202

- [ ] **Step 2: Implement all handlers**

Use `http.ServeMux` patterns with Go 1.22+ method routing:
```go
mux.HandleFunc("GET /api/v1/producciones", h.ListProducciones)
mux.HandleFunc("GET /api/v1/producciones/{id}", h.GetProduccion)
```

- [ ] **Step 3: Add Scalar docs**

Serve the Scalar UI at `/docs` using their CDN-hosted JS. The OpenAPI spec can be embedded as a Go constant or served from a YAML file.

```go
mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html")
    fmt.Fprint(w, scalarHTML)
})
mux.HandleFunc("GET /api/v1/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/yaml")
    w.Write(openapiSpec)
})
```

- [ ] **Step 4: Run tests and commit**

```bash
go test ./internal/http/ -v
git add .
git commit -m "feat: REST API with all endpoints and Scalar documentation"
```

---

