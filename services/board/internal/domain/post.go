// Package domain holds the board's business types.
package domain

import (
	"errors"
	"time"
)

var (
	ErrPostNotFound   = errors.New("post not found")
	ErrInvalidPost    = errors.New("invalid post")
	ErrNotAuthorized  = errors.New("not authorized")
)

const (
	MaxTitleLen   = 200
	MaxContentLen = 20_000
)

type Post struct {
	ID        int64
	AuthorID  string // IdP user id (token sub) — opaque to the board domain
	Title     string
	Content   string
	CreatedAt time.Time
	// AuthorNickname comes from the local authors read model (replicated via
	// user.created events); empty if the event hasn't arrived yet.
	AuthorNickname string
}

// Validate enforces the invariants a post must satisfy before persistence.
func (p *Post) Validate() error {
	if p.AuthorID == "" || p.Title == "" || p.Content == "" {
		return ErrInvalidPost
	}
	if len(p.Title) > MaxTitleLen || len(p.Content) > MaxContentLen {
		return ErrInvalidPost
	}
	return nil
}
