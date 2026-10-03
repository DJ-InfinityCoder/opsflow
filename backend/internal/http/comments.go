package http

import (
	"log/slog"
	stdhttp "net/http"

	"github.com/go-chi/chi/v5"

	"opsflow/backend/internal/service"
)

type commentHandlers struct {
	service *service.CommentService
	logger  *slog.Logger
}

func mountCommentRoutes(router chi.Router, authService *service.AuthService, commentService *service.CommentService, logger *slog.Logger) {
	handlers := commentHandlers{service: commentService, logger: logger}
	router.With(requireAuth(authService, logger)).Get("/items/{itemID}/comments", handlers.list)
	router.With(requireAuth(authService, logger)).Post("/items/{itemID}/comments", handlers.create)
}

func (h commentHandlers) list(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}
	itemID := chi.URLParam(r, "itemID")
	comments, err := h.service.List(r.Context(), user, itemID)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeJSON(w, stdhttp.StatusOK, struct {
		Comments any `json:"comments"`
	}{Comments: comments})
}

func (h commentHandlers) create(w stdhttp.ResponseWriter, r *stdhttp.Request) {
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
		Body string `json:"body"`
	}
	if err := decodeJSON(body, &input); err != nil {
		writeError(w, r, service.NewAppError(service.KindValidation, "request body is invalid", nil), h.logger)
		return
	}
	itemID := chi.URLParam(r, "itemID")
	comment, err := h.service.Create(r.Context(), user, itemID, input.Body)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeJSON(w, stdhttp.StatusCreated, comment)
}
