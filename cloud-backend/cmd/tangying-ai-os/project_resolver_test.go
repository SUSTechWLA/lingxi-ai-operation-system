package main

import "testing"

func TestProjectIDFromTaskInputReadsAgentRuntimeContext(t *testing.T) {
	got := projectIDFromTaskInput(map[string]interface{}{
		"source": "agentruntime",
		"context": map[string]interface{}{
			"projectId": "vp-agent-1",
		},
	})
	if got != "vp-agent-1" {
		t.Fatalf("expected project id from agent context, got %q", got)
	}
}

func TestProjectIDFromTaskInputReadsTopLevelFallback(t *testing.T) {
	got := projectIDFromTaskInput(map[string]interface{}{
		"projectID": "vp-top-level",
	})
	if got != "vp-top-level" {
		t.Fatalf("expected project id from top-level fallback, got %q", got)
	}
}
