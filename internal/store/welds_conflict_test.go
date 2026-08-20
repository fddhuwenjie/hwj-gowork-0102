package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"weld-ndt/internal/domain"
)

// TestWeldUpdateStaleVersionConflict reproduces the "two inspectors edit weld
// details sequentially" bug: the inspector holding the stale version must NOT
// silently overwrite the number that the newer submit already saved. The update
// has to fail with ErrVersionConflict, leaving the newest data intact.
func TestWeldUpdateStaleVersionConflict(t *testing.T) {
	st, _ := newTestStore(t)
	defer st.Close()
	ctx := context.Background()
	now := time.Now().UTC()

	weld := domain.NewWeld("w-conflict", now)
	weld.Number = "W-001"
	weld.EquipmentID = "eq-1"
	weld.MethodVersionID = "mv-1"
	if err := st.Weld.Create(ctx, st.DB, weld); err != nil {
		t.Fatal(err)
	}

	// Inspector A reads the weld at version 1.
	readA, err := st.Weld.Get(ctx, st.DB, weld.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Inspector B reads the same weld (also version 1), then submits first with
	// a new number, advancing the version to 2.
	readB, err := st.Weld.Get(ctx, st.DB, weld.ID)
	if err != nil {
		t.Fatal(err)
	}
	readB.Number = "W-001-B"
	if err := st.Weld.Update(ctx, st.DB, readB, readB.Version); err != nil {
		t.Fatalf("first update failed: %v", err)
	}

	// Inspector A now submits the stale version. This must NOT silently succeed
	// and overwrite B's number.
	readA.Number = "W-001-A"
	err = st.Weld.Update(ctx, st.DB, readA, readA.Version)
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("expected ErrVersionConflict for stale version, got %v", err)
	}

	// The newest data must be preserved: B's number and version 2.
	current, err := st.Weld.Get(ctx, st.DB, weld.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Number != "W-001-B" {
		t.Fatalf("newest number overwritten by stale submit: got %s", current.Number)
	}
	if current.Version != 2 {
		t.Fatalf("version regressed after stale submit: got %d", current.Version)
	}
	// The in-memory item passed to the failing update must not be advanced.
	if readA.Version != 1 {
		t.Fatalf("stale item version advanced despite conflict: got %d", readA.Version)
	}
}

// TestWeldUpdateCurrentVersionSucceeds confirms the happy path still works:
// submitting the current version updates the data and advances the version.
func TestWeldUpdateCurrentVersionSucceeds(t *testing.T) {
	st, _ := newTestStore(t)
	defer st.Close()
	ctx := context.Background()
	now := time.Now().UTC()

	weld := domain.NewWeld("w-ok", now)
	weld.Number = "W-100"
	weld.EquipmentID = "eq-1"
	weld.MethodVersionID = "mv-1"
	if err := st.Weld.Create(ctx, st.DB, weld); err != nil {
		t.Fatal(err)
	}

	current, err := st.Weld.Get(ctx, st.DB, weld.ID)
	if err != nil {
		t.Fatal(err)
	}
	current.Number = "W-101"
	if err := st.Weld.Update(ctx, st.DB, current, current.Version); err != nil {
		t.Fatalf("current-version update failed: %v", err)
	}
	if current.Version != 2 {
		t.Fatalf("version not advanced: got %d", current.Version)
	}

	reloaded, err := st.Weld.Get(ctx, st.DB, weld.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Number != "W-101" || reloaded.Version != 2 {
		t.Fatalf("unexpected persisted weld: number=%s version=%d", reloaded.Number, reloaded.Version)
	}
}
