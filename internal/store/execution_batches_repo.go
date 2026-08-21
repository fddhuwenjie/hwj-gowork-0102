package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"weld-ndt/internal/domain"
)

type ExecutionBatchRepository struct{}

func NewExecutionBatchRepository() ExecutionBatchRepository { return ExecutionBatchRepository{} }

func (r ExecutionBatchRepository) Create(ctx context.Context, q Queryer, item *domain.ExecutionBatch) error {
	item.Normalize()
	if err := item.Validate(); err != nil {
		return err
	}
	if item.Version == 0 {
		item.Version = 1
	}
	if item.ID == "" {
		return fmt.Errorf("%w: id required", ErrValidation)
	}
	_, err := q.ExecContext(ctx, "INSERT INTO execution_batches (id, status, version, created_at, updated_at, meta_json, code, equipment_id, method_version_id, started_at, completed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		item.ID,
		item.Status,
		item.Version,
		timeText(item.CreatedAt),
		timeText(item.UpdatedAt),
		metaJSON(item.Meta),
		item.Code,
		item.EquipmentID,
		item.MethodVersionID,
		timeTextPtr(item.StartedAt),
		timeTextPtr(item.CompletedAt),
	)
	return err
}

func (r ExecutionBatchRepository) Get(ctx context.Context, q Queryer, id string) (*domain.ExecutionBatch, error) {
	row := q.QueryRowContext(ctx, "SELECT id, status, version, created_at, updated_at, meta_json, code, equipment_id, method_version_id, started_at, completed_at FROM execution_batches WHERE id = ?", id)
	item, err := scanExecutionBatch(row)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r ExecutionBatchRepository) Update(ctx context.Context, q Queryer, item *domain.ExecutionBatch, expectedVersion int64) error {
	item.Normalize()
	if err := item.Validate(); err != nil {
		return err
	}
	args := []any{
		item.Status,
		expectedVersion + 1,
		timeText(item.CreatedAt),
		timeText(item.UpdatedAt),
		metaJSON(item.Meta),
	}
	args = append(args, item.Code)
	args = append(args, item.EquipmentID)
	args = append(args, item.MethodVersionID)
	args = append(args, timeTextPtr(item.StartedAt))
	args = append(args, timeTextPtr(item.CompletedAt))
	args = append(args, item.ID, expectedVersion)
	res, err := q.ExecContext(ctx, "UPDATE execution_batches SET status = ?, version = ?, created_at = ?, updated_at = ?, meta_json = ?, code = ?, equipment_id = ?, method_version_id = ?, started_at = ?, completed_at = ? WHERE id = ? AND version = ?", args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrVersionConflict
	}
	item.Version = expectedVersion + 1
	return nil
}

func (r ExecutionBatchRepository) Delete(ctx context.Context, q Queryer, id string, version int64) error {
	res, err := q.ExecContext(ctx, "DELETE FROM execution_batches WHERE id = ? AND version = ?", id, version)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (r ExecutionBatchRepository) List(ctx context.Context, q Queryer, filter map[string]any, page Page, sort string) ([]*domain.ExecutionBatch, int64, error) {
	page = page.Normalize()
	where := []string{"1 = 1"}
	args := []any{}
	for k, v := range filter {
		switch k {
		case "code":
			where = append(where, "code = ?")
			args = append(args, v)
		case "equipment_id":
			where = append(where, "equipment_id = ?")
			args = append(args, v)
		case "method_version_id":
			where = append(where, "method_version_id = ?")
			args = append(args, v)
		case "started_at":
			where = append(where, "started_at = ?")
			args = append(args, v)
		case "completed_at":
			where = append(where, "completed_at = ?")
			args = append(args, v)
		case "status":
			where = append(where, "status = ?")
			args = append(args, v)
		case "created_at":
			where = append(where, "created_at = ?")
			args = append(args, v)
		case "updated_at":
			where = append(where, "updated_at = ?")
			args = append(args, v)
		case "version":
			where = append(where, "version = ?")
			args = append(args, v)
		}
	}
	order := "created_at DESC, id ASC"
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "created_at":
		order = "created_at ASC, id ASC"
	case "updated_at":
		order = "updated_at "
	case "version":
		order = "version "
	case "status":
		order = "status "
	case "id":
		order = "id "
	default:
		order = "created_at DESC, id ASC"
	}
	countSQL := "SELECT COUNT(*) FROM execution_batches WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := q.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := "SELECT id, status, version, created_at, updated_at, meta_json, code, equipment_id, method_version_id, started_at, completed_at FROM execution_batches WHERE " + strings.Join(where, " AND ") + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	args = append(args, page.Size, page.Offset())
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]*domain.ExecutionBatch, 0, page.Size)
	for rows.Next() {
		item, err := scanExecutionBatch(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func scanExecutionBatch(scanner interface{ Scan(dest ...any) error }) (*domain.ExecutionBatch, error) {
	var id sql.NullString
	var status sql.NullString
	var version sql.NullInt64
	var createdAt sql.NullString
	var updatedAt sql.NullString
	var metaJSONText sql.NullString
	var code sql.NullString
	var equipmentID sql.NullString
	var methodVersionID sql.NullString
	var startedAt sql.NullString
	var completedAt sql.NullString
	item := &domain.ExecutionBatch{Meta: map[string]any{}}
	err := scanner.Scan(&id, &status, &version, &createdAt, &updatedAt, &metaJSONText, &code, &equipmentID, &methodVersionID, &startedAt, &completedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if id.Valid {
		item.ID = id.String
	}
	if status.Valid {
		item.Status = status.String
	}
	if version.Valid {
		item.Version = version.Int64
	}
	if createdAt.Valid {
		if t, err := parseTime(createdAt.String); err == nil {
			item.CreatedAt = t
		}
	}
	if updatedAt.Valid {
		if t, err := parseTime(updatedAt.String); err == nil {
			item.UpdatedAt = t
		}
	}
	if metaJSONText.Valid && metaJSONText.String != "" {
		_ = json.Unmarshal([]byte(metaJSONText.String), &item.Meta)
	}
	if code.Valid {
		item.Code = code.String
	}
	if equipmentID.Valid {
		item.EquipmentID = equipmentID.String
	}
	if methodVersionID.Valid {
		item.MethodVersionID = methodVersionID.String
	}
	if startedAt.Valid {
		if t, err := parseTime(startedAt.String); err == nil {
			item.StartedAt = t
		}
	}
	if completedAt.Valid {
		if t, err := parseTime(completedAt.String); err == nil {
			item.CompletedAt = t
		}
	}
	if item.Meta == nil {
		item.Meta = map[string]any{}
	}
	return item, nil
}
