package service

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"weld-ndt/internal/domain"
	"weld-ndt/internal/platform"
	"weld-ndt/internal/store"
)

func newMethodVersionServiceFixture(t *testing.T) (*store.Store, *MethodVersionService) {
	t.Helper()
	st, _ := storeTestStore(t)
	clock := platform.SystemClock{}
	ids := platform.RandomIDGenerator{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return st, NewMethodVersionService(st, clock, ids, log)
}

// TestMethodVersionServiceListCoversAllRecordsOldestFirst drives the service
// layer end-to-end, which is where the first-page offset defect lived: List
// used to advance the requested page before delegating to the repository, so
// the first window started at the second page and the earliest records were
// dropped while the independent COUNT(*) kept the total unchanged. This test
// asserts that paging through the service returns every record exactly once,
// oldest-first, starting at page 1.
func TestMethodVersionServiceListCoversAllRecordsOldestFirst(t *testing.T) {
	st, svc := newMethodVersionServiceFixture(t)
	defer st.Close()
	ctx := context.Background()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const n = 25
	for i := 0; i < n; i++ {
		mv := domain.NewMethodVersion("mv-svc-"+svcItoa(i), base.Add(time.Duration(i)*time.Second))
		mv.Code = "RT"
		mv.VersionNo = 2026
		mv.Standard = "NB/T"
		mv.Status = domain.MethodVersionStatusActive
		if _, err := svc.Create(ctx, mv, ""); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	const size = 10
	var collected []string
	var total int64
	for p := 1; ; p++ {
		items, tot, err := svc.List(ctx, nil, domain.Page{Page: p, Size: size}, "")
		if err != nil {
			t.Fatalf("list page %d: %v", p, err)
		}
		if p == 1 {
			total = tot
		}
		for _, it := range items {
			collected = append(collected, it.ID)
		}
		if len(items) < size {
			break
		}
	}

	if total != int64(n) {
		t.Fatalf("total = %d, want %d (count must not change when paging)", total, n)
	}
	if len(collected) != n {
		t.Fatalf("traversed %d records, want %d (full traversal must cover every record)", len(collected), n)
	}
	seen := make(map[string]int, len(collected))
	for _, id := range collected {
		seen[id]++
		if seen[id] > 1 {
			t.Fatalf("record %s returned more than once across pages", id)
		}
	}

	// Page 1 must start at the earliest record, proving the service does not
	// advance past the requested first page.
	if got, want := collected[0], "mv-svc-0"; got != want {
		t.Fatalf("page 1 does not start at the earliest record: got %s, want %s", got, want)
	}

	// Sequence must be strictly ascending by created_at across the whole traversal.
	prev := base
	for i, id := range collected {
		item, err := svc.Get(ctx, id)
		if err != nil {
			t.Fatalf("get %d: %v", i, err)
		}
		if item.CreatedAt.Before(prev) {
			t.Fatalf("created_at went backwards at index %d: %s < %s", i, item.CreatedAt, prev)
		}
		prev = item.CreatedAt
	}
}

// svcItoa is a local int->string helper to keep this test file free of extra imports.
func svcItoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
