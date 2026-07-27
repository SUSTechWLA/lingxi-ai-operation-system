package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/auth"
)

type fakeRelayRepository struct {
	page       EventPage
	summary    RunSummary
	ackCount   int64
	err        error
	lastUserID string
	lastCursor string
	lastLimit  int
	lastRunID  string
	lastIDs    []string
}

func (repo *fakeRelayRepository) Pull(_ context.Context, userID, cursor string, limit int) (EventPage, error) {
	repo.lastUserID, repo.lastCursor, repo.lastLimit = userID, cursor, limit
	return repo.page, repo.err
}

func (repo *fakeRelayRepository) Acknowledge(_ context.Context, userID string, eventIDs []string) (int64, error) {
	repo.lastUserID = userID
	repo.lastIDs = append([]string(nil), eventIDs...)
	return repo.ackCount, repo.err
}

func (repo *fakeRelayRepository) RunSummary(_ context.Context, userID, runID string) (RunSummary, error) {
	repo.lastUserID, repo.lastRunID = userID, runID
	return repo.summary, repo.err
}

func authenticated(userID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("userID", "untrusted-gin-value")
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), userID))
		c.Next()
	}
}

func TestPullEventsUsesAuthenticatedContextAndClampsLimit(t *testing.T) {
	repo := &fakeRelayRepository{page: EventPage{Events: []Event{{EventID: "evt_one"}}}}
	handler := NewHandler(repo)
	router := gin.New()
	router.GET("/events", authenticated("user_a"), handler.Pull)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events?userID=user_b&limit=9999", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
	if repo.lastUserID != "user_a" || repo.lastLimit != maxPullLimit {
		t.Fatalf("scope=%q limit=%d", repo.lastUserID, repo.lastLimit)
	}
}

func TestPullEventsFailsClosedWithoutAuthenticatedContext(t *testing.T) {
	repo := &fakeRelayRepository{}
	router := gin.New()
	router.GET("/events", NewHandler(repo).Pull)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/events", nil))
	if recorder.Code != http.StatusUnauthorized || repo.lastUserID != "" {
		t.Fatalf("status=%d repository user=%q", recorder.Code, repo.lastUserID)
	}
}

func TestPullEventsRejectsMalformedCursorAndLimit(t *testing.T) {
	repo := &fakeRelayRepository{}
	router := gin.New()
	router.GET("/events", authenticated("user_a"), NewHandler(repo).Pull)
	for _, target := range []string{"/events?cursor=bad", "/events?limit=nope", "/events?limit=0"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d body=%s", target, recorder.Code, recorder.Body)
		}
	}
}

func TestAcknowledgeEventsUsesAuthenticatedScopeAndRejectsUnknownFields(t *testing.T) {
	repo := &fakeRelayRepository{ackCount: 1}
	router := gin.New()
	router.POST("/events/ack", authenticated("user_a"), NewHandler(repo).Acknowledge)
	body := []byte(`{"eventIds":["evt_one"]}`)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/events/ack", bytes.NewReader(body)))
	if recorder.Code != http.StatusOK || repo.lastUserID != "user_a" || len(repo.lastIDs) != 1 {
		t.Fatalf("status=%d user=%q ids=%v body=%s", recorder.Code, repo.lastUserID, repo.lastIDs, recorder.Body)
	}
	var response struct {
		Data struct {
			Acknowledged int64 `json:"acknowledged"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Data.Acknowledged != 1 {
		t.Fatalf("response=%s error=%v", recorder.Body, err)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/events/ack", bytes.NewReader([]byte(`{"eventIds":["evt_one"],"userID":"user_b"}`))))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d body=%s", recorder.Code, recorder.Body)
	}
}

func TestAcknowledgeEventsRejectsOversizedAndInvalidIDs(t *testing.T) {
	repo := &fakeRelayRepository{}
	router := gin.New()
	router.POST("/events/ack", authenticated("user_a"), NewHandler(repo).Acknowledge)
	for _, body := range [][]byte{
		[]byte(`{"eventIds":[]}`),
		[]byte(`{"eventIds":["other"]}`),
		[]byte(`{"eventIds":["evt_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]}`),
		func() []byte {
			ids := make([]string, maxAckEventIDs+1)
			for index := range ids {
				ids[index] = "evt_one"
			}
			data, _ := json.Marshal(map[string]any{"eventIds": ids})
			return data
		}(),
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/events/ack", bytes.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
		}
	}
}

func TestRunSummaryUsesAuthenticatedUserAndPathRunID(t *testing.T) {
	repo := &fakeRelayRepository{summary: RunSummary{RunID: "wfr_run"}}
	router := gin.New()
	router.GET("/runs/:runId/summary", authenticated("user_a"), NewHandler(repo).RunSummary)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/runs/wfr_run/summary?userID=user_b", nil))
	if recorder.Code != http.StatusOK || repo.lastUserID != "user_a" || repo.lastRunID != "wfr_run" {
		t.Fatalf("status=%d user=%q run=%q body=%s", recorder.Code, repo.lastUserID, repo.lastRunID, recorder.Body)
	}
}

func TestRunSummaryMapsNotFoundWithoutCrossUserDisclosure(t *testing.T) {
	repo := &fakeRelayRepository{err: ErrSummaryNotFound}
	router := gin.New()
	router.GET("/runs/:runId/summary", authenticated("user_a"), NewHandler(repo).RunSummary)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/runs/wfr_missing/summary", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
	repo.err = errors.New("database unavailable")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/runs/wfr_missing/summary", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
}
