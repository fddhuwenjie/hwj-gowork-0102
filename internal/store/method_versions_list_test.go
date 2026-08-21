package store

import (
	"context"
	"testing"
	"time"

	"weld-ndt/internal/domain"
)

// newMethodVersion builds a valid method version with the given id and creation
// time so the repository's List ordering can be exercised deterministically.
func newMethodVersion(id string, createdAt time.Time) *domain.MethodVersion {
	mv := domain.NewMethodVersion(id, createdAt)
	mv.Code = "RT"
	mv.VersionNo = 2026
	mv.Standard = "NB/T"
	mv.Status = domain.MethodVersionStatusActive
	return mv
}

// TestMethodVersionListCoversAllRecordsOldestFirst asserts that the release
// history pagination starts at the earliest record and that a full forward
// traversal across pages returns every record exactly once, in ascending
// creation-time order. This is the regression guard for the two defects:
//   - the service must not advance the requested page before listing (which
//     skipped the first window and dropped the oldest records while the
//     independent COUNT(*) left the total unchanged);
//   - the repository must order oldest-first so successive windows line up
//     end-to-end instead of reversing within each page.
func TestMethodVersionListCoversAllRecordsOldestFirst(t *testing.T) {
	st, _ := newTestStore(t)
	defer st.Close()
	ctx := context.Background()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const n = 25 // > one page so pagination boundaries are exercised
	for i := 0; i < n; i++ {
		mv := newMethodVersion("mv-old-first-"+itoa(i), base.Add(time.Duration(i)*time.Second))
		if err := st.MethodVersion.Create(ctx, st.DB, mv); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	const size = 10
	var collected []string
	var total int64
	for p := 1; ; p++ {
		page := domain.Page{Page: p, Size: size}
		items, tot, err := st.MethodVersion.List(ctx, st.DB, nil, page, "")
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
		t.Fatalf("total = %d, want %d (must not drop count when paging)", total, n)
	}
	if len(collected) != n {
		t.Fatalf("traversed %d records, want %d (each record must be covered exactly once)", len(collected), n)
	}
	seen := make(map[string]int, len(collected))
	for _, id := range collected {
		seen[id]++
		if seen[id] > 1 {
			t.Fatalf("record %s returned more than once across pages", id)
		}
	}

	// Oldest-first: the first returned id must be the earliest record and the
	// sequence must be monotonically ascending by created_at.
	first, err := st.MethodVersion.Get(ctx, st.DB, collected[0])
	if err != nil {
		t.Fatalf("get first: %v", err)
	}
	if !first.CreatedAt.Equal(base) {
		t.Fatalf("first page does not start at the earliest record: got created_at %s, want %s", first.CreatedAt, base)
	}
	var prev time.Time
	for i, id := range collected {
		item, err := st.MethodVersion.Get(ctx, st.DB, id)
		if err != nil {
			t.Fatalf("get %d: %v", i, err)
		}
		if i == 0 {
			prev = item.CreatedAt
			continue
		}
		if item.CreatedAt.Before(prev) {
			t.Fatalf("page %d: created_at went backwards at index %d: %s < %s", (i/size)+1, i, item.CreatedAt, prev)
		}
		prev = item.CreatedAt
	}
}

// TestMethodVersionListFirstPageMatchesEarliest is a focused check that the very
// first page returns the same leading record as an oldest-first full scan,
// i.e. pagination genuinely starts at page 1 rather than a later window.
func TestMethodVersionListFirstPageMatchesEarliest(t *testing.T) {
	st, _ := newTestStore(t)
	defer st.Close()
	ctx := context.Background()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		mv := newMethodVersion("mv-page1-"+itoa(i), base.Add(time.Duration(i)*time.Second))
		if err := st.MethodVersion.Create(ctx, st.DB, mv); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	firstPage, _, err := st.MethodVersion.List(ctx, st.DB, nil, domain.Page{Page: 1, Size: 3}, "")
	if err != nil {
		t.Fatalf("list page 1: %v", err)
	}
	if len(firstPage) == 0 {
		t.Fatalf("page 1 returned no records")
	}
	if firstPage[0].ID != "mv-page1-0" {
		t.Fatalf("page 1 does not start at the earliest record: got %s, want mv-page1-0", firstPage[0].ID)
	}
}

// itoa keeps the test free of strconv usage to match the surrounding style.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
