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

// newEquipmentListFixture wires an EquipmentService against a fresh SQLite store.
func newEquipmentListFixture(t *testing.T) (*store.Store, *EquipmentService) {
	t.Helper()
	st, _ := storeTestStore(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewEquipmentService(st, platform.SystemClock{}, platform.RandomIDGenerator{}, log)
	return st, svc
}

// seedEquipment creates count equipment rows with strictly increasing CreatedAt
// (and matching increasing Code/ID) so ordering is unambiguous. It returns the
// rows in creation order (earliest first).
func seedEquipment(t *testing.T, svc *EquipmentService, count int) []*domain.Equipment {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	out := make([]*domain.Equipment, 0, count)
	for i := 0; i < count; i++ {
		idx := i + 1
		// Distinct CreatedAt per row; Code/Name/Type reflect the same order.
		created := base.Add(time.Duration(i) * time.Minute)
		eq := domain.NewEquipment("eq-list-"+itoa(idx), created)
		eq.Code = "EQ-" + itoa(idx)
		eq.Name = "Probe " + itoa(idx)
		eq.Type = "UT"
		if _, err := svc.Create(ctx, eq, ""); err != nil {
			t.Fatalf("seed equipment %d: %v", idx, err)
		}
		out = append(out, eq)
	}
	return out
}

// TestEquipmentListDefaultAscendingPaging locks down the two regressions this
// change fixes for maintenance records reviewed by creation time:
//  1. With sort=created_at and no explicit direction, the default is ascending
//     (earliest first), not descending.
//  2. The requested page number is honored: page 1 starts at offset 0, so the
//     earliest records are returned on the first page rather than skipped to
//     the next page. Consecutive pages tile without overlap or gaps and total
//     stays complete.
func TestEquipmentListDefaultAscendingPaging(t *testing.T) {
	st, svc := newEquipmentListFixture(t)
	defer st.Close()

	const n = 5
	seeded := seedEquipment(t, svc, n) // earliest = seeded[0]

	// Page through with size 2; sort=created_at (bare field -> default asc).
	var collected []*domain.Equipment
	var total int64
	for p := 1; ; p++ {
		page := domain.Page{Page: p, Size: 2}
		items, tot, err := svc.List(context.Background(), nil, page, "created_at")
		if err != nil {
			t.Fatalf("page %d: %v", p, err)
		}
		if p == 1 {
			total = tot
		}
		collected = append(collected, items...)
		if len(items) < page.Size {
			break
		}
	}

	// Total must reflect all rows regardless of paging.
	if total != n {
		t.Fatalf("total = %d, want %d", total, n)
	}
	if len(collected) != n {
		t.Fatalf("collected %d rows across pages, want %d (gap or overlap)", len(collected), n)
	}

	// Earliest records must be first, in ascending creation order.
	for i, want := range seeded {
		got := collected[i]
		if got.ID != want.ID {
			t.Fatalf("position %d: got id %s (%s), want %s (%s)",
				i, got.ID, got.CreatedAt.Format(time.RFC3339), want.ID, want.CreatedAt.Format(time.RFC3339))
		}
		if !got.CreatedAt.Equal(want.CreatedAt) {
			t.Fatalf("position %d: created_at %s, want %s", i, got.CreatedAt, want.CreatedAt)
		}
	}

	// No duplicates and no gaps: every seen id unique and equals the seed set.
	seen := map[string]bool{}
	for _, e := range collected {
		if seen[e.ID] {
			t.Fatalf("duplicate id across pages: %s", e.ID)
		}
		seen[e.ID] = true
	}
	for _, e := range seeded {
		if !seen[e.ID] {
			t.Fatalf("missing id across pages: %s (gap)", e.ID)
		}
	}
}

// TestEquipmentListDefaultDescendingUnchanged confirms the no-sort default is
// still newest-first (DESC), so we only changed the bare-field ascending
// contract, not the implicit default.
func TestEquipmentListDefaultDescendingUnchanged(t *testing.T) {
	st, svc := newEquipmentListFixture(t)
	defer st.Close()
	seeded := seedEquipment(t, svc, 4) // seeded[3] is newest

	items, total, err := svc.List(context.Background(), nil, domain.Page{Page: 1, Size: 4}, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 {
		t.Fatalf("total = %d, want 4", total)
	}
	// Newest first: seeded[3], seeded[2], seeded[1], seeded[0]
	for i, want := range []*domain.Equipment{seeded[3], seeded[2], seeded[1], seeded[0]} {
		if items[i].ID != want.ID {
			t.Fatalf("position %d: got %s, want %s", i, items[i].ID, want.ID)
		}
	}
}

// TestEquipmentListExplicitDirections checks that explicit asc/desc still work,
// guarding against the bare-field fix accidentally forcing one direction.
func TestEquipmentListExplicitDirections(t *testing.T) {
	st, svc := newEquipmentListFixture(t)
	defer st.Close()
	seeded := seedEquipment(t, svc, 3) // [0]=earliest, [2]=newest

	asc, _, err := svc.List(context.Background(), nil, domain.Page{Page: 1, Size: 3}, "created_at asc")
	if err != nil {
		t.Fatal(err)
	}
	if asc[0].ID != seeded[0].ID || asc[2].ID != seeded[2].ID {
		t.Fatalf("asc order wrong: %s,%s,%s", asc[0].ID, asc[1].ID, asc[2].ID)
	}

	desc, _, err := svc.List(context.Background(), nil, domain.Page{Page: 1, Size: 3}, "created_at desc")
	if err != nil {
		t.Fatal(err)
	}
	if desc[0].ID != seeded[2].ID || desc[2].ID != seeded[0].ID {
		t.Fatalf("desc order wrong: %s,%s,%s", desc[0].ID, desc[1].ID, desc[2].ID)
	}
}

// TestEquipmentListPageOneStartsAtOffsetZero directly targets the old Page++
// bug: with sort=created_at, page 1 size 2 must return the two earliest rows,
// not the second page.
func TestEquipmentListPageOneStartsAtOffsetZero(t *testing.T) {
	st, svc := newEquipmentListFixture(t)
	defer st.Close()
	seeded := seedEquipment(t, svc, 4)

	items, total, err := svc.List(context.Background(), nil, domain.Page{Page: 1, Size: 2}, "created_at")
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 {
		t.Fatalf("total = %d, want 4", total)
	}
	if len(items) != 2 {
		t.Fatalf("page 1 returned %d items, want 2", len(items))
	}
	if items[0].ID != seeded[0].ID || items[1].ID != seeded[1].ID {
		t.Fatalf("page 1 = [%s,%s], want [%s,%s] (earliest first)",
			items[0].ID, items[1].ID, seeded[0].ID, seeded[1].ID)
	}
}

// itoa avoids pulling strconv just for small fixed-range indices.
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
