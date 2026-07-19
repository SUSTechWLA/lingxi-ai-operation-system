package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
	"github.com/tangying-ai/aios-core/internal/core/auth"
)

func TestCreatorViewHandlerRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewCreatorViewHandler(&fakeCreatorViewProjectReader{}, &fakeCreatorViewReader{}).RegisterRoutes(router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/video-projects/vp-1/creation-view", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreatorViewHandlerVerifiesOwnerBeforeReturningView(t *testing.T) {
	gin.SetMode(gin.TestMode)
	project := &fakeCreatorViewProjectReader{project: &model.VideoProject{ID: "vp-1", UserID: "u-auth"}}
	view := &fakeCreatorViewReader{view: &model.CreationView{Steps: []model.CreatorStep{{ID: model.CreatorStepRequirements}}}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "u-auth"))
		c.Next()
	})
	NewCreatorViewHandler(project, view).RegisterRoutes(router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/video-projects/vp-1/creation-view", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if project.userID != "u-auth" || project.projectID != "vp-1" || view.userID != "u-auth" || view.projectID != "vp-1" {
		t.Fatalf("project=%+v view=%+v", project, view)
	}
}

func TestCreatorViewHandlerDoesNotExposeAViewForMissingOrForbiddenProject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	project := &fakeCreatorViewProjectReader{err: errors.New("not found")}
	view := &fakeCreatorViewReader{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "u-auth"))
		c.Next()
	})
	NewCreatorViewHandler(project, view).RegisterRoutes(router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/video-projects/vp-hidden/creation-view", nil))
	if rec.Code != http.StatusNotFound || view.projectID != "" {
		t.Fatalf("status=%d view=%+v", rec.Code, view)
	}
}

func TestCreatorViewRouteDoesNotConflictWithProjectIDRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "u-auth"))
		c.Next()
	})
	NewProjectHandler(&fakeProjectService{}).RegisterRoutes(router)
	NewCreatorViewHandler(
		&fakeCreatorViewProjectReader{project: &model.VideoProject{ID: "vp-1", UserID: "u-auth"}},
		&fakeCreatorViewReader{view: &model.CreationView{}},
	).RegisterRoutes(router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/video-projects/vp-1/creation-view", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

type fakeCreatorViewProjectReader struct {
	project   *model.VideoProject
	err       error
	userID    string
	projectID string
}

func (f *fakeCreatorViewProjectReader) GetProject(_ context.Context, userID, projectID string) (*model.VideoProject, error) {
	f.userID, f.projectID = userID, projectID
	return f.project, f.err
}

type fakeCreatorViewReader struct {
	view      *model.CreationView
	err       error
	userID    string
	projectID string
}

func (f *fakeCreatorViewReader) GetCreationView(_ context.Context, userID, projectID string) (*model.CreationView, error) {
	f.userID, f.projectID = userID, projectID
	return f.view, f.err
}
