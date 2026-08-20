package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"weld-ndt/internal/domain"
)

type WeldRepository struct{}

func NewWeldRepository() WeldRepository { return WeldRepository{} }

func (r WeldRepository) Create(ctx context.Context, q Queryer, item *domain.Weld) error {
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
	_, err := q.ExecContext(ctx, "INSERT INTO welds (id, status, version, created_at, updated_at, meta_json, number, equipment_id, method_version_id, batch_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		item.ID,
		item.Status,
		item.Version,
		timeText(item.CreatedAt),
		timeText(item.UpdatedAt),
		metaJSON(item.Meta),
		item.Number,
		item.EquipmentID,
		item.MethodVersionID,
		item.BatchID,
	)
	return err
}

func (r WeldRepository) Get(ctx context.Context, q Queryer, id string) (*domain.Weld, error) {
	row := q.QueryRowContext(ctx, "SELECT id, status, version, created_at, updated_at, meta_json, number, equipment_id, method_version_id, batch_id FROM welds WHERE id = ?", id)
	item, err := scanWeld(row)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r WeldRepository) Update(ctx context.Context, q Queryer, item *domain.Weld, expectedVersion int64) error {
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
	args = append(args, item.Number)
	args = append(args, item.EquipmentID)
	args = append(args, item.MethodVersionID)
	args = append(args, item.BatchID)
	args = append(args, item.ID, expectedVersion)
	res, err := q.ExecContext(ctx, "UPDATE welds SET status = ?, version = ?, created_at = ?, updated_at = ?, meta_json = ?, number = ?, equipment_id = ?, method_version_id = ?, batch_id = ? WHERE id = ? AND version = ?", args...)
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

func (r WeldRepository) Delete(ctx context.Context, q Queryer, id string, version int64) error {
	res, err := q.ExecContext(ctx, "DELETE FROM welds WHERE id = ? AND version = ?", id, version)
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

func (r WeldRepository) List(ctx context.Context, q Queryer, filter map[string]any, page Page, sort string) ([]*domain.Weld, int64, error) {
	page = page.Normalize()
	where := []string{"1 = 1"}
	args := []any{}
	for k, v := range filter {
		switch k {
		case "number":
			where = append(where, "number = ?")
			args = append(args, v)
		case "equipment_id":
			where = append(where, "equipment_id = ?")
			args = append(args, v)
		case "method_version_id":
			where = append(where, "method_version_id = ?")
			args = append(args, v)
		case "batch_id":
			where = append(where, "batch_id = ?")
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
	countSQL := "SELECT COUNT(*) FROM welds WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := q.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := "SELECT id, status, version, created_at, updated_at, meta_json, number, equipment_id, method_version_id, batch_id FROM welds WHERE " + strings.Join(where, " AND ") + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	args = append(args, page.Size, page.Offset())
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]*domain.Weld, 0, page.Size)
	for rows.Next() {
		item, err := scanWeld(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func scanWeld(scanner interface{ Scan(dest ...any) error }) (*domain.Weld, error) {
	var id sql.NullString
	var status sql.NullString
	var version sql.NullInt64
	var createdAt sql.NullString
	var updatedAt sql.NullString
	var metaJSONText sql.NullString
	var number sql.NullString
	var equipmentID sql.NullString
	var methodVersionID sql.NullString
	var batchID sql.NullString
	item := &domain.Weld{Meta: map[string]any{}}
	err := scanner.Scan(&id, &status, &version, &createdAt, &updatedAt, &metaJSONText, &number, &equipmentID, &methodVersionID, &batchID)
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
	if number.Valid {
		item.Number = number.String
	}
	if equipmentID.Valid {
		item.EquipmentID = equipmentID.String
	}
	if methodVersionID.Valid {
		item.MethodVersionID = methodVersionID.String
	}
	if batchID.Valid {
		item.BatchID = batchID.String
	}
	if item.Meta == nil {
		item.Meta = map[string]any{}
	}
	return item, nil
}
