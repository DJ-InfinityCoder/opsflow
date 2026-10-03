package authn

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-for-hs256-that-is-at-least-32-bytes"

func TestAuthenticateRejectsInvalidTokens(t *testing.T) {
	authenticator, err := NewHMACJWT(testSecret)
	if err != nil {
		t.Fatal(err)
	}

	expired := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		Email: "person@example.test",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "00000000-0000-4000-8000-000000000001",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
	})
	expiredToken, err := expired.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}

	validToken, _, err := authenticator.Issue(Identity{
		Subject: "00000000-0000-4000-8000-000000000002",
		Email:   "person@example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	issuedToken, err := jwt.ParseWithClaims(validToken, &claims{}, func(_ *jwt.Token) (any, error) {
		return []byte(testSecret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		t.Fatalf("parse issued JWT claims: %v", err)
	}
	issuedClaims, ok := issuedToken.Claims.(*claims)
	if !ok || issuedClaims.IssuedAt == nil || issuedClaims.ExpiresAt == nil {
		t.Fatal("issued token is missing issued-at or expiration claims")
	}
	if lifetime := issuedClaims.ExpiresAt.Time.Sub(issuedClaims.IssuedAt.Time); lifetime != TokenLifetime {
		t.Fatalf("expected token lifetime %s, got %s", TokenLifetime, lifetime)
	}
	signatureStart := strings.LastIndex(validToken, ".") + 1
	replacement := byte('a')
	if validToken[signatureStart] == replacement {
		replacement = 'b'
	}
	tamperedToken := validToken[:signatureStart] + string(replacement) + validToken[signatureStart+1:]

	tests := []struct {
		name  string
		token string
	}{
		{name: "expired", token: expiredToken},
		{name: "tampered", token: tamperedToken},
		{name: "missing", token: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := authenticator.Authenticate(context.Background(), test.token)
			if !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("expected ErrInvalidToken, got %v", err)
			}
		})
	}

	identity, err := authenticator.Authenticate(context.Background(), validToken)
	if err != nil {
		t.Fatalf("authenticate valid token: %v", err)
	}
	if identity.Subject != "00000000-0000-4000-8000-000000000002" || identity.Email != "person@example.test" {
		t.Fatalf("unexpected identity: %+v", identity)
	}
}
