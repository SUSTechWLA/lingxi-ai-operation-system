package media

import (
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/auth"
)

func TestMediaListUsesAuthenticatedUserNotQueryUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeMediaService{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "u_auth"))
		c.Next()
	})
	NewMediaHandler(fake).RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/media/list?userId=u_attacker", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if fake.listUserID != "u_auth" {
		t.Fatalf("List used userID %q, want authenticated user", fake.listUserID)
	}
}

type fakeMediaService struct {
	listUserID string
}

func (f *fakeMediaService) Upload(_ context.Context, userID string, _ []*multipart.FileHeader) ([]*MediaAsset, error) {
	return []*MediaAsset{{ID: "media-1", UserID: userID}}, nil
}

func (f *fakeMediaService) List(_ context.Context, userID string, _ int, _ int, _ string) ([]*MediaAsset, int, error) {
	f.listUserID = userID
	return []*MediaAsset{}, 0, nil
}

func (f *fakeMediaService) GetForUser(_ context.Context, userID string, id string) (*MediaAsset, error) {
	return &MediaAsset{ID: id, UserID: userID}, nil
}

func (f *fakeMediaService) UpdateTagsForUser(_ context.Context, _ string, _ string, _ []string) error {
	return nil
}
