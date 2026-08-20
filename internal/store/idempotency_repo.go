package store

import (
	"context"
	"database/sql"
	"time"
)

type IdempotencyRepository struct{}

func (IdempotencyRepository) Get(ctx context.Context, q Queryer, key string) ([]byte, bool, error) {
	var payload sql.NullString
	err := q.QueryRowContext(ctx, "SELECT response_json FROM idempotency_keys WHERE key = ?", key).Scan(&payload)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !payload.Valid {
		return nil, true, nil
	}
	return []byte(payload.String), true, nil
}

func (IdempotencyRepository) Save(ctx context.Context, q Queryer, key string, payload []byte) (bool, error) {
	_, err := q.ExecContext(ctx, "INSERT OR REPLACE INTO idempotency_keys(key,response_json,created_at) VALUES(?,?,?)", key, string(payload), time.Now().UTC().Format(time.RFC3339Nano))
	return true, err
}
