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

func TestBackgroundTaskStateMachine(t *testing.T) {
	now := time.Now().UTC()

	// Cross-level transition (pending -> succeeded) must be rejected.
	skip := NewBackgroundTask("bg-1", now)
	if err := skip.Transition(BackgroundTaskStatusSucceeded, now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition pending->succeeded, got %v", err)
	}
	if skip.Status != BackgroundTaskStatusPending {
		t.Fatalf("status changed on rejected transition: %s", skip.Status)
	}

	// Self / duplicate transition (pending -> pending) must be rejected.
	dup := NewBackgroundTask("bg-2", now)
	if err := dup.Transition(BackgroundTaskStatusPending, now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition pending->pending, got %v", err)
	}

	// Legitimate adjacent transitions must succeed and record previous status.
	task := NewBackgroundTask("bg-3", now)
	if err := task.Transition(BackgroundTaskStatusRunning, now); err != nil {
		t.Fatalf("expected pending->running: %v", err)
	}
	if task.Status != BackgroundTaskStatusRunning {
		t.Fatalf("unexpected status %s", task.Status)
	}
	if task.MetaString("previous_status") != BackgroundTaskStatusPending {
		t.Fatalf("previous_status not recorded: %q", task.MetaString("previous_status"))
	}
	if err := task.Transition(BackgroundTaskStatusSucceeded, now); err != nil {
		t.Fatalf("expected running->succeeded: %v", err)
	}
	if task.Status != BackgroundTaskStatusSucceeded {
		t.Fatalf("unexpected status %s", task.Status)
	}

	// Terminal states allow no further transitions.
	if err := task.Transition(BackgroundTaskStatusFailed, now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition succeeded->failed, got %v", err)
	}
}
