package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type AuditRecord struct {
	ID         string         `json:"id"`
	Status     string         `json:"status"`
	Version    int64          `json:"version"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Meta       map[string]any `json:"meta,omitempty"`
	Entity     string         `json:"entity"`
	EntityID   string         `json:"entity_id"`
	Action     string         `json:"action"`
	Actor      string         `json:"actor"`
	BeforeJSON string         `json:"before_json,omitempty"`
	AfterJSON  string         `json:"after_json,omitempty"`
}

const (
	AuditRecordStatusRecorded = "recorded"
)

var AuditRecordTransitions = map[string]map[string]bool{
	AuditRecordStatusRecorded: {},
}

func NewAuditRecord(id string, now time.Time) *AuditRecord {
	return &AuditRecord{ID: id, Status: AuditRecordStatusRecorded, Version: 1, CreatedAt: now, UpdatedAt: now, Meta: map[string]any{}}
}

func (e *AuditRecord) EnsureMeta() {
	if e.Meta == nil {
		e.Meta = map[string]any{}
	}
}

func (e *AuditRecord) Normalize() {
	e.ID = strings.TrimSpace(e.ID)
	e.Status = strings.TrimSpace(e.Status)
	e.Entity = strings.TrimSpace(e.Entity)
	e.EntityID = strings.TrimSpace(e.EntityID)
	e.Action = strings.TrimSpace(e.Action)
	e.Actor = strings.TrimSpace(e.Actor)
	e.BeforeJSON = strings.TrimSpace(e.BeforeJSON)
	e.AfterJSON = strings.TrimSpace(e.AfterJSON)
	e.EnsureMeta()
}

func (e *AuditRecord) Validate() error {
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
	if strings.TrimSpace(e.Entity) == "" {
		return fmt.Errorf("%w: Entity required", ErrValidation)
	}
	if strings.TrimSpace(e.EntityID) == "" {
		return fmt.Errorf("%w: EntityID required", ErrValidation)
	}
	if strings.TrimSpace(e.Action) == "" {
		return fmt.Errorf("%w: Action required", ErrValidation)
	}
	if strings.TrimSpace(e.Actor) == "" {
		return fmt.Errorf("%w: Actor required", ErrValidation)
	}
	return nil
}

func (e *AuditRecord) ValidStatus() bool {
	switch e.Status {
	case AuditRecordStatusRecorded:
	default:
		return false
	}
	return true
}

func (e *AuditRecord) CanTransition(to string) bool {
	if !e.ValidStatus() {
		return false
	}
	if _, ok := AuditRecordTransitions[e.Status]; !ok {
		return false
	}
	return AuditRecordTransitions[e.Status][to]
}

func (e *AuditRecord) Transition(to string, now time.Time) error {
	if !e.CanTransition(to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, e.Status, to)
	}
	e.Status = to
	e.UpdatedAt = now
	return nil
}

func (e *AuditRecord) Clone() *AuditRecord {
	data, _ := json.Marshal(e)
	var out AuditRecord
	_ = json.Unmarshal(data, &out)
	return &out
}

func (e *AuditRecord) ToMap() map[string]any {
	data, _ := json.Marshal(e)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}

func (e *AuditRecord) FromMap(in map[string]any) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, e)
}

func (e *AuditRecord) Patch(in map[string]any) error {
	current := e.ToMap()
	for k, v := range in {
		current[k] = v
	}
	return e.FromMap(current)
}

func (e *AuditRecord) Summary() map[string]any {
	return map[string]any{
		"entity":    e.Entity,
		"entity_id": e.EntityID,
		"action":    e.Action,
		"status":    e.Status,
	}
}

func (e *AuditRecord) Search(term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return true
	}
	if strings.Contains(strings.ToLower(e.Entity), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.EntityID), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Action), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Actor), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.BeforeJSON), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.AfterJSON), term) {
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

func (e *AuditRecord) StableSortKey() string {
	return fmt.Sprintf("%s|%020d|%s", e.CreatedAt.UTC().Format(time.RFC3339Nano), e.Version, e.ID)
}

func (e *AuditRecord) RiskScore() float64 {
	score := float64(len(e.Meta)) + float64(e.Version)
	if strings.Contains(e.Status, "repair") {
		score += 10
	}
	if strings.Contains(e.Status, "archived") {
		score += 2
	}
	return score
}

func (e *AuditRecord) MetaString(key string) string {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

func (e *AuditRecord) MetaInt(key string) int64 {
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

func (e *AuditRecord) MetaBool(key string) bool {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func (e *AuditRecord) MetaTime(key string) (time.Time, bool) {
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

func (e *AuditRecord) MetaKeys() []string {
	e.EnsureMeta()
	keys := make([]string, 0, len(e.Meta))
	for k := range e.Meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
