package artifact

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandlerRegisterRoutesAppliesMiddlewareBeforeArtifactLogic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(nil, nil, nil)
	handler.RegisterRoutes(router, func(c *gin.Context) {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "auth required"})
		c.Abort()
	})

	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/art-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
