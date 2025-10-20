package auth

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func HashPassword(password string) (string, error) {

	hash, err := argon2id.CreateHash(password, argon2id.DefaultParams)

	return hash, err

}

func CheckPasswordHash(password, hash string) (bool, error) {

	match, err := argon2id.ComparePasswordAndHash(password, hash)

	return match, err
}

func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    "chirpy",
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiresIn)),
		Subject:   userID.String(),
	})

	signed, err := token.SignedString([]byte(tokenSecret))

	return signed, err
}

func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
	var claims jwt.RegisteredClaims

	token, err := jwt.ParseWithClaims(tokenString, &claims, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(tokenSecret), nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	if !token.Valid {
		return uuid.Nil, fmt.Errorf("invalid token")
	}

	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

/*

look for the Authorization header in the headers parameter and
return the TOKEN_STRING if it exists (stripping off the Bearer
prefix and whitespace). If the header doesn't exist, return an error.

*/

func GetBearerToken(headers http.Header) (string, error) {

	auth := headers.Get("Authorization")
	if auth == "" {
		// return error
	}
	// expect "Bearer <token>"
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		// return error
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, prefix))
	// return token
}
