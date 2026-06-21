package apispec

import (
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
