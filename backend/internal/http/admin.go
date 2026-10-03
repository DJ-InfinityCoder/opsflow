package http

import (
	"log/slog"
	stdhttp "net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"opsflow/backend/internal/service"
)

type adminHandlers struct {
	service *service.AdminService
	logger  *slog.Logger
}

func mountAdminRoutes(router chi.Router, authService *service.AuthService, adminService *service.AdminService, logger *slog.Logger) {
	handlers := adminHandlers{service: adminService, logger: logger}
	router.With(requireAuth(authService, logger)).Get("/admin/jobs", handlers.jobs)
}

func (h adminHandlers) jobs(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}

	query := r.URL.Query()
	status := query.Get("status")
	var cursorID int64
	if rawCursor := query.Get("cursor"); rawCursor != "" {
		if id, err := strconv.ParseInt(rawCursor, 10, 64); err == nil {
			cursorID = id
		}
	}
	limit, err := queryLimit(r)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}

	page, err := h.service.ListJobs(r.Context(), user, status, cursorID, limit)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeJSON(w, stdhttp.StatusOK, page)
}
