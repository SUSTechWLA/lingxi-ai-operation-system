package artifact

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tangying-ai/aios-core/internal/core/auth"
	"github.com/tangying-ai/aios-core/internal/core/model"
	"github.com/tangying-ai/aios-core/internal/core/workflow"
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

func TestArtifactContentReviewTextPreservesInlineSourceBytes(t *testing.T) {
	for _, test := range []struct {
		name       string
		kind       ArtifactKind
		mimeType   string
		reviewText string
	}{
		{
			name:       "markdown",
			kind:       KindMarkdown,
			mimeType:   "text/markdown",
			reviewText: "  # 原稿\n\n第一行  \n第二行\n",
		},
		{
			name:       "json",
			kind:       KindJSON,
			mimeType:   "application/json",
			reviewText: "{\n  \"标题\": \"原样保留\",\n  \"emoji\": \"🙂\"\n}\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			artifact := &Artifact{
				ID: "artifact-" + test.name, ProjectID: "vp-1", StageName: "script",
				Kind: test.kind, MimeType: test.mimeType, StorageType: StorageInline, InlineJSON: test.reviewText,
			}
			handler := newHandlerForStore(&fakeHandlerArtifactStore{artifact: artifact}, nil, nil).
				WithProjectAccess(fakeArtifactProjectAccess{owners: map[string]string{"vp-1": "user-1"}})

			data := requestArtifactContentData(t, handler, artifact.ID)

			if got, ok := data["reviewText"].(string); !ok || got != test.reviewText {
				t.Fatalf("reviewText = %#v, want exact source %q", data["reviewText"], test.reviewText)
			}
		})
	}
}

func TestArtifactContentReviewTextPreservesEmptyInlineSource(t *testing.T) {
	for _, test := range []struct {
		name     string
		kind     ArtifactKind
		mimeType string
	}{
		{name: "markdown", kind: KindMarkdown, mimeType: "text/markdown"},
		{name: "json", kind: KindJSON, mimeType: "application/json"},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := &Artifact{
				ID: "artifact-empty-" + test.name, ProjectID: "vp-1", StageName: "script",
				Kind: test.kind, MimeType: test.mimeType, StorageType: StorageInline, InlineJSON: "",
			}
			handler := newHandlerForStore(&fakeHandlerArtifactStore{artifact: item}, nil, nil).
				WithProjectAccess(fakeArtifactProjectAccess{owners: map[string]string{"vp-1": "user-1"}})

			reviewText, err := handler.ResolveReviewableText(context.Background(), item)
			if err != nil || reviewText != "" {
				t.Fatalf("ResolveReviewableText() = %q, %v; want exact empty source", reviewText, err)
			}
			data := requestArtifactContentData(t, handler, item.ID)
			if reviewText, ok := data["reviewText"].(string); !ok || reviewText != "" {
				t.Fatalf("endpoint reviewText = %#v, want present exact empty source", data["reviewText"])
			}
		})
	}
}

func TestArtifactContentReviewTextPreservesHydratedLocalSourceBytes(t *testing.T) {
	const reviewText = "  # 本地原稿\n\n你好🙂，保留尾随空格。  \n"
	node := &model.Node{
		ID: "script_exec", TaskID: "task-local-1", Status: model.NodeSuccess,
		Input: map[string]interface{}{"parameters": map[string]interface{}{"stage": "script"}},
		Output: map[string]interface{}{
			"content": reviewText,
			"artifacts": []interface{}{map[string]interface{}{
				"unitId": "script-content", "kind": "MARKDOWN", "name": "script.md", "mimeType": "text/markdown",
			}},
		},
	}
	artifact := &Artifact{
		ID: "artifact-local", ProjectID: "vp-1", WorkflowRunID: "run-missing", TaskID: "task-local-1",
		StageName: "script", UnitID: "script-content", Kind: KindMarkdown, Name: "script.md",
		MimeType: "text/markdown", StorageType: StorageLocal,
	}
	handler := newHandlerForStore(
		&fakeHandlerArtifactStore{artifact: artifact},
		unavailableWorkflowRunRepository(t),
		&reviewTextNodeRepo{node: node},
	).WithProjectAccess(fakeArtifactProjectAccess{owners: map[string]string{"vp-1": "user-1"}})

	data := requestArtifactContentData(t, handler, artifact.ID)

	if got, ok := data["reviewText"].(string); !ok || got != reviewText {
		t.Fatalf("reviewText = %#v, want exact hydrated source %q", data["reviewText"], reviewText)
	}
}

func TestResolveReviewableTextPrefersNonEmptyInlineSourceOverHydration(t *testing.T) {
	const inlineSource = "  # legacy inline source\n\nkeep this exact text  \n"
	node := &model.Node{
		ID: "legacy_script_exec", TaskID: "task-legacy-inline", Status: model.NodeSuccess,
		Input: map[string]interface{}{"parameters": map[string]interface{}{"stage": "script"}},
		Output: map[string]interface{}{
			"content": "hydrated replacement that must not win",
			"artifacts": []interface{}{map[string]interface{}{
				"unitId": "script-content", "kind": "MARKDOWN", "name": "script.md", "mimeType": "text/markdown",
			}},
		},
	}
	nodeRepo := &reviewTextNodeRepo{node: node}
	item := &Artifact{
		ID: "artifact-legacy-inline", ProjectID: "vp-1", WorkflowRunID: "run-missing", TaskID: node.TaskID,
		StageName: "script", UnitID: "script-content", Kind: KindMarkdown, Name: "script.md",
		MimeType: "text/markdown", StorageType: StorageLocal, InlineJSON: inlineSource,
	}
	handler := newHandlerForStore(
		&fakeHandlerArtifactStore{artifact: item},
		unavailableWorkflowRunRepository(t),
		nodeRepo,
	)

	reviewText, err := handler.ResolveReviewableText(context.Background(), item)

	if err != nil || reviewText != inlineSource {
		t.Fatalf("ResolveReviewableText() = %q, %v; want exact inline source %q", reviewText, err, inlineSource)
	}
	if nodeRepo.findByIDCalls != 0 || nodeRepo.findByTaskIDCalls != 0 {
		t.Fatalf("hydrator repository calls = FindByID:%d FindByTaskID:%d; want none", nodeRepo.findByIDCalls, nodeRepo.findByTaskIDCalls)
	}
}

func TestArtifactContentReviewTextPreservesExactLocalZeroBytes(t *testing.T) {
	node := &model.Node{
		ID: "empty_script_exec", TaskID: "task-empty-local", Status: model.NodeSuccess,
		Input: map[string]interface{}{"parameters": map[string]interface{}{"stage": "script"}},
		Output: map[string]interface{}{
			"content": "",
			"artifacts": []interface{}{map[string]interface{}{
				"unitId": "script-content", "kind": "MARKDOWN", "name": "script.md", "mimeType": "text/markdown",
			}},
		},
	}
	item := &Artifact{
		ID: "artifact-empty-local", ProjectID: "vp-1", WorkflowRunID: "run-missing", TaskID: node.TaskID,
		StageName: "script", UnitID: "script-content", Kind: KindMarkdown, Name: "script.md",
		MimeType: "text/markdown", StorageType: StorageLocal,
	}
	handler := newHandlerForStore(
		&fakeHandlerArtifactStore{artifact: item},
		unavailableWorkflowRunRepository(t),
		&reviewTextNodeRepo{node: node},
	).WithProjectAccess(fakeArtifactProjectAccess{owners: map[string]string{"vp-1": "user-1"}})

	reviewText, err := handler.ResolveReviewableText(context.Background(), item)
	if err != nil || reviewText != "" {
		t.Fatalf("ResolveReviewableText() = %q, %v; want exact local zero-byte source", reviewText, err)
	}
	data := requestArtifactContentData(t, handler, item.ID)
	if reviewText, ok := data["reviewText"].(string); !ok || reviewText != "" {
		t.Fatalf("endpoint reviewText = %#v, want present exact local zero-byte source", data["reviewText"])
	}
}

func TestResolveReviewableTextRejectsAbsentLocalData(t *testing.T) {
	node := &model.Node{
		ID: "missing_script_exec", TaskID: "task-missing-local", Status: model.NodeSuccess,
		Input: map[string]interface{}{"parameters": map[string]interface{}{"stage": "script"}},
		Output: map[string]interface{}{
			"artifacts": []interface{}{map[string]interface{}{
				"unitId": "script-content", "kind": "MARKDOWN", "name": "script.md", "mimeType": "text/markdown",
			}},
		},
	}
	item := &Artifact{
		ID: "artifact-missing-local", ProjectID: "vp-1", WorkflowRunID: "run-missing", TaskID: node.TaskID,
		StageName: "script", UnitID: "script-content", Kind: KindMarkdown, Name: "script.md",
		MimeType: "text/markdown", StorageType: StorageLocal,
	}
	handler := newHandlerForStore(
		&fakeHandlerArtifactStore{artifact: item},
		unavailableWorkflowRunRepository(t),
		&reviewTextNodeRepo{node: node},
	).WithProjectAccess(fakeArtifactProjectAccess{owners: map[string]string{"vp-1": "user-1"}})

	if reviewText, err := handler.ResolveReviewableText(context.Background(), item); !errors.Is(err, ErrRevisionContentUnavailable) || reviewText != "" {
		t.Fatalf("ResolveReviewableText() = %q, %v; want unavailable", reviewText, err)
	}
	data := requestArtifactContentData(t, handler, item.ID)
	if _, ok := data["reviewText"]; ok {
		t.Fatalf("endpoint exposed absent local data as reviewText: %#v", data["reviewText"])
	}
}

func requestArtifactContentData(t *testing.T, handler *Handler, artifactID string) map[string]interface{} {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler.RegisterRoutes(router, func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.ContextWithUser(c.Request.Context(), "user-1"))
		c.Next()
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/artifacts/"+artifactID+"/content", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v; body = %s", err, rec.Body.String())
	}
	return response.Data
}

func unavailableWorkflowRunRepository(t *testing.T) *workflow.RunRepository {
	t.Helper()
	config, err := pgxpool.ParseConfig("postgres://unused:unused@localhost/unused")
	if err != nil {
		t.Fatalf("parse test pool config: %v", err)
	}
	config.BeforeConnect = func(context.Context, *pgx.ConnConfig) error {
		return errors.New("workflow run lookup intentionally unavailable")
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("create test pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return workflow.NewRunRepository(pool)
}

type reviewTextNodeRepo struct {
	node              *model.Node
	findByIDCalls     int
	findByTaskIDCalls int
}

func (f *reviewTextNodeRepo) FindByID(_ context.Context, id string) (*model.Node, error) {
	f.findByIDCalls++
	if f.node != nil && f.node.ID == id {
		return f.node, nil
	}
	return nil, nil
}
func (f *reviewTextNodeRepo) FindByTaskID(_ context.Context, taskID string) ([]*model.Node, error) {
	f.findByTaskIDCalls++
	if f.node != nil && f.node.TaskID == taskID {
		return []*model.Node{f.node}, nil
	}
	return nil, nil
}
func (f *reviewTextNodeRepo) FindByStatus(context.Context, model.NodeStatus) ([]*model.Node, error) {
	return nil, nil
}
func (f *reviewTextNodeRepo) FindChildNodes(context.Context, string) ([]*model.Node, error) {
	return nil, nil
}
func (f *reviewTextNodeRepo) Save(context.Context, *model.Node) error { return nil }
func (f *reviewTextNodeRepo) UpdateStatus(context.Context, string, model.NodeStatus, map[string]interface{}, string) error {
	return nil
}
func (f *reviewTextNodeRepo) FindStaleRunningNodes(context.Context, int) ([]*model.Node, error) {
	return nil, nil
}
func (f *reviewTextNodeRepo) UpdateHeartbeat(context.Context, string, float64, string) error {
	return nil
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
