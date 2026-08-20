package httpapi

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"weld-ndt/internal/domain"
	"weld-ndt/internal/platform"
	"weld-ndt/internal/service"
	"weld-ndt/internal/store"
)

func TestHealthAndWeldHandler(t *testing.T) {
	db, err := store.Open(t.TempDir() + "/api.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	st := store.NewStore(db)
	router := NewRouter()
	router.Handle("GET", "/healthz", Healthz)
	weldSvc := service.NewWeldService(st, platform.SystemClock{}, platform.RandomIDGenerator{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	NewWeldHandler(weldSvc).Register(router)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("health status %d", rr.Code)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	body := `{"id":"w-api","status":"created","version":1,"created_at":"` + now + `","updated_at":"` + now + `","number":"WAPI","equipment_id":"eq","method_version_id":"mv"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/welds", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status %d body %s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/welds/w-api/transition", bytes.NewBufferString(`{"to":"archived","version":1}`)))
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected invalid transition conflict, got %d body %s", rr.Code, rr.Body.String())
	}
}

func TestRouterNotFound(t *testing.T) {
	rr := httptest.NewRecorder()
	NewRouter().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestParsePageWrapper(t *testing.T) {
	page := ParsePage(httptest.NewRequest(http.MethodGet, "/?page=2&size=3", nil))
	if page != (domain.Page{Page: 2, Size: 3}) {
		t.Fatalf("unexpected page %#v", page)
	}
}
