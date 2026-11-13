// Package auth: хеширование паролей, JWT-сессии и проверка контактов.
package auth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrWeakPassword = errors.New("password is too short")
)

type TokenManager struct {
	secret []byte
	ttl    time.Duration
}

func NewTokenManager(secret string, ttl time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), ttl: ttl}
}

func (m *TokenManager) TTL() time.Duration { return m.ttl }

func (m *TokenManager) Issue(userID int64) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   fmt.Sprintf("%d", userID),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		Issuer:    "pro-remont",
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

func (m *TokenManager) Parse(token string) (int64, error) {
	parsed, err := jwt.ParseWithClaims(token, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return m.secret, nil
	}, jwt.WithIssuer("pro-remont"), jwt.WithExpirationRequired())
	if err != nil || !parsed.Valid {
		return 0, ErrInvalidToken
	}
	claims, ok := parsed.Claims.(*jwt.RegisteredClaims)
	if !ok {
		return 0, ErrInvalidToken
	}
	var id int64
	if _, err := fmt.Sscanf(claims.Subject, "%d", &id); err != nil || id <= 0 {
		return 0, ErrInvalidToken
	}
	return id, nil
}

func HashPassword(password string) (string, error) {
	if len([]rune(password)) < 6 {
		return "", ErrWeakPassword
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

var emailRe = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func NormalizeEmail(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) > 254 || !emailRe.MatchString(s) {
		return "", false
	}
	return s, true
}

// NormalizePhone приводит российские и международные номера к виду +7XXXXXXXXXX / +...
func NormalizePhone(s string) (string, bool) {
	digits := make([]rune, 0, 16)
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) == 0 {
		return "", false
	}
	d := string(digits)
	if len(d) == 11 && d[0] == '8' {
		d = "7" + d[1:]
	}
	if len(d) == 10 && d[0] == '9' {
		d = "7" + d
	}
	if len(d) < 10 || len(d) > 15 {
		return "", false
	}
	return "+" + d, true
}

func IsEmail(s string) bool {
	_, ok := NormalizeEmail(s)
	return ok
}

// GeneratePassword создаёт читаемый временный пароль для сотрудника.
func GeneratePassword() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	b := make([]byte, 12)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b), nil
}
