package httpapi

import (
	"net/http"
	"strconv"

	"weld-ndt/internal/domain"
	"weld-ndt/internal/platform"
	"weld-ndt/internal/service"
)

type RepairOrderHandler struct {
	svc *service.RepairOrderService
}

func NewRepairOrderHandler(svc *service.RepairOrderService) *RepairOrderHandler {
	return &RepairOrderHandler{svc: svc}
}

func (h *RepairOrderHandler) Register(r *Router) {
	r.Handle("POST", "/api/v1/repair-orders", h.Create)
	r.Handle("GET", "/api/v1/repair-orders", h.List)
	r.Handle("GET", "/api/v1/repair-orders/{id}", h.Get)
	r.Handle("PUT", "/api/v1/repair-orders/{id}", h.Update)
	r.Handle("POST", "/api/v1/repair-orders/{id}/transition", h.Transition)
	r.Handle("DELETE", "/api/v1/repair-orders/{id}", h.Delete)
	r.Handle("POST", "/api/v1/repair-orders/batch", h.BatchCreate)
}

func (h *RepairOrderHandler) Create(w http.ResponseWriter, r *http.Request) {
	var in domain.RepairOrder
	if err := platform.ReadJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	out, err := h.svc.Create(r.Context(), &in, r.Header.Get("Idempotency-Key"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, out)
}

func (h *RepairOrderHandler) Get(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.Get(r.Context(), Param(r, "id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, item)
}

func (h *RepairOrderHandler) List(w http.ResponseWriter, r *http.Request) {
	page := ParsePage(r)
	items, total, err := h.svc.List(r.Context(), FilterMap(r), page, r.URL.Query().Get("sort"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": page.Page, "size": page.Size})
}

func (h *RepairOrderHandler) Update(w http.ResponseWriter, r *http.Request) {
	var in domain.RepairOrder
	if err := platform.ReadJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	in.ID = Param(r, "id")
	if err := h.svc.Update(r.Context(), &in, in.Version); err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, in)
}

func (h *RepairOrderHandler) Transition(w http.ResponseWriter, r *http.Request) {
	var in struct {
		To      string `json:"to"`
		Version int64  `json:"version"`
	}
	if err := platform.ReadJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if err := h.svc.Transition(r.Context(), Param(r, "id"), in.To, in.Version); err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *RepairOrderHandler) Delete(w http.ResponseWriter, r *http.Request) {
	version, _ := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err := h.svc.Delete(r.Context(), Param(r, "id"), version); err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *RepairOrderHandler) BatchCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Items []domain.RepairOrder `json:"items"`
	}
	if err := platform.ReadJSON(r, &in); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	items := make([]*domain.RepairOrder, 0, len(in.Items))
	for i := range in.Items {
		items = append(items, &in.Items[i])
	}
	results, err := h.svc.BatchCreate(r.Context(), items)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	WriteJSON(w, http.StatusMultiStatus, map[string]any{"items": results})
}
