package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type AnomalyEvent struct {
	ID        string         `json:"id"`
	Status    string         `json:"status"`
	Version   int64          `json:"version"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Meta      map[string]any `json:"meta,omitempty"`
	WeldID    string         `json:"weld_id"`
	BatchID   string         `json:"batch_id"`
	Type      string         `json:"type"`
	Severity  string         `json:"severity"`
	RootCause string         `json:"root_cause,omitempty"`
}

const (
	AnomalyEventStatusOpen          = "open"
	AnomalyEventStatusInvestigating = "investigating"
	AnomalyEventStatusRepairOrdered = "repair_ordered"
	AnomalyEventStatusResolved      = "resolved"
	AnomalyEventStatusClosed        = "closed"
)

var AnomalyEventTransitions = map[string]map[string]bool{
	AnomalyEventStatusOpen:          {AnomalyEventStatusInvestigating: true, AnomalyEventStatusRepairOrdered: true},
	AnomalyEventStatusInvestigating: {AnomalyEventStatusRepairOrdered: true, AnomalyEventStatusResolved: true},
	AnomalyEventStatusRepairOrdered: {AnomalyEventStatusResolved: true},
	AnomalyEventStatusResolved:      {AnomalyEventStatusClosed: true},
	AnomalyEventStatusClosed:        {},
}

func NewAnomalyEvent(id string, now time.Time) *AnomalyEvent {
	return &AnomalyEvent{ID: id, Status: AnomalyEventStatusOpen, Version: 1, CreatedAt: now, UpdatedAt: now, Meta: map[string]any{}}
}

func (e *AnomalyEvent) EnsureMeta() {
	if e.Meta == nil {
		e.Meta = map[string]any{}
	}
}

func (e *AnomalyEvent) Normalize() {
	e.ID = strings.TrimSpace(e.ID)
	e.Status = strings.TrimSpace(e.Status)
	e.WeldID = strings.TrimSpace(e.WeldID)
	e.BatchID = strings.TrimSpace(e.BatchID)
	e.Type = strings.TrimSpace(e.Type)
	e.Severity = strings.TrimSpace(e.Severity)
	e.RootCause = strings.TrimSpace(e.RootCause)
	e.EnsureMeta()
}

func (e *AnomalyEvent) Validate() error {
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
	if strings.TrimSpace(e.BatchID) == "" {
		return fmt.Errorf("%w: BatchID required", ErrValidation)
	}
	if strings.TrimSpace(e.Type) == "" {
		return fmt.Errorf("%w: Type required", ErrValidation)
	}
	if strings.TrimSpace(e.Severity) == "" {
		return fmt.Errorf("%w: Severity required", ErrValidation)
	}
	return nil
}

func (e *AnomalyEvent) ValidStatus() bool {
	switch e.Status {
	case AnomalyEventStatusOpen:
	case AnomalyEventStatusInvestigating:
	case AnomalyEventStatusRepairOrdered:
	case AnomalyEventStatusResolved:
	case AnomalyEventStatusClosed:
	default:
		return false
	}
	return true
}

func (e *AnomalyEvent) CanTransition(to string) bool {
	if !e.ValidStatus() {
		return false
	}
	if _, ok := AnomalyEventTransitions[e.Status]; !ok {
		return false
	}
	return AnomalyEventTransitions[e.Status][to]
}

func (e *AnomalyEvent) Transition(to string, now time.Time) error {
	if !e.CanTransition(to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, e.Status, to)
	}
	e.Status = to
	e.UpdatedAt = now
	return nil
}

func (e *AnomalyEvent) Clone() *AnomalyEvent {
	data, _ := json.Marshal(e)
	var out AnomalyEvent
	_ = json.Unmarshal(data, &out)
	return &out
}

func (e *AnomalyEvent) ToMap() map[string]any {
	data, _ := json.Marshal(e)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}

func (e *AnomalyEvent) FromMap(in map[string]any) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, e)
}

func (e *AnomalyEvent) Patch(in map[string]any) error {
	current := e.ToMap()
	for k, v := range in {
		current[k] = v
	}
	return e.FromMap(current)
}

func (e *AnomalyEvent) Summary() map[string]any {
	return map[string]any{
		"weld_id":  e.WeldID,
		"batch_id": e.BatchID,
		"type":     e.Type,
		"status":   e.Status,
	}
}

func (e *AnomalyEvent) Search(term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return true
	}
	if strings.Contains(strings.ToLower(e.WeldID), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.BatchID), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Type), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Severity), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.RootCause), term) {
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

func (e *AnomalyEvent) StableSortKey() string {
	return fmt.Sprintf("%s|%020d|%s", e.CreatedAt.UTC().Format(time.RFC3339Nano), e.Version, e.ID)
}

func (e *AnomalyEvent) RiskScore() float64 {
	score := float64(len(e.Meta)) + float64(e.Version)
	if strings.Contains(e.Status, "repair") {
		score += 10
	}
	if strings.Contains(e.Status, "archived") {
		score += 2
	}
	return score
}

func (e *AnomalyEvent) MetaString(key string) string {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

func (e *AnomalyEvent) MetaInt(key string) int64 {
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

func (e *AnomalyEvent) MetaBool(key string) bool {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func (e *AnomalyEvent) MetaTime(key string) (time.Time, bool) {
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

func (e *AnomalyEvent) MetaKeys() []string {
	e.EnsureMeta()
	keys := make([]string, 0, len(e.Meta))
	for k := range e.Meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
