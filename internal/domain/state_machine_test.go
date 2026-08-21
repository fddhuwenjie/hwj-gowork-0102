package domain

import (
	"errors"
	"testing"
	"time"
)

func TestWeldStateMachine(t *testing.T) {
	weld := NewWeld("w1", time.Now().UTC())
	if err := weld.Transition(WeldStatusScheduled, time.Now().UTC()); err != nil {
		t.Fatalf("expected schedule transition: %v", err)
	}
	if err := weld.Transition(WeldStatusArchived, time.Now().UTC()); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition, got %v", err)
	}
}

func TestValidationBoundary(t *testing.T) {
	weld := NewWeld("", time.Now().UTC())
	if err := weld.Validate(); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestMethodVersionStateMachine(t *testing.T) {
	now := time.Now().UTC()
	// Adjacent transitions are permitted and advance the status.
	draft := NewMethodVersion("mv-1", now)
	if err := draft.Transition(MethodVersionStatusActive, now); err != nil {
		t.Fatalf("expected draft -> active: %v", err)
	}
	if draft.Status != MethodVersionStatusActive {
		t.Fatalf("expected active, got %s", draft.Status)
	}
	if err := draft.Transition(MethodVersionStatusDeprecated, now); err != nil {
		t.Fatalf("expected active -> deprecated: %v", err)
	}
	if draft.Status != MethodVersionStatusDeprecated {
		t.Fatalf("expected deprecated, got %s", draft.Status)
	}
	// A deprecated version is terminal: no further transitions.
	if err := draft.Transition(MethodVersionStatusActive, now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition from terminal state, got %v", err)
	}
}

func TestMethodVersionRejectsSkipAndSelfTransition(t *testing.T) {
	now := time.Now().UTC()
	// draft must not skip over active directly into the deprecated terminal.
	draft := NewMethodVersion("mv-skip", now)
	if err := draft.Transition(MethodVersionStatusDeprecated, now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected draft -> deprecated to be rejected, got %v", err)
	}
	if draft.Status != MethodVersionStatusDraft {
		t.Fatalf("status changed despite rejected transition: %s", draft.Status)
	}
	// A self / no-op transition back to the current state is rejected.
	if err := draft.Transition(MethodVersionStatusDraft, now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected self transition to be rejected, got %v", err)
	}
	if draft.Status != MethodVersionStatusDraft {
		t.Fatalf("status changed despite rejected self transition: %s", draft.Status)
	}
}
