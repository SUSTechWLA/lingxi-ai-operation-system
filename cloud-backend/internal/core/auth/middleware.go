package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
)

type Middleware struct {
	svc *Service
}

func NewMiddleware(svc *Service) *Middleware {
	return &Middleware{svc: svc}
}

func (m *Middleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			httpx.Fail(c, http.StatusUnauthorized, ErrUnauthorized.Error())
			c.Abort()
			return
		}
		user, err := m.svc.ValidateAccessToken(c.Request.Context(), token)
		if err != nil {
			writeAuthError(c, err)
			c.Abort()
			return
		}
		ctx := ContextWithUser(c.Request.Context(), user.ID)
		if deviceID := strings.TrimSpace(c.GetHeader("DeviceID")); deviceID != "" {
			ctx = ContextWithDevice(ctx, deviceID)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Set("userID", user.ID)
		c.Next()
	}
}

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
