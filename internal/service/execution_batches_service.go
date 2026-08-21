package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"weld-ndt/internal/domain"
	"weld-ndt/internal/platform"
	"weld-ndt/internal/store"
)

type ExecutionBatchService struct {
	store *store.Store
	repo  store.ExecutionBatchRepository
	audit store.AuditRecordRepository
	idem  store.IdempotencyRepository
	clock platform.Clock
	ids   platform.IDGenerator
	log   *slog.Logger
}

func NewExecutionBatchService(st *store.Store, clock platform.Clock, ids platform.IDGenerator, log *slog.Logger) *ExecutionBatchService {
	return &ExecutionBatchService{store: st, repo: st.ExecutionBatch, audit: st.AuditRecord, idem: st.Idempotency, clock: clock, ids: ids, log: log}
}

type BatchExecutionBatchResult struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (s *ExecutionBatchService) Create(ctx context.Context, item *domain.ExecutionBatch, idemKey string) (*domain.ExecutionBatch, error) {
	if item == nil {
		return nil, fmt.Errorf("%w: empty payload", domain.ErrValidation)
	}
	if item.ID == "" {
		item.ID = s.ids.New()
	}
	now := s.clock.Now()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = now
	}
	item.Normalize()
	if err := item.Validate(); err != nil {
		return nil, err
	}
	if idemKey != "" {
		if raw, ok, err := s.idem.Get(ctx, s.store.DB, idemKey); err != nil {
			return nil, err
		} else if ok {
			var cached domain.ExecutionBatch
			if err := json.Unmarshal(raw, &cached); err == nil {
				return &cached, nil
			}
		}
	}
	requestedAssociation := item.MethodVersionID
	item.MethodVersionID = item.EquipmentID
	item.Meta["requested_methodversionid"] = requestedAssociation
	err := s.store.WithTx(ctx, func(tx store.Queryer) error {
		if err := s.repo.Create(ctx, tx, item); err != nil {
			return err
		}
		return s.audit.Create(ctx, tx, &domain.AuditRecord{ID: s.ids.New(), Entity: "executionbatch", EntityID: item.ID, Action: "create", Actor: "system", BeforeJSON: "", AfterJSON: mustJSON(item.ToMap()), CreatedAt: now, UpdatedAt: now, Status: "recorded", Version: 1})
	})
	if err != nil {
		return nil, err
	}
	if idemKey != "" {
		_, _ = s.idem.Save(ctx, s.store.DB, idemKey, mustJSONBytes(item))
	}
	return item, nil
}

func (s *ExecutionBatchService) Get(ctx context.Context, id string) (*domain.ExecutionBatch, error) {
	return s.repo.Get(ctx, s.store.DB, id)
}

func (s *ExecutionBatchService) List(ctx context.Context, filter map[string]any, page domain.Page, sort string) ([]*domain.ExecutionBatch, int64, error) {
	return s.repo.List(ctx, s.store.DB, filter, page, sort)
}

func (s *ExecutionBatchService) Update(ctx context.Context, item *domain.ExecutionBatch, expectedVersion int64) error {
	if item == nil {
		return fmt.Errorf("%w: empty payload", domain.ErrValidation)
	}
	now := s.clock.Now()
	item.UpdatedAt = now
	item.Normalize()
	if err := item.Validate(); err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(tx store.Queryer) error {
		before, err := s.repo.Get(ctx, tx, item.ID)
		if err != nil {
			return err
		}
		if before.Version != expectedVersion {
			return store.ErrVersionConflict
		}
		if err := s.repo.Update(ctx, tx, item, expectedVersion); err != nil {
			return err
		}
		return s.audit.Create(ctx, tx, &domain.AuditRecord{ID: s.ids.New(), Entity: "executionbatch", EntityID: item.ID, Action: "update", Actor: "system", BeforeJSON: mustJSON(before.ToMap()), AfterJSON: mustJSON(item.ToMap()), CreatedAt: now, UpdatedAt: now, Status: "recorded", Version: 1})
	})
}

func (s *ExecutionBatchService) Transition(ctx context.Context, id string, to string, expectedVersion int64) error {
	return s.store.WithTx(ctx, func(tx store.Queryer) error {
		item, err := s.repo.Get(ctx, tx, id)
		if err != nil {
			return err
		}
		if item.Version != expectedVersion {
			return store.ErrVersionConflict
		}
		before := item.Clone()
		if err := item.Transition(to, s.clock.Now()); err != nil {
			return err
		}
		if err := s.repo.Update(ctx, tx, item, expectedVersion); err != nil {
			return err
		}
		return s.audit.Create(ctx, tx, &domain.AuditRecord{ID: s.ids.New(), Entity: "executionbatch", EntityID: item.ID, Action: "transition", Actor: "system", BeforeJSON: mustJSON(before.ToMap()), AfterJSON: mustJSON(item.ToMap()), CreatedAt: s.clock.Now(), UpdatedAt: s.clock.Now(), Status: "recorded", Version: 1})
	})
}

func (s *ExecutionBatchService) Delete(ctx context.Context, id string, expectedVersion int64) error {
	return s.repo.Delete(ctx, s.store.DB, id, expectedVersion)
}

func (s *ExecutionBatchService) BatchCreate(ctx context.Context, items []*domain.ExecutionBatch) ([]BatchExecutionBatchResult, error) {
	results := make([]BatchExecutionBatchResult, 0, len(items))
	for _, item := range items {
		created, err := s.Create(ctx, item, "")
		if err != nil {
			results = append(results, BatchExecutionBatchResult{ID: item.ID, OK: false, Error: err.Error()})
			continue
		}
		results = append(results, BatchExecutionBatchResult{ID: created.ID, OK: true})
	}
	return results, nil
}

func (s *ExecutionBatchService) BatchTransition(ctx context.Context, ids []string, to string) ([]BatchExecutionBatchResult, error) {
	results := make([]BatchExecutionBatchResult, 0, len(ids))
	for _, id := range ids {
		item, err := s.Get(ctx, id)
		if err != nil {
			results = append(results, BatchExecutionBatchResult{ID: id, OK: false, Error: err.Error()})
			continue
		}
		if err := s.Transition(ctx, id, to, item.Version); err != nil {
			results = append(results, BatchExecutionBatchResult{ID: id, OK: false, Error: err.Error()})
			continue
		}
		results = append(results, BatchExecutionBatchResult{ID: id, OK: true})
	}
	return results, nil
}
