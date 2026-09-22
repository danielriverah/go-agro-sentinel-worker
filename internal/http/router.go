package http

import (
	"context"
	"log/slog"
	"net/http"

	"agro-sentinel-worker/internal/auth"
)

// noopPermissionChecker es el PermissionChecker por defecto cuando
// Handlers.Permisos no se configuró explícitamente. RequirePermission y
// RequireGlobalPermission llaman a loader.Disponible() sin comprobar nil, así
// que pasar una interfaz nil haría panic; este stand-in evita eso y se
// comporta igual que "tablas no existen" (modo permisivo).
type noopPermissionChecker struct{}

func (noopPermissionChecker) Disponible() bool { return false }
func (noopPermissionChecker) CargarPermisos(ctx context.Context, usuarioID int64) (*auth.PermisosUsuario, error) {
	return nil, auth.ErrPermisosMissing
}
func (noopPermissionChecker) ObtenerPermisosCacheados(usuarioID int64) *auth.PermisosUsuario {
	return nil
}
func (noopPermissionChecker) InvalidarCache(usuarioID int64) {}

// escenaCentroCostoResolver adapta auth.CentroCostoResolver para las rutas de
// escenas. RequirePermission toma el {id} de la ruta y lo pasa directo a
// GetCentroCostoByMonitoringID asumiendo que es un s3_monitoring_produccion_id,
// pero en /api/v1/escenas/{id}/... {id} es el id de la escena. Este adaptador
// resuelve escena -> producción -> centro_costo_id antes de delegar.
type escenaCentroCostoResolver struct {
	Scenes      SceneRepository
	Productions ProductionRepository
}

func (e *escenaCentroCostoResolver) GetCentroCostoByMonitoringID(ctx context.Context, escenaID uint) (*int64, error) {
	scene, err := e.Scenes.GetByID(ctx, uint64(escenaID))
	if err != nil {
		return nil, err
	}
	if scene == nil {
		return nil, nil
	}
	// GetCentroCostoByMonitoringID (not GetByMonitoringID) on purpose:
	// domain.Production.CentroCostoID is only populated in memory during sync
	// (see internal/sync/sync.go) — GetByID/GetByMonitoringID select only
	// s3_monitoring_producciones columns, so p.CentroCostoID would be zero
	// here. GetCentroCostoByMonitoringID runs the real join against producciones.
	return e.Productions.GetCentroCostoByMonitoringID(ctx, scene.MonitoringProduccionID)
}

// deleteMonitoreoMiddleware protege el borrado irreversible de monitoreo.
//
// Si el sistema de permisos está disponible, usa RequirePermission
// (monitoreo.eliminar) igual que el resto de operaciones ranch-scoped. Si las
// tablas de permisos todavía no existen, RequirePermission pasaría todo en
// modo permisivo — inaceptable para una operación irreversible — así que en
// ese caso se cae en la lista explícita AUTH_DELETE_ALLOWED_USER_IDS (vacía =
// cerrado para todos). Disponible() se fija una sola vez al arrancar
// (tableExists en el probe de startup), así que decidir aquí, al construir el
// router, es equivalente a decidirlo por request.
func deleteMonitoreoMiddleware(h *Handlers) func(http.Handler) http.Handler {
	if h.Permisos.Disponible() {
		return auth.RequirePermission("monitoreo.eliminar", h.Permisos, h.Productions)
	}
	return auth.RequireUserIn(h.DeleteAllowedUserIDs)
}

// NewRouter builds the API's http.ServeMux, wiring health, docs and all
// versioned API routes, wrapped in the logging and auth middlewares.
func NewRouter(logger *slog.Logger, h *Handlers, authH *AuthHandlers, secretKey []byte) http.Handler {
	// Handlers.Permisos es opcional (nil = no configurado). Normalizarlo aquí
	// evita que cada middleware de permisos tenga que comprobar nil.
	if h.Permisos == nil {
		h.Permisos = noopPermissionChecker{}
	}

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

	// Permisos propios: cualquier usuario autenticado puede consultar y
	// refrescar los suyos, sin necesitar un permiso adicional para eso.
	mux.HandleFunc("GET /api/v1/auth/permisos", h.MisPermisos)
	mux.HandleFunc("POST /api/v1/auth/refrescar-permisos", h.RefrescarPermisos)

	mux.HandleFunc("GET /api/v1/producciones", h.ListProducciones)
	mux.HandleFunc("GET /api/v1/producciones/{id}", h.GetProduccion)
	mux.HandleFunc("GET /api/v1/producciones/{id}/timeline", h.GetProduccionTimeline)
	mux.HandleFunc("GET /api/v1/producciones/{id}/fases", h.GetProduccionFases)
	mux.Handle("PUT /api/v1/producciones/{id}/fases",
		auth.RequirePermission("fases.editar", h.Permisos, h.Productions)(http.HandlerFunc(h.PutProduccionFases)))
	// Destructivo e irreversible: ver deleteMonitoreoMiddleware para el porqué
	// del fallback cuando el sistema de permisos no está disponible.
	mux.Handle("DELETE /api/v1/producciones/{id}/monitoreo",
		deleteMonitoreoMiddleware(h)(http.HandlerFunc(h.EliminarMonitoreo)))
	// Ambas escriben posible_cosecha; el trigger de la tabla deriva bloqueado.
	mux.Handle("POST /api/v1/producciones/{id}/desbloquear",
		auth.RequirePermission("producciones.bloquear", h.Permisos, h.Productions)(http.HandlerFunc(h.DesbloquearProduccion)))
	mux.Handle("POST /api/v1/producciones/{id}/bloquear",
		auth.RequirePermission("producciones.bloquear", h.Permisos, h.Productions)(http.HandlerFunc(h.BloquearProduccion)))
	mux.HandleFunc("PATCH /api/v1/producciones/{id}/ia-auto", h.PatchIAAuto)
	mux.Handle("PUT /api/v1/producciones/{id}/poligono",
		auth.RequirePermission("producciones.editar", h.Permisos, h.Productions)(http.HandlerFunc(h.PutProduccionPoligono)))

	// Plantillas para copiar fases: sólo producciones que ya tienen.
	mux.HandleFunc("GET /api/v1/fases/plantillas", h.ListPlantillasFases)

	// Alertas (notificaciones de la IA).
	mux.HandleFunc("GET /api/v1/alertas", h.ListAlertas)
	mux.HandleFunc("POST /api/v1/alertas/{id}/vista", h.MarcarAlertaVista)
	mux.HandleFunc("POST /api/v1/alertas/{id}/resuelta", h.MarcarAlertaResuelta)

	mux.HandleFunc("GET /api/v1/escenas/{id}", h.GetEscena)
	mux.HandleFunc("GET /api/v1/escenas/{id}/archivos", h.ListEscenaArchivos)
	mux.HandleFunc("GET /api/v1/escenas/{id}/archivos/{tipo}/stream", h.ProxyEscenaArchivo)
	mux.HandleFunc("GET /api/v1/escenas/{id}/archivos/{tipo}", h.GetEscenaArchivo)
	mux.HandleFunc("GET /api/v1/escenas/{id}/analisis", h.GetEscenaAnalisis)
	mux.Handle("POST /api/v1/escenas/{id}/analizar",
		auth.RequirePermission("escenas.analizar", h.Permisos,
			&escenaCentroCostoResolver{Scenes: h.Scenes, Productions: h.Productions},
		)(http.HandlerFunc(h.TriggerEscenaAnalisis)))

	// Administración de usuarios.
	mux.Handle("GET /api/v1/admin/usuarios",
		auth.RequireGlobalPermission("usuarios.ver", h.Permisos)(http.HandlerFunc(authH.ListUsuarios)))
	mux.Handle("PUT /api/v1/admin/usuarios/{id}/activo",
		auth.RequireGlobalPermission("usuarios.administrar", h.Permisos)(http.HandlerFunc(authH.SetUsuarioActivo)))
	mux.Handle("PUT /api/v1/admin/usuarios/{id}/password",
		auth.RequireGlobalPermission("usuarios.administrar", h.Permisos)(http.HandlerFunc(authH.ResetPassword)))

	// Administración de roles y permisos.
	adminRoles := auth.RequireGlobalPermission("roles.administrar", h.Permisos)
	mux.Handle("GET /api/v1/admin/roles", adminRoles(http.HandlerFunc(h.ListRoles)))
	mux.Handle("POST /api/v1/admin/roles", adminRoles(http.HandlerFunc(h.CreateRol)))
	mux.Handle("PUT /api/v1/admin/roles/{id}", adminRoles(http.HandlerFunc(h.UpdateRol)))
	mux.Handle("DELETE /api/v1/admin/roles/{id}", adminRoles(http.HandlerFunc(h.DeleteRol)))
	mux.Handle("GET /api/v1/admin/permisos", adminRoles(http.HandlerFunc(h.ListPermisosCatalogo)))
	mux.Handle("GET /api/v1/admin/usuarios/{id}/permisos", adminRoles(http.HandlerFunc(h.GetUsuarioPermisos)))
	mux.Handle("PUT /api/v1/admin/usuarios/{id}/roles", adminRoles(http.HandlerFunc(h.SetUsuarioRoles)))
	mux.Handle("PUT /api/v1/admin/usuarios/{id}/permisos-directos", adminRoles(http.HandlerFunc(h.SetUsuarioPermisos)))
	mux.Handle("GET /api/v1/admin/centros-costos", adminRoles(http.HandlerFunc(h.ListCentrosCostos)))

	mux.HandleFunc("GET /api/v1/sync/status", h.SyncStatus)
	mux.HandleFunc("GET /api/v1/sync/events", h.SyncEvents)
	mux.Handle("POST /api/v1/sync/trigger",
		auth.RequireGlobalPermission("sync.ejecutar", h.Permisos)(http.HandlerFunc(h.TriggerSync)))

	// Worker status, control, and on-demand triggers.
	mux.HandleFunc("GET /api/v1/worker/status", WorkerStatus)
	mux.HandleFunc("GET /api/v1/worker/events", WorkerEvents)
	workerControlar := auth.RequireGlobalPermission("worker.controlar", h.Permisos)
	mux.Handle("POST /api/v1/worker/cancel", workerControlar(http.HandlerFunc(CancelWorker)))
	mux.Handle("POST /api/v1/worker/unlock", workerControlar(http.HandlerFunc(UnlockWorker)))
	mux.Handle("POST /api/v1/worker/run", workerControlar(http.HandlerFunc(TriggerWorkerAll)))
	mux.Handle("POST /api/v1/worker/run-production/{id}",
		auth.RequirePermission("monitoreo.worker", h.Permisos, h.Productions)(http.HandlerFunc(h.TriggerWorkerProduction)))

	return LoggingMiddleware(logger,
		auth.Middleware(secretKey)(mux),
	)
}
