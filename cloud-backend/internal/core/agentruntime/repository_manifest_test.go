package agentruntime

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

type manifestRow []interface{}

func (r manifestRow) Scan(dest ...interface{}) error {
	if len(dest) != len(r) {
		return fmt.Errorf("destinations=%d values=%d", len(dest), len(r))
	}
	for i := range dest {
		target := reflect.ValueOf(dest[i])
		if target.Kind() != reflect.Pointer || target.IsNil() {
			return fmt.Errorf("destination %d is not a pointer", i)
		}
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

func TestScanRunPreservesManifestAndNullableLineage(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	parent := "parent-1"
	stage := "stage-2"
	manifest := []byte(`{"schemaVersion":"1","runtime":"cloud-agent","runId":"run-1","toolRegistrySnapshotId":"tool_snapshot_hash","toolRegistrySha256":"hash","parentRunId":"parent-1","replayFromStageId":"stage-2","createdAt":"2026-07-27T12:00:00Z"}`)
	row := manifestRow{
		"run-1", (*string)(nil), (*string)(nil), (*string)(nil), "legacy prompt", []byte("null"),
		"CREATED", []byte("{}"), []byte("{}"), "trace-1", "tool_snapshot_hash", manifest,
		&parent, &stage, now, now,
	}
	run, err := scanRun(row)
	if err != nil {
		t.Fatal(err)
	}
	if run.TraceID != "trace-1" || run.ToolRegistrySnapshotID != "tool_snapshot_hash" || run.RunManifest == nil {
		t.Fatalf("manifest identity was not restored: %#v", run)
	}
	if run.ParentRunID == nil || *run.ParentRunID != parent || run.ReplayFromStageID == nil || *run.ReplayFromStageID != stage {
		t.Fatalf("lineage was not restored: %#v", run)
	}

	row[12], row[13] = nil, nil
	run, err = scanRun(row)
	if err != nil {
		t.Fatal(err)
	}
	if run.ParentRunID != nil || run.ReplayFromStageID != nil {
		t.Fatalf("SQL NULL lineage was not preserved: %#v", run)
	}
}

func TestScanRunRejectsMalformedRunManifestJSON(t *testing.T) {
	now := time.Now()
	_, err := scanRun(manifestRow{
		"run-1", (*string)(nil), (*string)(nil), (*string)(nil), "legacy prompt", []byte("null"),
		"CREATED", []byte("{}"), []byte("{}"), "", "", []byte(`{"schemaVersion":`),
		nil, nil, now, now,
	})
	if err == nil {
		t.Fatal("malformed run manifest JSON was accepted")
	}
}

func TestMarshalRunFieldsIncludesPrivacySafeManifest(t *testing.T) {
	now := time.Now().UTC()
	run := &Run{RunManifest: &RunManifest{
		SchemaVersion: "1", Runtime: "cloud-agent", RunID: "run-1",
		ToolRegistrySnapshotID: "snapshot-1", ToolRegistrySHA256: "hash-1", CreatedAt: now,
	}}
	_, _, _, manifestJSON := marshalRunFields(run)
	var decoded map[string]interface{}
	if err := json.Unmarshal(manifestJSON, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["toolRegistrySnapshotId"] != "snapshot-1" || decoded["toolRegistrySha256"] != "hash-1" {
		t.Fatalf("manifest JSON = %s", manifestJSON)
	}
}
