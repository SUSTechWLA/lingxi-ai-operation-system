package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	bidmodel "github.com/tangying-ai/aios-core/internal/agents/bid/model"
	"github.com/tangying-ai/aios-core/internal/core/auth"
)

func TestListBidProjectsUsesAuthenticatedUserNotQueryUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeBidService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "u_auth"))
		c.Next()
	})
	NewBidHandler(fake).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/bid/projects?userId=u_attacker", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fake.listUserID != "u_auth" {
		t.Fatalf("ListProjects used userID %q, want authenticated user", fake.listUserID)
	}
}

type fakeBidService struct {
	listUserID string
}

func (f *fakeBidService) CreateProject(ctx context.Context, userID string, req *bidmodel.CreateProjectRequest) (*bidmodel.BidProject, error) {
	return &bidmodel.BidProject{ID: "bid-1", UserID: userID, Name: req.Name}, nil
}

func (f *fakeBidService) GetProject(ctx context.Context, userID string, id string) (*bidmodel.BidProject, []*bidmodel.BidChapter, error) {
	return &bidmodel.BidProject{ID: id, UserID: userID}, []*bidmodel.BidChapter{}, nil
}

func (f *fakeBidService) ListProjects(ctx context.Context, status string, userID string, offset, limit int) ([]*bidmodel.BidProject, int, error) {
	f.listUserID = userID
	return []*bidmodel.BidProject{}, 0, nil
}

func (f *fakeBidService) UpdateProject(ctx context.Context, userID string, projectID string, req *bidmodel.UpdateProjectRequest) (*bidmodel.BidProject, error) {
	return &bidmodel.BidProject{ID: projectID, UserID: userID, Name: req.Name}, nil
}

func (f *fakeBidService) DeleteProject(ctx context.Context, userID string, projectID string) error {
	return nil
}

func (f *fakeBidService) SetTenderFile(ctx context.Context, userID string, projectID string, filePath string, fileName string) error {
	return nil
}

func (f *fakeBidService) StartGeneration(ctx context.Context, userID string, projectID string) (string, error) {
	return "task-1", nil
}

func (f *fakeBidService) PauseGeneration(ctx context.Context, userID string, projectID string) error {
	return nil
}

func (f *fakeBidService) ResumeGeneration(ctx context.Context, userID string, projectID string) error {
	return nil
}

func (f *fakeBidService) ApproveChapter(ctx context.Context, userID string, projectID, chapterID string) (*bidmodel.BidChapter, error) {
	return &bidmodel.BidChapter{ID: chapterID, ProjectID: projectID}, nil
}

func (f *fakeBidService) RejectChapter(ctx context.Context, userID string, projectID, chapterID, comment string) (*bidmodel.BidChapter, error) {
	return &bidmodel.BidChapter{ID: chapterID, ProjectID: projectID, ReviewComment: comment}, nil
}

func (f *fakeBidService) GetProgress(ctx context.Context, userID string, projectID string) (*bidmodel.ProgressResponse, error) {
	return &bidmodel.ProgressResponse{TaskID: projectID}, nil
}

func (f *fakeBidService) ListTemplates(ctx context.Context) ([]*bidmodel.BidTemplate, error) {
	return []*bidmodel.BidTemplate{}, nil
}

func (f *fakeBidService) GetExportStatus(ctx context.Context, userID string, projectID string) (*bidmodel.ExportStatusResponse, error) {
	return &bidmodel.ExportStatusResponse{Status: "NOT_STARTED"}, nil
}
