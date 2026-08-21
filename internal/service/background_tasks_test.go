package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"weld-ndt/internal/domain"
	"weld-ndt/internal/platform"
	"weld-ndt/internal/store"
)

func newBackgroundTaskFixture(t *testing.T) (*store.Store, *BackgroundTaskService) {
	t.Helper()
	path := t.TempDir() + "/bg.db"
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	st := store.NewStore(db)
	svc := NewBackgroundTaskService(st, platform.SystemClock{}, platform.RandomIDGenerator{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return st, svc
}

func seedBackgroundTask(t *testing.T, svc *BackgroundTaskService, id string) *domain.BackgroundTask {
	t.Helper()
	now := time.Now().UTC()
	task := domain.NewBackgroundTask(id, now)
	task.TaskType = "reindex"
	task.Attempts = 1
	task.MaxAttempts = 3
	created, err := svc.Create(context.Background(), task, "")
	if err != nil {
		t.Fatal(err)
	}
	return created
}

// auditCount returns the number of audit records for an entity_id matching the
// given action filter (empty action => any action).
func auditCount(t *testing.T, st *store.Store, entityID, action string) int64 {
	t.Helper()
	filter := map[string]any{"entity_id": entityID}
	if action != "" {
		filter["action"] = action
	}
	_, total, err := st.AuditRecord.List(context.Background(), st.DB, filter, domain.Page{Page: 1, Size: 200}, "created_at asc")
	if err != nil {
		t.Fatal(err)
	}
	return total
}

func TestBackgroundTaskRejectsCrossLevelTransition(t *testing.T) {
	st, svc := newBackgroundTaskFixture(t)
	defer st.Close()
	task := seedBackgroundTask(t, svc, "bg-cross")
	ctx := context.Background()

	// pending -> succeeded is a cross-level jump; must be rejected.
	err := svc.Transition(ctx, task.ID, domain.BackgroundTaskStatusSucceeded, task.Version)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}

	// Original state and version must be preserved.
	after, err := svc.Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != domain.BackgroundTaskStatusPending {
		t.Fatalf("status must remain pending, got %s", after.Status)
	}
	if after.Version != task.Version {
		t.Fatalf("version must remain %d, got %d", task.Version, after.Version)
	}
	// No success audit must be left behind; only the create record.
	if got := auditCount(t, st, task.ID, "transition"); got != 0 {
		t.Fatalf("expected 0 transition audits on rejected transition, got %d", got)
	}
}

func TestBackgroundTaskRejectsDuplicateTransition(t *testing.T) {
	st, svc := newBackgroundTaskFixture(t)
	defer st.Close()
	task := seedBackgroundTask(t, svc, "bg-dup")
	ctx := context.Background()

	// pending -> pending is a duplicate/self transition; must be rejected.
	err := svc.Transition(ctx, task.ID, domain.BackgroundTaskStatusPending, task.Version)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}

	after, err := svc.Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != domain.BackgroundTaskStatusPending {
		t.Fatalf("status must remain pending, got %s", after.Status)
	}
	if after.Version != task.Version {
		t.Fatalf("version must remain %d, got %d", task.Version, after.Version)
	}
	if got := auditCount(t, st, task.ID, "transition"); got != 0 {
		t.Fatalf("expected 0 transition audits on rejected transition, got %d", got)
	}
}

func TestBackgroundTaskAllowsAdjacentTransitions(t *testing.T) {
	st, svc := newBackgroundTaskFixture(t)
	defer st.Close()
	task := seedBackgroundTask(t, svc, "bg-ok")
	ctx := context.Background()

	// Legitimate adjacent chain: pending -> running -> succeeded.
	if err := svc.Transition(ctx, task.ID, domain.BackgroundTaskStatusRunning, task.Version); err != nil {
		t.Fatalf("expected pending->running: %v", err)
	}
	running, err := svc.Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if running.Status != domain.BackgroundTaskStatusRunning {
		t.Fatalf("expected running, got %s", running.Status)
	}
	if running.Version != task.Version+1 {
		t.Fatalf("expected version %d, got %d", task.Version+1, running.Version)
	}

	if err := svc.Transition(ctx, task.ID, domain.BackgroundTaskStatusSucceeded, running.Version); err != nil {
		t.Fatalf("expected running->succeeded: %v", err)
	}
	succeeded, err := svc.Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if succeeded.Status != domain.BackgroundTaskStatusSucceeded {
		t.Fatalf("expected succeeded, got %s", succeeded.Status)
	}
	if succeeded.Version != running.Version+1 {
		t.Fatalf("expected version %d, got %d", running.Version+1, succeeded.Version)
	}

	// Two successful transitions => two transition audit records with the
	// recorded status, and the persisted meta carries previous_status.
	if got := auditCount(t, st, task.ID, "transition"); got != 2 {
		t.Fatalf("expected 2 transition audits, got %d", got)
	}
	if succeeded.MetaString("previous_status") != domain.BackgroundTaskStatusRunning {
		t.Fatalf("expected previous_status=running, got %q", succeeded.MetaString("previous_status"))
	}
}

func TestBackgroundTaskVersionConflictOnStaleTransition(t *testing.T) {
	st, svc := newBackgroundTaskFixture(t)
	defer st.Close()
	task := seedBackgroundTask(t, svc, "bg-stale")
	ctx := context.Background()

	// Advance once, then attempt a transition with a stale (old) version.
	if err := svc.Transition(ctx, task.ID, domain.BackgroundTaskStatusRunning, task.Version); err != nil {
		t.Fatal(err)
	}
	err := svc.Transition(ctx, task.ID, domain.BackgroundTaskStatusSucceeded, task.Version) // stale version
	if !errors.Is(err, store.ErrVersionConflict) {
		t.Fatalf("expected ErrVersionConflict, got %v", err)
	}
}
