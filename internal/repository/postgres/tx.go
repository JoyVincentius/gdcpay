package postgres

import (
	"context"
	"database/sql"
)

// querier is satisfied by both *sql.DB and *sql.Tx, so repository methods
// can run against either a plain connection or an ambient transaction
// without knowing which.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type txContextKey struct{}

func withTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

func txFromContext(ctx context.Context) *sql.Tx {
	tx, _ := ctx.Value(txContextKey{}).(*sql.Tx)
	return tx
}

// TxManager implements domain.TxManager. Repositories pull the active
// transaction out of the context (see querier/withTx), so any repository
// call made inside WithinTransaction's fn automatically participates in the
// same transaction without the caller having to thread a *sql.Tx through.
type TxManager struct {
	db *sql.DB
}

func NewTxManager(db *sql.DB) *TxManager {
	return &TxManager{db: db}
}

func (m *TxManager) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if err := fn(withTx(ctx, tx)); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}
