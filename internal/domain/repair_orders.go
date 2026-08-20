package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type RepairOrder struct {
	ID                      string         `json:"id"`
	Status                  string         `json:"status"`
	Version                 int64          `json:"version"`
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`
	Meta                    map[string]any `json:"meta,omitempty"`
	WeldID                  string         `json:"weld_id"`
	AnomalyID               string         `json:"anomaly_id"`
	Round                   int64          `json:"round"`
	RequiredMethodVersionID string         `json:"required_method_version_id"`
}

const (
	RepairOrderStatusRequested  = "requested"
	RepairOrderStatusAssigned   = "assigned"
	RepairOrderStatusInProgress = "in_progress"
	RepairOrderStatusCompleted  = "completed"
	RepairOrderStatusVerified   = "verified"
)

var RepairOrderTransitions = map[string]map[string]bool{
	RepairOrderStatusRequested:  {RepairOrderStatusAssigned: true},
	RepairOrderStatusAssigned:   {RepairOrderStatusInProgress: true},
	RepairOrderStatusInProgress: {RepairOrderStatusCompleted: true},
	RepairOrderStatusCompleted:  {RepairOrderStatusVerified: true},
	RepairOrderStatusVerified:   {},
}

func NewRepairOrder(id string, now time.Time) *RepairOrder {
	return &RepairOrder{ID: id, Status: RepairOrderStatusRequested, Version: 1, CreatedAt: now, UpdatedAt: now, Meta: map[string]any{}}
}

func (e *RepairOrder) EnsureMeta() {
	if e.Meta == nil {
		e.Meta = map[string]any{}
	}
}

func (e *RepairOrder) Normalize() {
	e.ID = strings.TrimSpace(e.ID)
	e.Status = strings.TrimSpace(e.Status)
	e.WeldID = strings.TrimSpace(e.WeldID)
	e.AnomalyID = strings.TrimSpace(e.AnomalyID)
	e.RequiredMethodVersionID = strings.TrimSpace(e.RequiredMethodVersionID)
	e.EnsureMeta()
}

func (e *RepairOrder) Validate() error {
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
	if strings.TrimSpace(e.WeldID) == "" {
		return fmt.Errorf("%w: WeldID required", ErrValidation)
	}
	if strings.TrimSpace(e.AnomalyID) == "" {
		return fmt.Errorf("%w: AnomalyID required", ErrValidation)
	}
	if e.Round <= 0 {
		return fmt.Errorf("%w: Round must be positive", ErrValidation)
	}
	if strings.TrimSpace(e.RequiredMethodVersionID) == "" {
		return fmt.Errorf("%w: RequiredMethodVersionID required", ErrValidation)
	}
	return nil
}

func (e *RepairOrder) ValidStatus() bool {
	switch e.Status {
	case RepairOrderStatusRequested:
	case RepairOrderStatusAssigned:
	case RepairOrderStatusInProgress:
	case RepairOrderStatusCompleted:
	case RepairOrderStatusVerified:
	default:
		return false
	}
	return true
}

func (e *RepairOrder) CanTransition(to string) bool {
	if !e.ValidStatus() {
		return false
	}
	if _, ok := RepairOrderTransitions[e.Status]; !ok {
		return false
	}
	return RepairOrderTransitions[e.Status][to]
}

func (e *RepairOrder) Transition(to string, now time.Time) error {
	if !e.CanTransition(to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, e.Status, to)
	}
	e.Status = to
	e.UpdatedAt = now
	return nil
}

func (e *RepairOrder) Clone() *RepairOrder {
	data, _ := json.Marshal(e)
	var out RepairOrder
	_ = json.Unmarshal(data, &out)
	return &out
}

func (e *RepairOrder) ToMap() map[string]any {
	data, _ := json.Marshal(e)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}

func (e *RepairOrder) FromMap(in map[string]any) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, e)
}

func (e *RepairOrder) Patch(in map[string]any) error {
	current := e.ToMap()
	for k, v := range in {
		current[k] = v
	}
	return e.FromMap(current)
}

func (e *RepairOrder) Summary() map[string]any {
	return map[string]any{
		"weld_id":    e.WeldID,
		"anomaly_id": e.AnomalyID,
		"round":      e.Round,
		"status":     e.Status,
	}
}

func (e *RepairOrder) Search(term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return true
	}
	if strings.Contains(strings.ToLower(e.WeldID), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.AnomalyID), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.RequiredMethodVersionID), term) {
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

func (e *RepairOrder) StableSortKey() string {
	return fmt.Sprintf("%s|%020d|%s", e.CreatedAt.UTC().Format(time.RFC3339Nano), e.Version, e.ID)
}

func (e *RepairOrder) RiskScore() float64 {
	score := float64(len(e.Meta)) + float64(e.Version)
	if strings.Contains(e.Status, "repair") {
		score += 10
	}
	if strings.Contains(e.Status, "archived") {
		score += 2
	}
	return score
}

func (e *RepairOrder) MetaString(key string) string {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

func (e *RepairOrder) MetaInt(key string) int64 {
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

func (e *RepairOrder) MetaBool(key string) bool {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func (e *RepairOrder) MetaTime(key string) (time.Time, bool) {
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

func (e *RepairOrder) MetaKeys() []string {
	e.EnsureMeta()
	keys := make([]string, 0, len(e.Meta))
	for k := range e.Meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
