package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Equipment struct {
	ID        string         `json:"id"`
	Status    string         `json:"status"`
	Version   int64          `json:"version"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Meta      map[string]any `json:"meta,omitempty"`
	Code      string         `json:"code"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
}

const (
	EquipmentStatusActive      = "active"
	EquipmentStatusMaintenance = "maintenance"
	EquipmentStatusRetired     = "retired"
)

var EquipmentTransitions = map[string]map[string]bool{
	EquipmentStatusActive:      {EquipmentStatusMaintenance: true, EquipmentStatusRetired: true},
	EquipmentStatusMaintenance: {EquipmentStatusActive: true, EquipmentStatusRetired: true},
	EquipmentStatusRetired:     {},
}

func NewEquipment(id string, now time.Time) *Equipment {
	return &Equipment{ID: id, Status: EquipmentStatusActive, Version: 1, CreatedAt: now, UpdatedAt: now, Meta: map[string]any{}}
}

func (e *Equipment) EnsureMeta() {
	if e.Meta == nil {
		e.Meta = map[string]any{}
	}
}

func (e *Equipment) Normalize() {
	e.ID = strings.TrimSpace(e.ID)
	e.Status = strings.TrimSpace(e.Status)
	e.Code = strings.TrimSpace(e.Code)
	e.Name = strings.TrimSpace(e.Name)
	e.Type = strings.TrimSpace(e.Type)
	e.EnsureMeta()
}

func (e *Equipment) Validate() error {
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
	if strings.TrimSpace(e.Code) == "" {
		return fmt.Errorf("%w: Code required", ErrValidation)
	}
	if strings.TrimSpace(e.Name) == "" {
		return fmt.Errorf("%w: Name required", ErrValidation)
	}
	if strings.TrimSpace(e.Type) == "" {
		return fmt.Errorf("%w: Type required", ErrValidation)
	}
	return nil
}

func (e *Equipment) ValidStatus() bool {
	switch e.Status {
	case EquipmentStatusActive:
	case EquipmentStatusMaintenance:
	case EquipmentStatusRetired:
	default:
		return false
	}
	return true
}

func (e *Equipment) CanTransition(to string) bool {
	if !e.ValidStatus() {
		return false
	}
	if _, ok := EquipmentTransitions[e.Status]; !ok {
		return false
	}
	return EquipmentTransitions[e.Status][to]
}

func (e *Equipment) Transition(to string, now time.Time) error {
	if !e.CanTransition(to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, e.Status, to)
	}
	e.Status = to
	e.UpdatedAt = now
	return nil
}

func (e *Equipment) Clone() *Equipment {
	data, _ := json.Marshal(e)
	var out Equipment
	_ = json.Unmarshal(data, &out)
	return &out
}

func (e *Equipment) ToMap() map[string]any {
	data, _ := json.Marshal(e)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}

func (e *Equipment) FromMap(in map[string]any) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, e)
}

func (e *Equipment) Patch(in map[string]any) error {
	current := e.ToMap()
	for k, v := range in {
		current[k] = v
	}
	return e.FromMap(current)
}

func (e *Equipment) Summary() map[string]any {
	return map[string]any{
		"code":   e.Code,
		"name":   e.Name,
		"type":   e.Type,
		"status": e.Status,
	}
}

func (e *Equipment) Search(term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return true
	}
	if strings.Contains(strings.ToLower(e.Code), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Name), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Type), term) {
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

func (e *Equipment) StableSortKey() string {
	return fmt.Sprintf("%s|%020d|%s", e.CreatedAt.UTC().Format(time.RFC3339Nano), e.Version, e.ID)
}

func (e *Equipment) RiskScore() float64 {
	score := float64(len(e.Meta)) + float64(e.Version)
	if strings.Contains(e.Status, "repair") {
		score += 10
	}
	if strings.Contains(e.Status, "archived") {
		score += 2
	}
	return score
}

func (e *Equipment) MetaString(key string) string {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

func (e *Equipment) MetaInt(key string) int64 {
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

func (e *Equipment) MetaBool(key string) bool {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func (e *Equipment) MetaTime(key string) (time.Time, bool) {
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

func (e *Equipment) MetaKeys() []string {
	e.EnsureMeta()
	keys := make([]string, 0, len(e.Meta))
	for k := range e.Meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
