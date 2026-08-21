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

func newCalSvcTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(t.TempDir() + "/svc.db")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return store.NewStore(db)
}

func newCalSvc(t *testing.T, st *store.Store) *CalibrationCertificateService {
	t.Helper()
	return NewCalibrationCertificateService(
		st,
		fixedClock{},
		platform.RandomIDGenerator{},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }

func TestCalibrationCertificateServiceListByEquipmentID(t *testing.T) {
	st := newCalSvcTestStore(t)
	defer st.Close()
	svc := newCalSvc(t, st)
	ctx := context.Background()

	mk := func(equip, certNo string) {
		c := domain.NewCalibrationCertificate("", time.Now().UTC())
		c.EquipmentID = equip
		c.CertificateNo = certNo
		c.IssuedAt = time.Now().UTC()
		c.ExpiresAt = time.Now().Add(time.Hour)
		if _, err := svc.Create(ctx, c, ""); err != nil {
			t.Fatalf("create %s/%s: %v", equip, certNo, err)
		}
	}
	mk("EQ-9", "CN-9-A")
	mk("EQ-9", "CN-9-B")
	mk("EQ-OTHER", "CN-X-1")

	// Regression for "filter by owner returns empty": equipment_id must reach the repo intact.
	items, total, err := svc.List(ctx, map[string]any{"equipment_id": "EQ-9"}, domain.Page{Page: 1, Size: 20}, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total 2, got %d (filter not propagated as equipment_id)", total)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d (total %d) — pagination total != hit set", len(items), total)
	}
	for _, c := range items {
		if c.EquipmentID != "EQ-9" {
			t.Fatalf("foreign certificate %s leaked in (equipment_id=%s)", c.ID, c.EquipmentID)
		}
	}

	// Regression for "mixes in other devices": combining equipment_id with status must AND them,
	// not collapse every filter value onto the status column.
	items, total, err = svc.List(ctx, map[string]any{"equipment_id": "EQ-9", "status": "valid"}, domain.Page{Page: 1, Size: 20}, "")
	if err != nil {
		t.Fatalf("list combo: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("expected 2 valid EQ-9 certs, got total=%d items=%d", total, len(items))
	}

	// Unknown status on EQ-9 must yield empty — proves status filter is bound, not ignored.
	items, total, err = svc.List(ctx, map[string]any{"equipment_id": "EQ-9", "status": "revoked"}, domain.Page{Page: 1, Size: 20}, "")
	if err != nil {
		t.Fatalf("list revoked: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Fatalf("expected empty for revoked EQ-9, got total=%d items=%d", total, len(items))
	}
}
