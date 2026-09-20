package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gdcpay/internal/domain"
	"gdcpay/internal/dto"
	"gdcpay/internal/repository/memory"
	"gdcpay/internal/service"
)

func newAssignTestService() (*service.TaskService, *fakeTaskRepository, *fakeUserRepository, *fakeTaskLogRepository, *fakeNotifier) {
	tasks := newFakeTaskRepository()
	users := newFakeUserRepository()
	logs := newFakeTaskLogRepository()
	notifier := &fakeNotifier{}
	deps := service.TaskServiceDeps{
		Tasks:          tasks,
		Users:          users,
		TaskLogs:       logs,
		Notifier:       notifier,
		TxManager:      fakeTxManager{},
		Idempotency:    memory.NewIdempotencyStore(),
		IdempotencyTTL: 24 * time.Hour,
	}
	return service.NewTaskService(deps), tasks, users, logs, notifier
}

func TestAssign_HappyPath(t *testing.T) {
	svc, tasks, users, logs, notifier := newAssignTestService()
	ctx := context.Background()

	owner := &domain.User{ID: "owner-1", TeamID: "team-1"}
	teammate := &domain.User{ID: "teammate-1", TeamID: "team-1"}
	users.seed(owner)
	users.seed(teammate)

	task := &domain.Task{ID: "task-1", OwnerID: owner.ID, Title: "Ship it", Status: domain.TaskStatusTodo, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	tasks.seed(task)

	resp, err := svc.Assign(ctx, owner.ID, owner.TeamID, task.ID, dto.AssignTaskRequest{AssigneeID: teammate.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AssigneeID == nil || *resp.AssigneeID != teammate.ID {
		t.Fatalf("expected assignee %s, got %v", teammate.ID, resp.AssigneeID)
	}
	if logs.count() != 1 {
		t.Fatalf("expected 1 task_log entry, got %d", logs.count())
	}
	if notifier.calls != 1 {
		t.Fatalf("expected notifier to be called once, got %d", notifier.calls)
	}

	stored, _ := tasks.FindByID(ctx, task.ID)
	if stored.AssigneeID == nil || *stored.AssigneeID != teammate.ID {
		t.Fatalf("expected persisted assignee %s, got %v", teammate.ID, stored.AssigneeID)
	}
}

// A task's current assignee can see it but must not be able to reassign it
// — that's owner-only, same as Update/Delete. Since the assignee is visible
// (not an unrelated stranger), the failure must be 403, not a 404.
func TestAssign_ForbiddenForAssigneeReassigning(t *testing.T) {
	svc, tasks, users, _, _ := newAssignTestService()
	ctx := context.Background()

	owner := &domain.User{ID: "owner-1", TeamID: "team-1"}
	assignee := &domain.User{ID: "assignee-1", TeamID: "team-1"}
	teammate := &domain.User{ID: "teammate-1", TeamID: "team-1"}
	users.seed(owner)
	users.seed(assignee)
	users.seed(teammate)

	assigneeID := assignee.ID
	task := &domain.Task{ID: "task-1", OwnerID: owner.ID, AssigneeID: &assigneeID, Title: "Ship it", Status: domain.TaskStatusTodo, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	tasks.seed(task)

	_, err := svc.Assign(ctx, assignee.ID, assignee.TeamID, task.ID, dto.AssignTaskRequest{AssigneeID: teammate.ID})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestAssign_UnrelatedCallerGetsNotFound(t *testing.T) {
	svc, tasks, users, _, _ := newAssignTestService()
	ctx := context.Background()

	owner := &domain.User{ID: "owner-1", TeamID: "team-1"}
	stranger := &domain.User{ID: "stranger-1", TeamID: "team-2"}
	users.seed(owner)
	users.seed(stranger)

	task := &domain.Task{ID: "task-1", OwnerID: owner.ID, Title: "Ship it", Status: domain.TaskStatusTodo, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	tasks.seed(task)

	_, err := svc.Assign(ctx, stranger.ID, stranger.TeamID, task.ID, dto.AssignTaskRequest{AssigneeID: owner.ID})
	if !errors.Is(err, domain.ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound (existence must not leak), got %v", err)
	}
}

func TestAssign_RejectsAssigneeOutsideTeam(t *testing.T) {
	svc, tasks, users, _, _ := newAssignTestService()
	ctx := context.Background()

	owner := &domain.User{ID: "owner-1", TeamID: "team-1"}
	outsider := &domain.User{ID: "outsider-1", TeamID: "team-2"}
	users.seed(owner)
	users.seed(outsider)

	task := &domain.Task{ID: "task-1", OwnerID: owner.ID, Title: "Ship it", Status: domain.TaskStatusTodo, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	tasks.seed(task)

	_, err := svc.Assign(ctx, owner.ID, owner.TeamID, task.ID, dto.AssignTaskRequest{AssigneeID: outsider.ID})
	if !errors.Is(err, domain.ErrAssigneeNotInTeam) {
		t.Fatalf("expected ErrAssigneeNotInTeam, got %v", err)
	}
}

// Proves the transactional guarantee from the spec: if the notification step
// fails after the assignee update and task_log insert already ran, both
// must be rolled back so the task ends up completely unchanged.
func TestAssign_RollsBackOnNotificationFailure(t *testing.T) {
	svc, tasks, users, logs, notifier := newAssignTestService()
	ctx := context.Background()

	owner := &domain.User{ID: "owner-1", TeamID: "team-1"}
	teammate := &domain.User{ID: "teammate-1", TeamID: "team-1"}
	users.seed(owner)
	users.seed(teammate)

	createdAt := time.Now().Add(-time.Hour)
	task := &domain.Task{ID: "task-1", OwnerID: owner.ID, Title: "Ship it", Status: domain.TaskStatusTodo, CreatedAt: createdAt, UpdatedAt: createdAt}
	tasks.seed(task)

	notifier.failErr = errors.New("notification channel unavailable")

	_, err := svc.Assign(ctx, owner.ID, owner.TeamID, task.ID, dto.AssignTaskRequest{AssigneeID: teammate.ID})
	if err == nil {
		t.Fatal("expected an error from the failed notification step")
	}

	stored, _ := tasks.FindByID(ctx, task.ID)
	if stored.AssigneeID != nil {
		t.Fatalf("expected assignee to remain unset after rollback, got %v", *stored.AssigneeID)
	}
	if !stored.UpdatedAt.Equal(createdAt) {
		t.Fatalf("expected updated_at to remain unchanged after rollback, got %v", stored.UpdatedAt)
	}
	if logs.count() != 0 {
		t.Fatalf("expected no task_log entries after rollback, got %d", logs.count())
	}
}
