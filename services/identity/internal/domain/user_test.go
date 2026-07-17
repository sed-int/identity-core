package domain

import (
	"errors"
	"testing"
)

func TestStatusTransitions(t *testing.T) {
	allowed := []struct{ from, to Status }{
		{StatusPending, StatusActive},
		{StatusActive, StatusDormant},
		{StatusDormant, StatusActive},
		{StatusActive, StatusSuspended},
		{StatusDormant, StatusSuspended},
		{StatusSuspended, StatusActive},
		{StatusPending, StatusDeleted},
		{StatusActive, StatusDeleted},
		{StatusDormant, StatusDeleted},
		{StatusSuspended, StatusDeleted},
	}
	for _, tc := range allowed {
		if !tc.from.CanTransitionTo(tc.to) {
			t.Errorf("%s -> %s should be allowed", tc.from, tc.to)
		}
	}

	forbidden := []struct{ from, to Status }{
		{StatusPending, StatusDormant},
		{StatusPending, StatusSuspended},
		{StatusDeleted, StatusActive},   // DELETED is terminal
		{StatusDeleted, StatusPending},
		{StatusActive, StatusPending},   // no way back to PENDING
		{StatusSuspended, StatusDormant},
	}
	for _, tc := range forbidden {
		if tc.from.CanTransitionTo(tc.to) {
			t.Errorf("%s -> %s should be forbidden", tc.from, tc.to)
		}
	}
}

func TestUserTransitionTo(t *testing.T) {
	u := &User{Status: StatusPending}
	if err := u.TransitionTo(StatusActive); err != nil {
		t.Fatalf("PENDING->ACTIVE: %v", err)
	}
	if u.Status != StatusActive {
		t.Fatalf("status not applied, got %s", u.Status)
	}

	del := &User{Status: StatusDeleted}
	err := del.TransitionTo(StatusActive)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
	if del.Status != StatusDeleted {
		t.Fatalf("failed transition must not mutate status, got %s", del.Status)
	}
}
