// Package domain holds pure business rules for the identity service —
// no infrastructure imports allowed here.
package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrUserNotFound        = errors.New("user not found")
	ErrInvalidTransition   = errors.New("invalid status transition")
	ErrCredentialConflict  = errors.New("credential already registered")
	ErrLoginNotAllowed     = errors.New("account status does not allow login")
)

// Status is the account lifecycle state (PRD §5).
type Status string

const (
	StatusPending   Status = "PENDING"   // signup started, profile not completed
	StatusActive    Status = "ACTIVE"
	StatusDormant   Status = "DORMANT"   // >1 year inactive
	StatusSuspended Status = "SUSPENDED" // admin action
	StatusDeleted   Status = "DELETED"   // terminal, soft-delete
)

// transitions encodes the state machine from PRD §5:
// PENDING → ACTIVE ⇄ DORMANT; ACTIVE/DORMANT → SUSPENDED → ACTIVE; any → DELETED.
var transitions = map[Status][]Status{
	StatusPending:   {StatusActive, StatusDeleted},
	StatusActive:    {StatusDormant, StatusSuspended, StatusDeleted},
	StatusDormant:   {StatusActive, StatusSuspended, StatusDeleted},
	StatusSuspended: {StatusActive, StatusDeleted},
	StatusDeleted:   {},
}

func (s Status) CanTransitionTo(target Status) bool {
	for _, t := range transitions[s] {
		if t == target {
			return true
		}
	}
	return false
}

type AuthType string

const (
	AuthTypePhone AuthType = "PHONE"
	AuthTypeEmail AuthType = "EMAIL"
)

type User struct {
	ID          int64
	Status      Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
	LastLoginAt *time.Time
}

// TransitionTo validates and applies a status change.
func (u *User) TransitionTo(target Status) error {
	if !u.Status.CanTransitionTo(target) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, u.Status, target)
	}
	u.Status = target
	return nil
}

type Profile struct {
	UserID          int64
	Nickname        string
	ProfileImageURL string
	ReputationScore float64
}
