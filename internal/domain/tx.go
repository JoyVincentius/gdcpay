package domain

import "context"

// TxManager runs fn within a single atomic unit of work. If fn returns an
// error, every write made through repositories during fn must be undone;
// otherwise all of them must be durably applied. This is what lets the
// assign flow (update assignee + write task_logs + notify) succeed or fail
// as one operation.
type TxManager interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}
