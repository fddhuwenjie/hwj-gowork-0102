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

func newExecutionBatchFixture(t *testing.T) *ExecutionBatchService {
	t.Helper()
	st, _ := storeTestStore(t)
	clock := platform.SystemClock{}
	ids := platform.RandomIDGenerator{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewExecutionBatchService(st, clock, ids, log)
}

// TestExecutionBatchCreatePreservesBothAssociations guards against a cross-layer
// regression where the second association identifier (method_version_id) was
// overwritten by the first entity identifier (equipment_id) on Create:
//
//   - the service swapped item.MethodVersionID = item.EquipmentID and stashed the
//     requested value only in meta;
//   - the repo INSERT bound item.EquipmentID into both the equipment_id and
//     method_version_id columns.
//
// Both identifiers must be persisted verbatim, the detail (Get) must echo both
// requested values, and the List filter must find the record via each
// association independently.
func TestExecutionBatchCreatePreservesBothAssociations(t *testing.T) {
	svc := newExecutionBatchFixture(t)
	ctx := context.Background()

	const (
		eqID = "eq-001"
		mvID = "mv-001" // distinct from equipment_id; if collapsed these would be equal
	)
	now := time.Now().UTC()
	batch := domain.NewExecutionBatch("batch-assoc-1", now)
	batch.Code = "B-ASSOC-1"
	batch.EquipmentID = eqID
	batch.MethodVersionID = mvID

	if _, err := svc.Create(ctx, batch, ""); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Detail must preserve both identifiers exactly as requested.
	got, err := svc.Get(ctx, batch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.EquipmentID != eqID {
		t.Errorf("EquipmentID = %q, want %q (detail must echo first identifier)", got.EquipmentID, eqID)
	}
	if got.MethodVersionID != mvID {
		t.Errorf("MethodVersionID = %q, want %q (detail must echo second identifier, was collapsed into equipment_id)", got.MethodVersionID, mvID)
	}
	if got.EquipmentID == got.MethodVersionID {
		t.Errorf("identifiers collapsed: EquipmentID == MethodVersionID == %q; both must be preserved separately", got.EquipmentID)
	}

	page := domain.Page{Page: 1, Size: 50}

	// Filtering by the original method_version_id must find the record.
	items, total, err := svc.List(ctx, map[string]any{"method_version_id": mvID}, page, "")
	if err != nil {
		t.Fatalf("List by method_version_id: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != batch.ID {
		t.Errorf("filter by method_version_id=%q: total=%d items=%d; want the created record (filter must use the correct association)",
			mvID, total, len(items))
	}

	// Filtering by equipment_id must also find the record (independent association).
	items, total, err = svc.List(ctx, map[string]any{"equipment_id": eqID}, page, "")
	if err != nil {
		t.Fatalf("List by equipment_id: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != batch.ID {
		t.Errorf("filter by equipment_id=%q: total=%d items=%d; want the created record",
			eqID, total, len(items))
	}

	// A filter using the equipment_id value against the method_version_id column
	// must NOT match (the two identifiers are distinct and not interchangeable).
	_, total, err = svc.List(ctx, map[string]any{"method_version_id": eqID}, page, "")
	if err != nil {
		t.Fatalf("List by method_version_id=equipment_id: %v", err)
	}
	if total != 0 {
		t.Errorf("filter by method_version_id=%q (the equipment_id value): total=%d; want 0 (identifiers must not cross-match)", eqID, total)
	}
}

// TestExecutionBatchCreateIdempotencyPreservesAssociations ensures the
// idempotency cache path also returns both identifiers verbatim, since the
// cached copy is returned on retry and must reflect the persisted (correct) values.
func TestExecutionBatchCreateIdempotencyPreservesAssociations(t *testing.T) {
	svc := newExecutionBatchFixture(t)
	ctx := context.Background()

	const (
		eqID = "eq-idem"
		mvID = "mv-idem"
	)
	now := time.Now().UTC()
	batch := domain.NewExecutionBatch("batch-idem-1", now)
	batch.Code = "B-IDEM-1"
	batch.EquipmentID = eqID
	batch.MethodVersionID = mvID

	if _, err := svc.Create(ctx, batch, "idem-key-assoc"); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	// Replay with the same idempotency key.
	batch2 := domain.NewExecutionBatch("batch-idem-1-should-be-ignored", now)
	batch2.Code = "B-IDEM-1"
	batch2.EquipmentID = "eq-different"
	batch2.MethodVersionID = "mv-different"
	cached, err := svc.Create(ctx, batch2, "idem-key-assoc")
	if err != nil {
		t.Fatalf("replay Create: %v", err)
	}
	if cached.EquipmentID != eqID || cached.MethodVersionID != mvID {
		t.Errorf("idempotent replay returned EquipmentID=%q MethodVersionID=%q; want %q %q",
			cached.EquipmentID, cached.MethodVersionID, eqID, mvID)
	}
}
