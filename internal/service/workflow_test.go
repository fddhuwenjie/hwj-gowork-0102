package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"weld-ndt/internal/domain"
	"weld-ndt/internal/platform"
	"weld-ndt/internal/store"
)

func newWorkflowFixture(t *testing.T) (*store.Store, *WorkflowService, *WeldService) {
	t.Helper()
	st, _ := storeTestStore(t)
	clock := platform.SystemClock{}
	ids := platform.RandomIDGenerator{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return st, NewWorkflowService(st, clock, ids, log), NewWeldService(st, clock, ids, log)
}

func newMethodVersionFixture(t *testing.T) (*store.Store, *MethodVersionService, *domain.MethodVersion) {
	t.Helper()
	st, _ := storeTestStore(t)
	clock := platform.SystemClock{}
	ids := platform.RandomIDGenerator{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewMethodVersionService(st, clock, ids, log)
	ctx := context.Background()
	mv := domain.NewMethodVersion(ids.New(), time.Now().UTC())
	mv.Code = "UT"
	mv.VersionNo = 2026
	mv.Standard = "NB/T"
	created, err := svc.Create(ctx, mv, "")
	if err != nil {
		t.Fatal(err)
	}
	return st, svc, created
}

func storeTestStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	path := t.TempDir() + "/svc.db"
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return store.NewStore(db), path
}

func seedWorkflow(t *testing.T, st *store.Store) *domain.Weld {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	equipment := domain.NewEquipment("eq-1", now)
	equipment.Code = "EQ-1"
	equipment.Name = "RT-1"
	equipment.Type = "RT"
	if err := st.Equipment.Create(ctx, st.DB, equipment); err != nil {
		t.Fatal(err)
	}
	method := domain.NewMethodVersion("mv-1", now)
	method.Code = "RT"
	method.VersionNo = 2026
	method.Standard = "NB/T"
	method.Status = domain.MethodVersionStatusActive
	if err := st.MethodVersion.Create(ctx, st.DB, method); err != nil {
		t.Fatal(err)
	}
	batch := domain.NewExecutionBatch("batch-1", now)
	batch.Code = "B-1"
	batch.EquipmentID = equipment.ID
	batch.MethodVersionID = method.ID
	if err := st.ExecutionBatch.Create(ctx, st.DB, batch); err != nil {
		t.Fatal(err)
	}
	weld := domain.NewWeld("weld-1", now)
	weld.Number = "W-1"
	weld.EquipmentID = equipment.ID
	weld.MethodVersionID = method.ID
	if err := st.Weld.Create(ctx, st.DB, weld); err != nil {
		t.Fatal(err)
	}
	return weld
}

func TestWorkflowAbnormalRepairLoop(t *testing.T) {
	st, workflow, weldSvc := newWorkflowFixture(t)
	defer st.Close()
	weld := seedWorkflow(t, st)
	scheduled, err := workflow.ScheduleInspection(context.Background(), ScheduleInspectionRequest{WeldID: weld.ID, EquipmentID: "eq-1", MethodVersionID: "mv-1", BatchID: "batch-1", Version: weld.Version})
	if err != nil {
		t.Fatal(err)
	}
	if err := weldSvc.Transition(context.Background(), scheduled.ID, domain.WeldStatusInProgress, scheduled.Version); err != nil {
		t.Fatal(err)
	}
	running, err := st.Weld.Get(context.Background(), st.DB, scheduled.ID)
	if err != nil {
		t.Fatal(err)
	}
	report, err := workflow.SubmitReport(context.Background(), SubmitReportRequest{WeldID: running.ID, BatchID: "batch-1", Code: "R-1", FindingsCount: 1, Severity: "III", Version: running.Version})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != domain.NDTReportStatusSubmitted {
		t.Fatalf("unexpected report status %s", report.Status)
	}
	needsRepair, err := st.Weld.Get(context.Background(), st.DB, running.ID)
	if err != nil {
		t.Fatal(err)
	}
	repair, err := workflow.CreateRepair(context.Background(), CreateRepairRequest{WeldID: needsRepair.ID, BatchID: "batch-1", AnomalyID: "anom-1", Version: needsRepair.Version})
	if err != nil {
		t.Fatal(err)
	}
	if repair.Round != 1 {
		t.Fatalf("unexpected repair round %d", repair.Round)
	}
}

func TestConcurrentVersionConflict(t *testing.T) {
	st, _, weldSvc := newWorkflowFixture(t)
	defer st.Close()
	weld := seedWorkflow(t, st)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- weldSvc.Transition(context.Background(), weld.ID, domain.WeldStatusScheduled, weld.Version)
		}()
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("expected one successful update, got %d", success)
	}
}

// TestMethodVersionTransitionGuard ensures the method-version state machine is
// enforced at the service boundary: adjacent transitions succeed and advance
// the version, while skip and self transitions are rejected without bumping
// the version, recording an audit row, or mutating the stored status.
func TestMethodVersionTransitionGuard(t *testing.T) {
	st, mvSvc, mv := newMethodVersionFixture(t)
	defer st.Close()
	ctx := context.Background()

	// A skip from draft straight to the deprecated terminal must be rejected.
	err := mvSvc.Transition(ctx, mv.ID, domain.MethodVersionStatusDeprecated, mv.Version)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("expected invalid transition for skip, got %v", err)
	}
	persisted, err := st.MethodVersion.Get(ctx, st.DB, mv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != domain.MethodVersionStatusDraft {
		t.Fatalf("status mutated after rejected skip: %s", persisted.Status)
	}
	if persisted.Version != mv.Version {
		t.Fatalf("version bumped after rejected skip: %d", persisted.Version)
	}
	// No audit row should be recorded for the rejected transition.
	auditTotal := countAuditFor(ctx, t, st, mv.ID)
	if auditTotal != 1 {
		t.Fatalf("expected only the create audit row after rejected skip, got %d", auditTotal)
	}

	// A self / no-op transition back to the current state is also rejected.
	if err := mvSvc.Transition(ctx, mv.ID, domain.MethodVersionStatusDraft, mv.Version); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("expected invalid transition for self transition, got %v", err)
	}

	// The legitimate adjacent transition is still available.
	if err := mvSvc.Transition(ctx, mv.ID, domain.MethodVersionStatusActive, mv.Version); err != nil {
		t.Fatalf("expected draft -> active to succeed, got %v", err)
	}
	active, err := st.MethodVersion.Get(ctx, st.DB, mv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if active.Status != domain.MethodVersionStatusActive {
		t.Fatalf("expected active status, got %s", active.Status)
	}
	if active.Version != mv.Version+1 {
		t.Fatalf("expected version to advance to %d, got %d", mv.Version+1, active.Version)
	}
}

func countAuditFor(ctx context.Context, t *testing.T, st *store.Store, entityID string) int {
	t.Helper()
	items, _, err := st.AuditRecord.List(ctx, st.DB, map[string]any{"entity_id": entityID}, domain.Page{Page: 1, Size: 100}, "")
	if err != nil {
		t.Fatal(err)
	}
	return len(items)
}
