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
