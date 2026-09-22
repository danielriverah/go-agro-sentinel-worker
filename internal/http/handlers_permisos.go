package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"agro-sentinel-worker/internal/auth"
	"agro-sentinel-worker/internal/infrastructure/database"
)

// PermissionRepository es el subconjunto de database.PermissionRepo que usan
// los endpoints de administración de roles y permisos. Separado de
// PermissionChecker (que sólo evalúa permisos para el middleware) porque este
// necesita el CRUD completo.
type PermissionRepository interface {
	Disponible() bool
	ListPermisos(ctx context.Context) ([]database.PermisoCatalogo, error)
	ListRoles(ctx context.Context) ([]database.RolConPermisos, error)
	CreateRol(ctx context.Context, nombre, descripcion string, permisoIDs []int) (*database.RolConPermisos, error)
	UpdateRol(ctx context.Context, rolID int, nombre, descripcion string, permisoIDs []int) error
	DeleteRol(ctx context.Context, rolID int) error
	GetUsuarioAsignaciones(ctx context.Context, usuarioID int64) (*database.UsuarioAsignaciones, error)
	SetUsuarioRoles(ctx context.Context, usuarioID int64, asignaciones []database.AsignacionRolInput, asignadoPor int64) error
	SetUsuarioPermisos(ctx context.Context, usuarioID int64, asignaciones []database.AsignacionPermisoInput, asignadoPor int64) error
	ListCentrosCostos(ctx context.Context) ([]database.CentroCostoItem, error)
	CargarPermisos(ctx context.Context, usuarioID int64) (*auth.PermisosUsuario, error)
	InvalidarCache(usuarioID int64)
}

const msgPermisosFaltantes = "el sistema de permisos no está disponible; aplica scripts/phase5-auth-permisos-roles.sql"

// permisosDisponibles responde 503 y devuelve false si el CRUD de
// roles/permisos todavía no tiene tablas — evita que cada handler repita el
// mismo guard.
func (h *Handlers) permisosDisponibles(w http.ResponseWriter) bool {
	if h.PermisosRepo == nil || !h.PermisosRepo.Disponible() {
		Error(w, http.StatusServiceUnavailable, msgPermisosFaltantes)
		return false
	}
	return true
}

// mapPermisosError traduce los errores de negocio tipados de auth a códigos
// HTTP. errors.Is, nunca comparación de strings.
func mapPermisosError(w http.ResponseWriter, err error, msg string) {
	switch {
	case errors.Is(err, auth.ErrPermisosMissing):
		Error(w, http.StatusServiceUnavailable, msgPermisosFaltantes)
	case errors.Is(err, auth.ErrRolNotFound):
		Error(w, http.StatusNotFound, "rol no encontrado")
	case errors.Is(err, auth.ErrPermisoNotFound):
		Error(w, http.StatusNotFound, "permiso no encontrado")
	case errors.Is(err, auth.ErrCentroCostoNotFound):
		Error(w, http.StatusNotFound, "centro de costo no encontrado")
	case errors.Is(err, auth.ErrRolEsSistema):
		Error(w, http.StatusForbidden, "no se puede modificar ni eliminar un rol de sistema")
	case errors.Is(err, auth.ErrUltimoSuperadmin):
		Error(w, http.StatusConflict, "no puedes quitar el último Superadmin del sistema")
	case errors.Is(err, auth.ErrNoTeQuitesPermisos):
		Error(w, http.StatusForbidden, "no puedes modificar tus propios roles")
	default:
		Error(w, http.StatusInternalServerError, msg+": "+err.Error())
	}
}

// MisPermisos maneja GET /api/v1/auth/permisos.
// Devuelve los permisos efectivos del usuario autenticado (roles + directos).
// Si el sistema de permisos no está disponible responde degraded=true en vez
// de un error, para que el frontend caiga a modo permisivo sin romperse.
func (h *Handlers) MisPermisos(w http.ResponseWriter, r *http.Request) {
	if h.PermisosRepo == nil || !h.PermisosRepo.Disponible() {
		JSON(w, http.StatusOK, &auth.PermisosResponse{
			Global:    []string{},
			PorRancho: map[string][]string{},
			Degraded:  true,
		})
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		Error(w, http.StatusUnauthorized, "autenticación requerida")
		return
	}

	perms, err := h.PermisosRepo.CargarPermisos(r.Context(), claims.UserID)
	if err != nil {
		mapPermisosError(w, err, "cargando permisos")
		return
	}

	JSON(w, http.StatusOK, perms.ToResponse())
}

// RefrescarPermisos maneja POST /api/v1/auth/refrescar-permisos.
// Invalida el caché del usuario autenticado y recarga sus permisos —
// necesario tras un cambio de rol para no esperar a que expire el caché.
func (h *Handlers) RefrescarPermisos(w http.ResponseWriter, r *http.Request) {
	if h.PermisosRepo == nil || !h.PermisosRepo.Disponible() {
		JSON(w, http.StatusOK, &auth.PermisosResponse{
			Global:    []string{},
			PorRancho: map[string][]string{},
			Degraded:  true,
		})
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		Error(w, http.StatusUnauthorized, "autenticación requerida")
		return
	}

	h.PermisosRepo.InvalidarCache(claims.UserID)

	perms, err := h.PermisosRepo.CargarPermisos(r.Context(), claims.UserID)
	if err != nil {
		mapPermisosError(w, err, "recargando permisos")
		return
	}

	JSON(w, http.StatusOK, perms.ToResponse())
}

// ListRoles maneja GET /api/v1/admin/roles.
func (h *Handlers) ListRoles(w http.ResponseWriter, r *http.Request) {
	if !h.permisosDisponibles(w) {
		return
	}
	roles, err := h.PermisosRepo.ListRoles(r.Context())
	if err != nil {
		mapPermisosError(w, err, "listando roles")
		return
	}
	if roles == nil {
		roles = []database.RolConPermisos{}
	}
	JSON(w, http.StatusOK, roles)
}

// rolRequest es el cuerpo de POST/PUT .../roles.
type rolRequest struct {
	Nombre      string `json:"nombre"`
	Descripcion string `json:"descripcion"`
	PermisoIDs  []int  `json:"permiso_ids"`
}

// CreateRol maneja POST /api/v1/admin/roles.
func (h *Handlers) CreateRol(w http.ResponseWriter, r *http.Request) {
	if !h.permisosDisponibles(w) {
		return
	}

	var req rolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "body JSON inválido")
		return
	}
	if strings.TrimSpace(req.Nombre) == "" {
		Error(w, http.StatusBadRequest, "nombre es requerido")
		return
	}

	rol, err := h.PermisosRepo.CreateRol(r.Context(), req.Nombre, req.Descripcion, req.PermisoIDs)
	if err != nil {
		mapPermisosError(w, err, "creando rol")
		return
	}

	JSON(w, http.StatusCreated, rol)
}

// UpdateRol maneja PUT /api/v1/admin/roles/{id}.
func (h *Handlers) UpdateRol(w http.ResponseWriter, r *http.Request) {
	if !h.permisosDisponibles(w) {
		return
	}

	rolID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}

	var req rolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "body JSON inválido")
		return
	}
	if strings.TrimSpace(req.Nombre) == "" {
		Error(w, http.StatusBadRequest, "nombre es requerido")
		return
	}

	if err := h.PermisosRepo.UpdateRol(r.Context(), rolID, req.Nombre, req.Descripcion, req.PermisoIDs); err != nil {
		mapPermisosError(w, err, "actualizando rol")
		return
	}

	JSON(w, http.StatusOK, map[string]any{"rol_id": rolID})
}

// DeleteRol maneja DELETE /api/v1/admin/roles/{id}.
func (h *Handlers) DeleteRol(w http.ResponseWriter, r *http.Request) {
	if !h.permisosDisponibles(w) {
		return
	}

	rolID, ok := parseIntParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.PermisosRepo.DeleteRol(r.Context(), rolID); err != nil {
		mapPermisosError(w, err, "eliminando rol")
		return
	}

	JSON(w, http.StatusOK, map[string]any{"rol_id": rolID, "eliminado": true})
}

// ListPermisosCatalogo maneja GET /api/v1/admin/permisos.
func (h *Handlers) ListPermisosCatalogo(w http.ResponseWriter, r *http.Request) {
	if !h.permisosDisponibles(w) {
		return
	}

	permisos, err := h.PermisosRepo.ListPermisos(r.Context())
	if err != nil {
		mapPermisosError(w, err, "listando catálogo de permisos")
		return
	}
	if permisos == nil {
		permisos = []database.PermisoCatalogo{}
	}
	JSON(w, http.StatusOK, permisos)
}

// GetUsuarioPermisos maneja GET /api/v1/admin/usuarios/{id}/permisos.
// Devuelve las asignaciones actuales (roles + directos) para la UI de edición,
// no los permisos efectivos resueltos (eso es MisPermisos).
func (h *Handlers) GetUsuarioPermisos(w http.ResponseWriter, r *http.Request) {
	if !h.permisosDisponibles(w) {
		return
	}

	usuarioID, ok := parseUserIDParam(w, r)
	if !ok {
		return
	}

	asignaciones, err := h.PermisosRepo.GetUsuarioAsignaciones(r.Context(), usuarioID)
	if err != nil {
		mapPermisosError(w, err, "consultando permisos del usuario")
		return
	}

	JSON(w, http.StatusOK, asignaciones)
}

type setUsuarioRolesRequest struct {
	Asignaciones []database.AsignacionRolInput `json:"asignaciones"`
}

// SetUsuarioRoles maneja PUT /api/v1/admin/usuarios/{id}/roles.
// Reemplaza el conjunto completo de roles asignados al usuario.
//
// Autoprotección: nadie puede modificar sus propios roles por esta vía — de
// lo contrario un usuario con roles.administrar podría quitarse a sí mismo la
// única forma de recuperarlo, o auto-otorgarse Superadmin sin que otro lo
// revise.
func (h *Handlers) SetUsuarioRoles(w http.ResponseWriter, r *http.Request) {
	if !h.permisosDisponibles(w) {
		return
	}

	usuarioID, ok := parseUserIDParam(w, r)
	if !ok {
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		Error(w, http.StatusUnauthorized, "autenticación requerida")
		return
	}
	if claims.UserID == usuarioID {
		mapPermisosError(w, auth.ErrNoTeQuitesPermisos, "asignando roles")
		return
	}

	var req setUsuarioRolesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "body JSON inválido")
		return
	}

	if err := h.PermisosRepo.SetUsuarioRoles(r.Context(), usuarioID, req.Asignaciones, claims.UserID); err != nil {
		mapPermisosError(w, err, "asignando roles")
		return
	}

	JSON(w, http.StatusOK, map[string]any{"usuario_id": usuarioID})
}

type setUsuarioPermisosRequest struct {
	Asignaciones []database.AsignacionPermisoInput `json:"asignaciones"`
}

// SetUsuarioPermisos maneja PUT /api/v1/admin/usuarios/{id}/permisos-directos.
// Reemplaza el conjunto completo de permisos directos (excepciones) del usuario.
func (h *Handlers) SetUsuarioPermisos(w http.ResponseWriter, r *http.Request) {
	if !h.permisosDisponibles(w) {
		return
	}

	usuarioID, ok := parseUserIDParam(w, r)
	if !ok {
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		Error(w, http.StatusUnauthorized, "autenticación requerida")
		return
	}

	var req setUsuarioPermisosRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "body JSON inválido")
		return
	}

	if err := h.PermisosRepo.SetUsuarioPermisos(r.Context(), usuarioID, req.Asignaciones, claims.UserID); err != nil {
		mapPermisosError(w, err, "asignando permisos directos")
		return
	}

	JSON(w, http.StatusOK, map[string]any{"usuario_id": usuarioID})
}

// ListCentrosCostos maneja GET /api/v1/admin/centros-costos.
// Alimenta el selector de rancho en la UI de asignación de roles/permisos.
func (h *Handlers) ListCentrosCostos(w http.ResponseWriter, r *http.Request) {
	if h.PermisosRepo == nil {
		Error(w, http.StatusServiceUnavailable, msgPermisosFaltantes)
		return
	}

	centros, err := h.PermisosRepo.ListCentrosCostos(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "listando centros de costo: "+err.Error())
		return
	}
	if centros == nil {
		centros = []database.CentroCostoItem{}
	}
	JSON(w, http.StatusOK, centros)
}

// parseIntParam parsea el path value con nombre `name` como int (rol_id,
// permiso_id). A diferencia de parseUintParam/parseInt64Param admite el 0 y
// negativos como "inválido" pero no fuerza uint/int64 — los IDs de este CRUD
// son AUTO_INCREMENT INT normales.
func parseIntParam(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	raw := r.PathValue(name)
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		Error(w, http.StatusBadRequest, "invalid "+name)
		return 0, false
	}
	return v, true
}
