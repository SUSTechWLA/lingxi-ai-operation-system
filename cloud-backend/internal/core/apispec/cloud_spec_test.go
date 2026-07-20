package apispec

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGeneratedTypeScriptMatchesCloudSpec(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "frontend", "src", "utils", "api-types.generated.ts")
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated TypeScript: %v", err)
	}
	expected := RenderTypeScript(BuildCloudSpec())
	if !bytes.Equal(actual, expected) {
		t.Fatalf("generated TypeScript is stale; run make gen-ts from cloud-backend")
	}
}

func TestCloudSpec_CreatorRoutesMatchHandlers(t *testing.T) {
	spec := BuildCloudSpec()
	tests := []struct {
		method      string
		path        string
		request     string
		response    string
		idempotency bool
	}{
		{"GET", "/api/video-projects/:id/creation-view", "", "CreationViewResponse", false},
		{"GET", "/api/video-projects/:id/steps/:stepId/versions", "", "StepVersionsResponse", false},
		{"POST", "/api/video-projects/:id/steps/:stepId/revision-impact", "StepRevisionRequest", "StepImpactResponse", false},
		{"POST", "/api/video-projects/:id/steps/:stepId/revisions", "StepRevisionRequest", "StepMutationResponse", true},
		{"POST", "/api/video-projects/:id/steps/:stepId/confirm", "StepConfirmRequest", "CreationViewResponse", false},
		{"POST", "/api/video-projects/:id/steps/:stepId/versions/:version/restore", "StepRestoreRequest", "StepMutationResponse", true},
		{"POST", "/api/video-projects/:id/materials", "RegisterProjectMaterialRequest", "ProjectMaterialResponse", false},
		{"GET", "/api/video-projects/:id/shots", "", "ShotPageResponse", false},
		{"GET", "/api/video-projects/:id/shots/summary", "", "ShotSummaryResponse", false},
		{"GET", "/api/video-projects/:id/shots/:shotId/workspace", "", "ShotWorkspaceResponse", false},
		{"GET", "/api/video-projects/:id/shots/:shotId/history", "", "ShotHistoryResponse", false},
		{"POST", "/api/video-projects/:id/shots/:shotId/regeneration-impact", "", "ShotImpactResponse", false},
		{"POST", "/api/video-projects/:id/shots/:shotId/regenerations", "ShotRegenerationRequest", "ShotRegenerationResponse", true},
		{"POST", "/api/video-projects/:id/shots/:shotId/candidates/:candidateId/accept", "CandidateAcceptRequest", "ShotUnitResponse", true},
		{"POST", "/api/video-projects/:id/shots/:shotId/candidates/:candidateId/restore", "CandidateRestoreRequest", "ShotUnitResponse", true},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			op := operationForMethod(t, spec.Paths[tt.path], tt.method)
			if op == nil {
				t.Fatalf("creator route %s %s missing", tt.method, tt.path)
			}
			if len(op.Tags) == 0 || op.Tags[0] != "Video Projects" {
				t.Fatalf("tag = %v", op.Tags)
			}
			assertRequiredHeader(t, op, "Authorization", true)
			assertRequiredHeader(t, op, "Idempotency-Key", tt.idempotency)
			if _, ok := op.Responses["401"]; !ok {
				t.Fatal("authenticated creator route missing 401 response")
			}
			assertSchemaRef(t, op, tt.request, tt.response)
		})
	}
}

func TestCloudSpec_CreatorMutationSchemasAreStrict(t *testing.T) {
	spec := BuildCloudSpec()
	wantRequired := map[string][]string{
		"StepRevisionRequest":            {"artifactId", "baseVersion", "mode"},
		"StepConfirmRequest":             {"artifactId"},
		"StepRestoreRequest":             {"baseVersion"},
		"RegisterProjectMaterialRequest": {"name", "kind", "storageRef", "mimeType", "sizeBytes", "contentHash"},
		"ShotRegenerationRequest":        {"baseVersion", "scope", "locks"},
		"CandidateAcceptRequest":         {"baseVersion", "scope", "locks"},
		"CandidateRestoreRequest":        {"baseVersion", "scope", "locks"},
	}
	for name, required := range wantRequired {
		schema := spec.Components.Schemas[name]
		if schema == nil {
			t.Errorf("schema %s missing", name)
			continue
		}
		if !reflect.DeepEqual(schema.Required, required) {
			t.Errorf("schema %s required = %v, want %v", name, schema.Required, required)
		}
	}

	assertSchemaEnum(t, spec, "CreatorStep", "id", []any{"requirements", "direction", "script", "shots", "preview", "delivery"})
	assertSchemaEnum(t, spec, "CreatorStep", "state", []any{"not_started", "generating", "needs_review", "confirmed", "needs_attention", "failed"})
	assertSchemaEnum(t, spec, "StepRevisionRequest", "mode", []any{"direct", "instruction"})
	assertSchemaEnum(t, spec, "ArtifactSelection", "kind", []any{"rect", "time"})
	assertSchemaEnum(t, spec, "ShotRegenerationRequest", "scope", []any{"prompt", "reference", "base_media", "overlay", "audio_alignment", "full_shot"})
	assertSchemaEnum(t, spec, "CandidateAcceptRequest", "scope", []any{"candidate_accept"})
	assertSchemaEnum(t, spec, "CandidateRestoreRequest", "scope", []any{"candidate_restore"})
}

func TestCloudSpec_ShotPageDocumentsActualQuery(t *testing.T) {
	op := operationForMethod(t, BuildCloudSpec().Paths["/api/video-projects/:id/shots"], "GET")
	got := map[string]bool{}
	for _, parameter := range op.Parameters {
		if parameter.In == "query" {
			got[parameter.Name] = parameter.Required
		}
	}
	want := map[string]bool{"cursor": false, "limit": false, "status": false, "chapter": false, "query": false}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("query parameters = %v, want %v", got, want)
	}
}

func operationForMethod(t *testing.T, item *PathItem, method string) *Operation {
	t.Helper()
	if item == nil {
		return nil
	}
	switch method {
	case "GET":
		return item.Get
	case "POST":
		return item.Post
	default:
		t.Fatalf("unsupported method %s", method)
		return nil
	}
}

func assertRequiredHeader(t *testing.T, op *Operation, name string, want bool) {
	t.Helper()
	for _, parameter := range op.Parameters {
		if parameter.In == "header" && parameter.Name == name {
			if !want {
				t.Fatalf("unexpected required header %s", name)
			}
			if !parameter.Required {
				t.Fatalf("header %s is not required", name)
			}
			return
		}
	}
	if want {
		t.Fatalf("required header %s missing", name)
	}
}

func assertSchemaRef(t *testing.T, op *Operation, request, response string) {
	t.Helper()
	if request == "" {
		if op.RequestBody != nil {
			t.Fatalf("unexpected request body")
		}
	} else {
		if op.RequestBody == nil || !op.RequestBody.Required {
			t.Fatalf("required request body %s missing", request)
		}
		got := op.RequestBody.Content["application/json"].Schema.Ref
		if want := "#/components/schemas/" + request; got != want {
			t.Fatalf("request schema = %q, want %q", got, want)
		}
	}
	got := op.Responses["200"].Content["application/json"].Schema.Ref
	if want := "#/components/schemas/" + response; got != want {
		t.Fatalf("response schema = %q, want %q", got, want)
	}
}

func assertSchemaEnum(t *testing.T, spec *Spec, schemaName, property string, want []any) {
	t.Helper()
	schema := spec.Components.Schemas[schemaName]
	if schema == nil || schema.Properties[property] == nil || schema.Properties[property].Schema == nil {
		t.Fatalf("schema property %s.%s missing", schemaName, property)
	}
	if got := schema.Properties[property].Schema.Enum; !reflect.DeepEqual(got, want) {
		t.Fatalf("schema enum %s.%s = %v, want %v", schemaName, property, got, want)
	}
}

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

func TestBuildCloudSpec_ExposesClosedBetaRuntimeRoutes(t *testing.T) {
	spec := BuildCloudSpec()
	required := map[string]string{
		"/api/agent/runs/:runId/reviews/:reviewId/submit-edited": "POST",
		"/api/agent/runs/:runId/reviews/:reviewId/regenerate":    "POST",
		"/api/agent/runs/:runId/cancel":                          "POST",
		"/api/video/preflight":                                   "GET",
		"/api/video-projects/:id/workflow-runs/:rid/checkpoints": "GET",
		"/api/video-projects/:id/workflow-runs/:rid/recover":     "POST",
	}
	for path, method := range required {
		item := spec.Paths[path]
		if item == nil {
			t.Fatalf("closed beta route %q missing from cloud spec", path)
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
			t.Fatalf("closed beta route %s %s missing operation", method, path)
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
