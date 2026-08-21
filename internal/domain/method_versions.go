package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type MethodVersion struct {
	ID        string         `json:"id"`
	Status    string         `json:"status"`
	Version   int64          `json:"version"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Meta      map[string]any `json:"meta,omitempty"`
	Code      string         `json:"code"`
	VersionNo int64          `json:"version_no"`
	Standard  string         `json:"standard"`
}

const (
	MethodVersionStatusDraft      = "draft"
	MethodVersionStatusActive     = "active"
	MethodVersionStatusDeprecated = "deprecated"
)

// MethodVersionTransitions encodes the allowed adjacent transitions of the
// method-version state machine. Only directly adjacent states may be reached
// in a single step: draft -> active, active -> deprecated. Reaching a
// non-adjacent terminal state (e.g. draft -> deprecated) or re-entering the
// current state is rejected so that the version counter only advances for a
// legitimate adjacent transition and the previous status is preserved.
var MethodVersionTransitions = map[string]map[string]bool{
	MethodVersionStatusDraft:      {MethodVersionStatusActive: true},
	MethodVersionStatusActive:     {MethodVersionStatusDeprecated: true},
	MethodVersionStatusDeprecated: {},
}

func NewMethodVersion(id string, now time.Time) *MethodVersion {
	return &MethodVersion{ID: id, Status: MethodVersionStatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now, Meta: map[string]any{}}
}

func (e *MethodVersion) EnsureMeta() {
	if e.Meta == nil {
		e.Meta = map[string]any{}
	}
}

func (e *MethodVersion) Normalize() {
	e.ID = strings.TrimSpace(e.ID)
	e.Status = strings.TrimSpace(e.Status)
	e.Code = strings.TrimSpace(e.Code)
	e.Standard = strings.TrimSpace(e.Standard)
	e.EnsureMeta()
}

func (e *MethodVersion) Validate() error {
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
	if e.VersionNo <= 0 {
		return fmt.Errorf("%w: VersionNo must be positive", ErrValidation)
	}
	if strings.TrimSpace(e.Standard) == "" {
		return fmt.Errorf("%w: Standard required", ErrValidation)
	}
	return nil
}

func (e *MethodVersion) ValidStatus() bool {
	switch e.Status {
	case MethodVersionStatusDraft:
	case MethodVersionStatusActive:
	case MethodVersionStatusDeprecated:
	default:
		return false
	}
	return true
}

func (e *MethodVersion) CanTransition(to string) bool {
	if !e.ValidStatus() {
		return false
	}
	if _, ok := MethodVersionTransitions[e.Status]; !ok {
		return false
	}
	return MethodVersionTransitions[e.Status][to]
}

func (e *MethodVersion) Transition(to string, now time.Time) error {
	if !e.CanTransition(to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, e.Status, to)
	}
	e.Status = to
	e.UpdatedAt = now
	return nil
}

func (e *MethodVersion) Clone() *MethodVersion {
	data, _ := json.Marshal(e)
	var out MethodVersion
	_ = json.Unmarshal(data, &out)
	return &out
}

func (e *MethodVersion) ToMap() map[string]any {
	data, _ := json.Marshal(e)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}

func (e *MethodVersion) FromMap(in map[string]any) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, e)
}

func (e *MethodVersion) Patch(in map[string]any) error {
	current := e.ToMap()
	for k, v := range in {
		current[k] = v
	}
	return e.FromMap(current)
}

func (e *MethodVersion) Summary() map[string]any {
	return map[string]any{
		"code":       e.Code,
		"version_no": e.VersionNo,
		"standard":   e.Standard,
		"status":     e.Status,
	}
}

func (e *MethodVersion) Search(term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return true
	}
	if strings.Contains(strings.ToLower(e.Code), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Standard), term) {
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

func (e *MethodVersion) StableSortKey() string {
	return fmt.Sprintf("%s|%020d|%s", e.CreatedAt.UTC().Format(time.RFC3339Nano), e.Version, e.ID)
}

func (e *MethodVersion) RiskScore() float64 {
	score := float64(len(e.Meta)) + float64(e.Version)
	if strings.Contains(e.Status, "repair") {
		score += 10
	}
	if strings.Contains(e.Status, "archived") {
		score += 2
	}
	return score
}

func (e *MethodVersion) MetaString(key string) string {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

func (e *MethodVersion) MetaInt(key string) int64 {
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

func (e *MethodVersion) MetaBool(key string) bool {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func (e *MethodVersion) MetaTime(key string) (time.Time, bool) {
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

func (e *MethodVersion) MetaKeys() []string {
	e.EnsureMeta()
	keys := make([]string, 0, len(e.Meta))
	for k := range e.Meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
