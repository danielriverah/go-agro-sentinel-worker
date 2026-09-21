package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// jwtClaims es la estructura interna del payload del token.
type jwtClaims struct {
	Username string `json:"usr"`
	jwt.RegisteredClaims
}

// GenerateToken emite un JWT HS256 firmado con secretKey válido por ttl.
//
// Recibe:  *User, secretKey (mínimo 32 bytes), ttl.
// Retorna: token string listo para enviar al cliente y la hora de expiración.
func GenerateToken(user *User, secretKey []byte, ttl time.Duration) (tokenStr string, expiresAt time.Time, err error) {
	expiresAt = time.Now().Add(ttl)
	claims := jwtClaims{
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", user.ID),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err = token.SignedString(secretKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("firmando token: %w", err)
	}
	return tokenStr, expiresAt, nil
}

// ValidateToken verifica la firma y la expiración del token.
//
// Recibe:  tokenString (valor crudo sin "Bearer "), secretKey.
// Retorna: *Claims con UserID y Username, o uno de:
//   - ErrTokenMissing  → tokenString vacío
//   - ErrTokenExpired  → token expirado
//   - ErrTokenInvalid  → firma inválida, malformado o algoritmo distinto
func ValidateToken(tokenString string, secretKey []byte) (*Claims, error) {
	if tokenString == "" {
		return nil, ErrTokenMissing
	}

	token, err := jwt.ParseWithClaims(
		tokenString,
		&jwtClaims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("algoritmo inesperado: %v", t.Header["alg"])
			}
			return secretKey, nil
		},
	)

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}

	c, ok := token.Claims.(*jwtClaims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}

	var userID int64
	fmt.Sscanf(c.Subject, "%d", &userID)
	if userID == 0 {
		return nil, ErrTokenInvalid
	}

	return &Claims{
		UserID:   userID,
		Username: c.Username,
	}, nil
}

// ClaimsFromContext extrae los Claims inyectados por el middleware.
// Retorna nil si el contexto no fue autenticado.
func ClaimsFromContext(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsKey).(*Claims)
	return c
}
