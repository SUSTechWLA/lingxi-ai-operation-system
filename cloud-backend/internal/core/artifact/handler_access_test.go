package artifact

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tangying-ai/aios-core/internal/core/auth"
)

func TestArtifactContentIsHiddenFromAnotherProjectOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &fakeHandlerArtifactStore{artifact: &Artifact{
		ID: "artifact-secret", ProjectID: "project-owner-1", StageName: "script",
		StorageType: StorageInline, InlineJSON: "owner-only-content",
	}}
	handler := newHandlerForStore(store, nil, nil).WithProjectAccess(fakeArtifactProjectAccess{
		owners: map[string]string{"project-owner-1": "user-owner-1"},
	})
	router := gin.New()
	handler.RegisterRoutes(router, func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "user-owner-2"))
		c.Next()
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/artifacts/artifact-secret/content", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "owner-only-content") || strings.Contains(rec.Body.String(), "project-owner-1") {
		t.Fatalf("cross-user response leaked artifact data: %s", rec.Body.String())
	}
}

type fakeArtifactProjectAccess struct{ owners map[string]string }

func (f fakeArtifactProjectAccess) CanAccessProject(_ context.Context, userID, projectID string) bool {
	return f.owners[projectID] == userID
}

type fakeHandlerArtifactStore struct{ artifact *Artifact }

func (f *fakeHandlerArtifactStore) GetByID(_ context.Context, id string) (*Artifact, error) {
	if f.artifact == nil || f.artifact.ID != id {
		return nil, errors.New("not found")
	}
	return f.artifact, nil
}
func (f *fakeHandlerArtifactStore) GetCurrent(context.Context, string, string, string) (*Artifact, error) {
	return f.artifact, nil
}
func (f *fakeHandlerArtifactStore) GetHistory(context.Context, string, string, string) ([]*Artifact, error) {
	return []*Artifact{f.artifact}, nil
}
func (f *fakeHandlerArtifactStore) ListByProject(context.Context, string) ([]*Artifact, error) {
	return []*Artifact{f.artifact}, nil
}
func (f *fakeHandlerArtifactStore) CreateArtifact(context.Context, *CreateArtifactRequest) (*Artifact, error) {
	return f.artifact, nil
}
func (f *fakeHandlerArtifactStore) MarkDownstreamStale(context.Context, string, string, string) ([]string, error) {
	return nil, nil
}
