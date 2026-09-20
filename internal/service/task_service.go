package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"gdcpay/internal/domain"
	"gdcpay/internal/dto"
)

type TaskService struct {
	tasks          domain.TaskRepository
	users          domain.UserRepository
	taskLogs       domain.TaskLogRepository
	notifier       domain.Notifier
	txManager      domain.TxManager
	idempotency    domain.IdempotencyStore
	idempotencyTTL time.Duration
}

type TaskServiceDeps struct {
	Tasks          domain.TaskRepository
	Users          domain.UserRepository
	TaskLogs       domain.TaskLogRepository
	Notifier       domain.Notifier
	TxManager      domain.TxManager
	Idempotency    domain.IdempotencyStore
	IdempotencyTTL time.Duration
}

func NewTaskService(deps TaskServiceDeps) *TaskService {
	return &TaskService{
		tasks:          deps.Tasks,
		users:          deps.Users,
		taskLogs:       deps.TaskLogs,
		notifier:       deps.Notifier,
		txManager:      deps.TxManager,
		idempotency:    deps.Idempotency,
		idempotencyTTL: deps.IdempotencyTTL,
	}
}

func (s *TaskService) Create(ctx context.Context, userID string, req dto.CreateTaskRequest) (*dto.TaskResponse, error) {
	status := req.Status
	if status == "" {
		status = domain.TaskStatusTodo
	}

	now := time.Now().UTC()
	task := &domain.Task{
		ID:          uuid.NewString(),
		OwnerID:     userID,
		Title:       strings.TrimSpace(req.Title),
		Description: req.Description,
		Status:      status,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.tasks.Create(ctx, task); err != nil {
		return nil, domain.ErrInternal
	}

	resp := toTaskResponse(task)
	return &resp, nil
}

// CreateIdempotent wraps Create so that a POST /tasks retried with the same
// Idempotency-Key (e.g. a client retry after a dropped response) replays the
// original response byte-for-byte instead of creating a second task. Safety
// under concurrent duplicate requests is delegated entirely to the
// IdempotencyStore.
func (s *TaskService) CreateIdempotent(ctx context.Context, userID, idempotencyKey, requestHash string, req dto.CreateTaskRequest) (int, []byte, error) {
	return s.idempotency.Execute(ctx, idempotencyKey, userID, requestHash, s.idempotencyTTL, func(ctx context.Context) (int, []byte, error) {
		resp, err := s.Create(ctx, userID, req)
		if err != nil {
			return 0, nil, err
		}
		body, merr := json.Marshal(resp)
		if merr != nil {
			return 0, nil, domain.ErrInternal
		}
		return http.StatusCreated, body, nil
	})
}

func (s *TaskService) Get(ctx context.Context, userID, taskID string) (*dto.TaskResponse, error) {
	task, err := s.fetchVisible(ctx, userID, taskID)
	if err != nil {
		return nil, err
	}
	resp := toTaskResponse(task)
	return &resp, nil
}

func (s *TaskService) List(ctx context.Context, userID string, query dto.ListTasksQuery) (*dto.ListTasksResponse, error) {
	page := query.Page
	if page < 1 {
		page = 1
	}
	limit := query.Limit
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	filter := domain.TaskListFilter{
		UserID: userID,
		Status: query.Status,
		Search: strings.TrimSpace(query.Search),
		Page:   page,
		Limit:  limit,
	}

	tasks, total, err := s.tasks.List(ctx, filter)
	if err != nil {
		return nil, domain.ErrInternal
	}

	data := make([]dto.TaskResponse, 0, len(tasks))
	for i := range tasks {
		data = append(data, toTaskResponse(&tasks[i]))
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}

	return &dto.ListTasksResponse{
		Data:       data,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

func (s *TaskService) Update(ctx context.Context, userID, taskID string, req dto.UpdateTaskRequest) (*dto.TaskResponse, error) {
	task, err := s.fetchOwned(ctx, userID, taskID)
	if err != nil {
		return nil, err
	}

	if req.Title != nil {
		task.Title = strings.TrimSpace(*req.Title)
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.Status != nil {
		task.Status = *req.Status
	}
	task.UpdatedAt = time.Now().UTC()

	if err := s.tasks.Update(ctx, task); err != nil {
		return nil, domain.ErrInternal
	}

	resp := toTaskResponse(task)
	return &resp, nil
}

func (s *TaskService) Delete(ctx context.Context, userID, taskID string) error {
	if _, err := s.fetchOwned(ctx, userID, taskID); err != nil {
		return err
	}
	if err := s.tasks.Delete(ctx, taskID); err != nil {
		return domain.ErrInternal
	}
	return nil
}

// Assign reassigns a task to a teammate. It runs entirely inside one
// TxManager transaction — locking the task row, updating the assignee,
// writing the task_logs audit entry, and sending the (mocked) notification —
// so that if any step fails, none of it is applied.
func (s *TaskService) Assign(ctx context.Context, callerID, callerTeamID, taskID string, req dto.AssignTaskRequest) (*dto.TaskResponse, error) {
	var result *domain.Task

	err := s.txManager.WithinTransaction(ctx, func(ctx context.Context) error {
		task, err := s.tasks.FindByIDForUpdate(ctx, taskID)
		if err != nil {
			if errors.Is(err, domain.ErrTaskNotFound) {
				return domain.ErrTaskNotFound
			}
			return domain.ErrInternal
		}

		visible := task.OwnerID == callerID || (task.AssigneeID != nil && *task.AssigneeID == callerID)
		if !visible {
			return domain.ErrTaskNotFound
		}
		if task.OwnerID != callerID {
			return domain.ErrForbidden
		}

		assignee, err := s.users.FindByID(ctx, req.AssigneeID)
		if err != nil {
			if errors.Is(err, domain.ErrUserNotFound) {
				return domain.ErrUserNotFound
			}
			return domain.ErrInternal
		}
		if assignee.TeamID != callerTeamID {
			return domain.ErrAssigneeNotInTeam
		}

		now := time.Now().UTC()
		oldAssigneeID := task.AssigneeID
		newAssigneeID := assignee.ID

		if err := s.tasks.UpdateAssignee(ctx, task.ID, newAssigneeID, now); err != nil {
			return domain.ErrInternal
		}

		if err := s.taskLogs.Create(ctx, &domain.TaskLog{
			ID:            uuid.NewString(),
			TaskID:        task.ID,
			ChangedBy:     callerID,
			OldAssigneeID: oldAssigneeID,
			NewAssigneeID: &newAssigneeID,
			Action:        domain.TaskLogActionAssigned,
			CreatedAt:     now,
		}); err != nil {
			return domain.ErrInternal
		}

		if err := s.notifier.NotifyTaskAssigned(ctx, domain.AssignmentNotification{
			TaskID:     task.ID,
			TaskTitle:  task.Title,
			AssigneeID: newAssigneeID,
			AssignedBy: callerID,
		}); err != nil {
			return domain.ErrInternal
		}

		task.AssigneeID = &newAssigneeID
		task.UpdatedAt = now
		result = task
		return nil
	})
	if err != nil {
		return nil, err
	}

	resp := toTaskResponse(result)
	return &resp, nil
}

// fetchVisible returns the task only if it exists and the caller is its
// owner or assignee. A task that exists but belongs to someone unrelated
// resolves to the same 404 as a missing task, so existence is never leaked.
func (s *TaskService) fetchVisible(ctx context.Context, userID, taskID string) (*domain.Task, error) {
	task, err := s.tasks.FindByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, domain.ErrTaskNotFound) {
			return nil, domain.ErrTaskNotFound
		}
		return nil, domain.ErrInternal
	}

	if task.OwnerID != userID && (task.AssigneeID == nil || *task.AssigneeID != userID) {
		return nil, domain.ErrTaskNotFound
	}
	return task, nil
}

// fetchOwned additionally requires the caller to be the task's owner.
// Mutating a task (update/delete) is owner-only; an assignee can view a task
// but not change or remove it.
func (s *TaskService) fetchOwned(ctx context.Context, userID, taskID string) (*domain.Task, error) {
	task, err := s.fetchVisible(ctx, userID, taskID)
	if err != nil {
		return nil, err
	}
	if task.OwnerID != userID {
		return nil, domain.ErrForbidden
	}
	return task, nil
}

func toTaskResponse(t *domain.Task) dto.TaskResponse {
	return dto.TaskResponse{
		ID:          t.ID,
		OwnerID:     t.OwnerID,
		AssigneeID:  t.AssigneeID,
		Title:       t.Title,
		Description: t.Description,
		Status:      t.Status,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}
