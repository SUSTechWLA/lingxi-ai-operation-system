package main

import "testing"

func TestNewAgentRuntimeArtifactReviewStoreHandlesNilPool(t *testing.T) {
	if store := newAgentRuntimeArtifactReviewStore(nil); store != nil {
		t.Fatalf("nil pool should not create artifact review store, got %#v", store)
	}
}
