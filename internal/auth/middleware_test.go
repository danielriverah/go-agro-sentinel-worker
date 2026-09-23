package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func conClaims(r *http.Request, userID int64) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), claimsKey, &Claims{UserID: userID, Username: "u"}))
}

func ejecutar(t *testing.T, allowed []int64, req *http.Request) (int, bool) {
	t.Helper()
	llamado := false
	h := RequireUserIn(allowed)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		llamado = true
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code, llamado
}

// Desplegar sin configurar la lista no debe dejar abierta una operación
// irreversible: por omisión se cierra, no se abre.
func TestRequireUserInListaVaciaDeniegaATodos(t *testing.T) {
	req := conClaims(httptest.NewRequest("DELETE", "/x", nil), 1)

	code, llamado := ejecutar(t, nil, req)

	if code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", code)
	}
	if llamado {
		t.Error("el handler no debió ejecutarse con la lista vacía")
	}
}

func TestRequireUserInRechazaUsuarioNoListado(t *testing.T) {
	req := conClaims(httptest.NewRequest("DELETE", "/x", nil), 99)

	code, llamado := ejecutar(t, []int64{1, 2, 3}, req)

	if code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", code)
	}
	if llamado {
		t.Error("el handler no debió ejecutarse para un usuario fuera de la lista")
	}
}

func TestRequireUserInPermiteUsuarioListado(t *testing.T) {
	req := conClaims(httptest.NewRequest("DELETE", "/x", nil), 2)

	code, llamado := ejecutar(t, []int64{1, 2, 3}, req)

	if code != http.StatusOK {
		t.Errorf("status = %d, want 200", code)
	}
	if !llamado {
		t.Error("el handler debió ejecutarse para un usuario autorizado")
	}
}

// Sin sesión la respuesta es 401, no 403: distinguirlas importa para que el
// frontend sepa si debe reautenticar o mostrar "no autorizado".
func TestRequireUserInSinSesionDevuelve401(t *testing.T) {
	req := httptest.NewRequest("DELETE", "/x", nil)

	code, llamado := ejecutar(t, []int64{1}, req)

	if code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", code)
	}
	if llamado {
		t.Error("el handler no debió ejecutarse sin sesión")
	}
}
