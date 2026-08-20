package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type BackgroundTask struct {
	ID          string         `json:"id"`
	Status      string         `json:"status"`
	Version     int64          `json:"version"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	Meta        map[string]any `json:"meta,omitempty"`
	TaskType    string         `json:"task_type"`
	PayloadJSON string         `json:"payload_json,omitempty"`
	Attempts    int64          `json:"attempts"`
	MaxAttempts int64          `json:"max_attempts"`
	NextRunAt   time.Time      `json:"next_run_at,omitempty"`
	LastError   string         `json:"last_error,omitempty"`
}

const (
	BackgroundTaskStatusPending   = "pending"
	BackgroundTaskStatusRunning   = "running"
	BackgroundTaskStatusSucceeded = "succeeded"
	BackgroundTaskStatusFailed    = "failed"
	BackgroundTaskStatusCancelled = "cancelled"
)

var BackgroundTaskTransitions = map[string]map[string]bool{
	BackgroundTaskStatusPending:   {BackgroundTaskStatusRunning: true, BackgroundTaskStatusCancelled: true},
	BackgroundTaskStatusRunning:   {BackgroundTaskStatusSucceeded: true, BackgroundTaskStatusFailed: true, BackgroundTaskStatusCancelled: true},
	BackgroundTaskStatusFailed:    {BackgroundTaskStatusPending: true, BackgroundTaskStatusCancelled: true},
	BackgroundTaskStatusSucceeded: {},
	BackgroundTaskStatusCancelled: {},
}

func NewBackgroundTask(id string, now time.Time) *BackgroundTask {
	return &BackgroundTask{ID: id, Status: BackgroundTaskStatusPending, Version: 1, CreatedAt: now, UpdatedAt: now, Meta: map[string]any{}}
}

func (e *BackgroundTask) EnsureMeta() {
	if e.Meta == nil {
		e.Meta = map[string]any{}
	}
}

func (e *BackgroundTask) Normalize() {
	e.ID = strings.TrimSpace(e.ID)
	e.Status = strings.TrimSpace(e.Status)
	e.TaskType = strings.TrimSpace(e.TaskType)
	e.PayloadJSON = strings.TrimSpace(e.PayloadJSON)
	e.LastError = strings.TrimSpace(e.LastError)
	e.EnsureMeta()
}

func (e *BackgroundTask) Validate() error {
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
	if strings.TrimSpace(e.TaskType) == "" {
		return fmt.Errorf("%w: TaskType required", ErrValidation)
	}
	if e.Attempts <= 0 {
		return fmt.Errorf("%w: Attempts must be positive", ErrValidation)
	}
	if e.MaxAttempts <= 0 {
		return fmt.Errorf("%w: MaxAttempts must be positive", ErrValidation)
	}
	return nil
}

func (e *BackgroundTask) ValidStatus() bool {
	switch e.Status {
	case BackgroundTaskStatusPending:
	case BackgroundTaskStatusRunning:
	case BackgroundTaskStatusSucceeded:
	case BackgroundTaskStatusFailed:
	case BackgroundTaskStatusCancelled:
	default:
		return false
	}
	return true
}

func (e *BackgroundTask) CanTransition(to string) bool {
	if !e.ValidStatus() {
		return false
	}
	if _, ok := BackgroundTaskTransitions[e.Status]; !ok {
		return false
	}
	return BackgroundTaskTransitions[e.Status][to]
}

func (e *BackgroundTask) Transition(to string, now time.Time) error {
	if !e.CanTransition(to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, e.Status, to)
	}
	e.Status = to
	e.UpdatedAt = now
	return nil
}

func (e *BackgroundTask) Clone() *BackgroundTask {
	data, _ := json.Marshal(e)
	var out BackgroundTask
	_ = json.Unmarshal(data, &out)
	return &out
}

func (e *BackgroundTask) ToMap() map[string]any {
	data, _ := json.Marshal(e)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}

func (e *BackgroundTask) FromMap(in map[string]any) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, e)
}

func (e *BackgroundTask) Patch(in map[string]any) error {
	current := e.ToMap()
	for k, v := range in {
		current[k] = v
	}
	return e.FromMap(current)
}

func (e *BackgroundTask) Summary() map[string]any {
	return map[string]any{
		"task_type":    e.TaskType,
		"payload_json": e.PayloadJSON,
		"attempts":     e.Attempts,
		"status":       e.Status,
	}
}

func (e *BackgroundTask) Search(term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return true
	}
	if strings.Contains(strings.ToLower(e.TaskType), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.PayloadJSON), term) {
		return true
	}
	if strings.Contains(strings.ToLower(e.LastError), term) {
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

func (e *BackgroundTask) StableSortKey() string {
	return fmt.Sprintf("%s|%020d|%s", e.CreatedAt.UTC().Format(time.RFC3339Nano), e.Version, e.ID)
}

func (e *BackgroundTask) RiskScore() float64 {
	score := float64(len(e.Meta)) + float64(e.Version)
	if strings.Contains(e.Status, "repair") {
		score += 10
	}
	if strings.Contains(e.Status, "archived") {
		score += 2
	}
	return score
}

func (e *BackgroundTask) MetaString(key string) string {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

func (e *BackgroundTask) MetaInt(key string) int64 {
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

func (e *BackgroundTask) MetaBool(key string) bool {
	e.EnsureMeta()
	if v, ok := e.Meta[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func (e *BackgroundTask) MetaTime(key string) (time.Time, bool) {
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

func (e *BackgroundTask) MetaKeys() []string {
	e.EnsureMeta()
	keys := make([]string, 0, len(e.Meta))
	for k := range e.Meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
