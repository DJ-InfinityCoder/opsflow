package http

import (
	"log/slog"
	stdhttp "net/http"

	"github.com/go-chi/chi/v5"

	"opsflow/backend/internal/service"
)

type analyticsHandlers struct {
	service *service.AnalyticsService
	logger  *slog.Logger
}

func mountAnalyticsRoutes(router chi.Router, authService *service.AuthService, analyticsService *service.AnalyticsService, logger *slog.Logger) {
	handlers := analyticsHandlers{service: analyticsService, logger: logger}
	router.With(requireAuth(authService, logger)).Get("/analytics/summary", handlers.summary)
	router.With(requireAuth(authService, logger)).Get("/analytics/teams", handlers.teams)
}

func (h analyticsHandlers) teams(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}

	teams, err := h.service.ListTeamMetrics(r.Context(), user)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeJSON(w, stdhttp.StatusOK, teams)
}

func (h analyticsHandlers) summary(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}

	teamID := r.URL.Query().Get("team")
	if teamID == "" {
		writeError(w, r, service.NewAppError(service.KindValidation, "team query parameter is required", nil), h.logger)
		return
	}

	summary, err := h.service.GetSummary(r.Context(), user, teamID)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeJSON(w, stdhttp.StatusOK, summary)
}
