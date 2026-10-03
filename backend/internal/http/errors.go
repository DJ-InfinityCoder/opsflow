package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	stdhttp "net/http"

	"github.com/go-chi/chi/v5/middleware"

	"opsflow/backend/internal/service"
)

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Details   any    `json:"details"`
}

func writeError(w stdhttp.ResponseWriter, r *stdhttp.Request, err error, logger *slog.Logger) {
	var appErr *service.AppError
	if !errors.As(err, &appErr) {
		logger.ErrorContext(r.Context(), "unhandled application error", "error", err)
		appErr = service.ErrInternal
	} else if appErr.Cause != nil {
		logger.ErrorContext(r.Context(), "application error", "code", appErr.Kind, "error", appErr.Cause)
	}

	status := statusFor(appErr.Kind)
	details := appErr.Details
	if details == nil {
		details = map[string]any{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorEnvelope{Error: errorBody{
		Code:      string(appErr.Kind),
		Message:   appErr.Message,
		RequestID: middleware.GetReqID(r.Context()),
		Details:   details,
	}})
}

func statusFor(kind service.ErrorKind) int {
	switch kind {
	case service.KindNotFound:
		return stdhttp.StatusNotFound
	case service.KindForbidden:
		return stdhttp.StatusForbidden
	case service.KindUnauthorized:
		return stdhttp.StatusUnauthorized
	case service.KindConflict:
		return stdhttp.StatusConflict
	case service.KindValidation:
		return stdhttp.StatusUnprocessableEntity
	case service.KindUnavailable:
		return stdhttp.StatusServiceUnavailable
	case service.KindRateLimited:
		return stdhttp.StatusTooManyRequests
	case service.KindPreconditionRequired:
		return stdhttp.StatusPreconditionRequired
	case service.KindVersionConflict:
		return stdhttp.StatusConflict
	case service.KindAlreadyClaimed:
		return stdhttp.StatusConflict
	case service.KindIllegalTransition:
		return stdhttp.StatusUnprocessableEntity
	case service.KindIdempotencyKeyReused, service.KindIdempotencyKeyReuse:
		return stdhttp.StatusUnprocessableEntity
	case service.KindRequestInProgress:
		return stdhttp.StatusConflict
	default:
		return stdhttp.StatusInternalServerError
	}
}

func databaseUnavailable(err error) *service.AppError {
	return &service.AppError{
		Kind:    service.KindUnavailable,
		Message: "database is unavailable",
		Cause:   err,
	}
}
