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
)

// codeToErr mapea el campo "code" del JSON retornado por las funciones MySQL
// a los errores Go tipados. Los códigos no listados aquí retornan nil (error desconocido).
var codeToErr = map[string]error{
	"USER_NOT_FOUND":    ErrUserNotFound,
	"WRONG_PASSWORD":    ErrWrongPassword,
	"USER_INACTIVE":     ErrUserInactive,
	"USERNAME_TAKEN":    ErrUsernameTaken,
	"USERNAME_EMPTY":    ErrUsernameEmpty,
	"PASSWORD_TOO_SHORT": ErrPasswordTooShort,
}
