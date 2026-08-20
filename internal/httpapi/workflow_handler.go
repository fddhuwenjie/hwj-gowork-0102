package httpapi

import (
	"net/http"
	"strconv"

	"weld-ndt/internal/platform"
	"weld-ndt/internal/service"
)

type WorkflowHandler struct {
	svc *service.WorkflowService
}

func NewWorkflowHandler(svc *service.WorkflowService) *WorkflowHandler {
	return &WorkflowHandler{svc: svc}
}

func (h *WorkflowHandler) Register(r *Router) {
	r.Handle("POST", "/api/v1/operations/schedule-inspection", h.ScheduleInspection)
	r.Handle("POST", "/api/v1/operations/submit-report", h.SubmitReport)
	r.Handle("POST", "/api/v1/operations/create-repair", h.CreateRepair)
	r.Handle("POST", "/api/v1/operations/approve-review", h.ApproveReview)
	r.Handle("GET", "/api/v1/queries/overdue-reinspections", h.OverdueReinspections)
	r.Handle("GET", "/api/v1/queries/expired-calibration-reports", h.ExpiredCalibrationReportCounts)
}

func (h *WorkflowHandler) ScheduleInspection(w http.ResponseWriter, r *http.Request) {
	var in service.ScheduleInspectionRequest
	if err := platform.ReadJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	out, err := h.svc.ScheduleInspection(r.Context(), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, out)
}

func (h *WorkflowHandler) SubmitReport(w http.ResponseWriter, r *http.Request) {
	var in service.SubmitReportRequest
	if err := platform.ReadJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	out, err := h.svc.SubmitReport(r.Context(), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, out)
}

func (h *WorkflowHandler) CreateRepair(w http.ResponseWriter, r *http.Request) {
	var in service.CreateRepairRequest
	if err := platform.ReadJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	out, err := h.svc.CreateRepair(r.Context(), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, out)
}

func (h *WorkflowHandler) ApproveReview(w http.ResponseWriter, r *http.Request) {
	var in service.ApproveReviewRequest
	if err := platform.ReadJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	out, err := h.svc.ApproveReview(r.Context(), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, out)
}

func (h *WorkflowHandler) OverdueReinspections(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	out, err := h.svc.OverdueReinspections(r.Context(), days)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *WorkflowHandler) ExpiredCalibrationReportCounts(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ExpiredCalibrationReportCounts(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}
