package service

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"weld-ndt/internal/domain"
	"weld-ndt/internal/platform"
	"weld-ndt/internal/store"
)

func newBackgroundTaskTestService(t *testing.T) (*BackgroundTaskService, *store.Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bg.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	st := store.NewStore(db)
	svc := NewBackgroundTaskService(st, platform.SystemClock{}, platform.RandomIDGenerator{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return svc, st
}

// Regression: the service previously incremented the requested page before
// delegating to the repository, shifting every window by +1 page. Paging
// created_at ASC over the full set must return all tasks exactly once with no
// gaps and starting from the earliest record.
func TestBackgroundTaskServiceListNoPageOffset(t *testing.T) {
	svc, st := newBackgroundTaskTestService(t)
	defer st.Close()
	ctx := context.Background()

	base := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	for i, id := range []string{"earliest", "middle", "latest"} {
		tk := domain.NewBackgroundTask(id, base.Add(time.Duration(i+1)*time.Second))
		tk.TaskType = "inspect"
		tk.Attempts = 1
		tk.MaxAttempts = 3
		if _, err := svc.Create(ctx, tk, ""); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}

	const size = 1
	var seen []string
	for page := 1; page <= 3; page++ {
		p := domain.Page{Page: page, Size: size}
		items, total, err := svc.List(ctx, nil, p, "created_at")
		if err != nil {
			t.Fatalf("list page %d: %v", page, err)
		}
		if total != 3 {
			t.Fatalf("expected total 3, got %d", total)
		}
		if len(items) != 1 {
			t.Fatalf("page %d: expected 1 item, got %d", page, len(items))
		}
		seen = append(seen, items[0].ID)
	}

	want := []string{"earliest", "middle", "latest"}
	for i, id := range want {
		if seen[i] != id {
			t.Fatalf("window %d: expected %s, got %s (seen=%v)", i, id, seen[i], seen)
		}
	}

	// Beyond the end the service must return an empty page (no phantom extra row).
	items, _, err := svc.List(ctx, nil, domain.Page{Page: 4, Size: size}, "created_at")
	if err != nil {
		t.Fatalf("overflow list: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty page beyond end, got %d items", len(items))
	}
}
