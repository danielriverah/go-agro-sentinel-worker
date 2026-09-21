package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"agro-sentinel-worker/internal/auth"
)

// AuthHandlers agrupa los handlers de autenticación y sus dependencias.
type AuthHandlers struct {
	Repo      *auth.Repo
	SecretKey []byte
	TokenTTL  time.Duration
	Log       *slog.Logger
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	Username  string `json:"username"`
}

// Login maneja POST /api/v1/auth/login.
//
// 200 — credenciales correctas → {token, expires_at, username}
// 400 — body inválido o campos vacíos
// 401 — usuario no existe O contraseña incorrecta (mismo mensaje)
// 403 — cuenta desactivada
// 500 — fallo de BD
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "body JSON inválido")
		return
	}
	if req.Username == "" || req.Password == "" {
		Error(w, http.StatusBadRequest, "username y password son requeridos")
		return
	}

	user, err := h.Repo.ValidateLogin(r.Context(), req.Username, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrUserNotFound),
			errors.Is(err, auth.ErrWrongPassword):
			// Mismo mensaje para ambos — no revelar si el usuario existe.
			Error(w, http.StatusUnauthorized, "credenciales inválidas")
		case errors.Is(err, auth.ErrUserInactive):
			Error(w, http.StatusForbidden, "cuenta desactivada")
		default:
			h.Log.Error("login db error", "error", err)
			Error(w, http.StatusInternalServerError, "error interno")
		}
		return
	}

	tokenStr, expiresAt, err := auth.GenerateToken(user, h.SecretKey, h.TokenTTL)
	if err != nil {
		h.Log.Error("generate token error", "error", err)
		Error(w, http.StatusInternalServerError, "error interno")
		return
	}

	JSON(w, http.StatusOK, loginResponse{
		Token:     tokenStr,
		ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
		Username:  user.Username,
	})
}

type changePasswordRequest struct {
	PasswordActual string `json:"password_actual"`
	PasswordNueva  string `json:"password_nueva"`
}

// ChangePassword maneja POST /api/v1/auth/change-password.
// Requiere token válido (usuario autenticado cambia su propia contraseña).
//
// 200 — contraseña cambiada
// 400 — body inválido o campos vacíos
// 401 — contraseña actual incorrecta
// 403 — cuenta desactivada
// 500 — fallo de BD
func (h *AuthHandlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		Error(w, http.StatusUnauthorized, "autenticación requerida")
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "body JSON inválido")
		return
	}
	if req.PasswordActual == "" || req.PasswordNueva == "" {
		Error(w, http.StatusBadRequest, "password_actual y password_nueva son requeridos")
		return
	}

	err := h.Repo.ChangePassword(r.Context(), claims.UserID, req.PasswordActual, req.PasswordNueva)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrWrongPassword):
			Error(w, http.StatusUnauthorized, "contraseña actual incorrecta")
		case errors.Is(err, auth.ErrUserInactive):
			Error(w, http.StatusForbidden, "cuenta desactivada")
		case errors.Is(err, auth.ErrPasswordTooShort):
			Error(w, http.StatusBadRequest, "la nueva contraseña debe tener al menos 8 caracteres")
		default:
			h.Log.Error("change password db error", "error", err)
			Error(w, http.StatusInternalServerError, "error interno")
		}
		return
	}

	JSON(w, http.StatusOK, map[string]string{"status": "contraseña actualizada"})
}
