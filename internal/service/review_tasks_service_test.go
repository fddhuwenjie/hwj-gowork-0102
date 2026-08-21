package service

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"weld-ndt/internal/domain"
	"weld-ndt/internal/platform"
)

func newReviewTaskFixture(t *testing.T) *ReviewTaskService {
	t.Helper()
	st, _ := storeTestStore(t)
	t.Cleanup(func() { st.Close() })
	return NewReviewTaskService(st, platform.SystemClock{}, platform.RandomIDGenerator{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// makeReviewTask builds a valid review task ready for repo.Create. No foreign
// keys are declared on the review_tasks table, so report_id/weld_id only need
// to be non-empty to satisfy ReviewTask.Validate.
func makeReviewTask(id, reviewer, status string) *domain.ReviewTask {
	now := time.Now().UTC()
	rt := domain.NewReviewTask(id, now)
	rt.ReportID = "rep-" + id
	rt.WeldID = "weld-" + id
	rt.Reviewer = reviewer
	if status != "" {
		rt.Status = status
	}
	return rt
}

func TestReviewTaskListByReviewer(t *testing.T) {
	svc := newReviewTaskFixture(t)
	ctx := context.Background()

	// Seed tasks across two reviewers plus one empty-reviewer task. The empty
	// reviewer must never match the reviewer filter.
	for _, id := range []string{"rt-a1", "rt-a2", "rt-b1"} {
		reviewer := "alice"
		if id == "rt-b1" {
			reviewer = "bob"
		}
		if err := svc.repo.Create(ctx, svc.store.DB, makeReviewTask(id, reviewer, "")); err != nil {
			t.Fatal(err)
		}
	}
	// Task with an unset reviewer to guard against accidental matches.
	blank := makeReviewTask("rt-blank", "", "")
	if err := svc.repo.Create(ctx, svc.store.DB, blank); err != nil {
		t.Fatal(err)
	}

	// Fetch by reviewer=alice. Must return exactly alice's two tasks.
	got, total, err := svc.List(ctx, map[string]any{"reviewer": "alice"}, domain.Page{Page: 1, Size: 50}, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total=2 for reviewer=alice, got %d", total)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 items for reviewer=alice, got %d", len(got))
	}
	for _, item := range got {
		if item.Reviewer != "alice" {
			t.Fatalf("filter leaked non-matching reviewer %q", item.Reviewer)
		}
		if item.ReportID == "" {
			t.Fatalf("expected report_id to be preserved, got empty for %s", item.ID)
		}
	}

	// Count and detail must agree on the bob slice too.
	gotBob, totalBob, err := svc.List(ctx, map[string]any{"reviewer": "bob"}, domain.Page{Page: 1, Size: 50}, "")
	if err != nil {
		t.Fatalf("List bob: %v", err)
	}
	if totalBob != 1 || len(gotBob) != 1 {
		t.Fatalf("bob: expected total=1 and 1 item, got total=%d items=%d", totalBob, len(gotBob))
	}
	if gotBob[0].Reviewer != "bob" {
		t.Fatalf("bob item reviewer=%q", gotBob[0].Reviewer)
	}

	// Count vs paginated detail consistency: a page size smaller than the
	// result set must still report the full count for alice.
	_, totalPaged, err := svc.List(ctx, map[string]any{"reviewer": "alice"}, domain.Page{Page: 1, Size: 1}, "")
	if err != nil {
		t.Fatalf("List paged: %v", err)
	}
	if totalPaged != 2 {
		t.Fatalf("paged count for alice drifted from total: got %d want 2", totalPaged)
	}

	// A reviewer value that coincides with a report_id must NOT match via the
	// previously buggy report_id mapping. "rep-rt-a1" is a real report_id; the
	// reviewer filter on that string should return zero tasks.
	cross, totalCross, err := svc.List(ctx, map[string]any{"reviewer": "rep-rt-a1"}, domain.Page{Page: 1, Size: 50}, "")
	if err != nil {
		t.Fatalf("List cross: %v", err)
	}
	if totalCross != 0 || len(cross) != 0 {
		t.Fatalf("reviewer=report_id must not match: got total=%d items=%d", totalCross, len(cross))
	}

	// Get-by-id is unaffected and should still resolve the seeded task.
	fetched, err := svc.Get(ctx, "rt-a1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetched.Reviewer != "alice" {
		t.Fatalf("Get reviewer mismatch: %q", fetched.Reviewer)
	}
}

// TestReviewTaskListPreservesAllFilters guards the service-level regression
// where multiple query params were collapsed into a single status value.
func TestReviewTaskListPreservesAllFilters(t *testing.T) {
	svc := newReviewTaskFixture(t)
	ctx := context.Background()

	a1 := makeReviewTask("rt-pa1", "alice", domain.ReviewTaskStatusApproved)
	if err := svc.repo.Create(ctx, svc.store.DB, a1); err != nil {
		t.Fatal(err)
	}
	a2 := makeReviewTask("rt-pa2", "alice", domain.ReviewTaskStatusPending)
	if err := svc.repo.Create(ctx, svc.store.DB, a2); err != nil {
		t.Fatal(err)
	}

	// reviewer=alice AND status=approved must yield exactly one task.
	got, total, err := svc.List(ctx, map[string]any{"reviewer": "alice", "status": domain.ReviewTaskStatusApproved}, domain.Page{Page: 1, Size: 50}, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(got) != 1 {
		t.Fatalf("expected 1 approved alice task, got total=%d items=%d", total, len(got))
	}
	if got[0].ID != "rt-pa1" {
		t.Fatalf("expected rt-pa1, got %s", got[0].ID)
	}
}
