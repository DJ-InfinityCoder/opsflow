package http

import (
	"log/slog"
	stdhttp "net/http"

	"github.com/go-chi/chi/v5"

	"opsflow/backend/internal/service"
)

type feedHandlers struct {
	service *service.FeedService
	logger  *slog.Logger
}

func mountFeedRoutes(router chi.Router, authService *service.AuthService, feedService *service.FeedService, logger *slog.Logger) {
	handlers := feedHandlers{service: feedService, logger: logger}
	router.With(requireAuth(authService, logger)).Get("/feed", handlers.feed)
}

func (h feedHandlers) feed(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}

	query := r.URL.Query()
	teamID := query.Get("team")
	cursor := query.Get("cursor")
	limit, err := queryLimit(r)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}

	page, err := h.service.GetFeed(r.Context(), user, teamID, cursor, limit)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeJSON(w, stdhttp.StatusOK, page)
}
