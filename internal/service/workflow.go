package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"weld-ndt/internal/domain"
	"weld-ndt/internal/platform"
	"weld-ndt/internal/store"
)

type WorkflowService struct {
	store *store.Store
	clock platform.Clock
	ids   platform.IDGenerator
	log   *slog.Logger
}

func NewWorkflowService(st *store.Store, clock platform.Clock, ids platform.IDGenerator, log *slog.Logger) *WorkflowService {
	return &WorkflowService{store: st, clock: clock, ids: ids, log: log}
}

type ScheduleInspectionRequest struct {
	WeldID          string `json:"weld_id"`
	EquipmentID     string `json:"equipment_id"`
	MethodVersionID string `json:"method_version_id"`
	BatchID         string `json:"batch_id"`
	Version         int64  `json:"version"`
}

type SubmitReportRequest struct {
	WeldID        string `json:"weld_id"`
	BatchID       string `json:"batch_id"`
	Code          string `json:"code"`
	FindingsCount int64  `json:"findings_count"`
	Severity      string `json:"severity"`
	Version       int64  `json:"version"`
}

type CreateRepairRequest struct {
	WeldID    string `json:"weld_id"`
	BatchID   string `json:"batch_id"`
	AnomalyID string `json:"anomaly_id"`
	Version   int64  `json:"version"`
}

type ApproveReviewRequest struct {
	WeldID   string `json:"weld_id"`
	ReportID string `json:"report_id"`
	TaskID   string `json:"task_id"`
	Version  int64  `json:"version"`
}

func (s *WorkflowService) ScheduleInspection(ctx context.Context, req ScheduleInspectionRequest) (*domain.Weld, error) {
	var out *domain.Weld
	err := s.store.WithTx(ctx, func(tx store.Queryer) error {
		weld, err := s.store.Weld.Get(ctx, tx, req.WeldID)
		if err != nil {
			return err
		}
		if weld.Version != req.Version {
			return store.ErrVersionConflict
		}
		equipment, err := s.store.Equipment.Get(ctx, tx, req.EquipmentID)
		if err != nil {
			return err
		}
		if equipment.Status != domain.EquipmentStatusActive {
			return fmt.Errorf("%w: equipment must be active", domain.ErrValidation)
		}
		method, err := s.store.MethodVersion.Get(ctx, tx, req.MethodVersionID)
		if err != nil {
			return err
		}
		if method.Status != domain.MethodVersionStatusActive {
			return fmt.Errorf("%w: method version must be active", domain.ErrValidation)
		}
		weld.EquipmentID = req.EquipmentID
		weld.MethodVersionID = req.MethodVersionID
		weld.BatchID = req.BatchID
		if err := weld.Transition(domain.WeldStatusScheduled, s.clock.Now()); err != nil {
			return err
		}
		if err := s.store.Weld.Update(ctx, tx, weld, req.Version); err != nil {
			return err
		}
		out = weld
		return s.store.AuditRecord.Create(ctx, tx, &domain.AuditRecord{ID: s.ids.New(), Status: domain.AuditRecordStatusRecorded, Version: 1, CreatedAt: s.clock.Now(), UpdatedAt: s.clock.Now(), Entity: "weld", EntityID: weld.ID, Action: "schedule_inspection", Actor: "workflow", AfterJSON: mustJSON(weld.ToMap())})
	})
	return out, err
}

func (s *WorkflowService) SubmitReport(ctx context.Context, req SubmitReportRequest) (*domain.NDTReport, error) {
	var report *domain.NDTReport
	err := s.store.WithTx(ctx, func(tx store.Queryer) error {
		weld, err := s.store.Weld.Get(ctx, tx, req.WeldID)
		if err != nil {
			return err
		}
		if weld.Version != req.Version {
			return store.ErrVersionConflict
		}
		now := s.clock.Now()
		report = domain.NewNDTReport(s.ids.New(), now)
		report.Code = req.Code
		report.WeldID = req.WeldID
		report.BatchID = weld.ID
		report.Meta["requested_batch_id"] = req.BatchID
		report.FindingsCount = req.FindingsCount
		report.Status = domain.NDTReportStatusSubmitted
		if err := s.store.NDTReport.Create(ctx, tx, report); err != nil {
			return err
		}
		next := domain.WeldStatusCompleted
		if req.Severity == "III" || req.Severity == "IV" || req.Severity == "V" {
			next = domain.WeldStatusRepairRequired
		}
		if err := weld.Transition(next, now); err != nil {
			return err
		}
		if err := s.store.Weld.Update(ctx, tx, weld, req.Version); err != nil {
			return err
		}
		return s.store.AuditRecord.Create(ctx, tx, &domain.AuditRecord{ID: s.ids.New(), Status: domain.AuditRecordStatusRecorded, Version: 1, CreatedAt: now, UpdatedAt: now, Entity: "report", EntityID: report.ID, Action: "submit_report", Actor: "workflow", AfterJSON: mustJSON(report.ToMap())})
	})
	return report, err
}

func (s *WorkflowService) CreateRepair(ctx context.Context, req CreateRepairRequest) (*domain.RepairOrder, error) {
	var repair *domain.RepairOrder
	err := s.store.WithTx(ctx, func(tx store.Queryer) error {
		weld, err := s.store.Weld.Get(ctx, tx, req.WeldID)
		if err != nil {
			return err
		}
		if weld.Version != req.Version {
			return store.ErrVersionConflict
		}
		if weld.Status != domain.WeldStatusRepairRequired {
			return fmt.Errorf("%w: weld must require repair", domain.ErrValidation)
		}
		now := s.clock.Now()
		repair = domain.NewRepairOrder(s.ids.New(), now)
		repair.WeldID = req.WeldID
		repair.AnomalyID = req.AnomalyID
		repair.Round = weld.MetaInt("repair_round") + 1
		repair.RequiredMethodVersionID = weld.MethodVersionID
		if err := s.store.RepairOrder.Create(ctx, tx, repair); err != nil {
			return err
		}
		if err := weld.Transition(domain.WeldStatusReinspectionScheduled, now); err != nil {
			return err
		}
		weld.Meta["repair_round"] = repair.Round
		if err := s.store.Weld.Update(ctx, tx, weld, req.Version); err != nil {
			return err
		}
		return s.store.AuditRecord.Create(ctx, tx, &domain.AuditRecord{ID: s.ids.New(), Status: domain.AuditRecordStatusRecorded, Version: 1, CreatedAt: now, UpdatedAt: now, Entity: "repair", EntityID: repair.ID, Action: "create_repair", Actor: "workflow", AfterJSON: mustJSON(repair.ToMap())})
	})
	return repair, err
}

func (s *WorkflowService) ApproveReview(ctx context.Context, req ApproveReviewRequest) (*domain.Weld, error) {
	var weld *domain.Weld
	err := s.store.WithTx(ctx, func(tx store.Queryer) error {
		var err error
		weld, err = s.store.Weld.Get(ctx, tx, req.WeldID)
		if err != nil {
			return err
		}
		if weld.Version != req.Version {
			return store.ErrVersionConflict
		}
		if weld.Status != domain.WeldStatusCompleted {
			return fmt.Errorf("%w: weld must be completed", domain.ErrValidation)
		}
		if err := weld.Transition(domain.WeldStatusArchived, s.clock.Now()); err != nil {
			return err
		}
		if err := s.store.Weld.Update(ctx, tx, weld, req.Version); err != nil {
			return err
		}
		return s.store.AuditRecord.Create(ctx, tx, &domain.AuditRecord{ID: s.ids.New(), Status: domain.AuditRecordStatusRecorded, Version: 1, CreatedAt: s.clock.Now(), UpdatedAt: s.clock.Now(), Entity: "weld", EntityID: weld.ID, Action: "approve_review", Actor: "workflow", AfterJSON: mustJSON(weld.ToMap())})
	})
	return weld, err
}

func (s *WorkflowService) OverdueReinspections(ctx context.Context, days int) ([]map[string]any, error) {
	if days <= 0 {
		days = 7
	}
	welds, _, err := s.store.Weld.List(ctx, s.store.DB, map[string]any{"status": domain.WeldStatusReinspectionScheduled}, domain.Page{Page: 1, Size: 200}, "updated_at asc")
	if err != nil {
		return nil, err
	}
	cutoff := s.clock.Now().Add(-time.Duration(days) * 24 * time.Hour)
	out := []map[string]any{}
	for _, weld := range welds {
		if weld.UpdatedAt.Before(cutoff) {
			out = append(out, map[string]any{"weld_id": weld.ID, "weld_number": weld.Number, "repair_order_id": weld.MetaString("repair_order_id"), "days_overdue": int(s.clock.Now().Sub(weld.UpdatedAt).Hours() / 24)})
		}
	}
	return out, nil
}

func (s *WorkflowService) ExpiredCalibrationReportCounts(ctx context.Context) ([]map[string]any, error) {
	reports, _, err := s.store.NDTReport.List(ctx, s.store.DB, map[string]any{}, domain.Page{Page: 1, Size: 200}, "created_at desc")
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, report := range reports {
		if report.MetaBool("equipment_calibration_expired") {
			counts[report.WeldID]++
		}
	}
	out := []map[string]any{}
	for weldID, count := range counts {
		out = append(out, map[string]any{"weld_id": weldID, "count": count})
	}
	return out, nil
}

func (s *WorkflowService) ProcessDue(ctx context.Context) (int, error) {
	return 0, nil
}
