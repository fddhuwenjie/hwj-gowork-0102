package service

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"weld-ndt/internal/domain"
	"weld-ndt/internal/platform"
	"weld-ndt/internal/store"
)

func newAnomalyFixture(t *testing.T) (*store.Store, *AnomalyEventService) {
	t.Helper()
	st, _ := storeTestStore(t)
	clock := platform.SystemClock{}
	ids := platform.RandomIDGenerator{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return st, NewAnomalyEventService(st, clock, ids, log)
}

func validAnomaly(id string, now time.Time) *domain.AnomalyEvent {
	e := domain.NewAnomalyEvent(id, now)
	e.WeldID = "W-17"
	e.BatchID = "B-42"
	e.Type = "porosity"
	e.Severity = "high"
	return e
}

// TestAnomalyEventServicePreservesBothAssociations reproduces the reported defect:
// creating an event with weld_id=W-17 and batch_id=B-42 must keep both values
// distinct through create, detail (get), and batch_id filtering.
func TestAnomalyEventServicePreservesBothAssociations(t *testing.T) {
	st, svc := newAnomalyFixture(t)
	defer st.Close()
	ctx := context.Background()

	created, err := svc.Create(ctx, validAnomaly("anom-svc-1", time.Now().UTC()), "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Create returns the persisted association as the user submitted it.
	if created.WeldID != "W-17" || created.BatchID != "B-42" {
		t.Fatalf("create returned conflated associations: weld_id=%q batch_id=%q", created.WeldID, created.BatchID)
	}

	// Detail (Get) reads both back independently.
	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.WeldID != "W-17" || got.BatchID != "B-42" {
		t.Fatalf("get returned conflated associations: weld_id=%q batch_id=%q", got.WeldID, got.BatchID)
	}

	// Filter by batch_id finds the event (regression: it was dropped because
	// batch_id was persisted as the weld id).
	items, total, err := svc.List(ctx, map[string]any{"batch_id": "B-42"}, domain.Page{Page: 1, Size: 10}, "")
	if err != nil {
		t.Fatalf("list by batch_id: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("batch_id filter expected 1 match, got total=%d items=%d", total, len(items))
	}
}

// TestAnomalyEventServiceAuditRetainsBothValues asserts the audit record's
// after_json preserves the original weld and batch associations submitted by the
// user — the audit must not have captured a clobbered value.
func TestAnomalyEventServiceAuditRetainsBothValues(t *testing.T) {
	st, svc := newAnomalyFixture(t)
	defer st.Close()
	ctx := context.Background()

	created, err := svc.Create(ctx, validAnomaly("anom-audit-1", time.Now().UTC()), "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	audits, _, err := st.AuditRecord.List(ctx, st.DB, map[string]any{
		"entity":    "anomalyevent",
		"entity_id": created.ID,
		"action":    "create",
	}, domain.Page{Page: 1, Size: 10}, "")
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(audits) != 1 {
		t.Fatalf("expected 1 create audit record, got %d", len(audits))
	}
	var after map[string]any
	if err := json.Unmarshal([]byte(audits[0].AfterJSON), &after); err != nil {
		t.Fatalf("unmarshal after_json: %v", err)
	}
	if after["weld_id"] != "W-17" || after["batch_id"] != "B-42" {
		t.Fatalf("audit after_json does not retain both original values: weld_id=%v batch_id=%v after=%s",
			after["weld_id"], after["batch_id"], audits[0].AfterJSON)
	}
}
