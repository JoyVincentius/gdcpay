package domain

import (
	"context"
	"time"
)

// IdempotencyStore runs work at most once per key. A repeat call with the
// same key within ttl must return the first call's (status, body) without
// invoking work again, and concurrent calls with the same key must still
// result in exactly one invocation of work — implementations are the
// authority on that guarantee (a unique DB constraint, a mutex-guarded map,
// etc.), not the caller.
type IdempotencyStore interface {
	Execute(
		ctx context.Context,
		key, userID, requestHash string,
		ttl time.Duration,
		work func(ctx context.Context) (status int, body []byte, err error),
	) (status int, body []byte, err error)
}
