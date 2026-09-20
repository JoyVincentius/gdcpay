package service_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gdcpay/internal/dto"
	"gdcpay/internal/repository/memory"
	"gdcpay/internal/service"
)

func newTestTaskService() (*service.TaskService, *fakeTaskRepository) {
	repo := newFakeTaskRepository()
	store := memory.NewIdempotencyStore()
	deps := service.TaskServiceDeps{
		Tasks:          repo,
		Users:          newFakeUserRepository(),
		TaskLogs:       newFakeTaskLogRepository(),
		Notifier:       &fakeNotifier{},
		TxManager:      fakeTxManager{},
		Idempotency:    store,
		IdempotencyTTL: 24 * time.Hour,
	}
	return service.NewTaskService(deps), repo
}

// Sequential: a first request with a new Idempotency-Key creates the task
// (201); a second request with the same key does not create another task
// and returns the identical response.
func TestCreateIdempotent_Sequential(t *testing.T) {
	svc, repo := newTestTaskService()
	ctx := context.Background()
	req := dto.CreateTaskRequest{Title: "Write tests"}
	const key = "11111111-1111-1111-1111-111111111111"

	status1, body1, err := svc.CreateIdempotent(ctx, "user-1", key, "hash-1", req)
	if err != nil {
		t.Fatalf("first request: unexpected error: %v", err)
	}
	if status1 != 201 {
		t.Fatalf("first request: expected status 201, got %d", status1)
	}

	status2, body2, err := svc.CreateIdempotent(ctx, "user-1", key, "hash-1", req)
	if err != nil {
		t.Fatalf("second request: unexpected error: %v", err)
	}
	if status2 != status1 {
		t.Fatalf("second request: expected same status %d, got %d", status1, status2)
	}
	if string(body2) != string(body1) {
		t.Fatalf("second request: expected identical body, got %s vs %s", body2, body1)
	}

	if calls := atomic.LoadInt32(&repo.createCalls); calls != 1 {
		t.Fatalf("expected exactly 1 task to be created, got %d", calls)
	}
}

// Concurrent duplicate: N goroutines send the same Idempotency-Key at the
// same time. Exactly one task must be created in the database, and every
// caller must observe the same response.
func TestCreateIdempotent_ConcurrentDuplicate(t *testing.T) {
	svc, repo := newTestTaskService()
	ctx := context.Background()
	req := dto.CreateTaskRequest{Title: "Write tests"}
	const key = "22222222-2222-2222-2222-222222222222"
	const n = 50

	var wg sync.WaitGroup
	statuses := make([]int, n)
	bodies := make([][]byte, n)
	errs := make([]error, n)

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			status, body, err := svc.CreateIdempotent(ctx, "user-1", key, "hash-1", req)
			statuses[i], bodies[i], errs[i] = status, body, err
		}(i)
	}
	wg.Wait()

	for i := range errs {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: unexpected error: %v", i, errs[i])
		}
		if statuses[i] != 201 {
			t.Fatalf("goroutine %d: expected status 201, got %d", i, statuses[i])
		}
		if string(bodies[i]) != string(bodies[0]) {
			t.Fatalf("goroutine %d: expected identical body to goroutine 0, got %s vs %s", i, bodies[i], bodies[0])
		}
	}

	if calls := atomic.LoadInt32(&repo.createCalls); calls != 1 {
		t.Fatalf("expected exactly 1 task to be created under concurrency, got %d", calls)
	}
}

// Concurrent requests with different Idempotency-Keys must not be
// serialized against each other and must each create their own task.
func TestCreateIdempotent_ConcurrentDistinctKeys(t *testing.T) {
	svc, repo := newTestTaskService()
	ctx := context.Background()
	const n = 20

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			req := dto.CreateTaskRequest{Title: "Task"}
			key := uuidFor(i)
			if _, _, err := svc.CreateIdempotent(ctx, "user-1", key, "hash", req); err != nil {
				t.Errorf("goroutine %d: unexpected error: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	if calls := atomic.LoadInt32(&repo.createCalls); calls != n {
		t.Fatalf("expected %d tasks to be created, got %d", n, calls)
	}
}

func uuidFor(i int) string {
	return fmt.Sprintf("33333333-3333-3333-3333-%012d", i)
}
