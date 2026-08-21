package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"weld-ndt/internal/domain"
	"weld-ndt/internal/platform"
	"weld-ndt/internal/store"
)

func newEquipmentFixture(t *testing.T) (*store.Store, *EquipmentService) {
	t.Helper()
	path := t.TempDir() + "/eq.db"
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	st := store.NewStore(db)
	clock := platform.SystemClock{}
	ids := platform.RandomIDGenerator{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return st, NewEquipmentService(st, clock, ids, log)
}

func seedEquipment(t *testing.T, svc *EquipmentService) *domain.Equipment {
	t.Helper()
	now := time.Now().UTC()
	item := domain.NewEquipment("eq-stale", now)
	item.Code = "EQ-1"
	item.Name = "X-Ray-1"
	item.Type = "RT"
	ctx := context.Background()
	created, err := svc.Create(ctx, item, "")
	if err != nil {
		t.Fatal(err)
	}
	return created
}

// TestEquipmentUpdateRejectsStaleVersion reproduces the cross-layer version
// isolation bug: a maintainer reads an equipment name at version 1, another
// maintainer edits and saves the same name (v1 -> v2). The stale client then
// submits its old version-1 payload. The system must reject the stale
// version and keep the saved value, rather than return success and overwrite.
func TestEquipmentUpdateRejectsStaleVersion(t *testing.T) {
	st, svc := newEquipmentFixture(t)
	defer st.Close()
	ctx := context.Background()

	eq := seedEquipment(t, svc) // version 1, name "X-Ray-1"

	// Maintainer B: load current state, rename, save -> v2.
	current, err := svc.Get(ctx, eq.ID)
	if err != nil {
		t.Fatal(err)
	}
	current.Name = "X-Ray-2"
	if err := svc.Update(ctx, current, current.Version); err != nil {
		t.Fatalf("fresh update failed: %v", err)
	}

	// Maintainer A (stale client): still holding the v1 snapshot, edits the
	// same name and submits with the expected version 1 it was built from.
	stale, err := svc.Get(ctx, eq.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Restore the stale client's view: pretend it only ever saw version 1.
	stale.Name = "X-Ray-Stale"
	stale.Version = 1
	err = svc.Update(ctx, stale, 1)
	if !errors.Is(err, store.ErrVersionConflict) {
		t.Fatalf("expected version conflict for stale submit, got %v", err)
	}

	// The saved value must be the fresh edit, not the stale overwrite.
	saved, err := svc.Get(ctx, eq.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "X-Ray-2" {
		t.Fatalf("stale submit overwrote saved name: got %q want %q", saved.Name, "X-Ray-2")
	}
	if saved.Version != 2 {
		t.Fatalf("version advanced past stale submit: got %d want %d", saved.Version, 2)
	}
}

// TestEquipmentUpdateAdvancesVersion confirms the normal version-update path
// still works after the fix: a fresh version submits and the version
// increments, the saved name reflects the new value.
func TestEquipmentUpdateAdvancesVersion(t *testing.T) {
	st, svc := newEquipmentFixture(t)
	defer st.Close()
	ctx := context.Background()

	eq := seedEquipment(t, svc) // version 1
	current, err := svc.Get(ctx, eq.ID)
	if err != nil {
		t.Fatal(err)
	}
	current.Name = "X-Ray-Updated"
	if err := svc.Update(ctx, current, current.Version); err != nil {
		t.Fatalf("fresh update failed: %v", err)
	}
	saved, err := svc.Get(ctx, eq.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "X-Ray-Updated" {
		t.Fatalf("name not persisted: got %q", saved.Name)
	}
	if saved.Version != 2 {
		t.Fatalf("version not advanced: got %d want %d", saved.Version, 2)
	}
}
