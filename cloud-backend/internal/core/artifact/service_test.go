package artifact

import (
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
