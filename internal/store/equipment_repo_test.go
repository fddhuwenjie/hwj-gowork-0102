package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"weld-ndt/internal/domain"
)

func newEquipmentRow(t *testing.T, st *Store) *domain.Equipment {
	t.Helper()
	now := time.Now().UTC()
	item := domain.NewEquipment("eq-repo", now)
	item.Code = "EQ-R"
	item.Name = "Probe-A"
	item.Type = "UT"
	if err := st.Equipment.Create(context.Background(), st.DB, item); err != nil {
		t.Fatal(err)
	}
	return item
}

// TestEquipmentRepoUpdateVersionGuard verifies the repository defends against
// stale version overwrites at the storage layer: updating with a mismatched
// expectedVersion must affect zero rows and return ErrVersionConflict rather
// than silently overwriting the saved row.
func TestEquipmentRepoUpdateVersionGuard(t *testing.T) {
	st, _ := newTestStore(t)
	defer st.Close()
	ctx := context.Background()

	created := newEquipmentRow(t, st) // version 1

	// Bump to v2 via a fresh, correct update.
	fresh, err := st.Equipment.Get(ctx, st.DB, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Name = "Probe-B"
	if err := st.Equipment.Update(ctx, st.DB, fresh, fresh.Version); err != nil {
		t.Fatalf("fresh repo update failed: %v", err)
	}

	// Stale submit still carrying expectedVersion=1 must be rejected at the
	// repository layer even though the row exists.
	stale := *fresh
	stale.Name = "Probe-Stale"
	if err := st.Equipment.Update(ctx, st.DB, &stale, 1); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("expected ErrVersionConflict for stale repo update, got %v", err)
	}

	saved, err := st.Equipment.Get(ctx, st.DB, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "Probe-B" {
		t.Fatalf("stale repo update overwrote saved name: got %q want %q", saved.Name, "Probe-B")
	}
	if saved.Version != 2 {
		t.Fatalf("version advanced past stale repo update: got %d want %d", saved.Version, 2)
	}
}
