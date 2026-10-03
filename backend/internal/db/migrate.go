package db

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"opsflow/backend/migrations"
)

var migrationMu sync.Mutex

func RunMigrations(ctx context.Context, pool *pgxpool.Pool, command string) error {
	migrationMu.Lock()
	defer migrationMu.Unlock()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("configure goose dialect: %w", err)
	}

	database := stdlib.OpenDBFromPool(pool)
	defer database.Close()

	switch command {
	case "up":
		return goose.UpContext(ctx, database, ".")
	case "down":
		return goose.DownContext(ctx, database, ".")
	case "status":
		return goose.StatusContext(ctx, database, ".")
	default:
		return fmt.Errorf("unsupported migration command %q (use up, down, or status)", command)
	}
}
