package service

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"weld-ndt/internal/domain"
	"weld-ndt/internal/platform"
	"weld-ndt/internal/store"
)

func newReviewTaskFixture(t *testing.T) *ReviewTaskService {
	t.Helper()
	st, _ := storeTestStore(t)
	clock := platform.SystemClock{}
	ids := platform.RandomIDGenerator{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewReviewTaskService(st, clock, ids, log)
}

func newReviewTaskItem(svc *ReviewTaskService) *domain.ReviewTask {
	return &domain.ReviewTask{
		ID:        svc.ids.New(),
		Status:    domain.ReviewTaskStatusPending,
		Version:   1,
		CreatedAt: svc.clock.Now(),
		UpdatedAt: svc.clock.Now(),
		Meta:      map[string]any{},
		ReportID:  "rep-1",
		WeldID:    "weld-1",
		Reviewer:  "alice",
	}
}

// auditForTask lists audit rows that reference a given review-task id.
func auditForTask(ctx context.Context, svc *ReviewTaskService, taskID string) ([]*domain.AuditRecord, error) {
	rows, _, err := svc.audit.List(ctx, svc.store.DB, map[string]any{"entity": "reviewtask"}, domain.Page{Page: 1, Size: 200}, "created_at desc")
	if err != nil {
		return nil, err
	}
	var matched []*domain.AuditRecord
	for _, a := range rows {
		if a.EntityID == taskID {
			matched = append(matched, a)
		}
	}
	return matched, nil
}

// TestReviewTaskCreateWritesAuditTogether confirms the happy path: a normal
// create persists both the review task and its create-audit record, and the
// internal write_scope marker no longer leaks into stored meta.
func TestReviewTaskCreateWritesAuditTogether(t *testing.T) {
	svc := newReviewTaskFixture(t)
	defer svc.store.Close()
	ctx := context.Background()

	item := newReviewTaskItem(svc)
	created, err := svc.Create(ctx, item, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := svc.repo.Get(ctx, svc.store.DB, created.ID)
	if err != nil {
		t.Fatalf("get review task: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("unexpected review task id %s", got.ID)
	}
	if _, ok := got.Meta["write_scope"]; ok {
		t.Fatalf("internal write_scope marker leaked into stored meta: %v", got.Meta)
	}

	audits, err := auditForTask(ctx, svc, created.ID)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(audits) != 1 {
		t.Fatalf("expected exactly one create audit for %s, got %d", created.ID, len(audits))
	}
	if audits[0].Action != "create" || audits[0].AfterJSON == "" {
		t.Fatalf("unexpected audit record: %+v", audits[0])
	}
}

// TestReviewTaskCreateAuditFailureRollsBackBusiness confirms the atomicity
// contract: when the audit write fails after the business write has already run
// inside the same transaction, the whole transaction rolls back and no
// half-written review task survives in the store.
//
// The audit failure is induced by making the audit store unavailable (dropping
// the audit_records table) while review_tasks stays intact, so the business
// INSERT succeeds and the audit INSERT fails — the exact split the bug allowed
// to leak through. With the fix, WithTx rolls both back together.
func TestReviewTaskCreateAuditFailureRollsBackBusiness(t *testing.T) {
	svc := newReviewTaskFixture(t)
	defer svc.store.Close()
	ctx := context.Background()

	// Make the audit store unavailable: any audit INSERT now errors.
	if _, err := svc.store.DB.ExecContext(ctx, "DROP TABLE audit_records"); err != nil {
		t.Fatalf("drop audit table: %v", err)
	}

	item := newReviewTaskItem(svc)
	if _, err := svc.Create(ctx, item, ""); err == nil {
		t.Fatalf("expected create to fail when the audit store is unavailable")
	}

	// The review task must NOT survive: the business write rolled back with the
	// failed audit write.
	if _, err := svc.repo.Get(ctx, svc.store.DB, item.ID); err == nil {
		t.Fatalf("half-written review task survived audit failure")
	} else if err != store.ErrNotFound {
		t.Fatalf("expected not-found after rollback, got %v", err)
	}

	// And no audit row may remain for this task either.
	audits, err := auditForTask(ctx, svc, item.ID)
	if err != nil {
		// auditForTask lists audit_records, which we dropped; rebuild a minimal
		// table just to assert emptiness is unnecessary — if the table is gone,
		// there are certainly no audit rows, which is the assertion.
		return
	}
	if len(audits) != 0 {
		t.Fatalf("expected no audit records after rollback, got %d", len(audits))
	}
}
