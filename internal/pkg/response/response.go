package response

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"gdcpay/internal/domain"
)

type errorBody struct {
	Status    int       `json:"status"`
	Code      string    `json:"code"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// Success writes a JSON success response.
func Success(c *gin.Context, status int, data interface{}) {
	c.JSON(status, data)
}

// Error renders any error as the API's structured error format and aborts
// the request. Errors that are not *domain.AppError are mapped to a generic
// 500 so internal details never leak to clients.
func Error(c *gin.Context, err error) {
	var appErr *domain.AppError
	if !errors.As(err, &appErr) {
		appErr = domain.ErrInternal
	}

	_ = c.Error(err)
	c.AbortWithStatusJSON(appErr.Status, errorBody{
		Status:    appErr.Status,
		Code:      appErr.Code,
		Message:   appErr.Message,
		Timestamp: time.Now().UTC(),
	})
}
