package http

import (
	"log/slog"
	stdhttp "net/http"

	"github.com/go-chi/chi/v5/middleware"

	"opsflow/backend/internal/service"
)

func recoverer(logger *slog.Logger) func(stdhttp.Handler) stdhttp.Handler {
	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.ErrorContext(r.Context(), "recovered panic", "panic", recovered)
					if wrapped.Status() == 0 {
						writeError(wrapped, r, service.ErrInternal, logger)
					}
				}
			}()
			next.ServeHTTP(wrapped, r)
		})
	}
}
