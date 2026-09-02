// Package usecase orchestrates board operations.
package usecase

import (
	"context"

	"identity-service/services/board/internal/domain"
)

type PostRepo interface {
	Create(ctx context.Context, post *domain.Post) (*domain.Post, error)
	FindByID(ctx context.Context, id int64) (*domain.Post, error)
	List(ctx context.Context, after int64, limit int) ([]*domain.Post, error)
	Update(ctx context.Context, post *domain.Post) (*domain.Post, error)
	Delete(ctx context.Context, id int64, authorID string) error
}

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

type Board struct {
	repo PostRepo
}

func NewBoard(repo PostRepo) *Board {
	return &Board{repo: repo}
}

// CreatePost persists a post authored by the verified token subject.
func (b *Board) CreatePost(ctx context.Context, authorID, title, content string) (*domain.Post, error) {
	post := &domain.Post{AuthorID: authorID, Title: title, Content: content}
	if err := post.Validate(); err != nil {
		return nil, err
	}
	return b.repo.Create(ctx, post)
}

// ListPosts returns a page of posts plus the cursor for the next page (0 = end).
func (b *Board) ListPosts(ctx context.Context, after int64, pageSize int) ([]*domain.Post, int64, error) {
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	posts, err := b.repo.List(ctx, after, pageSize)
	if err != nil {
		return nil, 0, err
	}
	var next int64
	if len(posts) == pageSize {
		next = posts[len(posts)-1].ID
	}
	return posts, next, nil
}

// UpdatePost changes an existing post only when the verified actor owns it.
func (b *Board) UpdatePost(ctx context.Context, actorID string, id int64, title, content string) (*domain.Post, error) {
	if id <= 0 {
		return nil, domain.ErrInvalidPost
	}
	post, err := b.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if post.AuthorID != actorID {
		return nil, domain.ErrNotAuthorized
	}
	post.Title = title
	post.Content = content
	if err := post.Validate(); err != nil {
		return nil, err
	}
	return b.repo.Update(ctx, post)
}

// DeletePost removes an existing post only when the verified actor owns it.
func (b *Board) DeletePost(ctx context.Context, actorID string, id int64) error {
	if id <= 0 {
		return domain.ErrInvalidPost
	}
	post, err := b.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if post.AuthorID != actorID {
		return domain.ErrNotAuthorized
	}
	return b.repo.Delete(ctx, id, actorID)
}
