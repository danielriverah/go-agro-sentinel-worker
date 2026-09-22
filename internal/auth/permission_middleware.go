package auth

import (
	"context"
	"net/http"
	"strconv"
)

// PermissionLoader loads or returns cached effective permissions for a user.
type PermissionLoader interface {
	Disponible() bool
	CargarPermisos(ctx context.Context, usuarioID int64) (*PermisosUsuario, error)
	ObtenerPermisosCacheados(usuarioID int64) *PermisosUsuario
}

// CentroCostoResolver resolves a produccion monitoring ID to its centro_costo_id.
type CentroCostoResolver interface {
	GetCentroCostoByMonitoringID(ctx context.Context, monitoringID uint) (*int64, error)
}

func getOrLoadPermisos(ctx context.Context, loader PermissionLoader, userID int64) (*PermisosUsuario, error) {
	if p := loader.ObtenerPermisosCacheados(userID); p != nil {
		return p, nil
	}
	return loader.CargarPermisos(ctx, userID)
}

// RequireGlobalPermission checks a global-scope permission (no ranch needed).
// If the permission tables don't exist, it passes through (permissive mode).
func RequireGlobalPermission(permiso string, loader PermissionLoader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !loader.Disponible() {
				next.ServeHTTP(w, r)
				return
			}

			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				writeAuthError(w, http.StatusUnauthorized, "autenticación requerida")
				return
			}

			perms, err := getOrLoadPermisos(r.Context(), loader, claims.UserID)
			if err != nil {
				writeAuthError(w, http.StatusInternalServerError, "error cargando permisos")
				return
			}

			if !perms.TienePermisoGlobal(permiso) {
				writeAuthError(w, http.StatusForbidden, "no tienes permiso: "+permiso)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermission checks a ranch-scoped permission by resolving the {id}
// path parameter to a centro_costo_id.
// If the permission tables don't exist, it passes through (permissive mode).
func RequirePermission(permiso string, loader PermissionLoader, resolver CentroCostoResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !loader.Disponible() {
				next.ServeHTTP(w, r)
				return
			}

			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				writeAuthError(w, http.StatusUnauthorized, "autenticación requerida")
				return
			}

			perms, err := getOrLoadPermisos(r.Context(), loader, claims.UserID)
			if err != nil {
				writeAuthError(w, http.StatusInternalServerError, "error cargando permisos")
				return
			}

			idStr := r.PathValue("id")
			id, _ := strconv.ParseUint(idStr, 10, 64)
			if id == 0 {
				writeAuthError(w, http.StatusBadRequest, "id inválido")
				return
			}

			ccID, err := resolver.GetCentroCostoByMonitoringID(r.Context(), uint(id))
			if err != nil {
				writeAuthError(w, http.StatusInternalServerError, "error resolviendo rancho")
				return
			}

			if !perms.TienePermiso(permiso, ccID) {
				writeAuthError(w, http.StatusForbidden, "no tienes permiso: "+permiso)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
