package domain

import (
	"context"
	"time"
)

const (
	TaskStatusTodo       = "todo"
	TaskStatusInProgress = "in_progress"
	TaskStatusDone       = "done"
)

type Task struct {
	ID          string
	OwnerID     string
	AssigneeID  *string
	Title       string
	Description string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TaskListFilter scopes a task list query to what UserID is allowed to see
// (tasks they own or are assigned to), optionally narrowed by status/search
// and paginated.
type TaskListFilter struct {
	UserID string
	Status string
	Search string
	Page   int
	Limit  int
}

type TaskRepository interface {
	Create(ctx context.Context, task *Task) error
	FindByID(ctx context.Context, id string) (*Task, error)
	// FindByIDForUpdate is like FindByID but locks the row (SELECT ... FOR
	// UPDATE), for use inside a transaction that's about to mutate it —
	// e.g. assigning it — so concurrent assigns on the same task serialize
	// instead of racing.
	FindByIDForUpdate(ctx context.Context, id string) (*Task, error)
	Update(ctx context.Context, task *Task) error
	// UpdateAssignee changes only the assignee and updated_at columns.
	UpdateAssignee(ctx context.Context, taskID, assigneeID string, updatedAt time.Time) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, filter TaskListFilter) ([]Task, int, error)
}
