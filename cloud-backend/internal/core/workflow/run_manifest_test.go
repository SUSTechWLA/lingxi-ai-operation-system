package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/observability"
)

type workflowManifestRow []interface{}

func (r workflowManifestRow) Scan(dest ...interface{}) error {
	if len(dest) != len(r) {
		return fmt.Errorf("destinations=%d values=%d", len(dest), len(r))
	}
	for i := range dest {
		target := reflect.ValueOf(dest[i])
		if r[i] == nil {
			target.Elem().Set(reflect.Zero(target.Elem().Type()))
			continue
		}
		value := reflect.ValueOf(r[i])
		if !value.Type().AssignableTo(target.Elem().Type()) {
			return fmt.Errorf("value %d type %s cannot assign to %s", i, value.Type(), target.Elem().Type())
		}
		target.Elem().Set(value)
	}
	return nil
}

func TestTraceIDForRunUsesExistingCorrelation(t *testing.T) {
	ctx := observability.WithCorrelation(context.Background(), observability.Correlation{TraceID: "4bf92f3577b34da6a3ce929d0e0e4736"})
	if got := traceIDForRun(ctx, "task-fallback"); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace ID = %q", got)
	}
	if got := traceIDForRun(context.Background(), "task-fallback"); got != "task-fallback" {
		t.Fatalf("fallback trace ID = %q", got)
	}
}

func TestBuildWorkflowRunManifestExcludesRequestPayloads(t *testing.T) {
	now := time.Now().UTC()
	run := &WorkflowRun{
		ID: "wfr-1", TraceID: "trace-1", ToolRegistrySnapshotID: "snapshot-1", CreatedAt: now,
		Input: map[string]interface{}{"prompt": "raw-prompt-secret", "apiKey": "credential-secret"},
	}
	manifest := buildWorkflowRunManifest(run)
	wire, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.RunID != run.ID || manifest.TraceID != run.TraceID || manifest.ToolRegistrySnapshotID != run.ToolRegistrySnapshotID {
		t.Fatalf("manifest identity mismatch: %#v", manifest)
	}
	for _, forbidden := range []string{"raw-prompt-secret", "credential-secret", "prompt", "apiKey"} {
		if strings.Contains(string(wire), forbidden) {
			t.Fatalf("workflow run manifest leaked %q: %s", forbidden, wire)
		}
	}
}

func TestScanWorkflowRunPreservesManifestAndNullableLineage(t *testing.T) {
	now := time.Now().UTC()
	parent := "workflow-parent"
	stage := "render"
	run, err := scanWorkflowRun(workflowManifestRow{
		"wfr-1", "project-1", "user-1", "template-1", "1", "task-1", RunPending, 1,
		map[string]interface{}{}, map[string]interface{}{}, map[string]StageStatus{}, "trace-1", "snapshot-1",
		[]byte(`{"schemaVersion":"1","toolRegistrySnapshotId":"snapshot-1"}`), &parent, &stage,
		(*time.Time)(nil), (*time.Time)(nil), now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.ToolRegistrySnapshotID != "snapshot-1" || run.RunManifest == nil || run.RunManifest.SchemaVersion != "1" {
		t.Fatalf("run manifest was not restored: %#v", run)
	}
	if run.ParentRunID == nil || *run.ParentRunID != parent || run.ReplayFromStageID == nil || *run.ReplayFromStageID != stage {
		t.Fatalf("lineage was not restored: %#v", run)
	}
}

func TestScanWorkflowRunPreservesNullManifestAndLineage(t *testing.T) {
	now := time.Now().UTC()
	run, err := scanWorkflowRun(workflowManifestRow{
		"wfr-legacy", "project-1", "user-1", "template-1", "1", "task-1", RunPending, 1,
		map[string]interface{}{}, map[string]interface{}{}, map[string]StageStatus{}, "", "", nil, nil, nil,
		(*time.Time)(nil), (*time.Time)(nil), now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.RunManifest != nil || run.ParentRunID != nil || run.ReplayFromStageID != nil {
		t.Fatalf("legacy SQL NULLs were not preserved: %#v", run)
	}
}
