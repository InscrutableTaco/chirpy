package auth

import (
	"github.com/alexedwards/argon2id"
)

func CheckPasswordHash(password, hash string) (bool, error) {

	match, err := argon2id.ComparePasswordAndHash(password, hash)

	return match, err
}
