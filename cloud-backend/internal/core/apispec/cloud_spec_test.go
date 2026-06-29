package apispec

import (
	"strings"
	"testing"
)

func TestBuildCloudSpec_NoDuplicateOperationIDs(t *testing.T) {
	spec := BuildCloudSpec()
	seen := map[string]string{} // opId → firstPath
	for path, item := range spec.Paths {
		for _, op := range []struct {
			method string
			o      *Operation
		}{
			{"GET", item.Get},
			{"POST", item.Post},
			{"PUT", item.Put},
			{"DELETE", item.Delete},
			{"PATCH", item.Patch},
		} {
			if op.o == nil {
				continue
			}
			if op.o.OperationID == "" {
				t.Errorf("empty operationId at %s %s", op.method, path)
			}
			if first, ok := seen[op.o.OperationID]; ok {
				t.Errorf("duplicate operationId %q: first at %s, duplicate at %s %s",
					op.o.OperationID, first, op.method, path)
			}
			seen[op.o.OperationID] = op.method + " " + path
		}
	}
	t.Logf("Total operations: %d", len(seen))
}

func TestBuildCloudSpec_AllPathsHave200Response(t *testing.T) {
	spec := BuildCloudSpec()
	for path, item := range spec.Paths {
		for method, op := range map[string]*Operation{
			"GET": item.Get, "POST": item.Post, "PUT": item.Put,
			"DELETE": item.Delete, "PATCH": item.Patch,
		} {
			if op == nil {
				continue
			}
			if _, ok := op.Responses["200"]; !ok {
				t.Errorf("%s %s missing 200 response", method, path)
			}
		}
	}
}

func TestBuildCloudSpec_DoesNotExposeRemovedBusinessLines(t *testing.T) {
	spec := BuildCloudSpec()

	for _, tag := range spec.Tags {
		if tag.Name == "Bid" || tag.Name == "Chat" {
			t.Fatalf("removed business line tag %q must not be exposed", tag.Name)
		}
	}

	for _, removed := range []string{"bid", "chat"} {
		prefix := "/api/" + removed
		for path := range spec.Paths {
			if strings.HasPrefix(path, prefix) {
				t.Fatalf("removed business line path %q must not be exposed", path)
			}
		}
	}
}

func TestBuildCloudSpec_ExposesVideoScopedAssistant(t *testing.T) {
	spec := BuildCloudSpec()
	for _, path := range []string{
		"/api/video-projects/:id/assistant/message",
		"/api/video-projects/:id/assistant/revise",
		"/api/video-projects/:id/assistant/explain-stage",
	} {
		item := spec.Paths[path]
		if item == nil || item.Post == nil {
			t.Fatalf("video project assistant route %q must be exposed as POST", path)
		}
		if len(item.Post.Tags) == 0 || item.Post.Tags[0] != "Video Project Assistant" {
			t.Fatalf("video project assistant route %q tag = %v", path, item.Post.Tags)
		}
	}
}

func TestBuildCloudSpec_ExposesShotDrivenVideoRoutes(t *testing.T) {
	spec := BuildCloudSpec()
	required := map[string]string{
		"/api/video-projects/:id/spec":                                 "GET",
		"/api/video-projects/:id/spec/generate":                        "POST",
		"/api/video-projects/:id/shots":                                "GET",
		"/api/video-projects/:id/shots/generate":                       "POST",
		"/api/video-projects/:id/shots/:shotId/lock":                   "POST",
		"/api/video-projects/:id/shots/:shotId/regenerate":             "POST",
		"/api/video-projects/:id/shots/:shotId/visual-plan/generate":   "POST",
		"/api/video-projects/:id/shots/:shotId/render-strategy/decide": "POST",
		"/api/video-projects/:id/shots/:shotId/text-layers/generate":   "POST",
		"/api/video-projects/:id/assemble":                             "POST",
		"/api/video-projects/:id/publish-package/generate":             "POST",
	}
	for path, method := range required {
		item := spec.Paths[path]
		if item == nil {
			t.Fatalf("shot-driven route %q missing", path)
		}
		var op *Operation
		switch method {
		case "GET":
			op = item.Get
		case "POST":
			op = item.Post
		default:
			t.Fatalf("unsupported method %s", method)
		}
		if op == nil {
			t.Fatalf("shot-driven route %s %s missing operation", method, path)
		}
		if len(op.Tags) == 0 || op.Tags[0] != "Video Projects" {
			t.Fatalf("shot-driven route %s %s tag = %v", method, path, op.Tags)
		}
	}
}

func TestBuildCloudSpec_SnapshotPathCount(t *testing.T) {
	spec := BuildCloudSpec()
	// Snapshot: total number of unique paths (should grow with new endpoints)
	const expectedMin = 60
	if len(spec.Paths) < expectedMin {
		t.Fatalf("Expected at least %d paths, got %d — spec may be missing entries",
			expectedMin, len(spec.Paths))
	}
	t.Logf("Path count: %d", len(spec.Paths))
}
