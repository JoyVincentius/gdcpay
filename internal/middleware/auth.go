package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"gdcpay/internal/domain"
	"gdcpay/internal/pkg/jwtutil"
	"gdcpay/internal/pkg/response"
)

const (
	UserIDKey = "user_id"
	TeamIDKey = "team_id"
)

// Auth verifies the JWT bearer token and, on success, stores the caller's
// user_id and team_id in the request context for downstream handlers.
func Auth(tokenizer *jwtutil.Tokenizer) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			response.Error(c, domain.ErrUnauthorized)
			return
		}

		claims, err := tokenizer.Verify(token)
		if err != nil {
			response.Error(c, domain.ErrUnauthorized)
			return
		}

		c.Set(UserIDKey, claims.UserID)
		c.Set(TeamIDKey, claims.TeamID)
		c.Next()
	}
}
