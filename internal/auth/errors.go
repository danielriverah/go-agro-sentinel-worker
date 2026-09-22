package auth

import "errors"

// Errores de negocio exportados — el handler usa errors.Is(), nunca parsea strings.
var (
	// Credenciales / usuario
	ErrUserNotFound  = errors.New("USER_NOT_FOUND")
	ErrWrongPassword = errors.New("WRONG_PASSWORD")
	ErrUserInactive  = errors.New("USER_INACTIVE")

	// Creación / cambio de contraseña
	ErrUsernameTaken    = errors.New("USERNAME_TAKEN")
	ErrUsernameEmpty    = errors.New("USERNAME_EMPTY")
	ErrPasswordTooShort = errors.New("PASSWORD_TOO_SHORT")

	// Token JWT
	ErrTokenMissing = errors.New("TOKEN_MISSING")
	ErrTokenExpired = errors.New("TOKEN_EXPIRED")
	ErrTokenInvalid = errors.New("TOKEN_INVALID")

	// Administración de usuarios
	ErrLastActiveUser = errors.New("LAST_ACTIVE_USER")
	// ErrAdminProcsMissing indica que el DBA todavía no aplicó
	// scripts/phase3-usuarios-admin-procedures.sql.
	ErrAdminProcsMissing = errors.New("ADMIN_PROCS_MISSING")

	// Permisos y roles
	ErrPermisosMissing     = errors.New("PERMISOS_TABLES_MISSING")
	ErrRolEsSistema        = errors.New("ROL_ES_SISTEMA")
	ErrUltimoSuperadmin    = errors.New("ULTIMO_SUPERADMIN")
	ErrNoTeQuitesPermisos  = errors.New("NO_TE_QUITES_PERMISOS")
	ErrRolNotFound         = errors.New("ROL_NOT_FOUND")
	ErrPermisoNotFound     = errors.New("PERMISO_NOT_FOUND")
	ErrCentroCostoNotFound = errors.New("CENTRO_COSTO_NOT_FOUND")
)

// codeToErr mapea el campo "code" del JSON retornado por las funciones MySQL
// a los errores Go tipados. Los códigos no listados aquí retornan nil (error desconocido).
var codeToErr = map[string]error{
	"USER_NOT_FOUND":     ErrUserNotFound,
	"WRONG_PASSWORD":     ErrWrongPassword,
	"USER_INACTIVE":      ErrUserInactive,
	"USERNAME_TAKEN":     ErrUsernameTaken,
	"USERNAME_EMPTY":     ErrUsernameEmpty,
	"PASSWORD_TOO_SHORT": ErrPasswordTooShort,
	"LAST_ACTIVE_USER":   ErrLastActiveUser,
	"ROL_ES_SISTEMA":     ErrRolEsSistema,
	"ULTIMO_SUPERADMIN":  ErrUltimoSuperadmin,
	"ROL_NOT_FOUND":      ErrRolNotFound,
}
