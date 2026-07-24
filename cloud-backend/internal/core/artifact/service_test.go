package artifact

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestHashContent(t *testing.T) {
	h1 := HashContent([]byte("hello"))
	h2 := HashContent([]byte("hello"))
	h3 := HashContent([]byte("world"))

	if h1 != h2 {
		t.Error("same content should produce same hash")
	}
	if h1 == h3 {
		t.Error("different content should produce different hash")
	}
	if h1 == "" {
		t.Error("hash should not be empty")
	}
}

func TestArtifactModel(t *testing.T) {
	a := &Artifact{
		ProjectID:   "proj-1",
		StageName:   "script",
		UnitID:      "shot-01",
		Kind:        KindJSON,
		Version:     1,
		IsCurrent:   true,
		StorageType: "inline",
		ContentHash: "abc123",
	}

	if a.ProjectID != "proj-1" {
		t.Errorf("expected proj-1, got %s", a.ProjectID)
	}
	if a.StageName != "script" {
		t.Errorf("expected script, got %s", a.StageName)
	}
	if !a.IsCurrent {
		t.Error("expected isCurrent=true")
	}
}

func TestArtifactKindValues(t *testing.T) {
	kinds := map[ArtifactKind]bool{
		KindJSON:     true,
		KindMarkdown: true,
		KindImage:    true,
		KindAudio:    true,
		KindVideo:    true,
		KindBundle:   true,
		KindLog:      true,
	}

	for k := range kinds {
		if string(k) == "" {
			t.Error("artifact kind should not be empty")
		}
	}
}

func TestCreateArtifactRequest(t *testing.T) {
	req := &CreateArtifactRequest{
		ProjectID:   "proj-1",
		StageName:   "keyframe",
		UnitID:      "shot-01",
		Kind:        KindImage,
		Name:        "keyframe_01",
		StorageType: "inline",
		Data:        []byte(`{"url":"test.png"}`),
		MimeType:    "application/json",
	}

	if req.StorageType != "inline" {
		t.Errorf("expected inline, got %s", req.StorageType)
	}
	if len(req.Data) == 0 {
		t.Error("data should not be empty")
	}
	if req.ContentHash == "" {
		// Auto-computed in service when not provided
		t.Log("contentHash auto-computed by service layer")
	}
}

func TestArtifactLineageCandidateUsesAuthorizedExpectationNotObservedCurrent(t *testing.T) {
	req := &CreateArtifactRequest{
		ExpectedParentID:      "artifact-v2",
		ExpectedParentVersion: 2,
	}
	competingCurrent := &Artifact{ID: "artifact-v3", Version: 3}

	version, parentID, err := artifactLineageCandidate(req, competingCurrent)
	if err != nil {
		t.Fatalf("artifactLineageCandidate() error = %v", err)
	}
	if version != 3 || parentID != "artifact-v2" {
		t.Fatalf("candidate lineage = %s@%d, want artifact-v2@3", parentID, version)
	}
}

func TestArtifactLineageCandidateRejectsPartialExpectation(t *testing.T) {
	for _, req := range []*CreateArtifactRequest{
		{ExpectedParentID: "artifact-v2"},
		{ExpectedParentVersion: 2},
		{ExpectedParentID: "artifact-v2", ExpectedParentVersion: -1},
	} {
		if _, _, err := artifactLineageCandidate(req, &Artifact{ID: "artifact-v3", Version: 3}); !errors.Is(err, ErrArtifactVersionConflict) {
			t.Fatalf("artifactLineageCandidate(%+v) error = %v, want version conflict", req, err)
		}
	}
}

func TestCreateArtifactExpectedParentConflictCannotInsertStaleCandidate(t *testing.T) {
	competing := &Artifact{
		ID: "artifact-v2", ProjectID: "project-1", StageName: "keyframe", UnitID: "shot-1",
		Version: 2, ParentID: "artifact-v1", IsCurrent: true,
	}
	store := &artifactCreationRaceStore{current: competing}
	created, err := createArtifactWithStore(context.Background(), store, &CreateArtifactRequest{
		ID: "artifact-replacement", ProjectID: competing.ProjectID, StageName: competing.StageName, UnitID: competing.UnitID,
		Kind: KindImage, Name: "shot-1.webp", StorageType: StorageLocal,
		StorageRef: "local://projects/project-1/materials/replacement", MimeType: "image/webp",
		ContentHash: "sha256:replacement", ForceNewVersion: true,
		ExpectedParentID: "artifact-v1", ExpectedParentVersion: 1,
	})
	if !errors.Is(err, ErrArtifactVersionConflict) || created != nil {
		t.Fatalf("createArtifactWithStore() artifact=%+v error=%v, want version conflict", created, err)
	}
	if store.inserted != nil || store.current != competing || store.findCurrentCalls != 0 {
		t.Fatalf("stale candidate changed lineage: inserted=%+v current=%+v currentReads=%d", store.inserted, store.current, store.findCurrentCalls)
	}
}

type artifactCreationRaceStore struct {
	current          *Artifact
	inserted         *Artifact
	findCurrentCalls int
}

func (s *artifactCreationRaceStore) FindByHash(context.Context, string, string, string, string) (*Artifact, error) {
	return nil, errors.New("not found")
}

func (s *artifactCreationRaceStore) FindCurrent(context.Context, string, string, string) (*Artifact, error) {
	s.findCurrentCalls++
	return s.current, nil
}

func (s *artifactCreationRaceStore) Save(_ context.Context, candidate *Artifact) error {
	if s.current == nil || candidate.ParentID != s.current.ID || candidate.Version != s.current.Version+1 {
		return ErrArtifactVersionConflict
	}
	s.current.IsCurrent = false
	s.inserted = candidate
	s.current = candidate
	return nil
}

func TestBuildArtifactRecordStoresOnlyLocalMetadata(t *testing.T) {
	req := &CreateArtifactRequest{
		ProjectID:   "proj-1",
		StageName:   "script",
		UnitID:      "content",
		Kind:        KindMarkdown,
		Name:        "script.md",
		StorageType: "inline",
		Data:        []byte("## 用户脚本\n不能进入云端数据库。"),
		MimeType:    "text/markdown; charset=utf-8",
	}

	record := buildArtifactRecord(req, 2, "art-1")

	if record.StorageType != StorageLocal {
		t.Fatalf("storage type = %q, want %q", record.StorageType, StorageLocal)
	}
	if record.InlineJSON != "" {
		t.Fatalf("inline json should stay empty for local artifacts, got %q", record.InlineJSON)
	}
	if record.StorageRef == "" {
		t.Fatalf("storage ref should point to the local artifact location")
	}
	if record.SizeBytes != int64(len(req.Data)) {
		t.Fatalf("size bytes = %d, want %d", record.SizeBytes, len(req.Data))
	}
	if record.ContentHash == "" {
		t.Fatalf("content hash should be computed for local idempotency")
	}
	if stored, ok := record.Metadata["cloudPayloadStored"].(bool); !ok || stored {
		t.Fatalf("metadata should mark cloud payload as not stored: %+v", record.Metadata)
	}
}

func TestBuildArtifactRecordWorkflowNodeDoesNotStoreInlinePayload(t *testing.T) {
	req := &CreateArtifactRequest{
		ProjectID:   "proj-1",
		StageName:   "proposal",
		UnitID:      "proposal_generator",
		Kind:        KindMarkdown,
		Name:        "proposal.md",
		StorageType: StorageLocal,
		Data:        []byte("# 创作方案\n这里是待审核正文。"),
		MimeType:    "text/markdown; charset=utf-8",
		Provider:    "workflow-node",
	}

	record := buildArtifactRecord(req, 1, "")

	if record.StorageType != StorageLocal {
		t.Fatalf("workflow-node artifact storage type = %q, want %q", record.StorageType, StorageLocal)
	}
	if record.InlineJSON != "" {
		t.Fatalf("workflow-node payload must not be stored inline, got %q", record.InlineJSON)
	}
	if stored, ok := record.Metadata["cloudPayloadStored"].(bool); !ok || stored {
		t.Fatalf("workflow-node artifact should remain cloudPayloadStored=false: %+v", record.Metadata)
	}
	if localOnly, ok := record.Metadata["localOnly"].(bool); !ok || !localOnly {
		t.Fatalf("workflow-node artifact should remain localOnly=true: %+v", record.Metadata)
	}
}

func TestBuildArtifactRecordKeepsRevisionPayloadReviewable(t *testing.T) {
	req := &CreateArtifactRequest{
		ProjectID:   "proj-1",
		StageName:   "script",
		UnitID:      "content",
		Kind:        KindMarkdown,
		Name:        "script.md",
		StorageType: StorageInline,
		Data:        []byte("## 新稿\n开头更强。"),
		MimeType:    "text/markdown; charset=utf-8",
		Provider:    "artifact-revision",
	}

	record := buildArtifactRecord(req, 2, "art-1")

	if record.StorageType != StorageInline {
		t.Fatalf("revision storage type = %q, want %q", record.StorageType, StorageInline)
	}
	if record.InlineJSON != "## 新稿\n开头更强。" {
		t.Fatalf("revision payload should be reviewable inline, got %q", record.InlineJSON)
	}
	if stored, ok := record.Metadata["cloudPayloadStored"].(bool); !ok || !stored {
		t.Fatalf("revision metadata should mark cloudPayloadStored=true: %+v", record.Metadata)
	}
	if localOnly, ok := record.Metadata["localOnly"].(bool); !ok || localOnly {
		t.Fatalf("revision metadata should mark localOnly=false: %+v", record.Metadata)
	}
}

func TestBuildArtifactRecordKeepsExternalGenerationRequestReviewable(t *testing.T) {
	req := &CreateArtifactRequest{
		ProjectID:   "proj-1",
		StageName:   "external_generation_request",
		UnitID:      "shot-1",
		Kind:        KindJSON,
		Name:        "external_generation_request.json",
		StorageType: StorageInline,
		Data:        []byte(`{"prompt":"复制到外部平台生成视频","referenceImageLimit":6,"promptCharLimit":2000}`),
		MimeType:    "application/json",
		Provider:    "external-generation-request",
	}

	record := buildArtifactRecord(req, 1, "")

	if record.StorageType != StorageInline {
		t.Fatalf("external generation request storage type = %q, want %q", record.StorageType, StorageInline)
	}
	if record.InlineJSON == "" {
		t.Fatalf("external generation request must remain reviewable inline")
	}
	if stored, ok := record.Metadata["cloudPayloadStored"].(bool); !ok || !stored {
		t.Fatalf("request metadata should mark cloudPayloadStored=true: %+v", record.Metadata)
	}
	if localOnly, ok := record.Metadata["localOnly"].(bool); !ok || localOnly {
		t.Fatalf("request metadata should mark localOnly=false: %+v", record.Metadata)
	}
}

func TestBuildArtifactRecordKeepsVideoCreationProfileReviewable(t *testing.T) {
	req := &CreateArtifactRequest{
		ProjectID:   "proj-1",
		StageName:   "profile_selection",
		UnitID:      "video-creation-profile",
		Kind:        ArtifactKind("VIDEO_CREATION_PROFILE"),
		Name:        "video_creation_profile.json",
		StorageType: StorageInline,
		Data:        []byte(`{"profileId":"talking_head"}`),
		MimeType:    "application/json",
		Provider:    "video-creation-profile",
	}

	record := buildArtifactRecord(req, 1, "")

	if record.StorageType != StorageInline {
		t.Fatalf("video creation profile storage type = %q, want %q", record.StorageType, StorageInline)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(record.InlineJSON), &decoded); err != nil {
		t.Fatalf("video creation profile inline JSON should decode: %v; inline=%q", err, record.InlineJSON)
	}
	if decoded["profileId"] != "talking_head" {
		t.Fatalf("video creation profile inline profileId = %v, want talking_head", decoded["profileId"])
	}
	if stored, ok := record.Metadata["cloudPayloadStored"].(bool); !ok || !stored {
		t.Fatalf("video creation profile metadata should mark cloudPayloadStored=true: %+v", record.Metadata)
	}
	if localOnly, ok := record.Metadata["localOnly"].(bool); !ok || localOnly {
		t.Fatalf("video creation profile metadata should mark localOnly=false: %+v", record.Metadata)
	}
}

func TestBuildArtifactRecordKeepsTimeWindowPlanReviewable(t *testing.T) {
	req := &CreateArtifactRequest{
		ProjectID:   "proj-1",
		StageName:   "time_window",
		UnitID:      "time_window",
		Kind:        ArtifactKind("TIME_WINDOW_PLAN"),
		Name:        "time_window_plan.json",
		StorageType: StorageInline,
		Data:        []byte(`{"profileId":"cinematic_story","windows":[{"id":"SHOT_01_TW_01","durationSec":10}]}`),
		MimeType:    "application/json",
		Provider:    "time-window-plan",
	}

	record := buildArtifactRecord(req, 1, "")

	if record.StorageType != StorageInline {
		t.Fatalf("time window plan storage type = %q, want %q", record.StorageType, StorageInline)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(record.InlineJSON), &decoded); err != nil {
		t.Fatalf("time window plan inline JSON should decode: %v; inline=%q", err, record.InlineJSON)
	}
	if _, ok := decoded["windows"].([]interface{}); !ok {
		t.Fatalf("time window plan inline data should include windows, got %+v", decoded)
	}
	if stored, ok := record.Metadata["cloudPayloadStored"].(bool); !ok || !stored {
		t.Fatalf("time window plan metadata should mark cloudPayloadStored=true: %+v", record.Metadata)
	}
	if localOnly, ok := record.Metadata["localOnly"].(bool); !ok || localOnly {
		t.Fatalf("time window plan metadata should mark localOnly=false: %+v", record.Metadata)
	}
}
