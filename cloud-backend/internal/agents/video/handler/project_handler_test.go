package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
	"github.com/tangying-ai/aios-core/internal/core/auth"
)

func TestCreateVideoProjectUsesAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeProjectService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "u_auth"))
		c.Next()
	})
	NewProjectHandler(fake).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/api/video-projects", strings.NewReader(`{
		"name":"Test",
		"mode":"aigc_shot",
		"userId":"u_attacker"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fake.createdUserID != "u_auth" {
		t.Fatalf("CreateProject used userID %q, want authenticated user", fake.createdUserID)
	}
}

type fakeProjectService struct {
	createdUserID string
}

func (f *fakeProjectService) CreateProject(ctx context.Context, userID string, req *model.CreateProjectRequest) (*model.VideoProject, error) {
	f.createdUserID = userID
	return &model.VideoProject{ID: "vp-1", UserID: userID, Name: req.Name, Mode: req.Mode}, nil
}

func (f *fakeProjectService) ListProjects(ctx context.Context, userID string, modeFilter, statusFilter string, offset, limit int) ([]*model.VideoProject, int, error) {
	return []*model.VideoProject{}, 0, nil
}

func (f *fakeProjectService) GetProject(ctx context.Context, userID string, id string) (*model.VideoProject, error) {
	return &model.VideoProject{ID: id, UserID: userID}, nil
}

func (f *fakeProjectService) UpdateProject(ctx context.Context, userID string, id string, req *model.UpdateProjectRequest) (*model.VideoProject, error) {
	return &model.VideoProject{ID: id, UserID: userID, Name: req.Name}, nil
}

func (f *fakeProjectService) ArchiveProject(ctx context.Context, userID string, id string) error {
	return nil
}
