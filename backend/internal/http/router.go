package http

import (
	"log/slog"
	stdhttp "net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"opsflow/backend/internal/config"
	"opsflow/backend/internal/service"
)

func NewRouter(pool *pgxpool.Pool, logger *slog.Logger, cfg config.Config, authService *service.AuthService) stdhttp.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(requestLogger(logger))
	router.Use(recoverer(logger))
	router.Use(newIPRateLimiter().middleware(logger))
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "If-Match", "Idempotency-Key", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID", "Idempotent-Replay"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	router.Get("/healthz", func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	router.Get("/readyz", func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			writeError(w, r, databaseUnavailable(err), logger)
			return
		}
		w.WriteHeader(stdhttp.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	mountItemRoutes(router, authService, service.NewItemService(pool), logger)
	mountCommentRoutes(router, authService, service.NewCommentService(pool), logger)
	mountNotificationRoutes(router, authService, service.NewNotificationService(pool), logger)
	mountFeedRoutes(router, authService, service.NewFeedService(pool), logger)
	mountAnalyticsRoutes(router, authService, service.NewAnalyticsService(pool), logger)
	mountAdminRoutes(router, authService, service.NewAdminService(pool), logger)
	mountAuthRoutes(router, authService, !strings.EqualFold(strings.TrimSpace(cfg.AppEnv), "production"), logger)

	return router
}

func requestLogger(logger *slog.Logger) func(stdhttp.Handler) stdhttp.Handler {
	return func(next stdhttp.Handler) stdhttp.Handler {
		return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
			started := time.Now()
			wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(wrapped, r)
			status := wrapped.Status()
			if status == 0 {
				status = stdhttp.StatusOK
			}
			logger.InfoContext(r.Context(), "HTTP request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"latency", time.Since(started).String(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}
