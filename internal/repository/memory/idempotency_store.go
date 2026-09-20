// Package memory provides in-memory stand-ins for repository interfaces so
// business logic can be unit-tested without a live database.
package memory

import (
	"context"
	"sync"
	"time"

	"gdcpay/internal/domain"
)

type idempotencyEntry struct {
	done        chan struct{}
	requestHash string
	status      int
	body        []byte
	err         error
	expiresAt   time.Time
}

// IdempotencyStore is an in-memory domain.IdempotencyStore. It reproduces
// the Postgres implementation's at-most-once guarantee with a mutex-guarded
// map instead of a unique constraint: the first caller for a key becomes the
// "winner" and runs work(); every other caller for the same key blocks on
// the winner's done channel and then replays its result.
type IdempotencyStore struct {
	mu      sync.Mutex
	entries map[string]*idempotencyEntry
}

func NewIdempotencyStore() *IdempotencyStore {
	return &IdempotencyStore{entries: make(map[string]*idempotencyEntry)}
}

func (s *IdempotencyStore) Execute(
	ctx context.Context,
	key, userID, requestHash string,
	ttl time.Duration,
	work func(ctx context.Context) (int, []byte, error),
) (int, []byte, error) {
	for {
		s.mu.Lock()
		entry, exists := s.entries[key]
		if exists && time.Now().After(entry.expiresAt) {
			delete(s.entries, key)
			exists = false
		}

		if !exists {
			// expiresAt must be set before unlocking: while it's still the
			// zero value, any concurrent caller arriving before work()
			// finishes would see it as already-expired and wrongly claim
			// the key as a second winner.
			entry = &idempotencyEntry{
				done:        make(chan struct{}),
				requestHash: requestHash,
				expiresAt:   time.Now().Add(ttl),
			}
			s.entries[key] = entry
			s.mu.Unlock()

			status, body, err := work(ctx)
			entry.status, entry.body, entry.err = status, body, err
			close(entry.done)

			if err != nil {
				s.mu.Lock()
				if s.entries[key] == entry {
					delete(s.entries, key)
				}
				s.mu.Unlock()
				return 0, nil, err
			}
			return status, body, nil
		}
		s.mu.Unlock()

		<-entry.done // wait for the in-flight winner to finish

		if entry.err != nil {
			// The winner failed and freed the key; retry as a fresh attempt.
			continue
		}
		if entry.requestHash != requestHash {
			return 0, nil, domain.ErrIdempotencyKeyReused
		}
		return entry.status, entry.body, nil
	}
}
