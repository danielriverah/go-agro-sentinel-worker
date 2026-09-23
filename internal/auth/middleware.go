package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// Middleware retorna un http.Handler que valida el JWT en cada request.
//
// El token se busca en este orden:
//  1. Header: Authorization: Bearer <token>
//  2. Cookie: agro_token=<token>
//
// Rutas públicas (no requieren token):
//   - POST /api/v1/auth/login
//   - GET  /health
//   - GET  /health/dependencies
//   - GET  /docs
//   - GET  /api/v1/openapi.yaml
//
// En caso de token válido, inyecta *Claims en el contexto con claimsKey.
// En caso de error, responde JSON con el código HTTP apropiado y no llama next.
func Middleware(secretKey []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicPath(r) {
				next.ServeHTTP(w, r)
				return
			}

			tokenStr := extractToken(r)
			claims, err := ValidateToken(tokenStr, secretKey)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				switch err {
				case ErrTokenMissing:
					w.WriteHeader(http.StatusUnauthorized)
					json.NewEncoder(w).Encode(map[string]string{"error": "autenticación requerida"})
				case ErrTokenExpired:
					w.WriteHeader(http.StatusUnauthorized)
					json.NewEncoder(w).Encode(map[string]string{"error": "sesión expirada, inicia sesión de nuevo"})
				default: // ErrTokenInvalid
					w.WriteHeader(http.StatusUnauthorized)
					json.NewEncoder(w).Encode(map[string]string{"error": "token inválido"})
				}
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin protege las rutas de administración.
//
// Hoy sólo exige una sesión válida: el modelo de usuarios todavía no tiene el
// concepto de rol. Es el punto de extensión previsto — cuando exista la
// columna es_admin y viaje en los Claims, basta con comprobarla aquí; ni las
// rutas ni los manejadores cambian.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ClaimsFromContext(r.Context()) == nil {
			writeAuthError(w, http.StatusUnauthorized, "autenticación requerida")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireUserIn restringe una ruta a una lista explícita de usuarios.
//
// Existe para el borrado de monitoreo: es irreversible y toca MySQL, S3 y
// DynamoDB, así que tener sesión no basta mientras no exista un rol
// persistido. Es una medida puente — cuando llegue es_admin, esta lista se
// sustituye por la comprobación del rol.
//
// Una lista vacía deniega a todos: si el servicio se despliega sin configurar
// AUTH_DELETE_ALLOWED_USER_IDS, la operación queda cerrada, nunca abierta.
func RequireUserIn(allowed []int64) func(http.Handler) http.Handler {
	permitidos := make(map[int64]struct{}, len(allowed))
	for _, id := range allowed {
		permitidos[id] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				writeAuthError(w, http.StatusUnauthorized, "autenticación requerida")
				return
			}
			if _, ok := permitidos[claims.UserID]; !ok {
				writeAuthError(w, http.StatusForbidden,
					"tu usuario no está autorizado para esta operación; se configura en AUTH_DELETE_ALLOWED_USER_IDS")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeAuthError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// extractToken extrae el token del header Authorization o de la cookie agro_token.
func extractToken(r *http.Request) string {
	// 1. Header: Authorization: Bearer <token>
	if h := r.Header.Get("Authorization"); h != "" {
		if strings.HasPrefix(h, "Bearer ") {
			return strings.TrimPrefix(h, "Bearer ")
		}
	}
	// 2. Cookie fallback
	if c, err := r.Cookie("agro_token"); err == nil {
		return c.Value
	}
	return ""
}

// isPublicPath reporta si la ruta no requiere autenticación.
func isPublicPath(r *http.Request) bool {
	path := r.URL.Path
	switch {
	case r.Method == http.MethodPost && path == "/api/v1/auth/login":
		return true
	case path == "/health", path == "/health/dependencies":
		return true
	case path == "/docs", path == "/api/v1/openapi.yaml":
		return true
	}
	return false
}
