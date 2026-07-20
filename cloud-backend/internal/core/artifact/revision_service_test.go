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

func TestRevisionServiceCopiesStructuredCreatorProvenanceIntoNewArtifact(t *testing.T) {
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	revisions := NewRevisionService(repo)
	selection := map[string]interface{}{"kind": "time", "startMs": int64(100), "endMs": int64(900)}

	_, err := revisions.Revise(context.Background(), ReviseRequest{
		ArtifactID: "artifact-v1", DirectContent: []byte("direct replacement"),
		Provenance: map[string]interface{}{"mode": "direct", "baseVersion": 1, "selection": selection},
	})
	if err != nil {
		t.Fatalf("Revise error: %v", err)
	}
	want := map[string]interface{}{"kind": "time", "startMs": int64(100), "endMs": int64(900)}
	if got := repo.lastCreate.Metadata["selection"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("selection metadata = %#v, want %#v", got, want)
	}
	selection["startMs"] = int64(500)
	if got := repo.lastCreate.Metadata["selection"].(map[string]interface{})["startMs"]; got != int64(100) {
		t.Fatalf("stored provenance aliased caller selection: %#v", repo.lastCreate.Metadata["selection"])
	}
}

func TestRevisionServiceDirectIdenticalContentForcesNewVersion(t *testing.T) {
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	revisions := NewRevisionService(repo)

	result, err := revisions.Revise(context.Background(), ReviseRequest{
		ArtifactID: "artifact-v1", Message: "keep content", DirectContent: []byte("original content"),
	})
	if err != nil {
		t.Fatalf("Revise error: %v", err)
	}
	if !repo.lastCreate.ForceNewVersion || result.Artifact.ID == "artifact-v1" || result.Artifact.Version != 2 || result.Artifact.ParentID != "artifact-v1" {
		t.Fatalf("identical direct revision did not create a new current child: request=%+v artifact=%+v", repo.lastCreate, result.Artifact)
	}
	if repo.stale != 1 {
		t.Fatalf("identical direct revision stale calls = %d, want 1", repo.stale)
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

func TestRevisionServiceIdenticalGeneratedContentForcesNewVersion(t *testing.T) {
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	revisions := NewRevisionService(repo)
	revisions.SetConfig("", func(context.Context, string, string, ReviseLLMOptions) (string, error) {
		return "original content", nil
	})

	result, err := revisions.Revise(context.Background(), ReviseRequest{ArtifactID: "artifact-v1", Message: "keep content"})
	if err != nil {
		t.Fatalf("Revise error: %v", err)
	}
	if !repo.lastCreate.ForceNewVersion || result.Artifact.ID == "artifact-v1" || result.Artifact.Version != 2 || result.Artifact.ParentID != "artifact-v1" {
		t.Fatalf("identical generated revision did not create a new current child: request=%+v artifact=%+v", repo.lastCreate, result.Artifact)
	}
	if repo.stale != 1 {
		t.Fatalf("identical generated revision stale calls = %d, want 1", repo.stale)
	}
}

func TestRevisionServiceFailedCreateDoesNotMarkDownstreamStale(t *testing.T) {
	repo := newRevisionServiceFake(t, revisionTestArtifact())
	repo.createErr = errRevisionTestCreate
	revisions := NewRevisionService(repo)

	result, err := revisions.Revise(context.Background(), ReviseRequest{
		ArtifactID: "artifact-v1", Message: "replace", DirectContent: []byte("replacement"),
	})
	if err != errRevisionTestCreate || result != nil {
		t.Fatalf("failed revision = result=%+v err=%v", result, err)
	}
	if repo.stale != 0 {
		t.Fatalf("failed revision stale calls = %d, want 0", repo.stale)
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

func TestRevisionServiceRestorePreservesRemoteStorageTypes(t *testing.T) {
	for _, storageType := range []string{StorageMinIO, "url"} {
		t.Run(storageType, func(t *testing.T) {
			historical := revisionTestArtifact()
			historical.StorageType = storageType
			historical.StorageRef = "https://media.example.test/restored.mp4"
			historical.ContentHash = "historical-hash"
			historical.InlineJSON = ""
			historical.IsCurrent = false
			current := cloneArtifactForRevisionTest(historical)
			current.ID, current.Version, current.IsCurrent = "artifact-v3", 3, true
			repo := newRevisionServiceFake(t, historical, current)

			result, err := NewRevisionService(repo).Restore(context.Background(), RestoreRequest{ArtifactID: historical.ID})
			if err != nil {
				t.Fatalf("Restore error: %v", err)
			}
			if result.Artifact.StorageType != storageType || result.Artifact.StorageRef != historical.StorageRef || result.Artifact.ContentHash != historical.ContentHash {
				t.Fatalf("remote restore identity = %+v", result.Artifact)
			}
			if len(repo.lastCreate.Data) != 0 {
				t.Fatalf("remote restore must retain external bytes by reference, got %q", repo.lastCreate.Data)
			}
		})
	}
}

func TestForcedRestoreRecordPreservesRemoteMediaURLCompatibility(t *testing.T) {
	for _, storageType := range []string{StorageMinIO, "url"} {
		t.Run(storageType, func(t *testing.T) {
			ref := "https://media.example.test/restored.mp4"
			restored := buildArtifactRecord(&CreateArtifactRequest{
				ProjectID: "project-1", StageName: "script", UnitID: "main", Kind: KindVideo, Name: "restored.mp4",
				StorageType: storageType, StorageRef: ref, ContentHash: "historical-hash", ForceNewVersion: true,
			}, 4, "artifact-v3")
			if restored.StorageType != storageType || restored.StorageRef != ref {
				t.Fatalf("forced record changed remote storage: %+v", restored)
			}
			_, mediaURL, mediaURLs := artifactContent(restored)
			if mediaURL != ref || !reflect.DeepEqual(mediaURLs, []string{ref}) {
				t.Fatalf("legacy remote media response = mediaURL=%q mediaURLs=%v", mediaURL, mediaURLs)
			}
		})
	}
}

func TestRestoreDeepClonesTypedMutableMetadata(t *testing.T) {
	historical := revisionTestArtifact()
	historical.IsCurrent = false
	historical.Metadata = revisionTypedMetadata()
	current := *historical
	current.Metadata = revisionTypedMetadata()
	current.ID, current.Version, current.IsCurrent = "artifact-v3", 3, true
	originalHistorical := revisionTypedMetadata()
	originalCurrent := revisionTypedMetadata()
	repo := newRevisionServiceFake(t, historical, &current)

	result, err := NewRevisionService(repo).Restore(context.Background(), RestoreRequest{ArtifactID: historical.ID})
	if err != nil {
		t.Fatalf("Restore error: %v", err)
	}
	metadata := result.Artifact.Metadata
	metadata["bytes"].([]byte)[0] = 9
	metadata["strings"].([]string)[0] = "changed"
	metadata["typedMap"].(map[string]string)["owner"] = "changed"
	metadata["typedSlice"].([]revisionMetadataValue)[0].Labels[0] = "changed"
	metadata["pointer"].(*revisionMetadataValue).Labels[0] = "changed"
	if !reflect.DeepEqual(historical.Metadata, originalHistorical) {
		t.Fatalf("historical metadata was aliased: %+v", historical.Metadata)
	}
	if !reflect.DeepEqual(current.Metadata, originalCurrent) {
		t.Fatalf("current metadata was aliased: %+v", current.Metadata)
	}
}

func TestCloneMetadataPreservesCyclicValuesWithoutAliasing(t *testing.T) {
	cycle := map[string]interface{}{}
	cycle["self"] = cycle
	cloned := cloneMetadata(map[string]interface{}{"cycle": cycle})
	clonedCycle := cloned["cycle"].(map[string]interface{})
	clonedCycle["self"].(map[string]interface{})["changedThroughCycle"] = true
	if cycle["changedThroughCycle"] != nil {
		t.Fatal("cyclic metadata clone aliases the original map")
	}
	if clonedCycle["changedThroughCycle"] != true {
		t.Fatal("cyclic metadata clone did not preserve the cycle")
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

func TestRestoreUsesHistoricalBytesButCurrentExecutionIdentity(t *testing.T) {
	historical := revisionTestArtifact()
	historical.ID, historical.Version, historical.IsCurrent = "artifact-v1", 1, false
	historical.WorkflowRunID, historical.TaskID, historical.RoleAgentID = "old-run", "old-task", "old-role"
	historical.Metadata = map[string]interface{}{"producedByNode": "old-node", "producedByTool": "old-tool", "producedByRole": "old-role"}
	current := cloneArtifactForRevisionTest(historical)
	current.ID, current.Version, current.IsCurrent = "artifact-v3", 3, true
	current.WorkflowRunID, current.TaskID, current.RoleAgentID = "new-run", "new-task", "new-role"
	current.ProducedByNode, current.ProducedByTool, current.ProducedByRole = "new-node", "new-tool", "new-role"
	current.Metadata = map[string]interface{}{}
	repo := newRevisionServiceFake(t, historical, current)

	if _, err := NewRevisionService(repo).Restore(context.Background(), RestoreRequest{ArtifactID: historical.ID}); err != nil {
		t.Fatal(err)
	}
	got := repo.lastCreate
	if got.WorkflowRunID != "new-run" || got.TaskID != "new-task" || got.RoleAgentID != "new-role" {
		t.Fatalf("execution identity = run=%q task=%q role=%q", got.WorkflowRunID, got.TaskID, got.RoleAgentID)
	}
	if got.Metadata["producedByNode"] != "new-node" || got.Metadata["producedByTool"] != "new-tool" || got.ContentHash != historical.ContentHash {
		t.Fatalf("request=%+v metadata=%+v", got, got.Metadata)
	}
}

type revisionServiceFake struct {
	t          *testing.T
	byID       map[string]*Artifact
	stale      int
	lastCreate *CreateArtifactRequest
	createErr  error
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
	if f.createErr != nil {
		return nil, f.createErr
	}
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
var errRevisionTestCreate = &revisionTestCreateError{}

type revisionTestError struct{}

func (*revisionTestError) Error() string { return "not found" }

type revisionTestCreateError struct{}

func (*revisionTestCreateError) Error() string { return "create failed" }

type revisionMetadataValue struct {
	Labels []string
}

func revisionTypedMetadata() map[string]interface{} {
	return map[string]interface{}{
		"bytes":    []byte{1, 2},
		"strings":  []string{"historical"},
		"typedMap": map[string]string{"owner": "historical"},
		"typedSlice": []revisionMetadataValue{{
			Labels: []string{"historical"},
		}},
		"pointer": &revisionMetadataValue{Labels: []string{"historical"}},
	}
}

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
