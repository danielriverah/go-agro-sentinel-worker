package auth

// User representa al usuario autenticado. Solo contiene lo necesario para
// emitir el token; el hash y salt nunca salen del repo.
type User struct {
	ID       int64
	Username string
	Activo   bool
}

// Claims es lo que el middleware inyecta en el contexto tras validar el token.
type Claims struct {
	UserID   int64
	Username string
}

// contextKey es el tipo privado para las claves del context.Context,
// evitando colisiones con otros paquetes.
type contextKey int

const claimsKey contextKey = 0
