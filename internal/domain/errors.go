package domain

import "net/http"

// AppError is the canonical error type surfaced to API clients. Handlers and
// services return it directly so the response middleware can render a
// consistent {status, code, message, timestamp} body.
type AppError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *AppError) Error() string {
	return e.Message
}

func NewAppError(status int, code, message string) *AppError {
	return &AppError{Status: status, Code: code, Message: message}
}

func NewValidationError(message string) *AppError {
	return NewAppError(http.StatusBadRequest, "VALIDATION_ERROR", message)
}

var (
	ErrInvalidCredentials   = NewAppError(http.StatusUnauthorized, "INVALID_CREDENTIALS", "email or password is incorrect")
	ErrEmailAlreadyExists   = NewAppError(http.StatusConflict, "EMAIL_ALREADY_EXISTS", "email is already registered")
	ErrUserNotFound         = NewAppError(http.StatusNotFound, "USER_NOT_FOUND", "user not found")
	ErrTeamNotFound         = NewAppError(http.StatusNotFound, "TEAM_NOT_FOUND", "team not found")
	ErrUnauthorized         = NewAppError(http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
	ErrForbidden            = NewAppError(http.StatusForbidden, "FORBIDDEN", "you do not have permission to perform this action")
	ErrTaskNotFound         = NewAppError(http.StatusNotFound, "TASK_NOT_FOUND", "task not found")
	ErrIdempotencyKeyReused = NewAppError(http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was already used with a different request payload")
	ErrAssigneeNotInTeam    = NewAppError(http.StatusUnprocessableEntity, "ASSIGNEE_NOT_IN_TEAM", "assignee must belong to the same team as the task owner")
	ErrInternal             = NewAppError(http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
)
