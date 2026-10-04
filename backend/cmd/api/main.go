package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"opsflow/backend/internal/authn"
	"opsflow/backend/internal/config"
	"opsflow/backend/internal/db"
	apihttp "opsflow/backend/internal/http"
	"opsflow/backend/internal/repo"
	"opsflow/backend/internal/service"
	"opsflow/backend/internal/worker"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	jwtAuthenticator, err := authn.NewHMACJWT(cfg.JWTSecret)
	if err != nil {
		return fmt.Errorf("configure authentication: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBConnectTimeout)
	if err != nil {
		return err
	}
	defer pool.Close()
	authService := service.NewAuthService(repo.NewUserRepository(pool), jwtAuthenticator, jwtAuthenticator)

	if cfg.WorkerEnabled {
		go func() {
			logger.Info("starting background worker from API process")
			if err := worker.Run(ctx, pool, logger); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("background worker failed", "error", err)
			}
		}()
	}

	if selfPingURL := os.Getenv("SELF_PING_URL"); selfPingURL != "" {
		go func() {
			logger.Info("starting keep-alive self-ping routine", "url", selfPingURL)
			ticker := time.NewTicker(10 * time.Minute)
			defer ticker.Stop()
			client := &http.Client{Timeout: 10 * time.Second}
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					resp, err := client.Get(selfPingURL)
					if err != nil {
						logger.Warn("self-ping keep-alive failed", "url", selfPingURL, "error", err)
					} else {
						_ = resp.Body.Close()
						logger.Info("self-ping keep-alive heartbeat sent", "url", selfPingURL, "status", resp.StatusCode)
					}
				}
			}
		}()
	}

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           apihttp.NewRouter(pool, logger, cfg, authService),
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("API server listening", "addr", server.Addr, "app_env", cfg.AppEnv)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down API server: %w", err)
		}
		return nil
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve API: %w", err)
	}
}
