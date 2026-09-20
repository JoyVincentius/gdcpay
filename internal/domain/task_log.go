package domain

import (
	"context"
	"time"
)

const TaskLogActionAssigned = "assigned"

type TaskLog struct {
	ID            string
	TaskID        string
	ChangedBy     string
	OldAssigneeID *string
	NewAssigneeID *string
	Action        string
	CreatedAt     time.Time
}

type TaskLogRepository interface {
	Create(ctx context.Context, log *TaskLog) error
}
