// Package mysql implements the board repositories against MySQL.
package mysql

import (
	"context"
	"database/sql"
	"fmt"

	"identity-service/services/board/internal/domain"
)

type PostRepo struct {
	db *sql.DB
}

func NewPostRepo(db *sql.DB) *PostRepo {
	return &PostRepo{db: db}
}

func (r *PostRepo) Create(ctx context.Context, post *domain.Post) (*domain.Post, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO posts (author_id, title, content) VALUES (?, ?, ?)`,
		post.AuthorID, post.Title, post.Content)
	if err != nil {
		return nil, fmt.Errorf("insert post: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.FindByID(ctx, id)
}

func (r *PostRepo) FindByID(ctx context.Context, id int64) (*domain.Post, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT p.id, p.author_id, p.title, p.content, p.created_at,
		       COALESCE(a.nickname, '')
		FROM posts p LEFT JOIN authors a ON a.user_id = p.author_id
		WHERE p.id = ?`, id)
	var p domain.Post
	if err := row.Scan(&p.ID, &p.AuthorID, &p.Title, &p.Content, &p.CreatedAt, &p.AuthorNickname); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrPostNotFound
		}
		return nil, err
	}
	return &p, nil
}

// List returns newest-first keyset pagination: posts with id < after
// (after=0 means from the latest).
func (r *PostRepo) List(ctx context.Context, after int64, limit int) ([]*domain.Post, error) {
	if after == 0 {
		after = int64(^uint64(0) >> 1) // max int64: start from the newest
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT p.id, p.author_id, p.title, p.content, p.created_at,
		       COALESCE(a.nickname, '')
		FROM posts p LEFT JOIN authors a ON a.user_id = p.author_id
		WHERE p.id < ? ORDER BY p.id DESC LIMIT ?`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []*domain.Post
	for rows.Next() {
		var p domain.Post
		if err := rows.Scan(&p.ID, &p.AuthorID, &p.Title, &p.Content, &p.CreatedAt, &p.AuthorNickname); err != nil {
			return nil, err
		}
		posts = append(posts, &p)
	}
	return posts, rows.Err()
}

func (r *PostRepo) Update(ctx context.Context, post *domain.Post) (*domain.Post, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE posts SET title = ?, content = ? WHERE id = ? AND author_id = ?`,
		post.Title, post.Content, post.ID, post.AuthorID)
	if err != nil {
		return nil, fmt.Errorf("update post: %w", err)
	}
	count, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, domain.ErrPostNotFound
	}
	return r.FindByID(ctx, post.ID)
}

func (r *PostRepo) Delete(ctx context.Context, id int64, authorID string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM posts WHERE id = ? AND author_id = ?`, id, authorID)
	if err != nil {
		return fmt.Errorf("delete post: %w", err)
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return domain.ErrPostNotFound
	}
	return nil
}
