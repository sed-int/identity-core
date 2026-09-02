package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"identity-service/services/board/internal/domain"
)

type fakePostRepo struct {
	posts map[int64]*domain.Post
}

func (r *fakePostRepo) Create(_ context.Context, post *domain.Post) (*domain.Post, error) {
	copy := *post
	copy.ID = int64(len(r.posts) + 1)
	copy.CreatedAt = time.Now()
	r.posts[copy.ID] = &copy
	return &copy, nil
}

func (r *fakePostRepo) FindByID(_ context.Context, id int64) (*domain.Post, error) {
	post, ok := r.posts[id]
	if !ok {
		return nil, domain.ErrPostNotFound
	}
	copy := *post
	return &copy, nil
}

func (r *fakePostRepo) List(context.Context, int64, int) ([]*domain.Post, error) {
	return nil, nil
}

func (r *fakePostRepo) Update(_ context.Context, post *domain.Post) (*domain.Post, error) {
	if _, ok := r.posts[post.ID]; !ok {
		return nil, domain.ErrPostNotFound
	}
	copy := *post
	r.posts[post.ID] = &copy
	return &copy, nil
}

func (r *fakePostRepo) Delete(_ context.Context, id int64, authorID string) error {
	post, ok := r.posts[id]
	if !ok {
		return domain.ErrPostNotFound
	}
	if post.AuthorID != authorID {
		return domain.ErrNotAuthorized
	}
	delete(r.posts, id)
	return nil
}

func testBoard() (*Board, *fakePostRepo) {
	repo := &fakePostRepo{posts: map[int64]*domain.Post{
		1: {ID: 1, AuthorID: "owner", Title: "before", Content: "body", CreatedAt: time.Now()},
	}}
	return NewBoard(repo), repo
}

func TestUpdatePostByOwner(t *testing.T) {
	board, repo := testBoard()
	post, err := board.UpdatePost(context.Background(), "owner", 1, "after", "updated body")
	if err != nil {
		t.Fatal(err)
	}
	if post.Title != "after" || repo.posts[1].Content != "updated body" {
		t.Fatalf("post was not updated: %+v", post)
	}
}

func TestUpdatePostRejectsNonOwner(t *testing.T) {
	board, repo := testBoard()
	_, err := board.UpdatePost(context.Background(), "other", 1, "stolen", "body")
	if !errors.Is(err, domain.ErrNotAuthorized) {
		t.Fatalf("want ErrNotAuthorized, got %v", err)
	}
	if repo.posts[1].Title != "before" {
		t.Fatal("unauthorized update changed the post")
	}
}

func TestDeletePostByOwner(t *testing.T) {
	board, repo := testBoard()
	if err := board.DeletePost(context.Background(), "owner", 1); err != nil {
		t.Fatal(err)
	}
	if _, ok := repo.posts[1]; ok {
		t.Fatal("post was not deleted")
	}
}

func TestDeletePostRejectsNonOwner(t *testing.T) {
	board, repo := testBoard()
	err := board.DeletePost(context.Background(), "other", 1)
	if !errors.Is(err, domain.ErrNotAuthorized) {
		t.Fatalf("want ErrNotAuthorized, got %v", err)
	}
	if _, ok := repo.posts[1]; !ok {
		t.Fatal("unauthorized delete removed the post")
	}
}
