package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"weld-ndt/internal/domain"
)

// newAnomaly builds a valid anomaly event with distinct weld and batch associations.
// The values are intentionally different (W-17 / B-42) so a binding mistake that
// copies one field into the other becomes detectable.
func newAnomaly(t *testing.T) *domain.AnomalyEvent {
	t.Helper()
	now := time.Now().UTC()
	e := domain.NewAnomalyEvent("anom-1", now)
	e.WeldID = "W-17"
	e.BatchID = "B-42"
	e.Type = "porosity"
	e.Severity = "high"
	return e
}

func TestAnomalyEventPreservesDistinctWeldAndBatch(t *testing.T) {
	st, _ := newTestStore(t)
	defer st.Close()
	ctx := context.Background()
	original := newAnomaly(t)

	// 1) Create must persist both associations independently.
	if err := st.AnomalyEvent.Create(ctx, st.DB, original); err != nil {
		t.Fatalf("create: %v", err)
	}

	// 2) Get (detail) must read both back without conflating them.
	got, err := st.AnomalyEvent.Get(ctx, st.DB, original.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.WeldID != "W-17" || got.BatchID != "B-42" {
		t.Fatalf("associations not preserved independently: weld_id=%q batch_id=%q", got.WeldID, got.BatchID)
	}

	// 3) Filtering by batch_id must find the event (regression: it was missed
	//    because batch_id was stored as the weld id).
	items, total, err := st.AnomalyEvent.List(ctx, st.DB, map[string]any{"batch_id": "B-42"}, Page{Page: 1, Size: 10}, "")
	if err != nil {
		t.Fatalf("list by batch_id: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != original.ID {
		t.Fatalf("batch_id filter expected 1 match, got total=%d items=%d", total, len(items))
	}

	// Filtering by weld_id must also find the event.
	items, total, err = st.AnomalyEvent.List(ctx, st.DB, map[string]any{"weld_id": "W-17"}, Page{Page: 1, Size: 10}, "")
	if err != nil {
		t.Fatalf("list by weld_id: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("weld_id filter expected 1 match, got total=%d items=%d", total, len(items))
	}

	// A filter keyed on the *other* association must NOT match: weld_id=B-42 finds nothing.
	_, total, err = st.AnomalyEvent.List(ctx, st.DB, map[string]any{"weld_id": "B-42"}, Page{Page: 1, Size: 10}, "")
	if err != nil {
		t.Fatalf("list stray weld_id: %v", err)
	}
	if total != 0 {
		t.Fatalf("weld_id=B-42 should match nothing, got %d", total)
	}
}

// TestAnomalyEventRawColumnBinding inspects the on-disk row directly to prove
// the weld_id and batch_id columns hold distinct values at the storage layer
// (not just after a re-mapping in the scan path).
func TestAnomalyEventRawColumnBinding(t *testing.T) {
	st, _ := newTestStore(t)
	defer st.Close()
	ctx := context.Background()
	if err := st.AnomalyEvent.Create(ctx, st.DB, newAnomaly(t)); err != nil {
		t.Fatalf("create: %v", err)
	}
	var weldID, batchID string
	err := st.DB.QueryRowContext(ctx, "SELECT weld_id, batch_id FROM anomaly_events WHERE id = ?", "anom-1").Scan(&weldID, &batchID)
	if err != nil {
		t.Fatalf("raw query: %v", err)
	}
	if weldID != "W-17" || batchID != "B-42" {
		t.Fatalf("raw columns not bound correctly: weld_id=%q batch_id=%q", weldID, batchID)
	}

	// The requested_batchid workaround meta key must not be leaking in.
	var meta string
	_ = st.DB.QueryRowContext(ctx, "SELECT meta_json FROM anomaly_events WHERE id = ?", "anom-1").Scan(&meta)
	var parsed map[string]any
	_ = json.Unmarshal([]byte(meta), &parsed)
	if _, leaked := parsed["requested_batchid"]; leaked {
		t.Fatalf("requested_batchid workaround leaked into meta: %v", meta)
	}
}
