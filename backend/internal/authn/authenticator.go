package authn

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const TokenLifetime = 8 * time.Hour

var ErrInvalidToken = errors.New("invalid bearer token")

type Identity struct {
	Subject string
	Email   string
}

type Authenticator interface {
	Authenticate(ctx context.Context, token string) (Identity, error)
}

type TokenIssuer interface {
	Issue(identity Identity) (token string, expiresAt time.Time, err error)
}

type HMACJWT struct {
	secret []byte
	now    func() time.Time
}

type claims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

func NewHMACJWT(secret string) (*HMACJWT, error) {
	if len([]byte(secret)) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must contain at least 32 bytes")
	}
	return &HMACJWT{secret: []byte(secret), now: time.Now}, nil
}

func (a *HMACJWT) Issue(identity Identity) (string, time.Time, error) {
	if strings.TrimSpace(identity.Subject) == "" || strings.TrimSpace(identity.Email) == "" {
		return "", time.Time{}, fmt.Errorf("JWT subject and email are required")
	}
	now := a.now().UTC()
	expiresAt := now.Add(TokenLifetime)
	claims := claims{
		Email: identity.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   identity.Subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign JWT: %w", err)
	}
	return token, expiresAt, nil
}

func (a *HMACJWT) Authenticate(ctx context.Context, rawToken string) (Identity, error) {
	if err := ctx.Err(); err != nil {
		return Identity{}, err
	}
	if strings.TrimSpace(rawToken) == "" || len(rawToken) > 8192 {
		return Identity{}, ErrInvalidToken
	}

	parsed, err := jwt.ParseWithClaims(rawToken, &claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return a.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return Identity{}, ErrInvalidToken
	}
	parsedClaims, ok := parsed.Claims.(*claims)
	if !ok || !parsed.Valid || strings.TrimSpace(parsedClaims.Subject) == "" || strings.TrimSpace(parsedClaims.Email) == "" {
		return Identity{}, ErrInvalidToken
	}
	return Identity{Subject: parsedClaims.Subject, Email: parsedClaims.Email}, nil
}
