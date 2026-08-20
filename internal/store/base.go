package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"weld-ndt/internal/domain"

	_ "modernc.org/sqlite"
)

type Page = domain.Page

type Queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type Store struct {
	DB                      *sql.DB
	Weld                    WeldRepository
	Equipment               EquipmentRepository
	CalibrationCertificate  CalibrationCertificateRepository
	MethodVersion           MethodVersionRepository
	ExecutionBatch          ExecutionBatchRepository
	NDTReport               NDTReportRepository
	DiscontinuityIndication DiscontinuityIndicationRepository
	AnomalyEvent            AnomalyEventRepository
	RepairOrder             RepairOrderRepository
	ReviewTask              ReviewTaskRepository
	AuditRecord             AuditRecordRepository
	BackgroundTask          BackgroundTaskRepository
	Idempotency             IdempotencyRepository
}

func NewStore(db *sql.DB) *Store {
	return &Store{
		DB:                      db,
		Weld:                    NewWeldRepository(),
		Equipment:               NewEquipmentRepository(),
		CalibrationCertificate:  NewCalibrationCertificateRepository(),
		MethodVersion:           NewMethodVersionRepository(),
		ExecutionBatch:          NewExecutionBatchRepository(),
		NDTReport:               NewNDTReportRepository(),
		DiscontinuityIndication: NewDiscontinuityIndicationRepository(),
		AnomalyEvent:            NewAnomalyEventRepository(),
		RepairOrder:             NewRepairOrderRepository(),
		ReviewTask:              NewReviewTaskRepository(),
		AuditRecord:             NewAuditRecordRepository(),
		BackgroundTask:          NewBackgroundTaskRepository(),
		Idempotency:             IdempotencyRepository{},
	}
}

func (s *Store) WithTx(ctx context.Context, fn func(Queryer) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return err
	}
	return nil
}

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func (s *Store) Close() error {
	return s.DB.Close()
}

func Migrate(ctx context.Context, db *sql.DB) error {
	for _, stmt := range []string{
		`PRAGMA foreign_keys = ON`,

		`CREATE TABLE IF NOT EXISTS welds (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    meta_json TEXT NOT NULL,
    number TEXT,
    equipment_id TEXT,
    method_version_id TEXT,
    batch_id TEXT NULL
)`,

		`CREATE TABLE IF NOT EXISTS equipment (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    meta_json TEXT NOT NULL,
    code TEXT,
    name TEXT,
    type TEXT
)`,

		`CREATE TABLE IF NOT EXISTS calibration_certificates (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    meta_json TEXT NOT NULL,
    equipment_id TEXT,
    certificate_no TEXT,
    issued_at TEXT,
    expires_at TEXT
)`,

		`CREATE TABLE IF NOT EXISTS method_versions (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    meta_json TEXT NOT NULL,
    code TEXT,
    version_no INTEGER,
    standard TEXT
)`,

		`CREATE TABLE IF NOT EXISTS execution_batches (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    meta_json TEXT NOT NULL,
    code TEXT,
    equipment_id TEXT,
    method_version_id TEXT,
    started_at TEXT NULL,
    completed_at TEXT NULL
)`,

		`CREATE TABLE IF NOT EXISTS ndt_reports (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    meta_json TEXT NOT NULL,
    code TEXT,
    batch_id TEXT,
    weld_id TEXT,
    findings_count INTEGER
)`,

		`CREATE TABLE IF NOT EXISTS discontinuity_indications (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    meta_json TEXT NOT NULL,
    report_id TEXT,
    weld_id TEXT,
    type TEXT,
    severity TEXT,
    location TEXT
)`,

		`CREATE TABLE IF NOT EXISTS anomaly_events (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    meta_json TEXT NOT NULL,
    weld_id TEXT,
    batch_id TEXT,
    type TEXT,
    severity TEXT,
    root_cause TEXT NULL
)`,

		`CREATE TABLE IF NOT EXISTS repair_orders (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    meta_json TEXT NOT NULL,
    weld_id TEXT,
    anomaly_id TEXT,
    round INTEGER,
    required_method_version_id TEXT
)`,

		`CREATE TABLE IF NOT EXISTS review_tasks (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    meta_json TEXT NOT NULL,
    report_id TEXT,
    weld_id TEXT,
    reviewer TEXT NULL
)`,

		`CREATE TABLE IF NOT EXISTS audit_records (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    meta_json TEXT NOT NULL,
    entity TEXT,
    entity_id TEXT,
    action TEXT,
    actor TEXT,
    before_json TEXT NULL,
    after_json TEXT NULL
)`,

		`CREATE TABLE IF NOT EXISTS background_tasks (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    meta_json TEXT NOT NULL,
    task_type TEXT,
    payload_json TEXT NULL,
    attempts INTEGER,
    max_attempts INTEGER,
    next_run_at TEXT NULL,
    last_error TEXT NULL
)`,

		`CREATE TABLE IF NOT EXISTS idempotency_keys (key TEXT PRIMARY KEY, response_json TEXT NOT NULL, created_at TEXT NOT NULL)`,

		`CREATE INDEX IF NOT EXISTS idx_idempotency_created_at ON idempotency_keys(created_at)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func timeText(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func timeTextPtr(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return timeText(t)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

func metaJSON(m map[string]any) string {
	if m == nil {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func mustAny(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func BuildFilter(filter map[string]any, allowed []string) (string, []any) {
	where := []string{"1 = 1"}
	args := make([]any, 0, len(filter))
	allow := map[string]bool{}
	for _, k := range allowed {
		allow[k] = true
	}
	for k, v := range filter {
		if !allow[k] {
			continue
		}
		where = append(where, k+" = ?")
		args = append(args, v)
	}
	return strings.Join(where, " AND "), args
}

func BuildOrder(sort string) string {
	s := strings.TrimSpace(strings.ToLower(sort))
	switch s {
	case "created_at asc":
		return "created_at ASC, id ASC"
	case "created_at desc":
		return "created_at DESC, id ASC"
	case "updated_at asc":
		return "updated_at ASC, id ASC"
	case "updated_at desc":
		return "updated_at DESC, id ASC"
	case "status asc":
		return "status ASC, id ASC"
	case "status desc":
		return "status DESC, id ASC"
	default:
		return "created_at DESC, id ASC"
	}
}

func scanText(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func scanInt64(ni sql.NullInt64) int64 {
	if ni.Valid {
		return ni.Int64
	}
	return 0
}

func scanBool(nb sql.NullBool) bool {
	return nb.Valid && nb.Bool
}

func scanTime(ns sql.NullString) time.Time {
	if !ns.Valid || ns.String == "" {
		return time.Time{}
	}
	t, _ := parseTime(ns.String)
	return t
}
