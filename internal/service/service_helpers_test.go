package service_test

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"gdcpay/internal/domain"
)

// fakeTaskRepository is an in-memory domain.TaskRepository stub so these
// tests can run without a database connection, per the spec's requirement
// that idempotency/race-condition tests use mocks rather than a live DB.
type fakeTaskRepository struct {
	mu          sync.Mutex
	createCalls int32
	tasks       map[string]*domain.Task
}

func newFakeTaskRepository() *fakeTaskRepository {
	return &fakeTaskRepository{tasks: make(map[string]*domain.Task)}
}

func (f *fakeTaskRepository) seed(t *domain.Task) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tasks[t.ID] = t
}

func (f *fakeTaskRepository) Create(_ context.Context, task *domain.Task) error {
	atomic.AddInt32(&f.createCalls, 1)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tasks[task.ID] = task
	return nil
}

func (f *fakeTaskRepository) FindByID(_ context.Context, id string) (*domain.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tasks[id]
	if !ok {
		return nil, domain.ErrTaskNotFound
	}
	return t, nil
}

func (f *fakeTaskRepository) FindByIDForUpdate(ctx context.Context, id string) (*domain.Task, error) {
	return f.FindByID(ctx, id)
}

func (f *fakeTaskRepository) Update(_ context.Context, task *domain.Task) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.tasks[task.ID]; !ok {
		return domain.ErrTaskNotFound
	}
	f.tasks[task.ID] = task
	return nil
}

// UpdateAssignee mutates the task immediately (there's no real DB to defer
// the write) but registers an undo closure with the ambient txStage, if
// present, so a transaction that fails after this step can still be rolled
// back — mirroring what the real Postgres transaction guarantees.
func (f *fakeTaskRepository) UpdateAssignee(ctx context.Context, taskID, assigneeID string, updatedAt time.Time) error {
	f.mu.Lock()
	t, ok := f.tasks[taskID]
	if !ok {
		f.mu.Unlock()
		return domain.ErrTaskNotFound
	}
	prevAssignee := t.AssigneeID
	prevUpdatedAt := t.UpdatedAt
	a := assigneeID
	t.AssigneeID = &a
	t.UpdatedAt = updatedAt
	f.mu.Unlock()

	if stage := stageFromContext(ctx); stage != nil {
		stage.record(func() {
			f.mu.Lock()
			t.AssigneeID = prevAssignee
			t.UpdatedAt = prevUpdatedAt
			f.mu.Unlock()
		})
	}
	return nil
}

func (f *fakeTaskRepository) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.tasks[id]; !ok {
		return domain.ErrTaskNotFound
	}
	delete(f.tasks, id)
	return nil
}

func (f *fakeTaskRepository) List(_ context.Context, _ domain.TaskListFilter) ([]domain.Task, int, error) {
	return nil, 0, nil
}

// fakeUserRepository is an in-memory domain.UserRepository stub.
type fakeUserRepository struct {
	mu    sync.Mutex
	users map[string]*domain.User
}

func newFakeUserRepository() *fakeUserRepository {
	return &fakeUserRepository{users: make(map[string]*domain.User)}
}

func (f *fakeUserRepository) seed(u *domain.User) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users[u.ID] = u
}

func (f *fakeUserRepository) Create(_ context.Context, u *domain.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users[u.ID] = u
	return nil
}

func (f *fakeUserRepository) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, domain.ErrUserNotFound
}

func (f *fakeUserRepository) FindByID(_ context.Context, id string) (*domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}

// fakeTaskLogRepository is an in-memory domain.TaskLogRepository stub. Like
// fakeTaskRepository.UpdateAssignee, it registers an undo with the ambient
// txStage so a failed transaction removes the log entry it just wrote.
type fakeTaskLogRepository struct {
	mu   sync.Mutex
	logs []*domain.TaskLog
}

func newFakeTaskLogRepository() *fakeTaskLogRepository {
	return &fakeTaskLogRepository{}
}

func (f *fakeTaskLogRepository) Create(ctx context.Context, log *domain.TaskLog) error {
	f.mu.Lock()
	f.logs = append(f.logs, log)
	idx := len(f.logs) - 1
	f.mu.Unlock()

	if stage := stageFromContext(ctx); stage != nil {
		stage.record(func() {
			f.mu.Lock()
			f.logs = append(f.logs[:idx], f.logs[idx+1:]...)
			f.mu.Unlock()
		})
	}
	return nil
}

func (f *fakeTaskLogRepository) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.logs)
}

// fakeNotifier is a domain.Notifier stub that can be told to fail, so tests
// can force the assign transaction to roll back at its last step.
type fakeNotifier struct {
	mu      sync.Mutex
	calls   int
	failErr error
}

func (f *fakeNotifier) NotifyTaskAssigned(_ context.Context, _ domain.AssignmentNotification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.failErr
}

// txStage/fakeTxManager reproduce, without a database, the same guarantee
// Postgres gives for free: every write made during WithinTransaction's fn is
// undone if fn returns an error. Repositories that participate register an
// undo closure via stageFromContext; on failure they're run in reverse.
type txStageKey struct{}

type txStage struct {
	mu   sync.Mutex
	undo []func()
}

func (s *txStage) record(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.undo = append(s.undo, fn)
}

func stageFromContext(ctx context.Context) *txStage {
	s, _ := ctx.Value(txStageKey{}).(*txStage)
	return s
}

type fakeTxManager struct{}

func (fakeTxManager) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	stage := &txStage{}
	stagedCtx := context.WithValue(ctx, txStageKey{}, stage)

	if err := fn(stagedCtx); err != nil {
		stage.mu.Lock()
		undo := stage.undo
		stage.mu.Unlock()
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
		return err
	}
	return nil
}
