package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPublishHandlerRegisterRoutesAppliesMiddlewareBeforePublishLogic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewPublishHandler(nil)
	handler.RegisterRoutes(router, func(c *gin.Context) {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "auth required"})
		c.Abort()
	})

	req := httptest.NewRequest(http.MethodPost, "/api/ai/generate", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
