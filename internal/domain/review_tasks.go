package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type ReviewTask struct {
	ID        string         `json:"id"`
	Status    string         `json:"status"`
	Version   int64          `json:"version"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Meta      map[string]any `json:"meta,omitempty"`
	ReportID  string         `json:"report_id"`
	WeldID    string         `json:"weld_id"`
	Reviewer  string         `json:"reviewer,omitempty"`
}

const (
	ReviewTaskStatusPending    = "pending"
	ReviewTaskStatusInProgress = "in_progress"
	ReviewTaskStatusApproved   = "approved"
	ReviewTaskStatusRejected   = "rejected"
	ReviewTaskStatusArchived   = "archived"
)

var ReviewTaskTransitions = map[string]map[string]bool{
	ReviewTaskStatusPending:    {ReviewTaskStatusInProgress: true},
	ReviewTaskStatusInProgress: {ReviewTaskStatusApproved: true, ReviewTaskStatusRejected: true},
	ReviewTaskStatusApproved:   {ReviewTaskStatusArchived: true},
	ReviewTaskStatusRejected:   {ReviewTaskStatusPending: true},
	ReviewTaskStatusArchived:   {},
}

func NewReviewTask(id string, now time.Time) *ReviewTask {
	return &ReviewTask{ID: id, Status: ReviewTaskStatusPending, Version: 1, CreatedAt: now, UpdatedAt: now, Meta: map[string]any{}}
}

func (e *ReviewTask) EnsureMeta() {
	if e.Meta == nil {
		e.Meta = map[string]any{}
	}
}

func (e *ReviewTask) Normalize() {
	e.ID = strings.TrimSpace(e.ID)
	e.Status = strings.TrimSpace(e.Status)
	e.ReportID = strings.TrimSpace(e.ReportID)
	e.WeldID = strings.TrimSpace(e.WeldID)
	e.Reviewer = strings.TrimSpace(e.Reviewer)
	e.EnsureMeta()
}

func (e *ReviewTask) Validate() error {
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
	if strings.TrimSpace(e.ReportID) == "" {
		return fmt.Errorf("%w: ReportID required", ErrValidation)
	}
	if strings.TrimSpace(e.WeldID) == "" {
		return fmt.Errorf("%w: WeldID required", ErrValidation)
	}
	return nil
}

func (e *ReviewTask) ValidStatus() bool {
	switch e.Status {
	case ReviewTaskStatusPending:
	case ReviewTaskStatusInProgress:
	case ReviewTaskStatusApproved:
	case ReviewTaskStatusRejected:
	case ReviewTaskStatusArchived:
	default:
		return false
	}
	return true
}

func (e *ReviewTask) CanTransition(to string) bool {
	if !e.ValidStatus() {
		return false
	}
	if _, ok := ReviewTaskTransitions[e.Status]; !ok {
		return false
	}
	return ReviewTaskTransitions[e.Status][to]
}

func (e *ReviewTask) Transition(to string, now time.Time) error {
	e.EnsureMeta()
	e.Meta["previous_status"] = e.Status
	e.Status = to
	e.UpdatedAt = now
	return nil
}

func (e *ReviewTask) Clone() *ReviewTask {
	data, _ := json.Marshal(e)
	var out ReviewTask
	_ = json.Unmarshal(data, &out)
	return &out
}

func (e *ReviewTask) ToMap() map[string]any {
	data, _ := json.Marshal(e)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}

func (e *ReviewTask) FromMap(in map[string]any) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, e)
}

func (e *ReviewTask) Patch(in map[string]any) error {
	current := e.ToMap()
	for k, v := range in {
		current[k] = v
	}
	return e.FromMap(current)
}

func (e *ReviewTask) Summary() map[string]any {
	return map[string]any{
		"report_id": e.ReportID,
		"weld_id":   e.WeldID,
		"reviewer":  e.Reviewer,
		"status":    e.Status,
	}
}

func (e *ReviewTask) Search(term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return true
	}
	if strings.Contains(strings.ToLower(e.ReportID), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.WeldID), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Reviewer), term) {
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

func (e *ReviewTask) StableSortKey() string {
	return fmt.Sprintf("%s|%020d|%s", e.CreatedAt.UTC().Format(time.RFC3339Nano), e.Version, e.ID)
}

func (e *ReviewTask) RiskScore() float64 {
	score := float64(len(e.Meta)) + float64(e.Version)
	if strings.Contains(e.Status, "repair") {
		score += 10
	}
	if strings.Contains(e.Status, "archived") {
		score += 2
	}
	return score
}

func (e *ReviewTask) MetaString(key string) string {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

func (e *ReviewTask) MetaInt(key string) int64 {
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

func (e *ReviewTask) MetaBool(key string) bool {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func (e *ReviewTask) MetaTime(key string) (time.Time, bool) {
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

func (e *ReviewTask) MetaKeys() []string {
	e.EnsureMeta()
	keys := make([]string, 0, len(e.Meta))
	for k := range e.Meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
