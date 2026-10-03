package service

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"opsflow/backend/internal/db"
	"opsflow/backend/internal/model"
	"opsflow/backend/internal/repo"
)

type AdminService struct {
	pool *pgxpool.Pool
}

func NewAdminService(pool *pgxpool.Pool) *AdminService {
	return &AdminService{pool: pool}
}

type AdminJobsPage struct {
	Jobs       []model.OutboxJob `json:"jobs"`
	NextCursor int64             `json:"next_cursor,omitempty"`
}

func (s *AdminService) ListJobs(ctx context.Context, user model.User, status string, cursorID int64, limit int) (AdminJobsPage, error) {
	if !user.IsSystemAdmin {
		return AdminJobsPage{}, ErrForbidden
	}

	status = strings.TrimSpace(status)
	if status == "" {
		status = "failed"
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var page AdminJobsPage
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		r := repo.NewOutboxRepository(tx)
		jobs, err := r.ListJobs(ctx, status, cursorID, limit+1)
		if err != nil {
			return err
		}

		if len(jobs) > limit {
			page.NextCursor = jobs[limit-1].ID
			jobs = jobs[:limit]
		}
		page.Jobs = jobs
		return nil
	})
	if err != nil {
		return AdminJobsPage{}, err
	}
	if page.Jobs == nil {
		page.Jobs = []model.OutboxJob{}
	}
	return page, nil
}
