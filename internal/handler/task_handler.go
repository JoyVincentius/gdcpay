package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"

	"gdcpay/internal/domain"
	"gdcpay/internal/dto"
	"gdcpay/internal/middleware"
	"gdcpay/internal/pkg/response"
	"gdcpay/internal/service"
)

type TaskHandler struct {
	taskService *service.TaskService
}

func NewTaskHandler(taskService *service.TaskService) *TaskHandler {
	return &TaskHandler{taskService: taskService}
}

func (h *TaskHandler) Create(c *gin.Context) {
	idempotencyKey := c.GetHeader("Idempotency-Key")
	if _, err := uuid.Parse(idempotencyKey); err != nil {
		response.Error(c, domain.NewValidationError("Idempotency-Key header is required and must be a valid UUID"))
		return
	}

	var req dto.CreateTaskRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		response.Error(c, domain.NewValidationError(err.Error()))
		return
	}

	rawBody, _ := c.Get(gin.BodyBytesKey)
	bodyBytes, _ := rawBody.([]byte)
	sum := sha256.Sum256(bodyBytes)
	requestHash := hex.EncodeToString(sum[:])

	status, body, err := h.taskService.CreateIdempotent(c.Request.Context(), currentUserID(c), idempotencyKey, requestHash, req)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.Data(status, "application/json; charset=utf-8", body)
}

func (h *TaskHandler) List(c *gin.Context) {
	query := dto.ListTasksQuery{
		Status: c.Query("status"),
		Search: c.Query("search"),
		Page:   parseIntQuery(c, "page", 1),
		Limit:  parseIntQuery(c, "limit", 10),
	}

	res, err := h.taskService.List(c.Request.Context(), currentUserID(c), query)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, res)
}

func (h *TaskHandler) Get(c *gin.Context) {
	res, err := h.taskService.Get(c.Request.Context(), currentUserID(c), c.Param("id"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, res)
}

func (h *TaskHandler) Update(c *gin.Context) {
	var req dto.UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domain.NewValidationError(err.Error()))
		return
	}

	res, err := h.taskService.Update(c.Request.Context(), currentUserID(c), c.Param("id"), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, res)
}

func (h *TaskHandler) Delete(c *gin.Context) {
	if err := h.taskService.Delete(c.Request.Context(), currentUserID(c), c.Param("id")); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *TaskHandler) Assign(c *gin.Context) {
	var req dto.AssignTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domain.NewValidationError(err.Error()))
		return
	}

	res, err := h.taskService.Assign(c.Request.Context(), currentUserID(c), currentTeamID(c), c.Param("id"), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, http.StatusOK, res)
}

func currentUserID(c *gin.Context) string {
	v, _ := c.Get(middleware.UserIDKey)
	id, _ := v.(string)
	return id
}

func currentTeamID(c *gin.Context) string {
	v, _ := c.Get(middleware.TeamIDKey)
	id, _ := v.(string)
	return id
}

func parseIntQuery(c *gin.Context, key string, fallback int) int {
	raw := c.Query(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		return fallback
	}
	return v
}
