package domain

import "context"

type AssignmentNotification struct {
	TaskID     string
	TaskTitle  string
	AssigneeID string
	AssignedBy string
}

// Notifier delivers a notification about an assignment. The spec allows a
// mock/log-only implementation, but it must still be able to fail and, when
// it does, that failure must roll back the rest of the assign transaction —
// so it participates in the same call as the DB writes, not fire-and-forget.
type Notifier interface {
	NotifyTaskAssigned(ctx context.Context, notification AssignmentNotification) error
}
