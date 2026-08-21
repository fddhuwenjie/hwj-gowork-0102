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

func newReviewTaskFixture(t *testing.T) (*store.Store, *ReviewTaskService) {
	t.Helper()
	path := t.TempDir() + "/review.db"
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
	return st, NewReviewTaskService(st, clock, ids, log)
}

func seedReviewTask(t *testing.T, st *store.Store, id string) *domain.ReviewTask {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	rt := domain.NewReviewTask(id, now)
	rt.ReportID = "rep-1"
	rt.WeldID = "weld-1"
	rt.Reviewer = "alice"
	if err := st.ReviewTask.Create(ctx, st.DB, rt); err != nil {
		t.Fatal(err)
	}
	return rt
}

// TestReviewTaskUpdateStaleVersionConflicts reproduces the reported bug: a
// maintainer holding a stale version that tries to save a different reviewer
// must hit a version conflict and the latest reviewer must be preserved.
func TestReviewTaskUpdateStaleVersionConflicts(t *testing.T) {
	st, svc := newReviewTaskFixture(t)
	defer st.Close()
	ctx := context.Background()

	// Seed a review task at version 1.
	original := seedReviewTask(t, st, "rt-stale")
	if original.Version != 1 || original.Reviewer != "alice" {
		t.Fatalf("unexpected seed state: version=%d reviewer=%q", original.Version, original.Reviewer)
	}

	// Maintainer A: save a different reviewer carrying the current version (1).
	// This must succeed and bump the version to 2.
	updA := original.Clone()
	updA.Reviewer = "bob"
	if err := svc.Update(ctx, updA, 1); err != nil {
		t.Fatalf("maintainer A update failed: %v", err)
	}
	got, err := st.ReviewTask.Get(ctx, st.DB, "rt-stale")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 2 || got.Reviewer != "bob" {
		t.Fatalf("after A: expected version=2 reviewer=bob, got version=%d reviewer=%q", got.Version, got.Reviewer)
	}

	// Maintainer B still holds the stale version (1) from before A's save and
	// tries to save a different reviewer. This MUST conflict, and the latest
	// reviewer (bob) must be preserved.
	staleB := original.Clone() // still version 1
	staleB.Reviewer = "carol"
	err = svc.Update(ctx, staleB, 1)
	if !errors.Is(err, store.ErrVersionConflict) {
		t.Fatalf("expected version conflict for stale update, got %v", err)
	}
	final, err := st.ReviewTask.Get(ctx, st.DB, "rt-stale")
	if err != nil {
		t.Fatal(err)
	}
	if final.Version != 2 || final.Reviewer != "bob" {
		t.Fatalf("stale update clobbered latest: version=%d reviewer=%q (want version=2 reviewer=bob)", final.Version, final.Reviewer)
	}
}

// TestReviewTaskUpdateCurrentVersionSucceeds ensures an update carrying the
// current version still applies and increments the version, including a
// second consecutive update on the freshly written value.
func TestReviewTaskUpdateCurrentVersionSucceeds(t *testing.T) {
	st, svc := newReviewTaskFixture(t)
	defer st.Close()
	ctx := context.Background()

	original := seedReviewTask(t, st, "rt-current")
	if original.Version != 1 {
		t.Fatalf("unexpected seed version %d", original.Version)
	}

	// First update carrying the current version (1) -> must succeed, version 2.
	first := original.Clone()
	first.Reviewer = "dave"
	if err := svc.Update(ctx, first, 1); err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	if first.Version != 2 {
		t.Fatalf("first update did not bump version: got %d", first.Version)
	}

	// Second update carrying the now-current version (2) -> must succeed, version 3.
	second := first.Clone()
	second.Reviewer = "erin"
	if err := svc.Update(ctx, second, 2); err != nil {
		t.Fatalf("second update failed: %v", err)
	}
	got, err := st.ReviewTask.Get(ctx, st.DB, "rt-current")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 3 || got.Reviewer != "erin" {
		t.Fatalf("expected version=3 reviewer=erin, got version=%d reviewer=%q", got.Version, got.Reviewer)
	}
}

// TestReviewTaskRepositoryUpdateGuardsVersion confirms the repository's
// UPDATE statement itself rejects a version mismatch (defence-in-depth even
// if the service layer check were bypassed).
func TestReviewTaskRepositoryUpdateGuardsVersion(t *testing.T) {
	st, _ := newReviewTaskFixture(t)
	defer st.Close()
	ctx := context.Background()

	original := seedReviewTask(t, st, "rt-repo")
	upd := original.Clone()
	upd.Reviewer = "frank"

	// Wrong expected version must produce ErrVersionConflict at the repo layer.
	if err := st.ReviewTask.Update(ctx, st.DB, upd, 999); !errors.Is(err, store.ErrVersionConflict) {
		t.Fatalf("expected ErrVersionConflict for wrong version at repo, got %v", err)
	}
	// Correct version must succeed.
	if err := st.ReviewTask.Update(ctx, st.DB, upd, 1); err != nil {
		t.Fatalf("repo update with correct version failed: %v", err)
	}
	if upd.Version != 2 {
		t.Fatalf("repo update did not bump version: got %d", upd.Version)
	}
}
