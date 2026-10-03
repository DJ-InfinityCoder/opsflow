package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"opsflow/backend/internal/authn"
	"opsflow/backend/internal/model"
	"opsflow/backend/internal/repo"
)

type User = model.User
type Membership = model.Membership

type AuthUserStore interface {
	FindByEmail(ctx context.Context, email string) (User, error)
	FindByID(ctx context.Context, id string) (User, error)
	ListDemoUsers(ctx context.Context) ([]User, error)
	ListMemberships(ctx context.Context, userID string) ([]Membership, error)
}

type AuthService struct {
	users         AuthUserStore
	authenticator authn.Authenticator
	issuer        authn.TokenIssuer
}

type LoginResult struct {
	Token     string
	ExpiresAt time.Time
}

type MeResult struct {
	User        User         `json:"user"`
	Memberships []Membership `json:"memberships"`
}

func NewAuthService(users AuthUserStore, authenticator authn.Authenticator, issuer authn.TokenIssuer) *AuthService {
	return &AuthService{users: users, authenticator: authenticator, issuer: issuer}
}

func (s *AuthService) DevLogin(ctx context.Context, email string) (LoginResult, error) {
	email = strings.TrimSpace(email)
	if email == "" || !strings.Contains(email, "@") {
		return LoginResult{}, NewAppError(KindValidation, "a valid email is required", nil)
	}
	user, err := s.users.FindByEmail(ctx, email)
	if errors.Is(err, repo.ErrUserNotFound) {
		return LoginResult{}, ErrUnauthorized
	}
	if err != nil {
		return LoginResult{}, fmt.Errorf("load dev-login user: %w", err)
	}
	token, expiresAt, err := s.issuer.Issue(authn.Identity{Subject: user.ID, Email: user.Email})
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue dev-login token: %w", err)
	}
	return LoginResult{Token: token, ExpiresAt: expiresAt}, nil
}

func (s *AuthService) CurrentUser(ctx context.Context, token string) (User, error) {
	identity, err := s.authenticator.Authenticate(ctx, token)
	if err != nil {
		return User{}, ErrUnauthorized
	}
	user, err := s.users.FindByID(ctx, identity.Subject)
	if errors.Is(err, repo.ErrUserNotFound) {
		return User{}, ErrUnauthorized
	}
	if err != nil {
		return User{}, fmt.Errorf("load authenticated user: %w", err)
	}
	if !strings.EqualFold(user.Email, identity.Email) {
		return User{}, ErrUnauthorized
	}
	return user, nil
}

func (s *AuthService) DemoUsers(ctx context.Context) ([]User, error) {
	users, err := s.users.ListDemoUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("load demo users: %w", err)
	}
	return users, nil
}

func (s *AuthService) Me(ctx context.Context, user User) (MeResult, error) {
	memberships, err := s.users.ListMemberships(ctx, user.ID)
	if err != nil {
		return MeResult{}, fmt.Errorf("load current user memberships: %w", err)
	}
	return MeResult{User: user, Memberships: memberships}, nil
}
