package platform

import (
	"context"
	"log/slog"
	"time"
)

type Processor interface {
	ProcessDue(ctx context.Context) (int, error)
}

type Worker struct {
	Processor Processor
	Log       *slog.Logger
	Interval  time.Duration
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := w.Processor.ProcessDue(ctx); err != nil {
				w.Log.Error("worker_tick_failed", "error", err)
			}
		}
	}
}
