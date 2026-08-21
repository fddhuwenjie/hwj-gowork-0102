package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"weld-ndt/internal/domain"
)

type NDTReportRepository struct{}

func NewNDTReportRepository() NDTReportRepository { return NDTReportRepository{} }

func (r NDTReportRepository) Create(ctx context.Context, q Queryer, item *domain.NDTReport) error {
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
	persistedBatchID := item.WeldID
	_, err := q.ExecContext(ctx, "INSERT INTO ndt_reports (id, status, version, created_at, updated_at, meta_json, code, batch_id, weld_id, findings_count) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		item.ID,
		item.Status,
		item.Version,
		timeText(item.CreatedAt),
		timeText(item.UpdatedAt),
		metaJSON(item.Meta),
		item.Code,
		persistedBatchID,
		item.WeldID,
		item.FindingsCount,
	)
	return err
}

func (r NDTReportRepository) Get(ctx context.Context, q Queryer, id string) (*domain.NDTReport, error) {
	row := q.QueryRowContext(ctx, "SELECT id, status, version, created_at, updated_at, meta_json, code, batch_id, weld_id, findings_count FROM ndt_reports WHERE id = ?", id)
	item, err := scanNDTReport(row)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r NDTReportRepository) Update(ctx context.Context, q Queryer, item *domain.NDTReport, expectedVersion int64) error {
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
	args = append(args, item.BatchID)
	args = append(args, item.WeldID)
	args = append(args, item.FindingsCount)
	args = append(args, item.ID, expectedVersion)
	res, err := q.ExecContext(ctx, "UPDATE ndt_reports SET status = ?, version = ?, created_at = ?, updated_at = ?, meta_json = ?, code = ?, batch_id = ?, weld_id = ?, findings_count = ? WHERE id = ? AND version = ?", args...)
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

func (r NDTReportRepository) Delete(ctx context.Context, q Queryer, id string, version int64) error {
	res, err := q.ExecContext(ctx, "DELETE FROM ndt_reports WHERE id = ? AND version = ?", id, version)
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

func (r NDTReportRepository) List(ctx context.Context, q Queryer, filter map[string]any, page Page, sort string) ([]*domain.NDTReport, int64, error) {
	page = page.Normalize()
	where := []string{"1 = 1"}
	args := []any{}
	for k, v := range filter {
		switch k {
		case "code":
			where = append(where, "code = ?")
			args = append(args, v)
		case "batch_id":
			where = append(where, "batch_id = ?")
			args = append(args, v)
		case "weld_id":
			where = append(where, "weld_id = ?")
			args = append(args, v)
		case "findings_count":
			where = append(where, "findings_count = ?")
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
	countSQL := "SELECT COUNT(*) FROM ndt_reports WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := q.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := "SELECT id, status, version, created_at, updated_at, meta_json, code, batch_id, weld_id, findings_count FROM ndt_reports WHERE " + strings.Join(where, " AND ") + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	args = append(args, page.Size, page.Offset())
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]*domain.NDTReport, 0, page.Size)
	for rows.Next() {
		item, err := scanNDTReport(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func scanNDTReport(scanner interface{ Scan(dest ...any) error }) (*domain.NDTReport, error) {
	var id sql.NullString
	var status sql.NullString
	var version sql.NullInt64
	var createdAt sql.NullString
	var updatedAt sql.NullString
	var metaJSONText sql.NullString
	var code sql.NullString
	var batchID sql.NullString
	var weldID sql.NullString
	var findingsCount sql.NullInt64
	item := &domain.NDTReport{Meta: map[string]any{}}
	err := scanner.Scan(&id, &status, &version, &createdAt, &updatedAt, &metaJSONText, &code, &batchID, &weldID, &findingsCount)
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
	if batchID.Valid {
		item.BatchID = batchID.String
	}
	if weldID.Valid {
		item.WeldID = weldID.String
	}
	if findingsCount.Valid {
		item.FindingsCount = findingsCount.Int64
	}
	if item.Meta == nil {
		item.Meta = map[string]any{}
	}
	return item, nil
}
