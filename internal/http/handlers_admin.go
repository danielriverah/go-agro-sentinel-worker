package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"agro-sentinel-worker/internal/auth"
)

const msgProcsFaltantes = "los procedimientos de administración no existen todavía; aplica scripts/phase3-usuarios-admin-procedures.sql"

// ListUsuarios maneja GET /api/v1/admin/usuarios.
func (h *AuthHandlers) ListUsuarios(w http.ResponseWriter, r *http.Request) {
	usuarios, err := h.Repo.ListUsuarios(r.Context())
	if err != nil {
		if errors.Is(err, auth.ErrAdminProcsMissing) {
			Error(w, http.StatusServiceUnavailable, msgProcsFaltantes)
			return
		}
		h.Log.Error("listando usuarios", "error", err)
		Error(w, http.StatusInternalServerError, "no se pudieron listar los usuarios")
		return
	}

	if usuarios == nil {
		usuarios = []auth.UsuarioAdmin{}
	}
	JSON(w, http.StatusOK, usuarios)
}

type setActivoRequest struct {
	Activo bool `json:"activo"`
}

// SetUsuarioActivo maneja PUT /api/v1/admin/usuarios/{id}/activo.
func (h *AuthHandlers) SetUsuarioActivo(w http.ResponseWriter, r *http.Request) {
	userID, ok := parseUserIDParam(w, r)
	if !ok {
		return
	}

	var req setActivoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "body JSON inválido")
		return
	}

	// Desactivarse a uno mismo cierra la sesión en curso y, si además es el
	// último activo, deja el sistema sin acceso. Se corta aquí para poder dar
	// un mensaje claro, aunque la base también lo impida.
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil && claims.UserID == userID && !req.Activo {
		Error(w, http.StatusBadRequest, "no puedes desactivar tu propio usuario")
		return
	}

	if err := h.Repo.SetUsuarioActivo(r.Context(), userID, req.Activo); err != nil {
		switch {
		case errors.Is(err, auth.ErrAdminProcsMissing):
			Error(w, http.StatusServiceUnavailable, msgProcsFaltantes)
		case errors.Is(err, auth.ErrUserNotFound):
			Error(w, http.StatusNotFound, "usuario no encontrado")
		case errors.Is(err, auth.ErrLastActiveUser):
			Error(w, http.StatusBadRequest, "no puedes desactivar al último usuario activo")
		default:
			h.Log.Error("cambiando estado de usuario", "user_id", userID, "error", err)
			Error(w, http.StatusInternalServerError, "no se pudo cambiar el estado del usuario")
		}
		return
	}

	JSON(w, http.StatusOK, map[string]any{"user_id": userID, "activo": req.Activo})
}

type resetPasswordRequest struct {
	PasswordNueva string `json:"password_nueva"`
}

// ResetPassword maneja PUT /api/v1/admin/usuarios/{id}/password.
// A diferencia de ChangePassword, no exige la contraseña actual.
func (h *AuthHandlers) ResetPassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := parseUserIDParam(w, r)
	if !ok {
		return
	}

	var req resetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "body JSON inválido")
		return
	}
	if req.PasswordNueva == "" {
		Error(w, http.StatusBadRequest, "password_nueva es requerida")
		return
	}

	if err := h.Repo.AdminResetPassword(r.Context(), userID, req.PasswordNueva); err != nil {
		switch {
		case errors.Is(err, auth.ErrAdminProcsMissing):
			Error(w, http.StatusServiceUnavailable, msgProcsFaltantes)
		case errors.Is(err, auth.ErrUserNotFound):
			Error(w, http.StatusNotFound, "usuario no encontrado")
		case errors.Is(err, auth.ErrPasswordTooShort):
			Error(w, http.StatusBadRequest, "la contraseña es demasiado corta")
		default:
			// Sin datos del cuerpo en el log: la contraseña nueva nunca se registra.
			h.Log.Error("restableciendo contraseña", "user_id", userID, "error", err)
			Error(w, http.StatusInternalServerError, "no se pudo restablecer la contraseña")
		}
		return
	}

	JSON(w, http.StatusOK, map[string]any{"user_id": userID})
}

func parseUserIDParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		Error(w, http.StatusBadRequest, "id de usuario inválido")
		return 0, false
	}
	return id, true
}
