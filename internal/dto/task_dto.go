package dto

import "time"

type CreateTaskRequest struct {
	Title       string `json:"title" binding:"required,min=1,max=200"`
	Description string `json:"description" binding:"omitempty,max=2000"`
	Status      string `json:"status" binding:"omitempty,oneof=todo in_progress done"`
}

// UpdateTaskRequest fields are pointers so a request only needs to include
// the fields it wants to change; omitted fields keep their current value.
type UpdateTaskRequest struct {
	Title       *string `json:"title" binding:"omitempty,min=1,max=200"`
	Description *string `json:"description" binding:"omitempty,max=2000"`
	Status      *string `json:"status" binding:"omitempty,oneof=todo in_progress done"`
}

type AssignTaskRequest struct {
	AssigneeID string `json:"assignee_id" binding:"required,uuid"`
}

type ListTasksQuery struct {
	Status string
	Search string
	Page   int
	Limit  int
}

type TaskResponse struct {
	ID          string    `json:"id"`
	OwnerID     string    `json:"owner_id"`
	AssigneeID  *string   `json:"assignee_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ListTasksResponse struct {
	Data       []TaskResponse `json:"data"`
	Page       int            `json:"page"`
	Limit      int            `json:"limit"`
	Total      int            `json:"total"`
	TotalPages int            `json:"total_pages"`
}
