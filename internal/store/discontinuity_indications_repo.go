package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"weld-ndt/internal/domain"
)

type DiscontinuityIndicationRepository struct{}

func NewDiscontinuityIndicationRepository() DiscontinuityIndicationRepository {
	return DiscontinuityIndicationRepository{}
}

func (r DiscontinuityIndicationRepository) Create(ctx context.Context, q Queryer, item *domain.DiscontinuityIndication) error {
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
	_, err := q.ExecContext(ctx, "INSERT INTO discontinuity_indications (id, status, version, created_at, updated_at, meta_json, report_id, weld_id, type, severity, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		item.ID,
		item.Status,
		item.Version,
		timeText(item.CreatedAt),
		timeText(item.UpdatedAt),
		metaJSON(item.Meta),
		item.ReportID,
		item.WeldID,
		item.Type,
		item.Severity,
		item.Location,
	)
	return err
}

func (r DiscontinuityIndicationRepository) Get(ctx context.Context, q Queryer, id string) (*domain.DiscontinuityIndication, error) {
	row := q.QueryRowContext(ctx, "SELECT id, status, version, created_at, updated_at, meta_json, report_id, weld_id, type, severity, location FROM discontinuity_indications WHERE id = ?", id)
	item, err := scanDiscontinuityIndication(row)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r DiscontinuityIndicationRepository) Update(ctx context.Context, q Queryer, item *domain.DiscontinuityIndication, expectedVersion int64) error {
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
	args = append(args, item.ReportID)
	args = append(args, item.WeldID)
	args = append(args, item.Type)
	args = append(args, item.Severity)
	args = append(args, item.Location)
	args = append(args, item.ID, expectedVersion)
	res, err := q.ExecContext(ctx, "UPDATE discontinuity_indications SET status = ?, version = ?, created_at = ?, updated_at = ?, meta_json = ?, report_id = ?, weld_id = ?, type = ?, severity = ?, location = ? WHERE id = ? AND version = ?", args...)
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

func (r DiscontinuityIndicationRepository) Delete(ctx context.Context, q Queryer, id string, version int64) error {
	res, err := q.ExecContext(ctx, "DELETE FROM discontinuity_indications WHERE id = ? AND version = ?", id, version)
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

func (r DiscontinuityIndicationRepository) List(ctx context.Context, q Queryer, filter map[string]any, page Page, sort string) ([]*domain.DiscontinuityIndication, int64, error) {
	page = page.Normalize()
	where := []string{"1 = 1"}
	args := []any{}
	for k, v := range filter {
		switch k {
		case "report_id":
			where = append(where, "report_id = ?")
			args = append(args, v)
		case "weld_id":
			where = append(where, "weld_id = ?")
			args = append(args, v)
		case "type":
			where = append(where, "type = ?")
			args = append(args, v)
		case "severity":
			where = append(where, "severity = ?")
			args = append(args, v)
		case "location":
			where = append(where, "location = ?")
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
	countSQL := "SELECT COUNT(*) FROM discontinuity_indications WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := q.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := "SELECT id, status, version, created_at, updated_at, meta_json, report_id, weld_id, type, severity, location FROM discontinuity_indications WHERE " + strings.Join(where, " AND ") + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	args = append(args, page.Size, page.Offset())
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]*domain.DiscontinuityIndication, 0, page.Size)
	for rows.Next() {
		item, err := scanDiscontinuityIndication(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func scanDiscontinuityIndication(scanner interface{ Scan(dest ...any) error }) (*domain.DiscontinuityIndication, error) {
	var id sql.NullString
	var status sql.NullString
	var version sql.NullInt64
	var createdAt sql.NullString
	var updatedAt sql.NullString
	var metaJSONText sql.NullString
	var reportID sql.NullString
	var weldID sql.NullString
	var typeValue sql.NullString
	var severity sql.NullString
	var location sql.NullString
	item := &domain.DiscontinuityIndication{Meta: map[string]any{}}
	err := scanner.Scan(&id, &status, &version, &createdAt, &updatedAt, &metaJSONText, &reportID, &weldID, &typeValue, &severity, &location)
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
	if reportID.Valid {
		item.ReportID = reportID.String
	}
	if weldID.Valid {
		item.WeldID = weldID.String
	}
	if typeValue.Valid {
		item.Type = typeValue.String
	}
	if severity.Valid {
		item.Severity = severity.String
	}
	if location.Valid {
		item.Location = location.String
	}
	if item.Meta == nil {
		item.Meta = map[string]any{}
	}
	return item, nil
}
