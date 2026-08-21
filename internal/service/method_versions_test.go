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

// fixedIDGenerator returns the same ID on every call. It lets a test make the
// audit INSERT deterministically collide with a pre-seeded audit row so the
// transaction is forced to roll back.
type fixedIDGenerator struct{ id string }

func (g fixedIDGenerator) New() string { return g.id }

func newMethodVersionFixture(t *testing.T, ids platform.IDGenerator) (*store.Store, *MethodVersionService) {
	t.Helper()
	st, _ := storeTestStore(t)
	clock := platform.SystemClock{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return st, NewMethodVersionService(st, clock, ids, log)
}

// TestMethodVersionCreateAtomicRollbackOnAuditFailure verifies that when the
// audit write inside Create fails, the business row is rolled back together
// with it rather than left behind as a half-committed record. Before the fix,
// the method version was inserted on the autocommit DB connection and the audit
// ran in a separate transaction, so the row survived an audit failure.
func TestMethodVersionCreateAtomicRollbackOnAuditFailure(t *testing.T) {
	const collideID = "fixed-audit-id"
	st, svc := newMethodVersionFixture(t, fixedIDGenerator{id: collideID})
	defer st.Close()
	ctx := context.Background()
	now := time.Now().UTC()

	// Pre-seed an audit row whose ID the service will try to reuse, forcing the
	// audit INSERT to fail with a primary-key conflict.
	preAudit := domain.NewAuditRecord(collideID, now)
	preAudit.Entity = "methodversion"
	preAudit.EntityID = "seed"
	preAudit.Action = "create"
	preAudit.Actor = "system"
	if err := st.AuditRecord.Create(ctx, st.DB, preAudit); err != nil {
		t.Fatalf("seed audit: %v", err)
	}

	mv := domain.NewMethodVersion("", now)
	mv.Code = "UT"
	mv.VersionNo = 1
	mv.Standard = "ISO"
	if _, err := svc.Create(ctx, mv, ""); err == nil {
		t.Fatalf("expected create to fail when audit write fails, got nil")
	}

	// The business record must NOT survive the failed audit: a re-query must see
	// nothing. item.ID was assigned by the fixed generator as collideID.
	if _, err := st.MethodVersion.Get(ctx, st.DB, collideID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("method version survived audit failure (atomicity broken): %v", err)
	}

	// And the leaked internal write-scope marker must not appear anywhere even
	// though the row was never committed.
	if _, ok := mv.Meta["write_scope"]; ok {
		t.Fatalf("write_scope marker leaked into method version meta")
	}
}

// TestMethodVersionCreateProducesAuditAndCleanMeta verifies the happy path: a
// successful create still produces exactly one audit row bound to the new
// method version, and the business record carries no internal write-scope tag.
func TestMethodVersionCreateProducesAuditAndCleanMeta(t *testing.T) {
	st, svc := newMethodVersionFixture(t, platform.RandomIDGenerator{})
	defer st.Close()
	ctx := context.Background()
	now := time.Now().UTC()

	mv := domain.NewMethodVersion("", now)
	mv.Code = "RT"
	mv.VersionNo = 2026
	mv.Standard = "NB/T"
	created, err := svc.Create(ctx, mv, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Business record is committed.
	persisted, err := st.MethodVersion.Get(ctx, st.DB, created.ID)
	if err != nil {
		t.Fatalf("get persisted method version: %v", err)
	}
	if persisted.Code != "RT" {
		t.Fatalf("unexpected code %q", persisted.Code)
	}

	// Exactly one create audit row is bound to this entity.
	audits, total, err := st.AuditRecord.List(ctx, st.DB, map[string]any{
		"entity":    "methodversion",
		"entity_id": created.ID,
		"action":    "create",
	}, domain.Page{Page: 1, Size: 10}, "")
	if err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if total != 1 || len(audits) != 1 {
		t.Fatalf("expected 1 create audit, got total=%d items=%d", total, len(audits))
	}

	// No internal write-scope marker leaks into the persisted business record.
	for _, m := range []map[string]any{created.Meta, persisted.Meta} {
		if _, ok := m["write_scope"]; ok {
			t.Fatalf("write_scope marker leaked into method version meta: %#v", m)
		}
	}
}
