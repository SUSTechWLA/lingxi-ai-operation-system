package workflow

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/auth"
)

type recordingWorkflowService struct{ userID string }

func (*recordingWorkflowService) List(context.Context) ([]*Template, error)      { return nil, nil }
func (*recordingWorkflowService) Get(context.Context, string) (*Template, error) { return nil, nil }
func (*recordingWorkflowService) Create(context.Context, *CreateTemplateRequest) (*Template, error) {
	return nil, nil
}
func (*recordingWorkflowService) Update(context.Context, string, *UpdateTemplateRequest) (*Template, error) {
	return nil, nil
}
func (*recordingWorkflowService) Delete(context.Context, string) error { return nil }
func (s *recordingWorkflowService) Instantiate(_ context.Context, userID, _ string, _ map[string]interface{}) (string, error) {
	s.userID = userID
	return "task-auth", nil
}

func TestWorkflowInstantiateUsesAuthenticatedOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &recordingWorkflowService{}
	router := gin.New()
	NewHandler(service).RegisterRoutes(router, func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "user-auth"))
		c.Next()
	})
	req := httptest.NewRequest(http.MethodPost, "/api/workflows/template-1/instantiate", bytes.NewBufferString(`{"userId":"attacker","overrides":{}}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK || service.userID != "user-auth" {
		t.Fatalf("status=%d owner=%q body=%s", res.Code, service.userID, res.Body.String())
	}
}
