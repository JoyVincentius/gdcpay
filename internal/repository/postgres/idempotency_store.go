package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"gdcpay/internal/domain"
)

const maxIdempotencyAttempts = 5

type IdempotencyStore struct {
	db *sql.DB
}

func NewIdempotencyStore(db *sql.DB) *IdempotencyStore {
	return &IdempotencyStore{db: db}
}

// Execute claims the key by inserting a row under a unique constraint.
// Postgres blocks a concurrent INSERT of the same key until the first
// (winning) transaction commits or rolls back: if it committed, our insert
// is skipped by ON CONFLICT DO NOTHING and we fall through to read its
// result via SELECT ... FOR UPDATE; if it rolled back (work failed), the row
// never existed and our insert succeeds instead, making us the new winner.
// This gives an at-most-once guarantee without any application-level
// locking, even when many requests race on the same key.
func (s *IdempotencyStore) Execute(
	ctx context.Context,
	key, userID, requestHash string,
	ttl time.Duration,
	work func(ctx context.Context) (int, []byte, error),
) (int, []byte, error) {
	return s.execute(ctx, key, userID, requestHash, ttl, work, 0)
}

func (s *IdempotencyStore) execute(
	ctx context.Context,
	key, userID, requestHash string,
	ttl time.Duration,
	work func(ctx context.Context) (int, []byte, error),
	attempt int,
) (int, []byte, error) {
	if attempt >= maxIdempotencyAttempts {
		return 0, nil, domain.ErrInternal
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, nil, domain.ErrInternal
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO idempotency_keys (key, user_id, request_hash, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (key) DO NOTHING
	`, key, userID, requestHash, now, now.Add(ttl))
	if err != nil {
		return 0, nil, domain.ErrInternal
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, nil, domain.ErrInternal
	}

	if rows == 1 {
		// We won the race: run the work, persist its result, and commit.
		status, body, werr := work(ctx)
		if werr != nil {
			// Rollback (via defer) removes our placeholder row, freeing the
			// key so a retry with the same key can succeed later.
			return 0, nil, werr
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE idempotency_keys SET response_status = $1, response_body = $2 WHERE key = $3
		`, status, string(body), key); err != nil {
			return 0, nil, domain.ErrInternal
		}
		if err := tx.Commit(); err != nil {
			return 0, nil, domain.ErrInternal
		}
		committed = true
		return status, body, nil
	}

	// Someone else claimed the key first. Lock their row so we block until
	// their transaction resolves, then act on the final outcome.
	var (
		existingHash   string
		responseStatus sql.NullInt64
		responseBody   sql.NullString
		expiresAt      time.Time
	)
	err = tx.QueryRowContext(ctx, `
		SELECT request_hash, response_status, response_body, expires_at
		FROM idempotency_keys
		WHERE key = $1
		FOR UPDATE
	`, key).Scan(&existingHash, &responseStatus, &responseBody, &expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// The winner rolled back after we started waiting; the key is
			// free again, so retry as a fresh attempt.
			if err := tx.Commit(); err != nil {
				return 0, nil, domain.ErrInternal
			}
			committed = true
			return s.execute(ctx, key, userID, requestHash, ttl, work, attempt+1)
		}
		return 0, nil, domain.ErrInternal
	}

	if now.After(expiresAt) {
		// The 24h window has passed: recycle the key and retry as new.
		if _, err := tx.ExecContext(ctx, `DELETE FROM idempotency_keys WHERE key = $1`, key); err != nil {
			return 0, nil, domain.ErrInternal
		}
		if err := tx.Commit(); err != nil {
			return 0, nil, domain.ErrInternal
		}
		committed = true
		return s.execute(ctx, key, userID, requestHash, ttl, work, attempt+1)
	}

	if existingHash != requestHash {
		return 0, nil, domain.ErrIdempotencyKeyReused
	}

	if !responseStatus.Valid {
		return 0, nil, domain.ErrInternal
	}

	if err := tx.Commit(); err != nil {
		return 0, nil, domain.ErrInternal
	}
	committed = true
	return int(responseStatus.Int64), []byte(responseBody.String), nil
}
