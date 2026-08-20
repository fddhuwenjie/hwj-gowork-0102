package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type CalibrationCertificate struct {
	ID            string         `json:"id"`
	Status        string         `json:"status"`
	Version       int64          `json:"version"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	Meta          map[string]any `json:"meta,omitempty"`
	EquipmentID   string         `json:"equipment_id"`
	CertificateNo string         `json:"certificate_no"`
	IssuedAt      time.Time      `json:"issued_at"`
	ExpiresAt     time.Time      `json:"expires_at"`
}

const (
	CalibrationCertificateStatusValid   = "valid"
	CalibrationCertificateStatusExpired = "expired"
	CalibrationCertificateStatusRevoked = "revoked"
)

var CalibrationCertificateTransitions = map[string]map[string]bool{
	CalibrationCertificateStatusValid:   {CalibrationCertificateStatusExpired: true, CalibrationCertificateStatusRevoked: true},
	CalibrationCertificateStatusExpired: {CalibrationCertificateStatusRevoked: true},
	CalibrationCertificateStatusRevoked: {},
}

func NewCalibrationCertificate(id string, now time.Time) *CalibrationCertificate {
	return &CalibrationCertificate{ID: id, Status: CalibrationCertificateStatusValid, Version: 1, CreatedAt: now, UpdatedAt: now, Meta: map[string]any{}}
}

func (e *CalibrationCertificate) EnsureMeta() {
	if e.Meta == nil {
		e.Meta = map[string]any{}
	}
}

func (e *CalibrationCertificate) Normalize() {
	e.ID = strings.TrimSpace(e.ID)
	e.Status = strings.TrimSpace(e.Status)
	e.EquipmentID = strings.TrimSpace(e.EquipmentID)
	e.CertificateNo = strings.TrimSpace(e.CertificateNo)
	e.EnsureMeta()
}

func (e *CalibrationCertificate) Validate() error {
	if strings.TrimSpace(e.ID) == "" {
		return fmt.Errorf("%w: id required", ErrValidation)
	}
	if !e.ValidStatus() {
		return fmt.Errorf("%w: invalid status %s", ErrValidation, e.Status)
	}
	if e.Version < 1 {
		return fmt.Errorf("%w: version must be positive", ErrValidation)
	}
	if e.CreatedAt.IsZero() {
		return fmt.Errorf("%w: created_at required", ErrValidation)
	}
	if e.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: updated_at required", ErrValidation)
	}
	if e.Meta == nil {
		e.Meta = map[string]any{}
	}
	if strings.TrimSpace(e.EquipmentID) == "" {
		return fmt.Errorf("%w: EquipmentID required", ErrValidation)
	}
	if strings.TrimSpace(e.CertificateNo) == "" {
		return fmt.Errorf("%w: CertificateNo required", ErrValidation)
	}
	if e.IssuedAt.IsZero() {
		return fmt.Errorf("%w: IssuedAt required", ErrValidation)
	}
	if e.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: ExpiresAt required", ErrValidation)
	}
	return nil
}

func (e *CalibrationCertificate) ValidStatus() bool {
	switch e.Status {
	case CalibrationCertificateStatusValid:
	case CalibrationCertificateStatusExpired:
	case CalibrationCertificateStatusRevoked:
	default:
		return false
	}
	return true
}

func (e *CalibrationCertificate) CanTransition(to string) bool {
	if !e.ValidStatus() {
		return false
	}
	if _, ok := CalibrationCertificateTransitions[e.Status]; !ok {
		return false
	}
	return CalibrationCertificateTransitions[e.Status][to]
}

func (e *CalibrationCertificate) Transition(to string, now time.Time) error {
	if !e.CanTransition(to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, e.Status, to)
	}
	e.Status = to
	e.UpdatedAt = now
	return nil
}

func (e *CalibrationCertificate) Clone() *CalibrationCertificate {
	data, _ := json.Marshal(e)
	var out CalibrationCertificate
	_ = json.Unmarshal(data, &out)
	return &out
}

func (e *CalibrationCertificate) ToMap() map[string]any {
	data, _ := json.Marshal(e)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}

func (e *CalibrationCertificate) FromMap(in map[string]any) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, e)
}

func (e *CalibrationCertificate) Patch(in map[string]any) error {
	current := e.ToMap()
	for k, v := range in {
		current[k] = v
	}
	return e.FromMap(current)
}

func (e *CalibrationCertificate) Summary() map[string]any {
	return map[string]any{
		"equipment_id":   e.EquipmentID,
		"certificate_no": e.CertificateNo,
		"issued_at":      e.IssuedAt,
		"status":         e.Status,
	}
}

func (e *CalibrationCertificate) Search(term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return true
	}
	if strings.Contains(strings.ToLower(e.EquipmentID), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.CertificateNo), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.ID), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Status), term) {
		return true
	}
	for k, v := range e.Meta {
		if strings.Contains(strings.ToLower(k), term) || strings.Contains(strings.ToLower(fmt.Sprint(v)), term) {
			return true
		}
	}
	return false
}

func (e *CalibrationCertificate) StableSortKey() string {
	return fmt.Sprintf("%s|%020d|%s", e.CreatedAt.UTC().Format(time.RFC3339Nano), e.Version, e.ID)
}

func (e *CalibrationCertificate) RiskScore() float64 {
	score := float64(len(e.Meta)) + float64(e.Version)
	if strings.Contains(e.Status, "repair") {
		score += 10
	}
	if strings.Contains(e.Status, "archived") {
		score += 2
	}
	return score
}

func (e *CalibrationCertificate) MetaString(key string) string {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

func (e *CalibrationCertificate) MetaInt(key string) int64 {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		switch x := v.(type) {
		case int64:
			return x
		case int:
			return int64(x)
		case float64:
			return int64(x)
		}
	}
	return 0
}

func (e *CalibrationCertificate) MetaBool(key string) bool {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func (e *CalibrationCertificate) MetaTime(key string) (time.Time, bool) {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		switch x := v.(type) {
		case time.Time:
			return x, true
		case string:
			if t, err := time.Parse(time.RFC3339Nano, x); err == nil {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

func (e *CalibrationCertificate) MetaKeys() []string {
	e.EnsureMeta()
	keys := make([]string, 0, len(e.Meta))
	for k := range e.Meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
