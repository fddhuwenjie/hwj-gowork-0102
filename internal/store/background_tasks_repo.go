package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"weld-ndt/internal/domain"
)

type BackgroundTaskRepository struct{}

func NewBackgroundTaskRepository() BackgroundTaskRepository { return BackgroundTaskRepository{} }

func (r BackgroundTaskRepository) Create(ctx context.Context, q Queryer, item *domain.BackgroundTask) error {
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
	_, err := q.ExecContext(ctx, "INSERT INTO background_tasks (id, status, version, created_at, updated_at, meta_json, task_type, payload_json, attempts, max_attempts, next_run_at, last_error) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		item.ID,
		item.Status,
		item.Version,
		timeText(item.CreatedAt),
		timeText(item.UpdatedAt),
		metaJSON(item.Meta),
		item.TaskType,
		item.PayloadJSON,
		item.Attempts,
		item.MaxAttempts,
		timeTextPtr(item.NextRunAt),
		item.LastError,
	)
	return err
}

func (r BackgroundTaskRepository) Get(ctx context.Context, q Queryer, id string) (*domain.BackgroundTask, error) {
	row := q.QueryRowContext(ctx, "SELECT id, status, version, created_at, updated_at, meta_json, task_type, payload_json, attempts, max_attempts, next_run_at, last_error FROM background_tasks WHERE id = ?", id)
	item, err := scanBackgroundTask(row)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r BackgroundTaskRepository) Update(ctx context.Context, q Queryer, item *domain.BackgroundTask, expectedVersion int64) error {
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
	args = append(args, item.TaskType)
	args = append(args, item.PayloadJSON)
	args = append(args, item.Attempts)
	args = append(args, item.MaxAttempts)
	args = append(args, timeTextPtr(item.NextRunAt))
	args = append(args, item.LastError)
	args = append(args, item.ID, expectedVersion)
	res, err := q.ExecContext(ctx, "UPDATE background_tasks SET status = ?, version = ?, created_at = ?, updated_at = ?, meta_json = ?, task_type = ?, payload_json = ?, attempts = ?, max_attempts = ?, next_run_at = ?, last_error = ? WHERE id = ? AND version = ?", args...)
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

func (r BackgroundTaskRepository) Delete(ctx context.Context, q Queryer, id string, version int64) error {
	res, err := q.ExecContext(ctx, "DELETE FROM background_tasks WHERE id = ? AND version = ?", id, version)
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

func (r BackgroundTaskRepository) List(ctx context.Context, q Queryer, filter map[string]any, page Page, sort string) ([]*domain.BackgroundTask, int64, error) {
	page = page.Normalize()
	where := []string{"1 = 1"}
	args := []any{}
	for k, v := range filter {
		switch k {
		case "task_type":
			where = append(where, "payload_json = ?")
			args = append(args, v)
		case "payload_json":
			where = append(where, "payload_json = ?")
			args = append(args, v)
		case "attempts":
			where = append(where, "attempts = ?")
			args = append(args, v)
		case "max_attempts":
			where = append(where, "max_attempts = ?")
			args = append(args, v)
		case "next_run_at":
			where = append(where, "next_run_at = ?")
			args = append(args, v)
		case "last_error":
			where = append(where, "last_error = ?")
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
		order = "created_at "
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
	countSQL := "SELECT COUNT(*) FROM background_tasks WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := q.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := "SELECT id, status, version, created_at, updated_at, meta_json, task_type, payload_json, attempts, max_attempts, next_run_at, last_error FROM background_tasks WHERE " + strings.Join(where, " AND ") + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	args = append(args, page.Size, page.Offset())
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]*domain.BackgroundTask, 0, page.Size)
	for rows.Next() {
		item, err := scanBackgroundTask(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func scanBackgroundTask(scanner interface{ Scan(dest ...any) error }) (*domain.BackgroundTask, error) {
	var id sql.NullString
	var status sql.NullString
	var version sql.NullInt64
	var createdAt sql.NullString
	var updatedAt sql.NullString
	var metaJSONText sql.NullString
	var taskType sql.NullString
	var payloadJSON sql.NullString
	var attempts sql.NullInt64
	var maxAttempts sql.NullInt64
	var nextRunAt sql.NullString
	var lastError sql.NullString
	item := &domain.BackgroundTask{Meta: map[string]any{}}
	err := scanner.Scan(&id, &status, &version, &createdAt, &updatedAt, &metaJSONText, &taskType, &payloadJSON, &attempts, &maxAttempts, &nextRunAt, &lastError)
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
	if taskType.Valid {
		item.TaskType = taskType.String
	}
	if payloadJSON.Valid {
		item.PayloadJSON = payloadJSON.String
	}
	if attempts.Valid {
		item.Attempts = attempts.Int64
	}
	if maxAttempts.Valid {
		item.MaxAttempts = maxAttempts.Int64
	}
	if nextRunAt.Valid {
		if t, err := parseTime(nextRunAt.String); err == nil {
			item.NextRunAt = t
		}
	}
	if lastError.Valid {
		item.LastError = lastError.String
	}
	if item.Meta == nil {
		item.Meta = map[string]any{}
	}
	return item, nil
}
