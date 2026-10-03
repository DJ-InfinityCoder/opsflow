package http

import (
	"log/slog"
	stdhttp "net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"opsflow/backend/internal/service"
)

type notificationHandlers struct {
	service *service.NotificationService
	logger  *slog.Logger
}

func mountNotificationRoutes(router chi.Router, authService *service.AuthService, notifService *service.NotificationService, logger *slog.Logger) {
	handlers := notificationHandlers{service: notifService, logger: logger}
	router.With(requireAuth(authService, logger)).Get("/notifications", handlers.list)
	router.With(requireAuth(authService, logger)).Post("/notifications/read", handlers.markRead)
}

func (h notificationHandlers) list(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}

	query := r.URL.Query()
	cursor := query.Get("cursor")
	unreadOnly, _ := strconv.ParseBool(query.Get("unread_only"))
	limit, err := queryLimit(r)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}

	page, err := h.service.List(r.Context(), user, unreadOnly, cursor, limit)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeJSON(w, stdhttp.StatusOK, page)
}

func (h notificationHandlers) markRead(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}

	body, err := readRequestBody(w, r)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}

	var input struct {
		IDs []string `json:"ids"`
	}
	if len(body) > 0 {
		if err := decodeJSON(body, &input); err != nil {
			writeError(w, r, service.NewAppError(service.KindValidation, "request body is invalid", nil), h.logger)
			return
		}
	}

	updated, err := h.service.MarkRead(r.Context(), user, input.IDs)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeJSON(w, stdhttp.StatusOK, map[string]any{"updated": updated})
}
