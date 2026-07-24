package apispec

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
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
		{"POST", "/api/video-projects/:id/steps/:stepId/revision-impact", "StepRevisionPreviewRequest", "StepImpactResponse", false},
		{"POST", "/api/video-projects/:id/steps/:stepId/revisions", "StepRevisionMutationRequest", "StepMutationResponse", true},
		{"POST", "/api/video-projects/:id/steps/:stepId/confirm", "StepConfirmRequest", "CreationViewResponse", false},
		{"POST", "/api/video-projects/:id/steps/:stepId/versions/:version/restore", "StepRestoreRequest", "StepMutationResponse", true},
		{"POST", "/api/video-projects/:id/steps/:stepId/regeneration-impact", "", "StepImpactResponse", false},
		{"POST", "/api/video-projects/:id/steps/:stepId/regenerations", "StepRegenerationRequest", "StepRegenerationResponse", true},
		{"POST", "/api/video-projects/:id/materials", "RegisterProjectMaterialRequest", "ProjectMaterialResponse", false},
		{"GET", "/api/video-projects/:id/shots", "", "ShotPageResponse", false},
		{"GET", "/api/video-projects/:id/shots/summary", "", "ShotSummaryResponse", false},
		{"GET", "/api/video-projects/:id/shots/:shotId/workspace", "", "ShotWorkspaceResponse", false},
		{"GET", "/api/video-projects/:id/shots/:shotId/history", "", "ShotHistoryResponse", false},
		{"POST", "/api/video-projects/:id/shots/:shotId/regeneration-impact", "", "ShotImpactResponse", false},
		{"POST", "/api/video-projects/:id/shots/:shotId/regenerations", "ShotRegenerationRequest", "ShotRegenerationResponse", true},
		{"POST", "/api/video-projects/:id/shots/:shotId/candidates/:candidateId/accept", "CandidateAcceptRequest", "CreatorShotUnitResponse", true},
		{"POST", "/api/video-projects/:id/shots/:shotId/candidates/:candidateId/restore", "CandidateRestoreRequest", "CreatorShotUnitResponse", true},
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

func TestCloudSpecAgentStartRunDocumentsOptionalIdempotencyKey(t *testing.T) {
	op := operationForMethod(t, BuildCloudSpec().Paths["/api/agent/runs"], "POST")
	for _, parameter := range op.Parameters {
		if parameter.In == "header" && parameter.Name == "Idempotency-Key" {
			if parameter.Required {
				t.Fatal("agent start idempotency key must remain optional for existing callers")
			}
			return
		}
	}
	t.Fatal("agent start route must document Idempotency-Key")
}

func TestCloudSpec_CreatorMutationSchemasAreStrict(t *testing.T) {
	spec := BuildCloudSpec()
	wantRequired := map[string][]string{
		"StepRevisionPreviewRequest":     {"artifactId", "baseVersion"},
		"StepConfirmRequest":             {"artifactId"},
		"StepRestoreRequest":             {"baseVersion", "confirmedAffectedShotIds"},
		"StepRegenerationRequest":        {"confirmedAffectedStepIds"},
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
	for _, property := range []string{"hasHistory", "attemptCount", "artifactCount", "isStale"} {
		if _, ok := spec.Components.Schemas["CreatorStep"].Properties[property]; !ok {
			t.Errorf("CreatorStep.%s missing", property)
		}
	}
	for _, property := range []string{"processTimeline", "stepArtifacts"} {
		if _, ok := spec.Components.Schemas["CreationView"].Properties[property]; !ok {
			t.Errorf("CreationView.%s missing", property)
		}
	}
	preview := spec.Components.Schemas["StepRevisionPreviewRequest"]
	if got := sortedPropertyNames(preview); !reflect.DeepEqual(got, []string{"artifactId", "baseVersion"}) {
		t.Fatalf("preview properties = %v", got)
	}
	mutation := spec.Components.Schemas["StepRevisionMutationRequest"]
	if mutation == nil || len(mutation.OneOf) != 3 {
		t.Fatalf("mutation oneOf = %+v", mutation)
	}
	for _, branch := range mutation.OneOf {
		if branch == nil || branch.Schema == nil {
			t.Fatal("mutation has non-inline branch")
		}
		mode := inlineProperty(t, branch.Schema, "mode")
		if len(mode.Enum) != 1 {
			t.Fatalf("mutation mode enum = %v", mode.Enum)
		}
		content := "replacementMaterial"
		if mode.Enum[0] == "direct" {
			content = "directContent"
		} else if mode.Enum[0] == "instruction" {
			content = "instruction"
		}
		want := []string{"artifactId", "baseVersion", "mode", content, "confirmedAffectedShotIds"}
		if !reflect.DeepEqual(branch.Schema.Required, want) {
			t.Fatalf("mutation %v required = %v, want %v", mode.Enum[0], branch.Schema.Required, want)
		}
	}
	assertSchemaEnum(t, spec, "ShotRegenerationRequest", "scope", []any{"prompt", "reference", "base_media", "overlay", "audio_alignment", "full_shot"})
	assertSchemaEnum(t, spec, "CandidateAcceptRequest", "scope", []any{"candidate_accept"})
	assertSchemaEnum(t, spec, "CandidateRestoreRequest", "scope", []any{"candidate_restore"})
}

func TestCloudSpecCreatorArtifactDescriptorAllowListsClassificationHints(t *testing.T) {
	descriptor := BuildCloudSpec().Components.Schemas["CreatorArtifactDescriptor"]
	for _, property := range []string{"artifactType", "generationKind", "relatedShotId"} {
		field := descriptor.Properties[property]
		if field == nil || field.Schema == nil || field.Schema.Type != "string" {
			t.Errorf("CreatorArtifactDescriptor.%s = %#v, want optional string", property, field)
		}
		for _, required := range descriptor.Required {
			if required == property {
				t.Errorf("CreatorArtifactDescriptor.%s must remain optional", property)
			}
		}
	}
	if _, ok := descriptor.Properties["metadata"]; ok {
		t.Fatal("CreatorArtifactDescriptor must not expose raw artifact metadata")
	}
}

func TestCloudSpec_CreatorParametersAndMaterialConstraints(t *testing.T) {
	spec := BuildCloudSpec()
	stepIDs := []any{"requirements", "direction", "script", "shots", "preview", "delivery"}
	for path, item := range spec.Paths {
		if !strings.Contains(path, "/steps/:stepId") {
			continue
		}
		for _, op := range []*Operation{item.Get, item.Post} {
			if op == nil {
				continue
			}
			parameter := findParameter(op, "path", "stepId")
			if parameter == nil || inlineParameterSchema(parameter) == nil || !reflect.DeepEqual(inlineParameterSchema(parameter).Enum, stepIDs) {
				t.Errorf("%s stepId enum = %+v", path, parameter)
			}
		}
	}
	restore := operationForMethod(t, spec.Paths["/api/video-projects/:id/steps/:stepId/versions/:version/restore"], "POST")
	version := findParameter(restore, "path", "version")
	if version == nil || inlineParameterSchema(version) == nil || inlineParameterSchema(version).Minimum == nil || *inlineParameterSchema(version).Minimum != 1 {
		t.Fatalf("restore version constraint = %+v", version)
	}

	request := spec.Components.Schemas["RegisterProjectMaterialRequest"]
	assertSchemaEnum(t, spec, "RegisterProjectMaterialRequest", "kind", []any{"image", "audio", "video", "document"})
	size := inlineProperty(t, request, "sizeBytes")
	if size.Minimum == nil || *size.Minimum != 0 {
		t.Fatalf("sizeBytes minimum = %v", size.Minimum)
	}
	hash := inlineProperty(t, request, "contentHash")
	if hash.Pattern != `^sha256:[A-Za-z0-9][A-Za-z0-9._-]*$` {
		t.Fatalf("contentHash pattern = %q", hash.Pattern)
	}
	if !strings.Contains(inlineProperty(t, request, "storageRef").Description, "project-scoped") ||
		!strings.Contains(inlineProperty(t, request, "mimeType").Description, "match kind") {
		t.Fatal("material storageRef/mimeType semantics are undocumented")
	}

	shots := operationForMethod(t, spec.Paths["/api/video-projects/:id/shots"], "GET")
	limit := findParameter(shots, "query", "limit")
	if limit == nil || inlineParameterSchema(limit) == nil || inlineParameterSchema(limit).Minimum == nil || *inlineParameterSchema(limit).Minimum != 1 || inlineParameterSchema(limit).Maximum == nil || *inlineParameterSchema(limit).Maximum != 50 || inlineParameterSchema(limit).Default != 24 {
		t.Fatalf("shot limit schema = %+v", limit)
	}
	status := findParameter(shots, "query", "status")
	if status == nil || inlineParameterSchema(status) == nil || !reflect.DeepEqual(inlineParameterSchema(status).Enum, []any{"all", "needs_attention", "confirmed", "generating", "failed", "pending", "approved", "rejected", "stale"}) {
		t.Fatalf("shot status schema = %+v", status)
	}
}

func TestCloudSpec_CreatorResponsesAndFiniteStates(t *testing.T) {
	spec := BuildCloudSpec()
	for _, name := range []string{
		"CreationViewResponse", "StepVersionsResponse", "StepImpactResponse", "StepMutationResponse",
		"ProjectMaterialResponse", "ShotPageResponse", "ShotSummaryResponse", "ShotWorkspaceResponse",
		"ShotHistoryResponse", "ShotImpactResponse", "ShotRegenerationResponse", "CreatorShotUnitResponse",
	} {
		schema := spec.Components.Schemas[name]
		if schema == nil || !reflect.DeepEqual(schema.Required, []string{"code", "message", "data"}) {
			t.Errorf("%s envelope required = %v", name, requiredOf(schema))
		}
	}
	creatorShot := spec.Components.Schemas["CreatorShotUnitResponse"]
	if data := inlineProperty(t, creatorShot, "data"); !reflect.DeepEqual(data.Required, []string{"shot"}) {
		t.Fatalf("creator shot data required = %v", data.Required)
	}

	assertSchemaEnum(t, spec, "CreatorTask", "scope", []any{"requirements", "direction", "script", "shots", "preview", "delivery"})
	assertSchemaEnum(t, spec, "CreatorTask", "status", []any{"generating", "running", "processing", "queued", "dispatching"})
	assertSchemaEnum(t, spec, "ShotListItem", "reviewStatus", []any{"pending", "approved", "rejected", "stale"})
	assertSchemaEnum(t, spec, "ShotListItem", "generationStatus", []any{"PLANNED", "GENERATING", "CANDIDATE_RENDERED", "SHOT_QA_RUNNING", "SHOT_QA_PASSED", "SHOT_QA_FAILED", "HUMAN_REVIEW_REQUIRED", "ACCEPTED_FOR_ASSEMBLY", "stale", "queued", "dispatching", "running", "failed", "cancelled"})
	assertSchemaEnum(t, spec, "ShotListItem", "qaStatus", []any{"PLANNED", "GENERATING", "CANDIDATE_RENDERED", "SHOT_QA_RUNNING", "SHOT_QA_PASSED", "SHOT_QA_FAILED", "HUMAN_REVIEW_REQUIRED", "ACCEPTED_FOR_ASSEMBLY", "stale"})
	assertSchemaEnum(t, spec, "ShotRegenerationTask", "status", []any{"queued", "dispatching", "running", "completed", "failed", "cancelled"})
	assertSchemaEnum(t, spec, "ShotRegenerationTask", "scope", []any{"prompt", "reference", "base_media", "overlay", "audio_alignment", "full_shot"})
	assertSchemaEnum(t, spec, "ShotCandidate", "status", []any{"CANDIDATE_RENDERED", "SHOT_QA_RUNNING", "SHOT_QA_PASSED", "SHOT_QA_FAILED", "HUMAN_REVIEW_REQUIRED", "ACCEPTED_FOR_ASSEMBLY", "stale"})
	assertSchemaEnum(t, spec, "ShotCandidate", "executionMode", []any{"unknown", "real", "fixture", "fallback", "placeholder"})
	assertSchemaEnum(t, spec, "ShotQAReport", "status", []any{"PLANNED", "GENERATING", "CANDIDATE_RENDERED", "SHOT_QA_RUNNING", "SHOT_QA_PASSED", "SHOT_QA_FAILED", "HUMAN_REVIEW_REQUIRED", "ACCEPTED_FOR_ASSEMBLY", "stale"})
	assertSchemaEnum(t, spec, "VideoProject", "mode", []any{"aigc_shot", "voice_visual", "cinematic_story"})
	assertSchemaEnum(t, spec, "VideoProject", "status", []any{"DRAFT", "RUNNING", "PAUSED", "COMPLETED", "ARCHIVED"})
	assertSchemaEnum(t, spec, "VideoProject", "generationMode", []any{"provider_api", "manual_import"})
	config := inlineProperty(t, spec.Components.Schemas["VideoProject"], "config")
	if config.Type != "object" || config.AdditionalProperties == nil {
		t.Fatalf("known VideoProject.config must be a free-form object: %+v", config)
	}
}

func TestCloudSpec_ExactRevisionUnionAndClosedMaterialRequest(t *testing.T) {
	spec := BuildCloudSpec()
	mutation := spec.Components.Schemas["StepRevisionMutationRequest"]
	if mutation == nil || len(mutation.OneOf) != 3 {
		t.Fatalf("mutation schema = %+v", mutation)
	}
	for _, branch := range mutation.OneOf {
		if branch == nil || branch.Schema == nil {
			t.Fatal("mutation branch must be inline")
		}
		assertSchemaClosed(t, branch.Schema)
		mode := inlineProperty(t, branch.Schema, "mode").Enum[0]
		if mode == "direct" && branch.Schema.Properties["instruction"] != nil {
			t.Fatal("direct branch exposes instruction")
		}
		if mode == "instruction" && branch.Schema.Properties["directContent"] != nil {
			t.Fatal("instruction branch exposes directContent")
		}
		if mode == "direct" && branch.Schema.Properties["modelProviders"] != nil {
			t.Fatal("direct branch must not accept model provider credentials")
		}
		if mode == "instruction" && branch.Schema.Properties["modelProviders"] == nil {
			t.Fatal("instruction branch must accept transient model provider credentials")
		}
		if mode == "replace" {
			for _, forbidden := range []string{"instruction", "directContent", "modelProviders"} {
				if branch.Schema.Properties[forbidden] != nil {
					t.Fatalf("replace branch exposes forbidden field %q", forbidden)
				}
			}
			replacement := inlineProperty(t, branch.Schema, "replacementMaterial")
			if replacement.Ref != "#/components/schemas/ReplacementMaterialIdentity" {
				t.Fatalf("replace branch replacementMaterial = %+v", replacement)
			}
		}
	}

	material := spec.Components.Schemas["RegisterProjectMaterialRequest"]
	assertSchemaClosed(t, material)
	wantProperties := []string{"contentHash", "kind", "mimeType", "name", "sizeBytes", "storageRef"}
	if got := sortedPropertyNames(material); !reflect.DeepEqual(got, wantProperties) {
		t.Fatalf("material properties = %v, want %v", got, wantProperties)
	}
	for _, forbidden := range []string{"bytes", "inlineBytes", "inlineJson", "storageType"} {
		if material.Properties[forbidden] != nil {
			t.Errorf("material request exposes forbidden field %q", forbidden)
		}
	}
}

func TestCloudSpec_ArtifactSelectionBranchesAreClosedAndDisjoint(t *testing.T) {
	selection := BuildCloudSpec().Components.Schemas["ArtifactSelection"]
	if selection == nil || len(selection.OneOf) != 3 {
		t.Fatalf("selection schema = %+v", selection)
	}
	for _, branch := range selection.OneOf {
		if branch == nil || branch.Schema == nil {
			t.Fatal("selection branch must be inline")
		}
		assertSchemaClosed(t, branch.Schema)
		wire := serializedObjectSchema(t, branch.Schema)
		kind := inlineProperty(t, branch.Schema, "kind").Enum[0]
		switch kind {
		case "rect":
			if got, want := sortedRawPropertyNames(wire.Properties), []string{"height", "kind", "width", "x", "y"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("rect properties = %v, want %v", got, want)
			}
			if got, want := wire.Required, []string{"kind", "x", "y", "width", "height"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("rect required = %v, want %v", got, want)
			}
		case "time":
			if got, want := sortedRawPropertyNames(wire.Properties), []string{"endMs", "kind", "startMs"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("time properties = %v, want %v", got, want)
			}
			if got, want := wire.Required, []string{"kind", "startMs", "endMs"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("time required = %v, want %v", got, want)
			}
		case "text":
			if got, want := sortedRawPropertyNames(wire.Properties), []string{"end", "kind", "start", "text"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("text properties = %v, want %v", got, want)
			}
			if got, want := wire.Required, []string{"kind", "start", "end", "text"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("text required = %v, want %v", got, want)
			}
		default:
			t.Fatalf("unexpected selection kind %v", kind)
		}
	}

	mutation := BuildCloudSpec().Components.Schemas["StepRevisionMutationRequest"]
	for _, branch := range mutation.OneOf {
		selectionProperty := inlineProperty(t, branch.Schema, "selection")
		if !selectionProperty.Nullable {
			t.Fatalf("optional selection must accept JSON null: %+v", selectionProperty)
		}
		mode := inlineProperty(t, branch.Schema, "mode").Enum[0]
		if mode == "replace" {
			if selectionProperty.Type != "object" || selectionProperty.AdditionalProperties == nil ||
				selectionProperty.AdditionalProperties.Allowed == nil || *selectionProperty.AdditionalProperties.Allowed {
				t.Fatalf("replace selection must be a closed rectangle: %+v", selectionProperty)
			}
			if kind := inlineProperty(t, selectionProperty, "kind"); !reflect.DeepEqual(kind.Enum, []any{"rect"}) {
				t.Fatalf("replace selection kind = %v", kind.Enum)
			}
		}
	}
}

func TestCloudSpec_ArtifactContentResponseExposesOptionalReviewText(t *testing.T) {
	content := BuildCloudSpec().Components.Schemas["ArtifactContentResponse"]
	if content == nil {
		t.Fatal("ArtifactContentResponse schema is missing")
	}
	data := inlineProperty(t, content, "data")
	reviewText := inlineProperty(t, data, "reviewText")
	if reviewText.Type != "string" {
		t.Fatalf("reviewText schema = %+v", reviewText)
	}
}

func TestCloudSpec_SerializesCreatorSchemaRefsAsOpenAPI(t *testing.T) {
	encoded, err := json.Marshal(BuildCloudSpec())
	if err != nil {
		t.Fatalf("marshal cloud spec: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("unmarshal cloud spec JSON: %v", err)
	}

	components := jsonObject(t, document["components"], "components")
	schemas := jsonObject(t, components["schemas"], "components.schemas")
	selection := jsonObject(t, schemas["ArtifactSelection"], "ArtifactSelection")
	selectionBranches := jsonArray(t, selection["oneOf"], "ArtifactSelection.oneOf")
	if len(selectionBranches) != 3 {
		t.Fatalf("ArtifactSelection.oneOf length = %d", len(selectionBranches))
	}
	for index, value := range selectionBranches {
		branch := jsonObject(t, value, "ArtifactSelection.oneOf branch")
		assertNoSchemaWrapper(t, branch, "ArtifactSelection.oneOf")
		if branch["type"] != "object" || branch["additionalProperties"] != false {
			t.Fatalf("ArtifactSelection.oneOf[%d] is not a closed object: %#v", index, branch)
		}
	}

	mutation := jsonObject(t, schemas["StepRevisionMutationRequest"], "StepRevisionMutationRequest")
	mutationBranches := jsonArray(t, mutation["oneOf"], "StepRevisionMutationRequest.oneOf")
	if len(mutationBranches) != 3 {
		t.Fatalf("StepRevisionMutationRequest.oneOf length = %d", len(mutationBranches))
	}
	for index, value := range mutationBranches {
		branch := jsonObject(t, value, "StepRevisionMutationRequest.oneOf branch")
		assertNoSchemaWrapper(t, branch, "StepRevisionMutationRequest.oneOf")
		properties := jsonObject(t, branch["properties"], "StepRevisionMutationRequest.properties")
		selectionRef := jsonObject(t, properties["selection"], "StepRevisionMutationRequest.selection")
		mode := jsonArray(t, jsonObject(t, properties["mode"], "StepRevisionMutationRequest.mode")["enum"], "StepRevisionMutationRequest.mode.enum")[0]
		if mode == "replace" {
			if selectionRef["type"] != "object" || selectionRef["additionalProperties"] != false || selectionRef["nullable"] != true {
				t.Fatalf("replace selection = %#v", selectionRef)
			}
			continue
		}
		if selectionRef["$ref"] != "#/components/schemas/ArtifactSelection" || selectionRef["nullable"] != true {
			t.Fatalf("StepRevisionMutationRequest.oneOf[%d] selection = %#v", index, selectionRef)
		}
	}

	paths := jsonObject(t, document["paths"], "paths")
	revisionsPath := jsonObject(t, paths["/api/video-projects/:id/steps/:stepId/revisions"], "revisions path")
	post := jsonObject(t, revisionsPath["post"], "revisions POST")
	requestBody := jsonObject(t, post["requestBody"], "revisions request body")
	content := jsonObject(t, requestBody["content"], "revisions request content")
	mediaType := jsonObject(t, content["application/json"], "revisions JSON media type")
	requestRef := jsonObject(t, mediaType["schema"], "revisions request schema")
	if !reflect.DeepEqual(requestRef, map[string]any{"$ref": "#/components/schemas/StepRevisionMutationRequest"}) {
		t.Fatalf("revisions request schema = %#v", requestRef)
	}
}

func jsonObject(t *testing.T, value any, path string) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want JSON object", path, value)
	}
	return object
}

func jsonArray(t *testing.T, value any, path string) []any {
	t.Helper()
	array, ok := value.([]any)
	if !ok {
		t.Fatalf("%s = %#v, want JSON array", path, value)
	}
	return array
}

func assertNoSchemaWrapper(t *testing.T, value any, path string) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		if _, wrapped := typed["schema"]; wrapped {
			t.Fatalf("%s contains non-OpenAPI SchemaRef wrapper: %#v", path, typed)
		}
		for name, child := range typed {
			assertNoSchemaWrapper(t, child, path+"."+name)
		}
	case []any:
		for index, child := range typed {
			assertNoSchemaWrapper(t, child, path+"["+strconv.Itoa(index)+"]")
		}
	}
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

func sortedPropertyNames(schema *Schema) []string {
	if schema == nil {
		return nil
	}
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func findParameter(op *Operation, in, name string) *Parameter {
	if op == nil {
		return nil
	}
	for index := range op.Parameters {
		parameter := &op.Parameters[index]
		if parameter.In == in && parameter.Name == name {
			return parameter
		}
	}
	return nil
}

func inlineParameterSchema(parameter *Parameter) *Schema {
	if parameter == nil || parameter.Schema == nil {
		return nil
	}
	return parameter.Schema.Schema
}

func requiredOf(schema *Schema) []string {
	if schema == nil {
		return nil
	}
	return schema.Required
}

func assertSchemaClosed(t *testing.T, schema *Schema) {
	t.Helper()
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	if !strings.Contains(string(encoded), `"additionalProperties":false`) {
		t.Fatalf("schema is not closed: %s", encoded)
	}
}

type serializedObject struct {
	AdditionalProperties bool                       `json:"additionalProperties"`
	Properties           map[string]json.RawMessage `json:"properties"`
	Required             []string                   `json:"required"`
}

func serializedObjectSchema(t *testing.T, schema *Schema) serializedObject {
	t.Helper()
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var wire serializedObject
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatalf("unmarshal serialized schema: %v", err)
	}
	if !wire.AdditionalProperties {
		return wire
	}
	t.Fatalf("serialized schema is open: %s", encoded)
	return serializedObject{}
}

func sortedRawPropertyNames(properties map[string]json.RawMessage) []string {
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
