package postgres

import (
	"context"
	"database/sql"

	"gdcpay/internal/domain"
)

type TaskLogRepository struct {
	db *sql.DB
}

func NewTaskLogRepository(db *sql.DB) *TaskLogRepository {
	return &TaskLogRepository{db: db}
}

func (r *TaskLogRepository) q(ctx context.Context) querier {
	if tx := txFromContext(ctx); tx != nil {
		return tx
	}
	return r.db
}

func (r *TaskLogRepository) Create(ctx context.Context, log *domain.TaskLog) error {
	const query = `
		INSERT INTO task_logs (id, task_id, changed_by, old_assignee_id, new_assignee_id, action, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.q(ctx).ExecContext(ctx, query,
		log.ID, log.TaskID, log.ChangedBy, log.OldAssigneeID, log.NewAssigneeID, log.Action, log.CreatedAt,
	)
	return err
}
