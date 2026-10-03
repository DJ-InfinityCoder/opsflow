package service

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"opsflow/backend/internal/authz"
	"opsflow/backend/internal/db"
	"opsflow/backend/internal/model"
	"opsflow/backend/internal/repo"
)

type AnalyticsService struct {
	pool *pgxpool.Pool
}

func NewAnalyticsService(pool *pgxpool.Pool) *AnalyticsService {
	return &AnalyticsService{pool: pool}
}

func (s *AnalyticsService) GetSummary(ctx context.Context, user model.User, teamID string) (model.AnalyticsSummary, error) {
	teamID = strings.TrimSpace(teamID)
	if !isUUID(teamID) {
		return model.AnalyticsSummary{}, ErrNotFound
	}

	var summary model.AnalyticsSummary
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		authorizer := authz.NewAuthorizer(authz.NewPostgresMembershipStore(tx))
		if err := authorizer.Authorize(ctx, user, authz.Action{Name: authz.ActionAnalyticsView}, authz.Item{TeamID: teamID}); err != nil {
			return err
		}

		analyticsRepo := repo.NewAnalyticsRepository(tx)
		res, err := analyticsRepo.GetTeamSummary(ctx, teamID)
		if err != nil {
			return err
		}
		summary = res
		return nil
	})
	if err != nil {
		return model.AnalyticsSummary{}, err
	}
	return summary, nil
}
