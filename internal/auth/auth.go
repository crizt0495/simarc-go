package auth

import (
	"crypto/subtle"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/argon2"
)

// Argon2id hash
func HashPassword(password string, salt []byte) string {
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)
	return fmt.Sprintf("%x", hash)
}

func VerifyPassword(hash string, password string, salt []byte) bool {
	h := HashPassword(password, salt)
	return subtle.ConstantTimeCompare([]byte(hash), []byte(h)) == 1
}

// JWT claims
func GenerateToken(userID string, secret []byte, exp time.Duration) (string, error) {
	claims := jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(exp).Unix(),
		"iat": time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}
