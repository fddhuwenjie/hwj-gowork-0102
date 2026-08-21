package store

import (
	"context"
	"testing"
	"time"

	"weld-ndt/internal/domain"
)

// TestEquipmentListFilterByCodeExact verifies the cross-layer derived-query fix:
// filtering the equipment list by "code" must return only the matching record(s),
// must report a total consistent with the returned set, and must NOT bleed in
// unrelated rows whose "name" happens to equal the queried code (the previous
// store-layer bug mapped code -> name) nor collapse all values onto "status"
// (the previous service-layer bug).
func TestEquipmentListFilterByCodeExact(t *testing.T) {
	st, _ := newTestStore(t)
	defer st.Close()
	ctx := context.Background()
	now := time.Now().UTC()

	mk := func(id, code, name, typ string) *domain.Equipment {
		e := domain.NewEquipment(id, now)
		e.Code = code
		e.Name = name
		e.Type = typ
		return e
	}

	// Target record.
	target := mk("eq-1", "DEV-001", "Probe A", "ut")
	// Decoy whose Name equals the target's Code. Under the old store bug
	// ("code" -> "name = ?") this decoy would leak into the results.
	decoy := mk("eq-2", "DEV-002", "DEV-001", "ut")
	// Decoy whose Code equals the target's Name. Under the old service bug
	// (every value mapped onto "status") filtering by code "DEV-001" would
	// have been reinterpreted as status "DEV-001" and matched nothing.
	nameLikeCode := mk("eq-3", "Probe A", "Probe B", "ut")

	for _, e := range []*domain.Equipment{target, decoy, nameLikeCode} {
		if err := st.Equipment.Create(ctx, st.DB, e); err != nil {
			t.Fatal(err)
		}
	}

	// Sanity: the target exists by id (mirrors "direct detail view still exists").
	got, err := st.Equipment.Get(ctx, st.DB, target.ID)
	if err != nil {
		t.Fatalf("target should exist by id: %v", err)
	}
	if got.Code != target.Code {
		t.Fatalf("expected code %q, got %q", target.Code, got.Code)
	}

	page := Page{Page: 1, Size: 20}
	items, total, err := st.Equipment.List(ctx, st.DB, map[string]any{"code": target.Code}, page, "")
	if err != nil {
		t.Fatalf("list by code: %v", err)
	}

	if total != int64(len(items)) {
		t.Fatalf("total %d != returned items %d (count and detail must share the same condition)", total, len(items))
	}
	if total != 1 {
		t.Fatalf("expected exactly 1 matching record for code %q, got total=%d", target.Code, total)
	}
	if len(items) != 1 || items[0].ID != target.ID {
		var ids []string
		for _, it := range items {
			ids = append(ids, it.ID)
		}
		t.Fatalf("expected only [%s], got %v", target.ID, ids)
	}

	// No-match filter must return empty results with a zero total.
	empty, emptyTotal, err := st.Equipment.List(ctx, st.DB, map[string]any{"code": "NO-SUCH-CODE"}, page, "")
	if err != nil {
		t.Fatalf("list by missing code: %v", err)
	}
	if emptyTotal != 0 || len(empty) != 0 {
		t.Fatalf("expected empty result for unknown code, got total=%d items=%d", emptyTotal, len(empty))
	}
}
