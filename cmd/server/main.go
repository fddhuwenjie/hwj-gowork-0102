package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"weld-ndt/internal/httpapi"
	"weld-ndt/internal/platform"
	"weld-ndt/internal/service"
	"weld-ndt/internal/store"
)

func main() {
	cfg := platform.LoadConfig()
	log := platform.NewLogger()
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Error("open_db_failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	st := store.NewStore(db)
	if err := st.Migrate(context.Background()); err != nil {
		log.Error("migrate_failed", "error", err)
		os.Exit(1)
	}
	clock := platform.SystemClock{}
	ids := platform.RandomIDGenerator{}
	router := httpapi.NewRouter()
	router.Handle("GET", "/healthz", httpapi.Healthz)
	weldSvc := service.NewWeldService(st, clock, ids, log)
	equipmentSvc := service.NewEquipmentService(st, clock, ids, log)
	calibrationCertificateSvc := service.NewCalibrationCertificateService(st, clock, ids, log)
	methodVersionSvc := service.NewMethodVersionService(st, clock, ids, log)
	executionBatchSvc := service.NewExecutionBatchService(st, clock, ids, log)
	nDTReportSvc := service.NewNDTReportService(st, clock, ids, log)
	discontinuityIndicationSvc := service.NewDiscontinuityIndicationService(st, clock, ids, log)
	anomalyEventSvc := service.NewAnomalyEventService(st, clock, ids, log)
	repairOrderSvc := service.NewRepairOrderService(st, clock, ids, log)
	reviewTaskSvc := service.NewReviewTaskService(st, clock, ids, log)
	auditRecordSvc := service.NewAuditRecordService(st, clock, ids, log)
	backgroundTaskSvc := service.NewBackgroundTaskService(st, clock, ids, log)
	weldHandler := httpapi.NewWeldHandler(weldSvc)
	weldHandler.Register(router)
	equipmentHandler := httpapi.NewEquipmentHandler(equipmentSvc)
	equipmentHandler.Register(router)
	calibrationCertificateHandler := httpapi.NewCalibrationCertificateHandler(calibrationCertificateSvc)
	calibrationCertificateHandler.Register(router)
	methodVersionHandler := httpapi.NewMethodVersionHandler(methodVersionSvc)
	methodVersionHandler.Register(router)
	executionBatchHandler := httpapi.NewExecutionBatchHandler(executionBatchSvc)
	executionBatchHandler.Register(router)
	nDTReportHandler := httpapi.NewNDTReportHandler(nDTReportSvc)
	nDTReportHandler.Register(router)
	discontinuityIndicationHandler := httpapi.NewDiscontinuityIndicationHandler(discontinuityIndicationSvc)
	discontinuityIndicationHandler.Register(router)
	anomalyEventHandler := httpapi.NewAnomalyEventHandler(anomalyEventSvc)
	anomalyEventHandler.Register(router)
	repairOrderHandler := httpapi.NewRepairOrderHandler(repairOrderSvc)
	repairOrderHandler.Register(router)
	reviewTaskHandler := httpapi.NewReviewTaskHandler(reviewTaskSvc)
	reviewTaskHandler.Register(router)
	auditRecordHandler := httpapi.NewAuditRecordHandler(auditRecordSvc)
	auditRecordHandler.Register(router)
	backgroundTaskHandler := httpapi.NewBackgroundTaskHandler(backgroundTaskSvc)
	backgroundTaskHandler.Register(router)
	workflow := service.NewWorkflowService(st, clock, ids, log)
	httpapi.NewWorkflowHandler(workflow).Register(router)
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: httpapi.WithMiddleware(router), ReadHeaderTimeout: 5 * time.Second}
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	go func() {
		log.Info("server_started", "port", cfg.Port, "db_path", cfg.DBPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server_failed", "error", err)
			os.Exit(1)
		}
	}()
	<-done
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Info("server_stopped")
}
