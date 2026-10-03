package authz

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type MembershipDB interface {
	Query(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, arguments ...any) pgx.Row
}

type PostgresMembershipStore struct {
	db MembershipDB
}

func NewPostgresMembershipStore(db MembershipDB) *PostgresMembershipStore {
	return &PostgresMembershipStore{db: db}
}

func (s *PostgresMembershipStore) RoleForTeam(ctx context.Context, userID, teamID string) (Role, bool, error) {
	var role Role
	err := s.db.QueryRow(ctx, `
		SELECT role
		FROM team_members
		WHERE user_id = $1::uuid AND team_id = $2::uuid
		FOR SHARE
	`, userID, teamID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load team membership role: %w", err)
	}
	return role, true, nil
}

func (s *PostgresMembershipStore) ListTeamIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT team_id::text
		FROM team_members
		WHERE user_id = $1::uuid
		ORDER BY team_id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list accessible team IDs: %w", err)
	}
	defer rows.Close()
	return collectTeamIDs(rows)
}

func (s *PostgresMembershipStore) ListAllTeamIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT id::text FROM teams ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list all team IDs: %w", err)
	}
	defer rows.Close()
	return collectTeamIDs(rows)
}

type rowScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func collectTeamIDs(rows rowScanner) ([]string, error) {
	teamIDs := make([]string, 0)
	for rows.Next() {
		var teamID string
		if err := rows.Scan(&teamID); err != nil {
			return nil, fmt.Errorf("scan team ID: %w", err)
		}
		teamIDs = append(teamIDs, teamID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate team IDs: %w", err)
	}
	return teamIDs, nil
}
