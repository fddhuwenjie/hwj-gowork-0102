package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"weld-ndt/internal/domain"
)

type CalibrationCertificateRepository struct{}

func NewCalibrationCertificateRepository() CalibrationCertificateRepository {
	return CalibrationCertificateRepository{}
}

func (r CalibrationCertificateRepository) Create(ctx context.Context, q Queryer, item *domain.CalibrationCertificate) error {
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
	_, err := q.ExecContext(ctx, "INSERT INTO calibration_certificates (id, status, version, created_at, updated_at, meta_json, equipment_id, certificate_no, issued_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		item.ID,
		item.Status,
		item.Version,
		timeText(item.CreatedAt),
		timeText(item.UpdatedAt),
		metaJSON(item.Meta),
		item.EquipmentID,
		item.CertificateNo,
		timeText(item.IssuedAt),
		timeText(item.ExpiresAt),
	)
	return err
}

func (r CalibrationCertificateRepository) Get(ctx context.Context, q Queryer, id string) (*domain.CalibrationCertificate, error) {
	row := q.QueryRowContext(ctx, "SELECT id, status, version, created_at, updated_at, meta_json, equipment_id, certificate_no, issued_at, expires_at FROM calibration_certificates WHERE id = ?", id)
	item, err := scanCalibrationCertificate(row)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r CalibrationCertificateRepository) Update(ctx context.Context, q Queryer, item *domain.CalibrationCertificate, expectedVersion int64) error {
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
	args = append(args, item.EquipmentID)
	args = append(args, item.CertificateNo)
	args = append(args, timeText(item.IssuedAt))
	args = append(args, timeText(item.ExpiresAt))
	args = append(args, item.ID, expectedVersion)
	res, err := q.ExecContext(ctx, "UPDATE calibration_certificates SET status = ?, version = ?, created_at = ?, updated_at = ?, meta_json = ?, equipment_id = ?, certificate_no = ?, issued_at = ?, expires_at = ? WHERE id = ? AND version = ?", args...)
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

func (r CalibrationCertificateRepository) Delete(ctx context.Context, q Queryer, id string, version int64) error {
	res, err := q.ExecContext(ctx, "DELETE FROM calibration_certificates WHERE id = ? AND version = ?", id, version)
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

func (r CalibrationCertificateRepository) List(ctx context.Context, q Queryer, filter map[string]any, page Page, sort string) ([]*domain.CalibrationCertificate, int64, error) {
	page = page.Normalize()
	where := []string{"1 = 1"}
	args := []any{}
	for k, v := range filter {
		switch k {
		case "equipment_id":
			where = append(where, "equipment_id = ?")
			args = append(args, v)
		case "certificate_no":
			where = append(where, "certificate_no = ?")
			args = append(args, v)
		case "issued_at":
			where = append(where, "issued_at = ?")
			args = append(args, v)
		case "expires_at":
			where = append(where, "expires_at = ?")
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
	countSQL := "SELECT COUNT(*) FROM calibration_certificates WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := q.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := "SELECT id, status, version, created_at, updated_at, meta_json, equipment_id, certificate_no, issued_at, expires_at FROM calibration_certificates WHERE " + strings.Join(where, " AND ") + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	args = append(args, page.Size, page.Offset())
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]*domain.CalibrationCertificate, 0, page.Size)
	for rows.Next() {
		item, err := scanCalibrationCertificate(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func scanCalibrationCertificate(scanner interface{ Scan(dest ...any) error }) (*domain.CalibrationCertificate, error) {
	var id sql.NullString
	var status sql.NullString
	var version sql.NullInt64
	var createdAt sql.NullString
	var updatedAt sql.NullString
	var metaJSONText sql.NullString
	var equipmentID sql.NullString
	var certificateNo sql.NullString
	var issuedAt sql.NullString
	var expiresAt sql.NullString
	item := &domain.CalibrationCertificate{Meta: map[string]any{}}
	err := scanner.Scan(&id, &status, &version, &createdAt, &updatedAt, &metaJSONText, &equipmentID, &certificateNo, &issuedAt, &expiresAt)
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
	if equipmentID.Valid {
		item.EquipmentID = equipmentID.String
	}
	if certificateNo.Valid {
		item.CertificateNo = certificateNo.String
	}
	if issuedAt.Valid {
		if t, err := parseTime(issuedAt.String); err == nil {
			item.IssuedAt = t
		}
	}
	if expiresAt.Valid {
		if t, err := parseTime(expiresAt.String); err == nil {
			item.ExpiresAt = t
		}
	}
	if item.Meta == nil {
		item.Meta = map[string]any{}
	}
	return item, nil
}
