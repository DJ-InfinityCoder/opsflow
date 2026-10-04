package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"opsflow/backend/internal/model"
)

type CommentRepository struct {
	db DBTX
}

func NewCommentRepository(db DBTX) *CommentRepository {
	return &CommentRepository{db: db}
}

func (r *CommentRepository) Insert(ctx context.Context, itemID, authorID, body string, mentions []string) (model.Comment, error) {
	var comment model.Comment
	err := r.db.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO comments (item_id, author_id, body, mentions)
			VALUES ($1::uuid, $2::uuid, $3, $4::uuid[])
			RETURNING id, item_id, author_id, body, mentions, created_at
		)
		SELECT ins.id::text, ins.item_id::text, ins.author_id::text,
		       coalesce(u.name, ins.author_id::text),
		       ins.body, ins.mentions::text[], ins.created_at
		FROM ins
		LEFT JOIN users u ON u.id = ins.author_id
	`, itemID, authorID, body, mentions).Scan(
		&comment.ID, &comment.ItemID, &comment.AuthorID, &comment.AuthorName,
		&comment.Body, &comment.Mentions, &comment.CreatedAt,
	)
	if err != nil {
		return model.Comment{}, fmt.Errorf("insert comment: %w", err)
	}
	if comment.Mentions == nil {
		comment.Mentions = []string{}
	}
	return comment, nil
}

func (r *CommentRepository) ListByItem(ctx context.Context, itemID string) ([]model.Comment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT c.id::text, c.item_id::text, c.author_id::text,
		       coalesce(u.name, c.author_id::text),
		       c.body, c.mentions::text[], c.created_at
		FROM comments c
		LEFT JOIN users u ON u.id = c.author_id
		WHERE c.item_id = $1::uuid
		ORDER BY c.created_at ASC, c.id ASC
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()

	comments := make([]model.Comment, 0)
	for rows.Next() {
		var c model.Comment
		if err := rows.Scan(&c.ID, &c.ItemID, &c.AuthorID, &c.AuthorName, &c.Body, &c.Mentions, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		if c.Mentions == nil {
			c.Mentions = []string{}
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

// FindTeamMemberByUsername finds a user in the given team matching the handle.
func (r *CommentRepository) FindTeamMemberByUsername(ctx context.Context, teamID, handle string) (model.User, bool, error) {
	var user model.User
	err := r.db.QueryRow(ctx, `
		SELECT u.id::text, u.email, u.name, u.is_system_admin
		FROM users u
		JOIN team_members tm ON tm.user_id = u.id AND tm.team_id = $1::uuid
		WHERE lower(coalesce(u.username, split_part(u.email, '@', 1))) = lower($2)
		   OR lower(split_part(u.email, '@', 1)) = lower($2)
		   OR lower(replace(u.name, ' ', '')) = lower(replace($2, ' ', ''))
		LIMIT 1
	`, teamID, handle).Scan(&user.ID, &user.Email, &user.Name, &user.IsSystemAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, false, nil
	}
	if err != nil {
		return model.User{}, false, fmt.Errorf("find team member by handle: %w", err)
	}
	return user, true, nil
}

// UserExistsByHandle checks if a user exists in the system by handle/email/name.
func (r *CommentRepository) UserExistsByHandle(ctx context.Context, handle string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT true
		FROM users u
		WHERE lower(coalesce(u.username, split_part(u.email, '@', 1))) = lower($1)
		   OR lower(split_part(u.email, '@', 1)) = lower($1)
		   OR lower(replace(u.name, ' ', '')) = lower(replace($1, ' ', ''))
		LIMIT 1
	`, handle).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check user exists by handle: %w", err)
	}
	return exists, nil
}
