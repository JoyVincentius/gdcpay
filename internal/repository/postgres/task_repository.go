package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"gdcpay/internal/domain"
)

type TaskRepository struct {
	db *sql.DB
}

func NewTaskRepository(db *sql.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

// q returns the ambient transaction from ctx if one was started via
// TxManager.WithinTransaction, otherwise the plain connection pool.
func (r *TaskRepository) q(ctx context.Context) querier {
	if tx := txFromContext(ctx); tx != nil {
		return tx
	}
	return r.db
}

func (r *TaskRepository) Create(ctx context.Context, task *domain.Task) error {
	const query = `
		INSERT INTO tasks (id, owner_id, assignee_id, title, description, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.q(ctx).ExecContext(ctx, query,
		task.ID, task.OwnerID, task.AssigneeID, task.Title, task.Description, task.Status, task.CreatedAt, task.UpdatedAt,
	)
	return err
}

func (r *TaskRepository) FindByID(ctx context.Context, id string) (*domain.Task, error) {
	const query = `
		SELECT id, owner_id, assignee_id, title, description, status, created_at, updated_at
		FROM tasks
		WHERE id = $1
	`
	return r.scan(r.q(ctx).QueryRowContext(ctx, query, id))
}

func (r *TaskRepository) FindByIDForUpdate(ctx context.Context, id string) (*domain.Task, error) {
	const query = `
		SELECT id, owner_id, assignee_id, title, description, status, created_at, updated_at
		FROM tasks
		WHERE id = $1
		FOR UPDATE
	`
	return r.scan(r.q(ctx).QueryRowContext(ctx, query, id))
}

func (r *TaskRepository) scan(row *sql.Row) (*domain.Task, error) {
	var t domain.Task
	err := row.Scan(&t.ID, &t.OwnerID, &t.AssigneeID, &t.Title, &t.Description, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrTaskNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *TaskRepository) Update(ctx context.Context, task *domain.Task) error {
	const query = `
		UPDATE tasks
		SET title = $1, description = $2, status = $3, updated_at = $4
		WHERE id = $5
	`
	result, err := r.q(ctx).ExecContext(ctx, query, task.Title, task.Description, task.Status, task.UpdatedAt, task.ID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrTaskNotFound
	}
	return nil
}

func (r *TaskRepository) UpdateAssignee(ctx context.Context, taskID, assigneeID string, updatedAt time.Time) error {
	const query = `
		UPDATE tasks
		SET assignee_id = $1, updated_at = $2
		WHERE id = $3
	`
	result, err := r.q(ctx).ExecContext(ctx, query, assigneeID, updatedAt, taskID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrTaskNotFound
	}
	return nil
}

func (r *TaskRepository) Delete(ctx context.Context, id string) error {
	const query = `DELETE FROM tasks WHERE id = $1`
	result, err := r.q(ctx).ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrTaskNotFound
	}
	return nil
}

// List scopes results to tasks the user owns or is assigned to, optionally
// narrowed by status/title search, and returns the total match count via a
// window function so pagination metadata costs no extra round trip.
func (r *TaskRepository) List(ctx context.Context, filter domain.TaskListFilter) ([]domain.Task, int, error) {
	conditions := []string{"(owner_id = $1 OR assignee_id = $1)"}
	args := []interface{}{filter.UserID}

	if filter.Status != "" {
		args = append(args, filter.Status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		conditions = append(conditions, fmt.Sprintf("title ILIKE $%d", len(args)))
	}

	limit := filter.Limit
	offset := (filter.Page - 1) * filter.Limit
	args = append(args, limit, offset)

	query := fmt.Sprintf(`
		SELECT id, owner_id, assignee_id, title, description, status, created_at, updated_at,
		       COUNT(*) OVER() AS total_count
		FROM tasks
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(conditions, " AND "), len(args)-1, len(args))

	rows, err := r.q(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	tasks := make([]domain.Task, 0, filter.Limit)
	total := 0
	for rows.Next() {
		var t domain.Task
		if err := rows.Scan(
			&t.ID, &t.OwnerID, &t.AssigneeID, &t.Title, &t.Description, &t.Status, &t.CreatedAt, &t.UpdatedAt, &total,
		); err != nil {
			return nil, 0, err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return tasks, total, nil
}
