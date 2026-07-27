package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

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
