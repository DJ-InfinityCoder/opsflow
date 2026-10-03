package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type IdempotencyState string

const (
	IdempotencyNew        IdempotencyState = "new"
	IdempotencyReplay     IdempotencyState = "replay"
	IdempotencyKeyReuse   IdempotencyState = "key_reuse"
	IdempotencyInProgress IdempotencyState = "in_progress"
)

type IdempotencyResult struct {
	State      IdempotencyState
	StatusCode int
	Response   json.RawMessage
}

type IdempotencyRecord struct {
	Endpoint    string
	RequestHash string
	StatusCode  int
	Response    json.RawMessage
	Found       bool
}

// BeginIdempotency attempts to claim or check an idempotency key inside the action's transaction.
// Sequence:
// 1. INSERT ... ON CONFLICT (user_id, key) DO NOTHING
// 2. If row was inserted (1 row affected), returns IdempotencyNew (first in flight).
// 3. If row already exists, queries the row with FOR UPDATE NOWAIT:
//    - Lock failure (55P03): concurrent duplicate in progress -> IdempotencyInProgress
//    - Hash mismatch: -> IdempotencyKeyReuse
//    - Stored response not yet written (status_code is null): -> IdempotencyInProgress
//    - Same hash and stored response: -> IdempotencyReplay with stored status and body
func BeginIdempotency(ctx context.Context, db DBTX, userID, key, endpoint, requestHash string) (IdempotencyResult, error) {
	var acquired bool
	if err := db.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1, 0))`, userID+":"+key).Scan(&acquired); err != nil {
		return IdempotencyResult{}, fmt.Errorf("try lock idempotency key: %w", err)
	}
	if !acquired {
		return IdempotencyResult{State: IdempotencyInProgress}, nil
	}

	tag, err := db.Exec(ctx, `
		INSERT INTO idempotency_keys (user_id, key, endpoint, request_hash, status_code, response_body)
		VALUES ($1::uuid, $2, $3, $4, NULL, NULL)
		ON CONFLICT (user_id, key) DO NOTHING
	`, userID, key, endpoint, requestHash)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "55P03" || pgErr.Code == "57014") {
			return IdempotencyResult{State: IdempotencyInProgress}, nil
		}
		return IdempotencyResult{}, fmt.Errorf("insert idempotency key: %w", err)
	}

	if tag.RowsAffected() == 1 {
		return IdempotencyResult{State: IdempotencyNew}, nil
	}

	var storedEndpoint, storedHash string
	var statusCode *int
	var responseBody []byte
	err = db.QueryRow(ctx, `
		SELECT endpoint, request_hash, status_code, response_body
		FROM idempotency_keys
		WHERE user_id = $1::uuid AND key = $2
		FOR UPDATE NOWAIT
	`, userID, key).Scan(&storedEndpoint, &storedHash, &statusCode, &responseBody)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "55P03" || pgErr.Code == "57014") {
			return IdempotencyResult{State: IdempotencyInProgress}, nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return IdempotencyResult{State: IdempotencyInProgress}, nil
		}
		return IdempotencyResult{}, fmt.Errorf("read existing idempotency key: %w", err)
	}

	if storedHash != requestHash {
		return IdempotencyResult{State: IdempotencyKeyReuse}, nil
	}

	if statusCode == nil || len(responseBody) == 0 {
		return IdempotencyResult{State: IdempotencyInProgress}, nil
	}

	return IdempotencyResult{
		State:      IdempotencyReplay,
		StatusCode: *statusCode,
		Response:   json.RawMessage(responseBody),
	}, nil
}

// StoreIdempotencyResponse records the final response into the idempotency_keys table.
func StoreIdempotencyResponse(ctx context.Context, db DBTX, userID, key string, statusCode int, response []byte) error {
	_, err := db.Exec(ctx, `
		UPDATE idempotency_keys
		SET status_code = $3, response_body = $4::jsonb
		WHERE user_id = $1::uuid AND key = $2
	`, userID, key, statusCode, response)
	if err != nil {
		return fmt.Errorf("store idempotency response: %w", err)
	}
	return nil
}

// DeleteExpiredIdempotencyKeys deletes keys older than the specified duration.
func DeleteExpiredIdempotencyKeys(ctx context.Context, db DBTX, olderThan time.Duration) (int64, error) {
	seconds := int(olderThan.Seconds())
	if seconds <= 0 {
		seconds = 86400
	}
	tag, err := db.Exec(ctx, `
		DELETE FROM idempotency_keys
		WHERE created_at < now() - ($1 || ' seconds')::interval
	`, fmt.Sprintf("%d", seconds))
	if err != nil {
		return 0, fmt.Errorf("delete expired idempotency keys: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Backward-compatible helpers
func LockIdempotency(ctx context.Context, db DBTX, userID, key string) error {
	var locked bool
	if err := db.QueryRow(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0)) IS NULL`, userID+":"+key).Scan(&locked); err != nil {
		return fmt.Errorf("lock idempotency key: %w", err)
	}
	return nil
}

func GetIdempotency(ctx context.Context, db DBTX, userID, key string) (IdempotencyRecord, error) {
	var record IdempotencyRecord
	var statusCode *int
	var responseBody []byte
	err := db.QueryRow(ctx, `
		SELECT endpoint, request_hash, status_code, response_body
		FROM idempotency_keys
		WHERE user_id = $1::uuid AND key = $2
	`, userID, key).Scan(&record.Endpoint, &record.RequestHash, &statusCode, &responseBody)
	if errors.Is(err, pgx.ErrNoRows) {
		return IdempotencyRecord{}, nil
	}
	if err != nil {
		return IdempotencyRecord{}, fmt.Errorf("load idempotency response: %w", err)
	}
	record.Found = true
	if statusCode != nil {
		record.StatusCode = *statusCode
	}
	record.Response = responseBody
	return record, nil
}

func StoreIdempotency(ctx context.Context, db DBTX, userID, key, endpoint, requestHash string, statusCode int, response []byte) error {
	_, err := db.Exec(ctx, `
		INSERT INTO idempotency_keys (user_id, key, endpoint, request_hash, status_code, response_body)
		VALUES ($1::uuid, $2, $3, $4, $5, $6::jsonb)
		ON CONFLICT (user_id, key) DO UPDATE
		SET status_code = EXCLUDED.status_code,
		    response_body = EXCLUDED.response_body
	`, userID, key, endpoint, requestHash, statusCode, response)
	if err != nil {
		return fmt.Errorf("store idempotency response: %w", err)
	}
	return nil
}
