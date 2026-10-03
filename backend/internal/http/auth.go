package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	stdhttp "net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"opsflow/backend/internal/service"
)

type authHandlers struct {
	service    *service.AuthService
	devEnabled bool
	logger     *slog.Logger
}

type currentUserKey struct{}

func mountAuthRoutes(router chi.Router, authService *service.AuthService, devEnabled bool, logger *slog.Logger) {
	handlers := authHandlers{service: authService, devEnabled: devEnabled, logger: logger}
	router.Post("/auth/dev-login", handlers.devLogin)
	router.Get("/auth/demo-users", handlers.demoUsers)
	router.With(requireAuth(authService, logger)).Get("/me", handlers.me)
}

func (h authHandlers) devLogin(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.devEnabled {
		writeError(w, r, service.NewAppError(service.KindNotFound, "not found", nil), h.logger)
		return
	}
	var request struct {
		Email string `json:"email"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, r, service.NewAppError(service.KindValidation, "request body must contain an email", nil), h.logger)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, r, service.NewAppError(service.KindValidation, "request body must contain a single JSON object", nil), h.logger)
		return
	}

	result, err := h.service.DevLogin(r.Context(), request.Email)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeJSON(w, stdhttp.StatusOK, struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresAt   string `json:"expires_at"`
	}{
		AccessToken: result.Token,
		TokenType:   "Bearer",
		ExpiresAt:   result.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h authHandlers) demoUsers(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if !h.devEnabled {
		writeError(w, r, service.NewAppError(service.KindNotFound, "not found", nil), h.logger)
		return
	}
	users, err := h.service.DemoUsers(r.Context())
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeJSON(w, stdhttp.StatusOK, struct {
		Users []service.User `json:"users"`
	}{Users: users})
}

func (h authHandlers) me(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := r.Context().Value(currentUserKey{}).(service.User)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}
	result, err := h.service.Me(r.Context(), user)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeJSON(w, stdhttp.StatusOK, result)
}

func requireAuth(authService *service.AuthService, logger *slog.Logger) func(stdhttp.Handler) stdhttp.Handler {
	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			authorization := strings.TrimSpace(r.Header.Get("Authorization"))
			scheme, token, found := strings.Cut(authorization, " ")
			if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeError(w, r, service.ErrUnauthorized, logger)
				return
			}
			user, err := authService.CurrentUser(r.Context(), strings.TrimSpace(token))
			if err != nil {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeError(w, r, err, logger)
				return
			}
			ctx := context.WithValue(r.Context(), currentUserKey{}, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeJSON(w stdhttp.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
