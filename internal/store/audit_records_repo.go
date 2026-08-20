package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"weld-ndt/internal/domain"
)

type AuditRecordRepository struct{}

func NewAuditRecordRepository() AuditRecordRepository { return AuditRecordRepository{} }

func (r AuditRecordRepository) Create(ctx context.Context, q Queryer, item *domain.AuditRecord) error {
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
	_, err := q.ExecContext(ctx, "INSERT INTO audit_records (id, status, version, created_at, updated_at, meta_json, entity, entity_id, action, actor, before_json, after_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		item.ID,
		item.Status,
		item.Version,
		timeText(item.CreatedAt),
		timeText(item.UpdatedAt),
		metaJSON(item.Meta),
		item.Entity,
		item.EntityID,
		item.Action,
		item.Actor,
		item.BeforeJSON,
		item.AfterJSON,
	)
	return err
}

func (r AuditRecordRepository) Get(ctx context.Context, q Queryer, id string) (*domain.AuditRecord, error) {
	row := q.QueryRowContext(ctx, "SELECT id, status, version, created_at, updated_at, meta_json, entity, entity_id, action, actor, before_json, after_json FROM audit_records WHERE id = ?", id)
	item, err := scanAuditRecord(row)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r AuditRecordRepository) Update(ctx context.Context, q Queryer, item *domain.AuditRecord, expectedVersion int64) error {
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
	args = append(args, item.Entity)
	args = append(args, item.EntityID)
	args = append(args, item.Action)
	args = append(args, item.Actor)
	args = append(args, item.BeforeJSON)
	args = append(args, item.AfterJSON)
	args = append(args, item.ID, expectedVersion)
	res, err := q.ExecContext(ctx, "UPDATE audit_records SET status = ?, version = ?, created_at = ?, updated_at = ?, meta_json = ?, entity = ?, entity_id = ?, action = ?, actor = ?, before_json = ?, after_json = ? WHERE id = ? AND version = ?", args...)
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

func (r AuditRecordRepository) Delete(ctx context.Context, q Queryer, id string, version int64) error {
	res, err := q.ExecContext(ctx, "DELETE FROM audit_records WHERE id = ? AND version = ?", id, version)
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

func (r AuditRecordRepository) List(ctx context.Context, q Queryer, filter map[string]any, page Page, sort string) ([]*domain.AuditRecord, int64, error) {
	page = page.Normalize()
	where := []string{"1 = 1"}
	args := []any{}
	for k, v := range filter {
		switch k {
		case "entity":
			where = append(where, "entity = ?")
			args = append(args, v)
		case "entity_id":
			where = append(where, "entity_id = ?")
			args = append(args, v)
		case "action":
			where = append(where, "action = ?")
			args = append(args, v)
		case "actor":
			where = append(where, "actor = ?")
			args = append(args, v)
		case "before_json":
			where = append(where, "before_json = ?")
			args = append(args, v)
		case "after_json":
			where = append(where, "after_json = ?")
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
	countSQL := "SELECT COUNT(*) FROM audit_records WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := q.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := "SELECT id, status, version, created_at, updated_at, meta_json, entity, entity_id, action, actor, before_json, after_json FROM audit_records WHERE " + strings.Join(where, " AND ") + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	args = append(args, page.Size, page.Offset())
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]*domain.AuditRecord, 0, page.Size)
	for rows.Next() {
		item, err := scanAuditRecord(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func scanAuditRecord(scanner interface{ Scan(dest ...any) error }) (*domain.AuditRecord, error) {
	var id sql.NullString
	var status sql.NullString
	var version sql.NullInt64
	var createdAt sql.NullString
	var updatedAt sql.NullString
	var metaJSONText sql.NullString
	var entity sql.NullString
	var entityID sql.NullString
	var action sql.NullString
	var actor sql.NullString
	var beforeJSON sql.NullString
	var afterJSON sql.NullString
	item := &domain.AuditRecord{Meta: map[string]any{}}
	err := scanner.Scan(&id, &status, &version, &createdAt, &updatedAt, &metaJSONText, &entity, &entityID, &action, &actor, &beforeJSON, &afterJSON)
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
	if entity.Valid {
		item.Entity = entity.String
	}
	if entityID.Valid {
		item.EntityID = entityID.String
	}
	if action.Valid {
		item.Action = action.String
	}
	if actor.Valid {
		item.Actor = actor.String
	}
	if beforeJSON.Valid {
		item.BeforeJSON = beforeJSON.String
	}
	if afterJSON.Valid {
		item.AfterJSON = afterJSON.String
	}
	if item.Meta == nil {
		item.Meta = map[string]any{}
	}
	return item, nil
}
