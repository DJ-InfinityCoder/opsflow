package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"opsflow/backend/internal/config"
	"opsflow/backend/internal/db"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	command := "up"
	if len(os.Args) > 2 {
		return fmt.Errorf("usage: go run ./cmd/migrate [up|down|status]")
	}
	if len(os.Args) == 2 {
		command = os.Args[1]
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBConnectTimeout)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.RunMigrations(ctx, pool, command); err != nil {
		return fmt.Errorf("migration %s: %w", command, err)
	}
	return nil
}
