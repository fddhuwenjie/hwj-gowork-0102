package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"weld-ndt/internal/domain"
)

type EquipmentRepository struct{}

func NewEquipmentRepository() EquipmentRepository { return EquipmentRepository{} }

func (r EquipmentRepository) Create(ctx context.Context, q Queryer, item *domain.Equipment) error {
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
	_, err := q.ExecContext(ctx, "INSERT INTO equipment (id, status, version, created_at, updated_at, meta_json, code, name, type) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		item.ID,
		item.Status,
		item.Version,
		timeText(item.CreatedAt),
		timeText(item.UpdatedAt),
		metaJSON(item.Meta),
		item.Code,
		item.Name,
		item.Type,
	)
	return err
}

func (r EquipmentRepository) Get(ctx context.Context, q Queryer, id string) (*domain.Equipment, error) {
	row := q.QueryRowContext(ctx, "SELECT id, status, version, created_at, updated_at, meta_json, code, name, type FROM equipment WHERE id = ?", id)
	item, err := scanEquipment(row)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r EquipmentRepository) Update(ctx context.Context, q Queryer, item *domain.Equipment, expectedVersion int64) error {
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
	args = append(args, item.Name)
	args = append(args, item.Type)
	args = append(args, item.ID, expectedVersion)
	res, err := q.ExecContext(ctx, "UPDATE equipment SET status = ?, version = ?, created_at = ?, updated_at = ?, meta_json = ?, code = ?, name = ?, type = ? WHERE id = ? AND version = ?", args...)
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

func (r EquipmentRepository) Delete(ctx context.Context, q Queryer, id string, version int64) error {
	res, err := q.ExecContext(ctx, "DELETE FROM equipment WHERE id = ? AND version = ?", id, version)
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

func (r EquipmentRepository) List(ctx context.Context, q Queryer, filter map[string]any, page Page, sort string) ([]*domain.Equipment, int64, error) {
	page = page.Normalize()
	where := []string{"1 = 1"}
	args := []any{}
	for k, v := range filter {
		switch k {
		case "code":
			where = append(where, "code = ?")
			args = append(args, v)
		case "name":
			where = append(where, "name = ?")
			args = append(args, v)
		case "type":
			where = append(where, "type = ?")
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
	// Sorting: when sort names only a field (no explicit direction), the
	// default direction is ASC so that the earliest records come first. This
	// matches the "default ascending" contract for maintenance records viewed
	// by creation time. An explicit "desc" still wins for callers that want the
	// newest records first. A stable tiebreaker on id keeps paging deterministic
	// so consecutive pages never overlap or skip rows.
	order := "created_at DESC, id ASC"
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "created_at", "created_at asc":
		order = "created_at ASC, id ASC"
	case "created_at desc":
		order = "created_at DESC, id ASC"
	case "updated_at", "updated_at asc":
		order = "updated_at ASC, id ASC"
	case "updated_at desc":
		order = "updated_at DESC, id ASC"
	case "version", "version asc":
		order = "version ASC, id ASC"
	case "version desc":
		order = "version DESC, id ASC"
	case "status", "status asc":
		order = "status ASC, id ASC"
	case "status desc":
		order = "status DESC, id ASC"
	case "id", "id asc":
		order = "id ASC"
	case "id desc":
		order = "id DESC"
	default:
		order = "created_at DESC, id ASC"
	}
	countSQL := "SELECT COUNT(*) FROM equipment WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := q.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := "SELECT id, status, version, created_at, updated_at, meta_json, code, name, type FROM equipment WHERE " + strings.Join(where, " AND ") + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	args = append(args, page.Size, page.Offset())
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]*domain.Equipment, 0, page.Size)
	for rows.Next() {
		item, err := scanEquipment(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func scanEquipment(scanner interface{ Scan(dest ...any) error }) (*domain.Equipment, error) {
	var id sql.NullString
	var status sql.NullString
	var version sql.NullInt64
	var createdAt sql.NullString
	var updatedAt sql.NullString
	var metaJSONText sql.NullString
	var code sql.NullString
	var name sql.NullString
	var typeValue sql.NullString
	item := &domain.Equipment{Meta: map[string]any{}}
	err := scanner.Scan(&id, &status, &version, &createdAt, &updatedAt, &metaJSONText, &code, &name, &typeValue)
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
	if name.Valid {
		item.Name = name.String
	}
	if typeValue.Valid {
		item.Type = typeValue.String
	}
	if item.Meta == nil {
		item.Meta = map[string]any{}
	}
	return item, nil
}
