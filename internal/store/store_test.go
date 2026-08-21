package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"weld-ndt/internal/domain"
)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "weld-ndt.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return NewStore(db), path
}

func TestRepositoryCreateListAndReopen(t *testing.T) {
	st, path := newTestStore(t)
	now := time.Now().UTC()
	weld := domain.NewWeld("w-reopen", now)
	weld.Number = "W-001"
	weld.EquipmentID = "eq-1"
	weld.MethodVersionID = "mv-1"
	if err := st.Weld.Create(context.Background(), st.DB, weld); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	db2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	st2 := NewStore(db2)
	got, err := st2.Weld.Get(context.Background(), st2.DB, "w-reopen")
	if err != nil {
		t.Fatal(err)
	}
	if got.Number != "W-001" {
		t.Fatalf("unexpected weld number %s", got.Number)
	}
}

func TestDiscontinuityIndicationPreservesDistinctAssociations(t *testing.T) {
	st, _ := newTestStore(t)
	defer st.Close()
	now := time.Now().UTC()

	item := &domain.DiscontinuityIndication{
		ID:        "di-1",
		Status:    domain.DiscontinuityIndicationStatusOpen,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
		Meta:      map[string]any{},
		ReportID:  "report-A",
		WeldID:    "weld-B",
		Type:      "crack",
		Severity:  "III",
		Location:  "HAZ",
	}
	if err := st.DiscontinuityIndication.Create(context.Background(), st.DB, item); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := st.DiscontinuityIndication.Get(context.Background(), st.DB, "di-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ReportID != "report-A" || got.WeldID != "weld-B" {
		t.Fatalf("associations collapsed: report_id=%q weld_id=%q", got.ReportID, got.WeldID)
	}

	// Filtering by weld must hit, by report must hit, and each must not bleed
	// the other association's value across to the wrong column.
	cases := []struct {
		name     string
		filter   map[string]any
		wantHit  bool
	}{
		{"by weld", map[string]any{"weld_id": "weld-B"}, true},
		{"by report", map[string]any{"report_id": "report-A"}, true},
		{"weld value in report column", map[string]any{"report_id": "weld-B"}, false},
		{"report value in weld column", map[string]any{"weld_id": "report-A"}, false},
	}
	for _, c := range cases {
		items, total, err := st.DiscontinuityIndication.List(context.Background(), st.DB, c.filter, domain.Page{Page: 1, Size: 10}, "")
		if err != nil {
			t.Fatalf("%s: list: %v", c.name, err)
		}
		if (total > 0) != c.wantHit {
			t.Fatalf("%s: expected hit=%v got total=%d", c.name, c.wantHit, total)
		}
		if c.wantHit && (len(items) != 1 || items[0].ID != "di-1") {
			t.Fatalf("%s: unexpected items %+v", c.name, items)
		}
	}
}

func TestTransactionRollback(t *testing.T) {
	st, _ := newTestStore(t)
	defer st.Close()
	errRollback := errors.New("rollback")
	err := st.WithTx(context.Background(), func(tx Queryer) error {
		weld := domain.NewWeld("w-rollback", time.Now().UTC())
		weld.Number = "W-ROLL"
		weld.EquipmentID = "eq"
		weld.MethodVersionID = "mv"
		if err := st.Weld.Create(context.Background(), tx, weld); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("expected rollback error: %v", err)
	}
	_, err = st.Weld.Get(context.Background(), st.DB, "w-rollback")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("rollback left data behind: %v", err)
	}
}
