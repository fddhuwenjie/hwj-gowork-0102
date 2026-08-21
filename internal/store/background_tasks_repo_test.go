package store

import (
	"context"
	"testing"
	"time"

	"weld-ndt/internal/domain"
)

// TestBackgroundTaskListPaginationAndSort guards the regression where:
//   - the service incremented the requested page before delegating to the
//     repository (window off-by-one -> earliest record skipped, gaps between
//     windows), and
//   - the repository ordered bare created_at without a deterministic id
//     tiebreaker / wrong direction, so equal created_at values could be
//     reordered across LIMIT/OFFSET queries (overlaps or gaps).
//
// After the fix, paging by created_at ASC over the full set must return every
// task exactly once with no duplicates and no gaps, starting from the earliest.
func TestBackgroundTaskListPaginationAndSort(t *testing.T) {
	st, _ := newTestStore(t)
	defer st.Close()

	ctx := context.Background()
	base := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)

	// Three tasks at three distinct timestamps; earliest must come first.
	tasks := []*domain.BackgroundTask{
		domain.NewBackgroundTask("bt-a", base.Add(1*time.Second)),
		domain.NewBackgroundTask("bt-b", base.Add(2*time.Second)),
		domain.NewBackgroundTask("bt-c", base.Add(3*time.Second)),
	}
	for _, tk := range tasks {
		tk.TaskType = "inspect"
		tk.Attempts = 1
		tk.MaxAttempts = 3
		if err := st.BackgroundTask.Create(ctx, st.DB, tk); err != nil {
			t.Fatalf("create %s: %v", tk.ID, err)
		}
	}

	// Page by created_at with no explicit direction: convention requires
	// earliest-first (ASC) with a stable id tiebreaker.
	const size = 1
	var seen []string
	total := int64(-1)
	for page := 1; ; page++ {
		p := domain.Page{Page: page, Size: size}
		items, n, err := st.BackgroundTask.List(ctx, st.DB, nil, p, "created_at")
		if err != nil {
			t.Fatalf("list page %d: %v", page, err)
		}
		if total == -1 {
			total = n
		} else if n != total {
			t.Fatalf("total changed across pages: %d -> %d", total, n)
		}
		if len(items) == 0 {
			break
		}
		for _, it := range items {
			seen = append(seen, it.ID)
		}
	}

	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}
	want := []string{"bt-a", "bt-b", "bt-c"}
	if len(seen) != len(want) {
		t.Fatalf("expected %d items across windows, got %d (%v)", len(want), len(seen), seen)
	}
	for i, id := range want {
		if seen[i] != id {
			t.Fatalf("window %d: expected %s, got %s (seen=%v)", i, id, seen[i], seen)
		}
	}
}

// TestBackgroundTaskListEqualCreatedAtStableOrdering ensures that when several
// tasks share the same created_at, ASC paging with the id tiebreaker yields a
// total order that pages cleanly: every id appears exactly once across windows.
func TestBackgroundTaskListEqualCreatedAtStableOrdering(t *testing.T) {
	st, _ := newTestStore(t)
	defer st.Close()

	ctx := context.Background()
	same := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	ids := []string{"bt-1", "bt-2", "bt-3"}
	for _, id := range ids {
		tk := domain.NewBackgroundTask(id, same)
		tk.TaskType = "inspect"
		tk.Attempts = 1
		tk.MaxAttempts = 3
		if err := st.BackgroundTask.Create(ctx, st.DB, tk); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}

	const size = 1
	var seen []string
	for page := 1; page <= len(ids); page++ {
		p := domain.Page{Page: page, Size: size}
		items, total, err := st.BackgroundTask.List(ctx, st.DB, nil, p, "created_at")
		if err != nil {
			t.Fatalf("list page %d: %v", page, err)
		}
		if total != int64(len(ids)) {
			t.Fatalf("expected total %d, got %d", len(ids), total)
		}
		if len(items) != 1 {
			t.Fatalf("page %d: expected 1 item, got %d", page, len(items))
		}
		seen = append(seen, items[0].ID)
	}

	got := map[string]bool{}
	for _, id := range seen {
		if got[id] {
			t.Fatalf("duplicate %s across windows (overlap): %v", id, seen)
		}
		got[id] = true
	}
	if len(got) != len(ids) {
		t.Fatalf("missing tasks after paging: seen=%v", seen)
	}

	// An empty page beyond the end confirms no overlap and a clean stop.
	p := domain.Page{Page: len(ids) + 1, Size: size}
	items, _, err := st.BackgroundTask.List(ctx, st.DB, nil, p, "created_at")
	if err != nil {
		t.Fatalf("list overflow page: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty page beyond end, got %d items", len(items))
	}
}
