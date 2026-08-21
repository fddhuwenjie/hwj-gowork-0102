package store

import (
	"context"
	"testing"
	"time"

	"weld-ndt/internal/domain"
)

func TestCalibrationCertificateListByEquipmentID(t *testing.T) {
	st, _ := newTestStore(t)
	defer st.Close()
	ctx := context.Background()
	now := time.Now().UTC()

	// Two devices; EQ-9 owns two certificates, EQ-OTHER owns one.
	mk := func(id, equip, certNo string) {
		c := domain.NewCalibrationCertificate(id, now)
		c.EquipmentID = equip
		c.CertificateNo = certNo
		c.IssuedAt = now
		c.ExpiresAt = now.Add(24 * time.Hour)
		if err := st.CalibrationCertificate.Create(ctx, st.DB, c); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	mk("cert-eq9-a", "EQ-9", "CN-9-A")
	mk("cert-eq9-b", "EQ-9", "CN-9-B")
	mk("cert-other", "EQ-OTHER", "CN-X-1")

	// Regression: filtering by equipment_id must scope to EQ-9 only.
	items, total, err := st.CalibrationCertificate.List(ctx, st.DB, map[string]any{"equipment_id": "EQ-9"}, domain.Page{Page: 1, Size: 20}, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total 2 for EQ-9, got %d", total)
	}
	if len(items) != int(total) {
		t.Fatalf("total %d does not match returned %d items", total, len(items))
	}
	for _, c := range items {
		if c.EquipmentID != "EQ-9" {
			t.Fatalf("filter leaked foreign certificate %s (equipment_id=%s)", c.ID, c.EquipmentID)
		}
	}

	// The "mix-in other devices" symptom: a bogus equipment value must yield empty.
	items, total, err = st.CalibrationCertificate.List(ctx, st.DB, map[string]any{"equipment_id": "NOPE"}, domain.Page{Page: 1, Size: 20}, "")
	if err != nil {
		t.Fatalf("list nope: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Fatalf("expected empty for unknown equipment, got total=%d items=%d", total, len(items))
	}

	// Pagination total must stay consistent with the full hit set across pages.
	items, total, err = st.CalibrationCertificate.List(ctx, st.DB, map[string]any{"equipment_id": "EQ-9"}, domain.Page{Page: 1, Size: 1}, "")
	if err != nil {
		t.Fatalf("list p1: %v", err)
	}
	if total != 2 {
		t.Fatalf("paged total should still be 2, got %d", total)
	}
	if len(items) != 1 {
		t.Fatalf("page 1 size=1 should return 1 item, got %d", len(items))
	}
	items2, _, err := st.CalibrationCertificate.List(ctx, st.DB, map[string]any{"equipment_id": "EQ-9"}, domain.Page{Page: 2, Size: 1}, "")
	if err != nil {
		t.Fatalf("list p2: %v", err)
	}
	if len(items2) != 1 {
		t.Fatalf("page 2 size=1 should return 1 item, got %d", len(items2))
	}
	if items[0].ID == items2[0].ID {
		t.Fatalf("pages returned duplicate id %s", items[0].ID)
	}
}
