package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Weld struct {
	ID              string         `json:"id"`
	Status          string         `json:"status"`
	Version         int64          `json:"version"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	Meta            map[string]any `json:"meta,omitempty"`
	Number          string         `json:"number"`
	EquipmentID     string         `json:"equipment_id"`
	MethodVersionID string         `json:"method_version_id"`
	BatchID         string         `json:"batch_id,omitempty"`
}

const (
	WeldStatusCreated               = "created"
	WeldStatusScheduled             = "scheduled"
	WeldStatusInProgress            = "in_progress"
	WeldStatusCompleted             = "completed"
	WeldStatusRepairRequired        = "repair_required"
	WeldStatusReinspectionScheduled = "reinspection_scheduled"
	WeldStatusArchived              = "archived"
)

var WeldTransitions = map[string]map[string]bool{
	WeldStatusCreated:               {WeldStatusScheduled: true},
	WeldStatusScheduled:             {WeldStatusInProgress: true},
	WeldStatusInProgress:            {WeldStatusCompleted: true, WeldStatusRepairRequired: true},
	WeldStatusCompleted:             {WeldStatusRepairRequired: true, WeldStatusArchived: true},
	WeldStatusRepairRequired:        {WeldStatusReinspectionScheduled: true},
	WeldStatusReinspectionScheduled: {WeldStatusInProgress: true},
	WeldStatusArchived:              {},
}

func NewWeld(id string, now time.Time) *Weld {
	return &Weld{ID: id, Status: WeldStatusCreated, Version: 1, CreatedAt: now, UpdatedAt: now, Meta: map[string]any{}}
}

func (e *Weld) EnsureMeta() {
	if e.Meta == nil {
		e.Meta = map[string]any{}
	}
}

func (e *Weld) Normalize() {
	e.ID = strings.TrimSpace(e.ID)
	e.Status = strings.TrimSpace(e.Status)
	e.Number = strings.TrimSpace(e.Number)
	e.EquipmentID = strings.TrimSpace(e.EquipmentID)
	e.MethodVersionID = strings.TrimSpace(e.MethodVersionID)
	e.BatchID = strings.TrimSpace(e.BatchID)
	e.EnsureMeta()
}

func (e *Weld) Validate() error {
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
	if strings.TrimSpace(e.Number) == "" {
		return fmt.Errorf("%w: Number required", ErrValidation)
	}
	if strings.TrimSpace(e.EquipmentID) == "" {
		return fmt.Errorf("%w: EquipmentID required", ErrValidation)
	}
	if strings.TrimSpace(e.MethodVersionID) == "" {
		return fmt.Errorf("%w: MethodVersionID required", ErrValidation)
	}
	return nil
}

func (e *Weld) ValidStatus() bool {
	switch e.Status {
	case WeldStatusCreated:
	case WeldStatusScheduled:
	case WeldStatusInProgress:
	case WeldStatusCompleted:
	case WeldStatusRepairRequired:
	case WeldStatusReinspectionScheduled:
	case WeldStatusArchived:
	default:
		return false
	}
	return true
}

func (e *Weld) CanTransition(to string) bool {
	if !e.ValidStatus() {
		return false
	}
	if _, ok := WeldTransitions[e.Status]; !ok {
		return false
	}
	return WeldTransitions[e.Status][to]
}

func (e *Weld) Transition(to string, now time.Time) error {
	if !e.CanTransition(to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, e.Status, to)
	}
	e.Status = to
	e.UpdatedAt = now
	return nil
}

func (e *Weld) Clone() *Weld {
	data, _ := json.Marshal(e)
	var out Weld
	_ = json.Unmarshal(data, &out)
	return &out
}

func (e *Weld) ToMap() map[string]any {
	data, _ := json.Marshal(e)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}

func (e *Weld) FromMap(in map[string]any) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, e)
}

func (e *Weld) Patch(in map[string]any) error {
	current := e.ToMap()
	for k, v := range in {
		current[k] = v
	}
	return e.FromMap(current)
}

func (e *Weld) Summary() map[string]any {
	return map[string]any{
		"number":            e.Number,
		"equipment_id":      e.EquipmentID,
		"method_version_id": e.MethodVersionID,
		"status":            e.Status,
	}
}

func (e *Weld) Search(term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return true
	}
	if strings.Contains(strings.ToLower(e.Number), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.EquipmentID), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.MethodVersionID), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.BatchID), term) {
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

func (e *Weld) StableSortKey() string {
	return fmt.Sprintf("%s|%020d|%s", e.CreatedAt.UTC().Format(time.RFC3339Nano), e.Version, e.ID)
}

func (e *Weld) RiskScore() float64 {
	score := float64(len(e.Meta)) + float64(e.Version)
	if strings.Contains(e.Status, "repair") {
		score += 10
	}
	if strings.Contains(e.Status, "archived") {
		score += 2
	}
	return score
}

func (e *Weld) MetaString(key string) string {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

func (e *Weld) MetaInt(key string) int64 {
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

func (e *Weld) MetaBool(key string) bool {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func (e *Weld) MetaTime(key string) (time.Time, bool) {
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

func (e *Weld) MetaKeys() []string {
	e.EnsureMeta()
	keys := make([]string, 0, len(e.Meta))
	for k := range e.Meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
