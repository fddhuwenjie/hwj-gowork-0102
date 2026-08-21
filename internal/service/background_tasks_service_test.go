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
	path := t.TempDir() + "/bt.db"
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	st := store.NewStore(db)
	clock := platform.SystemClock{}
	ids := platform.RandomIDGenerator{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return st, NewBackgroundTaskService(st, clock, ids, log)
}

func newSeedTask(now time.Time) *domain.BackgroundTask {
	t := domain.NewBackgroundTask("bt-1", now)
	t.TaskType = "report.summary"
	t.PayloadJSON = `{"v":"1"}`
	t.Attempts = 1
	t.MaxAttempts = 3
	return t
}

// TestBackgroundTaskUpdateVersionConflict is the regression test for the
// reported defect: after a maintainer rewrites a background task's payload
// (bumping its version), a second save carrying the now-stale version must be
// rejected with a version conflict instead of overwriting the latest payload
// and bumping the version again. A save carrying the current version still
// commits.
func TestBackgroundTaskUpdateVersionConflict(t *testing.T) {
	st, svc := newBackgroundTaskFixture(t)
	defer st.Close()
	ctx := context.Background()
	now := time.Now().UTC()

	created, err := svc.Create(ctx, newSeedTask(now), "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Version != 1 {
		t.Fatalf("expected version 1 after create, got %d", created.Version)
	}

	// Maintainer rewrites the payload, carrying the current version (1).
	latest := *created
	latest.PayloadJSON = `{"v":"2"}`
	if err := svc.Update(ctx, &latest, created.Version); err != nil {
		t.Fatalf("update with current version should commit: %v", err)
	}
	updated, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 {
		t.Fatalf("expected version 2 after maintainer update, got %d", updated.Version)
	}
	if updated.PayloadJSON != `{"v":"2"}` {
		t.Fatalf("latest payload not persisted: %q", updated.PayloadJSON)
	}

	// A second save carrying the stale version (1) must conflict, not overwrite.
	stale := *updated
	stale.PayloadJSON = `{"v":"old"}`
	err = svc.Update(ctx, &stale, created.Version) // created.Version == 1, but DB is at 2
	if !errors.Is(err, store.ErrVersionConflict) {
		t.Fatalf("expected version conflict for stale payload, got %v", err)
	}

	// The latest payload and version must be preserved after the rejected save.
	preserved, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preserved.Version != 2 {
		t.Fatalf("version changed after rejected stale save: %d", preserved.Version)
	}
	if preserved.PayloadJSON != `{"v":"2"}` {
		t.Fatalf("latest payload was overwritten by stale save: %q", preserved.PayloadJSON)
	}

	// A save carrying the now-current version (2) still commits and bumps to 3.
	current := *preserved
	current.PayloadJSON = `{"v":"3"}`
	if err := svc.Update(ctx, &current, preserved.Version); err != nil {
		t.Fatalf("update with current version should still commit: %v", err)
	}
	final, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Version != 3 || final.PayloadJSON != `{"v":"3"}` {
		t.Fatalf("expected version 3 / payload v3, got %d / %q", final.Version, final.PayloadJSON)
	}
}
