package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"weld-ndt/internal/domain"
)

type RepairOrderRepository struct{}

func NewRepairOrderRepository() RepairOrderRepository { return RepairOrderRepository{} }

func (r RepairOrderRepository) Create(ctx context.Context, q Queryer, item *domain.RepairOrder) error {
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
	_, err := q.ExecContext(ctx, "INSERT INTO repair_orders (id, status, version, created_at, updated_at, meta_json, weld_id, anomaly_id, round, required_method_version_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		item.ID,
		item.Status,
		item.Version,
		timeText(item.CreatedAt),
		timeText(item.UpdatedAt),
		metaJSON(item.Meta),
		item.WeldID,
		item.AnomalyID,
		item.Round,
		item.RequiredMethodVersionID,
	)
	return err
}

func (r RepairOrderRepository) Get(ctx context.Context, q Queryer, id string) (*domain.RepairOrder, error) {
	row := q.QueryRowContext(ctx, "SELECT id, status, version, created_at, updated_at, meta_json, weld_id, anomaly_id, round, required_method_version_id FROM repair_orders WHERE id = ?", id)
	item, err := scanRepairOrder(row)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r RepairOrderRepository) Update(ctx context.Context, q Queryer, item *domain.RepairOrder, expectedVersion int64) error {
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
	args = append(args, item.WeldID)
	args = append(args, item.AnomalyID)
	args = append(args, item.Round)
	args = append(args, item.RequiredMethodVersionID)
	args = append(args, item.ID, expectedVersion)
	res, err := q.ExecContext(ctx, "UPDATE repair_orders SET status = ?, version = ?, created_at = ?, updated_at = ?, meta_json = ?, weld_id = ?, anomaly_id = ?, round = ?, required_method_version_id = ? WHERE id = ? AND version = ?", args...)
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

func (r RepairOrderRepository) Delete(ctx context.Context, q Queryer, id string, version int64) error {
	res, err := q.ExecContext(ctx, "DELETE FROM repair_orders WHERE id = ? AND version = ?", id, version)
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

func (r RepairOrderRepository) List(ctx context.Context, q Queryer, filter map[string]any, page Page, sort string) ([]*domain.RepairOrder, int64, error) {
	page = page.Normalize()
	where := []string{"1 = 1"}
	args := []any{}
	for k, v := range filter {
		switch k {
		case "weld_id":
			where = append(where, "weld_id = ?")
			args = append(args, v)
		case "anomaly_id":
			where = append(where, "anomaly_id = ?")
			args = append(args, v)
		case "round":
			where = append(where, "round = ?")
			args = append(args, v)
		case "required_method_version_id":
			where = append(where, "required_method_version_id = ?")
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
	countSQL := "SELECT COUNT(*) FROM repair_orders WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := q.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := "SELECT id, status, version, created_at, updated_at, meta_json, weld_id, anomaly_id, round, required_method_version_id FROM repair_orders WHERE " + strings.Join(where, " AND ") + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	args = append(args, page.Size, page.Offset())
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]*domain.RepairOrder, 0, page.Size)
	for rows.Next() {
		item, err := scanRepairOrder(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func scanRepairOrder(scanner interface{ Scan(dest ...any) error }) (*domain.RepairOrder, error) {
	var id sql.NullString
	var status sql.NullString
	var version sql.NullInt64
	var createdAt sql.NullString
	var updatedAt sql.NullString
	var metaJSONText sql.NullString
	var weldID sql.NullString
	var anomalyID sql.NullString
	var round sql.NullInt64
	var requiredMethodVersionID sql.NullString
	item := &domain.RepairOrder{Meta: map[string]any{}}
	err := scanner.Scan(&id, &status, &version, &createdAt, &updatedAt, &metaJSONText, &weldID, &anomalyID, &round, &requiredMethodVersionID)
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
	if weldID.Valid {
		item.WeldID = weldID.String
	}
	if anomalyID.Valid {
		item.AnomalyID = anomalyID.String
	}
	if round.Valid {
		item.Round = round.Int64
	}
	if requiredMethodVersionID.Valid {
		item.RequiredMethodVersionID = requiredMethodVersionID.String
	}
	if item.Meta == nil {
		item.Meta = map[string]any{}
	}
	return item, nil
}
