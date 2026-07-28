package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/database"
	"github.com/tangying-ai/aios-core/internal/core/observability"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestBuildToolSnapshotIsByteStable(t *testing.T) {
	a := []*tool.ToolManifest{
		{
			Name: "zeta", Capabilities: []string{"write", "read"},
			InputSchema: map[string]interface{}{
				"required": []interface{}{"z", "a"},
				"properties": map[string]interface{}{
					"mode": map[string]interface{}{"enum": []interface{}{"slow", "fast"}},
				},
			},
		},
		{Name: "alpha", Tags: []string{"video", "alpha"}},
	}
	b := []*tool.ToolManifest{
		{Name: "alpha", Tags: []string{"alpha", "video"}},
		{
			Name: "zeta", Capabilities: []string{"read", "write"},
			InputSchema: map[string]interface{}{
				"properties": map[string]interface{}{
					"mode": map[string]interface{}{"enum": []interface{}{"fast", "slow"}},
				},
				"required": []interface{}{"a", "z"},
			},
		},
	}

	left, err := BuildToolSnapshot(a)
	if err != nil {
		t.Fatal(err)
	}
	right, err := BuildToolSnapshot(b)
	if err != nil {
		t.Fatal(err)
	}
	if left.SHA256 != right.SHA256 || !bytes.Equal(left.CanonicalJSON, right.CanonicalJSON) {
		t.Fatalf("equivalent registries produced different snapshots:\nleft=%s\nright=%s", left.CanonicalJSON, right.CanonicalJSON)
	}
	if left.ID != right.ID || !strings.Contains(left.ID, left.SHA256) {
		t.Fatalf("snapshot IDs must be derived from the exact hash: left=%q right=%q hash=%q", left.ID, right.ID, left.SHA256)
	}
	if left.CreatedAt.IsZero() || right.CreatedAt.IsZero() {
		t.Fatal("snapshot creation time is required")
	}
}

func TestBuildToolSnapshotDoesNotMutateInputAndIgnoresRegistrationTime(t *testing.T) {
	registeredAt := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	manifest := &tool.ToolManifest{
		Name:         "stable",
		Capabilities: []string{"z", "a"},
		Parameters:   nil,
		Output:       nil,
		RegisteredAt: registeredAt,
	}

	first, err := BuildToolSnapshot([]*tool.ToolManifest{manifest})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(manifest.Capabilities, ","); got != "z,a" || !manifest.RegisteredAt.Equal(registeredAt) || manifest.Parameters != nil || manifest.Output != nil {
		t.Fatalf("input manifest was mutated: %#v", manifest)
	}

	manifest.RegisteredAt = registeredAt.Add(time.Hour)
	manifest.Parameters = map[string]tool.ParamDef{}
	manifest.Output = map[string]tool.ParamDef{}
	second, err := BuildToolSnapshot([]*tool.ToolManifest{manifest})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || !bytes.Equal(first.CanonicalJSON, second.CanonicalJSON) {
		t.Fatalf("registration time or nil/empty representation changed identity:\nfirst=%s\nsecond=%s", first.CanonicalJSON, second.CanonicalJSON)
	}
}

func TestBuildToolSnapshotRejectsAmbiguousLogicalNames(t *testing.T) {
	_, err := BuildToolSnapshot([]*tool.ToolManifest{{Name: "studio.render"}, {Name: " studio.render "}})
	if err == nil || !strings.Contains(err.Error(), "duplicate logical tool name") {
		t.Fatalf("error = %v, want duplicate logical tool name", err)
	}
}

func TestBuildToolSnapshotPreservesNeighboringLargeIntegers(t *testing.T) {
	build := func(number json.Number) ToolSnapshot {
		t.Helper()
		snapshot, err := BuildToolSnapshot([]*tool.ToolManifest{{
			Name: "large-number",
			InputSchema: map[string]interface{}{
				"type": "integer", "maximum": number,
			},
		}})
		if err != nil {
			t.Fatal(err)
		}
		return snapshot
	}

	left := build(json.Number("9007199254740992"))
	right := build(json.Number("9007199254740993"))
	if left.ID == right.ID || bytes.Equal(left.CanonicalJSON, right.CanonicalJSON) {
		t.Fatalf("neighboring large integers collided:\nleft=%s\nright=%s", left.CanonicalJSON, right.CanonicalJSON)
	}
	if !bytes.Contains(right.CanonicalJSON, []byte("9007199254740993")) {
		t.Fatalf("large integer lost precision: %s", right.CanonicalJSON)
	}
}

func TestRequestToolSnapshotPreservesJSONNumberTypeAndIsolation(t *testing.T) {
	schema := map[string]interface{}{"maximum": json.Number("9007199254740993")}
	snapshot := newRequestToolSnapshot([]*tool.ToolManifest{{Name: "large-number", InputSchema: schema}}, nil)
	schema["maximum"] = json.Number("1")

	cloned := snapshot.GetManifest("large-number")
	maximum, ok := cloned.InputSchema["maximum"].(json.Number)
	if !ok || maximum.String() != "9007199254740993" {
		t.Fatalf("snapshot maximum = %#v (%T), want lossless json.Number", cloned.InputSchema["maximum"], cloned.InputSchema["maximum"])
	}
	cloned.InputSchema["maximum"] = json.Number("2")
	if got := snapshot.GetManifest("large-number").InputSchema["maximum"]; got != json.Number("9007199254740993") {
		t.Fatalf("snapshot mutated through returned manifest: %#v", got)
	}
}

func TestRequestToolSnapshotDoesNotExposeMutableManifests(t *testing.T) {
	snapshot := newRequestToolSnapshot([]*tool.ToolManifest{{Name: "alpha", Version: "1"}}, nil)
	listed := snapshot.ListManifests()
	listed[0].Version = "mutated"
	listed[0].Capabilities = append(listed[0].Capabilities, "new")
	fromLookup := snapshot.GetManifest("alpha")
	if fromLookup.Version != "1" || len(fromLookup.Capabilities) != 0 {
		t.Fatalf("snapshot contents were mutable through ListManifests: %#v", fromLookup)
	}
	fromLookup.Version = "mutated-again"
	if got := snapshot.GetManifest("alpha").Version; got != "1" {
		t.Fatalf("snapshot contents were mutable through GetManifest: %q", got)
	}
}

type mutableSnapshotResolver struct {
	manifests []*tool.ToolManifest
	calls     int
}

type mutableFallbackCatalog struct {
	manifests []*tool.ToolManifest
}

func (c *mutableFallbackCatalog) GetManifest(name string) *tool.ToolManifest {
	for _, manifest := range c.manifests {
		if manifest != nil && manifest.Name == name {
			return manifest
		}
	}
	return nil
}

func (c *mutableFallbackCatalog) ListManifests() []*tool.ToolManifest {
	return append([]*tool.ToolManifest(nil), c.manifests...)
}

type fallbackSnapshotPlanner struct {
	live         *mutableFallbackCatalog
	seenVersions []string
}

func (p *fallbackSnapshotPlanner) GeneratePlan(_ context.Context, req StartRunRequest) (*AgentPlan, error) {
	if req.requestToolSnapshot == nil {
		return nil, fmt.Errorf("request snapshot missing")
	}
	manifest := req.requestToolSnapshot.GetManifest("alpha")
	if manifest == nil {
		return nil, fmt.Errorf("alpha missing from request snapshot")
	}
	p.seenVersions = append(p.seenVersions, manifest.Version)
	p.live.manifests[0].Version = "2"
	p.live.manifests[0].InputSchema = map[string]interface{}{
		"type": "object", "required": []interface{}{"prompt"},
		"properties": map[string]interface{}{"prompt": map[string]interface{}{"type": "string"}},
	}
	arguments := map[string]interface{}{}
	if manifest.Version == "2" {
		arguments["prompt"] = "new run"
	}
	return &AgentPlan{Goal: "alpha", Domain: "general", Steps: []AgentStep{{ID: "alpha", Tool: "alpha", Arguments: arguments}}}, nil
}

func TestRunnerFreezesResolverLessCatalogBeforePlanning(t *testing.T) {
	live := &mutableFallbackCatalog{manifests: []*tool.ToolManifest{{Name: "alpha", Version: "1", Endpoint: "builtin://alpha"}}}
	planner := &fallbackSnapshotPlanner{live: live}
	runner := NewRunner(
		&fakeOrchestrator{taskID: "task-fallback"}, newMemoryRunStore(), planner,
		NewPlanGuard(live, nil), NewPlanCompiler(live),
	)

	first, err := runner.Start(context.Background(), StartRunRequest{RunID: "fallback-one", UserID: "user-a", Message: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if len(planner.seenVersions) != 1 || planner.seenVersions[0] != "1" {
		t.Fatalf("planner versions = %#v, want frozen version 1", planner.seenVersions)
	}
	if got := first.toolSnapshot.CanonicalJSON; !bytes.Contains(got, []byte(`"version":"1"`)) || bytes.Contains(got, []byte(`"version":"2"`)) {
		t.Fatalf("current run snapshot changed with live catalog: %s", got)
	}

	second, err := runner.Start(context.Background(), StartRunRequest{RunID: "fallback-two", UserID: "user-a", Message: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if len(planner.seenVersions) != 2 || planner.seenVersions[1] != "2" {
		t.Fatalf("planner versions = %#v, want new run version 2", planner.seenVersions)
	}
	if first.ToolRegistrySnapshotID == second.ToolRegistrySnapshotID {
		t.Fatal("new run did not observe fallback catalog change")
	}
}

func (r *mutableSnapshotResolver) Resolve(context.Context, string, string, string) (*RequestToolSnapshot, error) {
	r.calls++
	return newRequestToolSnapshot(r.manifests, nil), nil
}

func TestRunnerPinsOneToolSnapshotPerRunAndNewRunsSeeRegistryChanges(t *testing.T) {
	manifest := &tool.ToolManifest{Name: "alpha", Version: "1", Endpoint: "builtin://alpha"}
	resolver := &mutableSnapshotResolver{manifests: []*tool.ToolManifest{manifest}}
	runner := NewRunner(
		&fakeOrchestrator{taskID: "task-snapshot"},
		newMemoryRunStore(),
		staticPlanner{plan: &AgentPlan{Goal: "alpha", Domain: "general", Steps: []AgentStep{{ID: "alpha", Tool: "alpha"}}}},
		NewPlanGuard(nil, nil),
		NewPlanCompiler(nil),
	).WithRequestToolResolver(resolver)

	correlation := observability.Correlation{TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"}
	ctx := observability.WithCorrelation(context.Background(), correlation)
	firstReq := StartRunRequest{RunID: "run-one", UserID: "user-a", Message: "secret prompt one"}
	firstShell := newRunShell(firstReq)
	if err := runner.attachToolSnapshot(ctx, &firstReq, firstShell); err != nil {
		t.Fatal(err)
	}
	firstBytes := append([]byte(nil), firstShell.toolSnapshot.CanonicalJSON...)
	firstID := firstShell.ToolRegistrySnapshotID

	// The registry changes after run creation but before planning starts. The
	// existing run must keep the already attached request snapshot.
	manifest.Version = "2"
	manifest.Description = "new registration"
	first, err := runner.completeStart(ctx, firstReq, firstShell)
	if err != nil {
		t.Fatal(err)
	}
	if firstID == "" || first.RunManifest == nil || first.RunManifest.ToolRegistrySnapshotID != firstID {
		t.Fatalf("run did not persist its tool snapshot identity: %#v", first)
	}
	if first.TraceID != correlation.TraceID || first.RunManifest.TraceID != correlation.TraceID {
		t.Fatalf("run created a trace unrelated to context: run=%q manifest=%q", first.TraceID, first.RunManifest.TraceID)
	}

	if !bytes.Equal(firstBytes, first.toolSnapshot.CanonicalJSON) || first.ToolRegistrySnapshotID != firstID {
		t.Fatal("registry mutation altered the existing run snapshot")
	}
	second, err := runner.Start(ctx, StartRunRequest{RunID: "run-two", UserID: "user-a", Message: "secret prompt two"})
	if err != nil {
		t.Fatal(err)
	}
	if resolver.calls != 2 {
		t.Fatalf("resolver calls = %d, want once per run", resolver.calls)
	}
	if second.ToolRegistrySnapshotID == firstID || bytes.Equal(second.toolSnapshot.CanonicalJSON, firstBytes) {
		t.Fatal("new run did not observe the changed registry")
	}
}

func TestRunnerKeepsExistingToolSnapshotWhenSynthesisToolRegisters(t *testing.T) {
	original := []*tool.ToolManifest{
		{Name: "ip_avatar_3d.check_status", Version: "1"},
		{Name: "ip_avatar_3d.check_gpt_sovits_voice", Version: "1"},
		{Name: "ip_avatar_3d.render_talking_video", Version: "1"},
	}
	resolver := &mutableSnapshotResolver{manifests: original}
	runner := NewRunner(
		&fakeOrchestrator{taskID: "task-voice-snapshot"}, newMemoryRunStore(),
		staticPlanner{plan: &AgentPlan{Goal: "voice", Domain: "general", Steps: []AgentStep{{ID: "check", Tool: "ip_avatar_3d.check_status"}}}},
		NewPlanGuard(nil, nil), NewPlanCompiler(nil),
	).WithRequestToolResolver(resolver)
	ctx := context.Background()
	firstReq := StartRunRequest{RunID: "voice-run-one", UserID: "user-a", Message: "voice"}
	firstShell := newRunShell(firstReq)
	if err := runner.attachToolSnapshot(ctx, &firstReq, firstShell); err != nil {
		t.Fatal(err)
	}
	firstJSON := append([]byte(nil), firstShell.toolSnapshot.CanonicalJSON...)
	resolver.manifests = append(resolver.manifests, &tool.ToolManifest{Name: "ip_avatar_3d.synthesize_reference_voice", Version: "1"})
	if _, err := runner.completeStart(ctx, firstReq, firstShell); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, firstShell.toolSnapshot.CanonicalJSON) || bytes.Contains(firstShell.toolSnapshot.CanonicalJSON, []byte("synthesize_reference_voice")) {
		t.Fatal("running task tool snapshot changed after registration")
	}
	second, err := runner.Start(ctx, StartRunRequest{RunID: "voice-run-two", UserID: "user-a", Message: "voice"})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(firstJSON, second.toolSnapshot.CanonicalJSON) || !bytes.Contains(second.toolSnapshot.CanonicalJSON, []byte("synthesize_reference_voice")) {
		t.Fatal("new task did not discover the appended synthesis tool")
	}
}

func TestRunnerRejectsDuplicateNamesBeforePlanning(t *testing.T) {
	resolver := &fixedRequestToolResolver{snapshot: newRequestToolSnapshot([]*tool.ToolManifest{
		{Name: "alpha", Version: "1", Endpoint: "builtin://alpha"},
		{Name: "alpha", Version: "2", Endpoint: "builtin://alpha-v2"},
	}, nil)}
	runner := NewRunner(
		&fakeOrchestrator{taskID: "task-duplicate"},
		newMemoryRunStore(),
		staticPlanner{plan: &AgentPlan{Goal: "alpha", Domain: "general", Steps: []AgentStep{{ID: "alpha", Tool: "alpha"}}}},
		NewPlanGuard(nil, nil),
		NewPlanCompiler(nil),
	).WithRequestToolResolver(resolver)

	_, err := runner.Start(context.Background(), StartRunRequest{UserID: "user-a", Message: "alpha"})
	if err == nil || !strings.Contains(err.Error(), "duplicate logical tool name") {
		t.Fatalf("Start error = %v, want duplicate logical tool name", err)
	}
}

func TestRunManifestContainsOnlyBoundedNonSecretMetadata(t *testing.T) {
	manifest := &tool.ToolManifest{
		Name: "alpha", Endpoint: "builtin://alpha",
		Transport: &tool.ToolTransport{Headers: map[string]string{"Authorization": "Bearer tool-secret"}},
	}
	resolver := &mutableSnapshotResolver{manifests: []*tool.ToolManifest{manifest}}
	runner := NewRunner(
		&fakeOrchestrator{taskID: "task-private"},
		newMemoryRunStore(),
		staticPlanner{plan: &AgentPlan{Goal: "alpha", Domain: "general", Steps: []AgentStep{{ID: "alpha", Tool: "alpha", Arguments: map[string]interface{}{"token": "argument-secret"}}}}},
		NewPlanGuard(nil, nil),
		NewPlanCompiler(nil),
	).WithRequestToolResolver(resolver)

	parent := "parent-run"
	replayStage := "stage-safe"
	run, err := runner.Start(context.Background(), StartRunRequest{
		RunID: "run-private", UserID: "user-a", Message: "raw prompt secret",
		Context: map[string]interface{}{"apiKey": "context-secret"}, ParentRunID: &parent, ReplayFromStageID: &replayStage,
	})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(run.RunManifest)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(wire)
	for _, forbidden := range []string{"raw prompt secret", "context-secret", "argument-secret", "tool-secret", "Authorization"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("run manifest leaked %q: %s", forbidden, serialized)
		}
	}
	if run.ParentRunID == nil || *run.ParentRunID != parent || run.ReplayFromStageID == nil || *run.ReplayFromStageID != replayStage {
		t.Fatalf("nullable replay lineage was not preserved: %#v", run)
	}
}

func TestAgentRunManifestCatalogBounds(t *testing.T) {
	manifest := &RunManifest{
		SchemaVersion: runManifestSchemaVersion, Runtime: "cloud-agent", RunID: "run-1",
		ToolRegistrySnapshotID: "snapshot-1", ToolRegistrySHA256: strings.Repeat("a", 64), CreatedAt: time.Now().UTC(),
	}
	for i := 0; i < database.RunManifestMaxRunnerCatalogs; i++ {
		manifest.MCPRunnerRevisions = append(manifest.MCPRunnerRevisions, RequestMCPRunnerRevision{
			RunnerID: fmt.Sprintf("runner-%d", i), Revision: strings.Repeat("a", 64),
		})
	}
	if err := validateAgentRunManifest(manifest); err != nil {
		t.Fatalf("manifest at catalog count bound failed: %v", err)
	}
	manifest.MCPRunnerRevisions = append(manifest.MCPRunnerRevisions, RequestMCPRunnerRevision{RunnerID: "overflow", Revision: strings.Repeat("b", 64)})
	if err := validateAgentRunManifest(manifest); err == nil || !strings.Contains(err.Error(), database.RunManifestLimitExceededCode) {
		t.Fatalf("catalog overflow error = %v", err)
	}
}

func TestAgentRunManifestStringAndSerializedBounds(t *testing.T) {
	manifest := &RunManifest{
		SchemaVersion:          strings.Repeat("v", database.RunManifestMaxVersionBytes),
		Runtime:                "cloud-agent",
		RunID:                  strings.Repeat("r", database.RunManifestMaxIdentifierBytes),
		TraceID:                strings.Repeat("t", database.RunManifestMaxIdentifierBytes),
		ToolRegistrySnapshotID: strings.Repeat("s", database.RunManifestMaxIdentifierBytes),
		ToolRegistrySHA256:     strings.Repeat("h", database.RunManifestMaxHashBytes),
		MCPRunnerRevisions: []RequestMCPRunnerRevision{{
			RunnerID: strings.Repeat("r", database.RunManifestMaxRunnerIDBytes),
			DeviceID: strings.Repeat("d", database.RunManifestMaxIdentifierBytes),
			Revision: strings.Repeat("v", database.RunManifestMaxVersionBytes),
		}},
		CreatedAt: time.Now().UTC(),
	}
	if err := validateAgentRunManifest(manifest); err != nil {
		t.Fatalf("manifest at string bounds failed: %v", err)
	}

	manifest.MCPRunnerRevisions[0].RunnerID += "x"
	if err := validateAgentRunManifest(manifest); err == nil || !strings.Contains(err.Error(), database.RunManifestLimitExceededCode) {
		t.Fatalf("runner ID overflow error = %v", err)
	}
	manifest.MCPRunnerRevisions[0].RunnerID = "runner"
	manifest.MCPRunnerRevisions = manifest.MCPRunnerRevisions[:0]
	for i := 0; i < database.RunManifestMaxRunnerCatalogs; i++ {
		manifest.MCPRunnerRevisions = append(manifest.MCPRunnerRevisions, RequestMCPRunnerRevision{
			RunnerID: strings.Repeat("r", database.RunManifestMaxRunnerIDBytes),
			DeviceID: strings.Repeat("d", database.RunManifestMaxIdentifierBytes),
			Revision: strings.Repeat("v", database.RunManifestMaxVersionBytes),
		})
	}
	if err := validateAgentRunManifest(manifest); err == nil || !strings.Contains(err.Error(), database.RunManifestLimitExceededCode) || !strings.Contains(err.Error(), "serialized bytes") {
		t.Fatalf("serialized size overflow error = %v", err)
	}
}
