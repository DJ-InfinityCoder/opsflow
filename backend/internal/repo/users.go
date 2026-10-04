package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"opsflow/backend/internal/model"
)

var ErrUserNotFound = errors.New("user not found")

var DemoUserEmails = []string{
	"alicia@opsflow.local",
	"marcus@opsflow.local",
	"priya@opsflow.local",
	"noah@opsflow.local",
	"elena@opsflow.local",
	"jonas@opsflow.local",
	"nina@opsflow.local",
	"omar@opsflow.local",
	"admin@opsflow.dev",
	"lead@opsflow.dev",
	"operator@opsflow.dev",
	"reporter@opsflow.dev",
	"lead-reporter@opsflow.dev",
}

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (model.User, error) {
	var user model.User
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, email, name, is_system_admin
		FROM users
		WHERE lower(email) = lower($1)
	`, email).Scan(&user.ID, &user.Email, &user.Name, &user.IsSystemAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, ErrUserNotFound
	}
	if err != nil {
		return model.User{}, fmt.Errorf("find user by email: %w", err)
	}
	return user, nil
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (model.User, error) {
	var user model.User
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, email, name, is_system_admin
		FROM users
		WHERE id = $1::uuid
	`, id).Scan(&user.ID, &user.Email, &user.Name, &user.IsSystemAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, ErrUserNotFound
	}
	if err != nil {
		return model.User{}, fmt.Errorf("find user by id: %w", err)
	}
	return user, nil
}

func (r *UserRepository) ListDemoUsers(ctx context.Context) ([]model.User, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, email, name, is_system_admin
		FROM users
		WHERE email = ANY($1::text[])
		ORDER BY array_position($1::text[], email)
	`, DemoUserEmails)
	if err != nil {
		return nil, fmt.Errorf("list demo users: %w", err)
	}
	defer rows.Close()

	users := make([]model.User, 0, len(DemoUserEmails))
	for rows.Next() {
		var user model.User
		if err := rows.Scan(&user.ID, &user.Email, &user.Name, &user.IsSystemAdmin); err != nil {
			return nil, fmt.Errorf("scan demo user: %w", err)
		}
		users = append(users, user)
	}
	if len(users) == 0 {
		fallbackRows, err := r.pool.Query(ctx, `
			SELECT id::text, email, name, is_system_admin
			FROM users
			ORDER BY is_system_admin DESC, email ASC
			LIMIT 10
		`)
		if err == nil {
			defer fallbackRows.Close()
			for fallbackRows.Next() {
				var u model.User
				if err := fallbackRows.Scan(&u.ID, &u.Email, &u.Name, &u.IsSystemAdmin); err == nil {
					users = append(users, u)
				}
			}
		}
	}
	return users, nil
}

func (r *UserRepository) ListMemberships(ctx context.Context, userID string) ([]model.Membership, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT tm.team_id::text, t.name, tm.role
		FROM team_members AS tm
		JOIN teams AS t ON t.id = tm.team_id
		WHERE tm.user_id = $1::uuid
		ORDER BY t.name, tm.team_id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list team memberships: %w", err)
	}
	defer rows.Close()

	memberships := make([]model.Membership, 0)
	for rows.Next() {
		var membership model.Membership
		if err := rows.Scan(&membership.TeamID, &membership.TeamName, &membership.Role); err != nil {
			return nil, fmt.Errorf("scan team membership: %w", err)
		}
		memberships = append(memberships, membership)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate team memberships: %w", err)
	}
	return memberships, nil
}
