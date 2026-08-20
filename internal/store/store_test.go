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
