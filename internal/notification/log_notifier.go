// Package notification holds Notifier implementations.
package notification

import (
	"context"
	"log/slog"

	"gdcpay/internal/domain"
)

// LogNotifier is a mock notifier: it logs the assignment instead of
// delivering it through a real channel (email, push, Slack, etc.), per the
// spec's "boleh mock/log saja" allowance. It still runs inside the assign
// transaction, so a real implementation swapped in later (e.g. publishing to
// a queue) keeps the same rollback-on-failure guarantee for free.
type LogNotifier struct {
	logger *slog.Logger
}

func NewLogNotifier(logger *slog.Logger) *LogNotifier {
	return &LogNotifier{logger: logger}
}

func (n *LogNotifier) NotifyTaskAssigned(_ context.Context, notif domain.AssignmentNotification) error {
	n.logger.Info("task assignment notification",
		"task_id", notif.TaskID,
		"task_title", notif.TaskTitle,
		"assignee_id", notif.AssigneeID,
		"assigned_by", notif.AssignedBy,
	)
	return nil
}
