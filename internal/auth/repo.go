package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

// Repo llama las funciones/procedimientos MySQL de autenticación.
// Nunca hace SELECT directo a la tabla de usuarios.
type Repo struct {
	db *sql.DB
}

// NewRepo crea un Repo con la conexión MySQL dada.
func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

// dbResult es la forma que comparten todas las respuestas JSON de las funciones MySQL.
type dbResult struct {
	OK       bool   `json:"ok"`
	Code     string `json:"code"`
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Activo   int    `json:"activo"`
}

// mapCode convierte el campo "code" del JSON al error Go tipado.
// Si el código no está en el mapa retorna un error genérico con el código original.
func mapCode(code string) error {
	if err, ok := codeToErr[code]; ok {
		return err
	}
	return fmt.Errorf("función MySQL retornó código desconocido: %q", code)
}

// scanFunction escanea el resultado de una FUNCTION MySQL (SELECT fn_...(?, ?)).
func (r *Repo) scanFunction(ctx context.Context, query string, args ...any) (*dbResult, error) {
	var raw string
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}
	var res dbResult
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return nil, fmt.Errorf("json inesperado de función MySQL: %w", err)
	}
	return &res, nil
}

// scanProcedure escanea el result set de un PROCEDURE MySQL (CALL sp_...(?, ?)).
func (r *Repo) scanProcedure(ctx context.Context, query string, args ...any) (*dbResult, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}
	defer rows.Close()

	var raw string
	if rows.Next() {
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
	}
	if raw == "" {
		return nil, fmt.Errorf("procedimiento MySQL retornó resultado vacío")
	}
	var res dbResult
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return nil, fmt.Errorf("json inesperado de procedimiento MySQL: %w", err)
	}
	return &res, nil
}

// ValidateLogin verifica credenciales llamando fn_validate_login.
//
// Recibe:  username y password en texto plano.
// Retorna: *User, nil si las credenciales son correctas.
//
// Errores de negocio: ErrUserNotFound, ErrWrongPassword, ErrUserInactive.
// Otros errores indican fallo de BD (responder 500).
func (r *Repo) ValidateLogin(ctx context.Context, username, password string) (*User, error) {
	res, err := r.scanFunction(ctx, "SELECT fn_validate_login(?, ?)", username, password)
	if err != nil {
		return nil, err
	}
	if !res.OK {
		return nil, mapCode(res.Code)
	}
	return &User{
		ID:       res.UserID,
		Username: res.Username,
		Activo:   res.Activo == 1,
	}, nil
}

// GetUsuario obtiene datos de un usuario por ID llamando fn_get_usuario.
// Usado para refrescar datos sin re-validar contraseña.
//
// Errores de negocio: ErrUserNotFound.
func (r *Repo) GetUsuario(ctx context.Context, userID int64) (*User, error) {
	res, err := r.scanFunction(ctx, "SELECT fn_get_usuario(?)", userID)
	if err != nil {
		return nil, err
	}
	if !res.OK {
		return nil, mapCode(res.Code)
	}
	return &User{
		ID:       res.UserID,
		Username: res.Username,
		Activo:   res.Activo == 1,
	}, nil
}

// CreateUsuario crea un nuevo usuario llamando sp_create_usuario.
//
// Recibe:  username y password en texto plano (el SP hace el hash SHA2-256 + salt).
// Retorna: *User del usuario creado.
//
// Errores de negocio: ErrUsernameTaken, ErrUsernameEmpty, ErrPasswordTooShort.
func (r *Repo) CreateUsuario(ctx context.Context, username, password string) (*User, error) {
	res, err := r.scanProcedure(ctx, "CALL sp_create_usuario(?, ?)", username, password)
	if err != nil {
		return nil, err
	}
	if !res.OK {
		return nil, mapCode(res.Code)
	}
	return &User{
		ID:       res.UserID,
		Username: res.Username,
		Activo:   true,
	}, nil
}

// UsuarioAdmin es una fila de la lista de administración. Nunca lleva hash ni
// salt: esos no salen de la base.
type UsuarioAdmin struct {
	ID            int64  `json:"user_id"`
	Username      string `json:"username"`
	Activo        int    `json:"activo"`
	FechaCreacion string `json:"fecha_creacion"`
}

// listResult es la forma que devuelve fn_list_usuarios.
type listResult struct {
	OK       bool           `json:"ok"`
	Code     string         `json:"code"`
	Usuarios []UsuarioAdmin `json:"usuarios"`
}

// procMissing traduce el error de MySQL "el procedimiento no existe" al error
// de negocio, para que la API pueda decir qué script falta por aplicar en vez
// de devolver un 500 opaco.
func procMissing(err error) bool {
	var myErr *mysql.MySQLError
	if !errors.As(err, &myErr) {
		return false
	}
	// 1305 = ER_SP_DOES_NOT_EXIST, 1370 = ER_PROCACCESS_DENIED_ERROR
	return myErr.Number == 1305 || myErr.Number == 1370
}

// ListUsuarios devuelve todos los usuarios llamando fn_list_usuarios.
//
// Error de negocio: ErrAdminProcsMissing cuando el procedimiento no existe.
func (r *Repo) ListUsuarios(ctx context.Context) ([]UsuarioAdmin, error) {
	var raw string
	if err := r.db.QueryRowContext(ctx, "SELECT fn_list_usuarios()").Scan(&raw); err != nil {
		if procMissing(err) {
			return nil, ErrAdminProcsMissing
		}
		return nil, fmt.Errorf("db: %w", err)
	}

	var res listResult
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return nil, fmt.Errorf("json inesperado de fn_list_usuarios: %w", err)
	}
	if !res.OK {
		return nil, mapCode(res.Code)
	}
	return res.Usuarios, nil
}

// SetUsuarioActivo activa o desactiva un usuario llamando sp_set_usuario_activo.
//
// Errores de negocio: ErrUserNotFound, ErrLastActiveUser, ErrAdminProcsMissing.
func (r *Repo) SetUsuarioActivo(ctx context.Context, userID int64, activo bool) error {
	flag := 0
	if activo {
		flag = 1
	}

	res, err := r.scanProcedure(ctx, "CALL sp_set_usuario_activo(?, ?)", userID, flag)
	if err != nil {
		if procMissing(err) {
			return ErrAdminProcsMissing
		}
		return err
	}
	if !res.OK {
		return mapCode(res.Code)
	}
	return nil
}

// AdminResetPassword restablece la contraseña sin conocer la actual, llamando
// sp_admin_reset_password. Es lo que distingue a esta operación de
// ChangePassword, que exige la contraseña vigente.
//
// Errores de negocio: ErrUserNotFound, ErrPasswordTooShort, ErrAdminProcsMissing.
func (r *Repo) AdminResetPassword(ctx context.Context, userID int64, nueva string) error {
	res, err := r.scanProcedure(ctx, "CALL sp_admin_reset_password(?, ?)", userID, nueva)
	if err != nil {
		if procMissing(err) {
			return ErrAdminProcsMissing
		}
		return err
	}
	if !res.OK {
		return mapCode(res.Code)
	}
	return nil
}

// ChangePassword cambia la contraseña llamando sp_change_password.
// Requiere la contraseña actual para confirmar identidad.
//
// Errores de negocio: ErrUserNotFound, ErrUserInactive, ErrWrongPassword, ErrPasswordTooShort.
func (r *Repo) ChangePassword(ctx context.Context, userID int64, passwordActual, passwordNueva string) error {
	res, err := r.scanProcedure(ctx,
		"CALL sp_change_password(?, ?, ?)", userID, passwordActual, passwordNueva,
	)
	if err != nil {
		return err
	}
	if !res.OK {
		return mapCode(res.Code)
	}
	return nil
}
