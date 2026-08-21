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

// TestEquipmentServiceListByCode guards the service-layer half of the cross-layer
// derived-query fix. The old List collapsed every filter value onto the "status"
// key, so filtering by equipment code silently became a status filter and matched
// nothing (or the wrong rows). The handler-supplied filter map must pass through
// unchanged.
func TestEquipmentServiceListByCode(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(t.TempDir() + "/svc-eq.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	st := store.NewStore(db)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewEquipmentService(st, platform.SystemClock{}, platform.RandomIDGenerator{}, log)
	now := time.Now().UTC()

	mk := func(id, code, name string) *domain.Equipment {
		e := domain.NewEquipment(id, now)
		e.Code = code
		e.Name = name
		e.Type = "ut"
		return e
	}

	target := mk("eq-1", "DEV-001", "Probe A")
	// Decoy whose Name equals the target's Code: under a status-collapse bug
	// the value "DEV-001" would not even reach the code column.
	nameDecoy := mk("eq-2", "Probe A", "DEV-001")
	for _, e := range []*domain.Equipment{target, nameDecoy} {
		if _, err := svc.Create(ctx, e, ""); err != nil {
			t.Fatal(err)
		}
	}

	items, total, err := svc.List(ctx, map[string]any{"code": target.Code}, domain.Page{Page: 1, Size: 20}, "")
	if err != nil {
		t.Fatalf("list by code: %v", err)
	}
	if total != int64(len(items)) {
		t.Fatalf("total %d != returned items %d", total, len(items))
	}
	if total != 1 || len(items) != 1 || items[0].ID != target.ID {
		var ids []string
		for _, it := range items {
			ids = append(ids, it.ID)
		}
		t.Fatalf("expected only [%s] for code %q, got total=%d items=%v", target.ID, target.Code, total, ids)
	}
}
