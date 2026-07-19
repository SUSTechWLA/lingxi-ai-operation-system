package artifact

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRevisionServiceRestoreCreatesImmutableCurrentChild(t *testing.T) {
	ctx := context.Background()
	repo := newRevisionServiceFake(t,
		&Artifact{
			ID: "artifact-v1", ProjectID: "project-1", StageName: "script", UnitID: "main",
			Kind: KindMarkdown, Name: "script.md", Version: 1, StorageType: StorageLocal,
			StorageRef: "local://original", MimeType: "text/markdown", SizeBytes: 12,
			ContentHash: "hash-v1", Metadata: map[string]interface{}{"nested": map[string]interface{}{"kept": true}},
		},
		&Artifact{
			ID: "artifact-v3", ProjectID: "project-1", StageName: "script", UnitID: "main",
			Kind: KindMarkdown, Name: "script.md", Version: 3, StorageType: StorageLocal,
			StorageRef: "local://current", MimeType: "text/markdown", SizeBytes: 12,
			ContentHash: "hash-v3", IsCurrent: true, Metadata: map[string]interface{}{"current": true},
		},
	)
	originalV1 := cloneArtifactForRevisionTest(repo.byID["artifact-v1"])
	revisions := NewRevisionService(repo)

	result, err := revisions.Restore(ctx, RestoreRequest{
		ArtifactID: "artifact-v1", ReviewerID: "user-1", Reason: "恢复第一版",
	})
	if err != nil {
		t.Fatalf("Restore error: %v", err)
	}
	if result.Artifact.Version != 4 || result.Artifact.ParentID != "artifact-v3" {
		t.Fatalf("restored artifact = %+v", result.Artifact)
	}
	if result.Artifact.Metadata["restoredFromArtifactId"] != "artifact-v1" {
		t.Fatalf("metadata = %+v", result.Artifact.Metadata)
	}
	if result.Artifact.Metadata["restoredByReviewerId"] != "user-1" || result.Artifact.Metadata["restoreReason"] != "恢复第一版" {
		t.Fatalf("restore attribution = %+v", result.Artifact.Metadata)
	}
	if result.Artifact.StorageRef != originalV1.StorageRef || result.Artifact.ContentHash != originalV1.ContentHash {
		t.Fatalf("restore did not preserve stored content identity: %+v", result.Artifact)
	}
	if !reflect.DeepEqual(repo.byID["artifact-v1"], originalV1) {
		t.Fatal("historical artifact was mutated")
	}
}

func TestRevisionServiceDirectContentBypassesGeneratorAndMarksDownstreamOnce(t *testing.T) {
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	revisions := NewRevisionService(repo)
	called := false
	revisions.SetConfig("", func(context.Context, string, string, ReviseLLMOptions) (string, error) {
		called = true
		return "generated", nil
	})

	result, err := revisions.Revise(context.Background(), ReviseRequest{
		ArtifactID: "artifact-v1", Message: "use supplied text", DirectContent: []byte("direct replacement"),
	})
	if err != nil {
		t.Fatalf("Revise error: %v", err)
	}
	if called {
		t.Fatal("direct content must not call the revision generator")
	}
	if string(repo.lastCreate.Data) != "direct replacement" || result.Artifact.Version != 2 {
		t.Fatalf("direct revision = %+v, create request = %+v", result.Artifact, repo.lastCreate)
	}
	if repo.stale != 1 || !reflect.DeepEqual(result.StaleStageNames, []string{"downstream"}) {
		t.Fatalf("downstream stale calls = %d, stages = %v", repo.stale, result.StaleStageNames)
	}
}

func TestRevisionServiceInstructionUsesGeneratorAndForwardsTextProvider(t *testing.T) {
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	revisions := NewRevisionService(repo)
	provider := map[string]interface{}{"provider": "openai", "model": "gpt-test"}
	var systemPrompt, userPrompt string
	var gotOptions ReviseLLMOptions
	revisions.SetConfig("", func(_ context.Context, system, user string, options ReviseLLMOptions) (string, error) {
		systemPrompt, userPrompt, gotOptions = system, user, options
		return "generator replacement", nil
	})

	_, err := revisions.Revise(context.Background(), ReviseRequest{
		ArtifactID: "artifact-v1", Message: "make it shorter",
		ModelProviders: map[string]interface{}{"text_to_text": provider},
	})
	if err != nil {
		t.Fatalf("Revise error: %v", err)
	}
	if gotOptions.ModelProvider["model"] != "gpt-test" || !strings.Contains(systemPrompt, "script") ||
		!strings.Contains(userPrompt, "original content") || !strings.Contains(userPrompt, "make it shorter") {
		t.Fatalf("generator inputs = system=%q user=%q opts=%+v", systemPrompt, userPrompt, gotOptions)
	}
	if string(repo.lastCreate.Data) != "generator replacement" {
		t.Fatalf("generated content was not versioned: %q", repo.lastCreate.Data)
	}
}

func TestForcedRestoreVersionBypassesOnlyContentHashDedupAndPreservesStoredIdentity(t *testing.T) {
	req := &CreateArtifactRequest{
		ProjectID: "project-1", StageName: "script", UnitID: "main", Kind: KindMarkdown, Name: "script.md",
		StorageType: StorageLocal, StorageRef: "local://historical", ContentHash: "unchanged-hash",
		Metadata:        map[string]interface{}{"nested": map[string]interface{}{"immutable": true}},
		ForceNewVersion: true, RestoredFromID: "artifact-v1",
	}
	if contentHashDedupEnabled(req) {
		t.Fatal("forced restores must bypass content-hash deduplication")
	}
	if !contentHashDedupEnabled(&CreateArtifactRequest{ContentHash: "unchanged-hash"}) {
		t.Fatal("ordinary artifact creation must retain content-hash deduplication")
	}
	record := buildArtifactRecord(req, 4, "artifact-v3")
	if record.StorageRef != req.StorageRef || record.ContentHash != req.ContentHash || record.Kind != req.Kind {
		t.Fatalf("restore identity was changed: %+v", record)
	}
	if record.Metadata["restoredFromArtifactId"] != "artifact-v1" {
		t.Fatalf("restore provenance missing: %+v", record.Metadata)
	}
	record.Metadata["nested"].(map[string]interface{})["mutated"] = true
	if req.Metadata["nested"].(map[string]interface{})["mutated"] != nil {
		t.Fatal("restored metadata aliases the historical metadata map")
	}
	inlineRecord := buildArtifactRecord(&CreateArtifactRequest{
		ProjectID: "project-1", StageName: "script", UnitID: "main", Kind: KindMarkdown, Name: "script.md",
		StorageType: StorageInline, StorageRef: "local://inline", Data: []byte("historical inline bytes"),
		ContentHash: "inline-hash", Provider: "legacy-provider", ForceNewVersion: true,
	}, 5, "artifact-v4")
	if inlineRecord.StorageType != StorageInline || inlineRecord.InlineJSON != "historical inline bytes" {
		t.Fatalf("forced restore did not preserve inline bytes: %+v", inlineRecord)
	}
}

func TestReviseArtifactKeepsLegacySuccessResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	handler := &Handler{revisions: NewRevisionService(repo)}
	request := httptest.NewRequest(http.MethodPost, "/api/artifacts/artifact-v1/revise", strings.NewReader(`{"message":"make it tighter"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	context.Params = gin.Params{{Key: "id", Value: "artifact-v1"}}

	handler.ReviseArtifact(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("legacy revise status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Code != 200 || response.Message != "success" {
		t.Fatalf("legacy response = %s, unmarshal error = %v", recorder.Body.String(), err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(response.Data, &data); err != nil || data["artifact"] == nil || data["content"] == nil || data["mediaUrls"] == nil {
		t.Fatalf("legacy data = %s, unmarshal error = %v", response.Data, err)
	}
}

func TestHandlerSetRevisionConfigConfiguresSharedService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	handler := &Handler{revisions: NewRevisionService(repo)}
	handler.SetRevisionConfig("", func(context.Context, string, string, ReviseLLMOptions) (string, error) {
		return "configured generator content", nil
	})
	request := httptest.NewRequest(http.MethodPost, "/api/artifacts/artifact-v1/revise", strings.NewReader(`{"message":"make it tighter"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	context.Params = gin.Params{{Key: "id", Value: "artifact-v1"}}

	handler.ReviseArtifact(context)

	if recorder.Code != http.StatusOK || string(repo.lastCreate.Data) != "configured generator content" {
		t.Fatalf("handler did not use configured shared generator: status=%d data=%q", recorder.Code, repo.lastCreate.Data)
	}
}

func revisionTestArtifact() *Artifact {
	return &Artifact{
		ID: "artifact-v1", ProjectID: "project-1", StageName: "script", UnitID: "main", Kind: KindMarkdown,
		Name: "script.md", Version: 1, StorageType: StorageInline, InlineJSON: "original content",
		StorageRef: "local://original", MimeType: "text/markdown", ContentHash: "hash-v1", IsCurrent: true,
		Metadata: map[string]interface{}{},
	}
}

type revisionServiceFake struct {
	t          *testing.T
	byID       map[string]*Artifact
	stale      int
	lastCreate *CreateArtifactRequest
}

func newRevisionServiceFake(t *testing.T, artifacts ...*Artifact) *revisionServiceFake {
	t.Helper()
	fake := &revisionServiceFake{t: t, byID: make(map[string]*Artifact, len(artifacts))}
	for _, artifact := range artifacts {
		fake.byID[artifact.ID] = artifact
	}
	return fake
}

func (f *revisionServiceFake) GetByID(_ context.Context, id string) (*Artifact, error) {
	artifact, ok := f.byID[id]
	if !ok {
		return nil, errRevisionTestNotFound
	}
	return artifact, nil
}

func (f *revisionServiceFake) GetCurrent(_ context.Context, projectID, stageName, unitID string) (*Artifact, error) {
	for _, artifact := range f.byID {
		if artifact.ProjectID == projectID && artifact.StageName == stageName && artifact.UnitID == unitID && artifact.IsCurrent {
			return artifact, nil
		}
	}
	return nil, errRevisionTestNotFound
}

func (f *revisionServiceFake) CreateArtifact(_ context.Context, req *CreateArtifactRequest) (*Artifact, error) {
	f.lastCreate = cloneCreateRequestForRevisionTest(req)
	var current *Artifact
	for _, artifact := range f.byID {
		if artifact.ProjectID == req.ProjectID && artifact.StageName == req.StageName && artifact.UnitID == req.UnitID && artifact.IsCurrent {
			current = artifact
			artifact.IsCurrent = false
		}
	}
	if current == nil {
		f.t.Fatal("CreateArtifact must use an existing current artifact")
	}
	created := &Artifact{
		ID: "artifact-v4", ProjectID: req.ProjectID, WorkflowRunID: req.WorkflowRunID, TaskID: req.TaskID,
		StageName: req.StageName, RoleAgentID: req.RoleAgentID, UnitID: req.UnitID, Kind: req.Kind, Name: req.Name,
		Version: current.Version + 1, ParentID: current.ID, StorageType: req.StorageType, StorageRef: req.StorageRef,
		MimeType: req.MimeType, SizeBytes: req.SizeBytes, ContentHash: req.ContentHash, PromptHash: req.PromptHash,
		Provider: req.Provider, Model: req.Model, IsCurrent: true, Metadata: cloneMetadata(req.Metadata),
	}
	if req.StorageType == StorageInline {
		created.InlineJSON = string(req.Data)
	}
	if req.RestoredFromID != "" {
		created.Metadata["restoredFromArtifactId"] = req.RestoredFromID
	}
	f.byID[created.ID] = created
	return created, nil
}

func cloneCreateRequestForRevisionTest(req *CreateArtifactRequest) *CreateArtifactRequest {
	cloned := *req
	cloned.Data = append([]byte(nil), req.Data...)
	cloned.Metadata = cloneMetadata(req.Metadata)
	return &cloned
}

func (f *revisionServiceFake) MarkDownstreamStale(_ context.Context, _, _, _ string) ([]string, error) {
	f.stale++
	return []string{"downstream"}, nil
}

var errRevisionTestNotFound = &revisionTestError{}

type revisionTestError struct{}

func (*revisionTestError) Error() string { return "not found" }

func cloneArtifactForRevisionTest(artifact *Artifact) *Artifact {
	copy := *artifact
	copy.Metadata = deepCloneRevisionMetadata(artifact.Metadata)
	return &copy
}

func deepCloneRevisionMetadata(metadata map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(metadata))
	for key, value := range metadata {
		switch typed := value.(type) {
		case map[string]interface{}:
			cloned[key] = deepCloneRevisionMetadata(typed)
		default:
			cloned[key] = typed
		}
	}
	return cloned
}
