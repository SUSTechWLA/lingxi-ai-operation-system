package agentruntime

import (
	"errors"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/model"
)

func TestAtomicReviewGateNodeTypes(t *testing.T) {
	for _, nodeType := range []string{string(model.NodeTypeControl), string(model.NodeTypeReviewGate)} {
		if !validAtomicReviewGateNodeType(nodeType) {
			t.Fatalf("node type %q should be valid", nodeType)
		}
	}
	if validAtomicReviewGateNodeType(string(model.NodeTypeTool)) {
		t.Fatal("execution node must not be accepted as a review gate")
	}
}

func TestValidateReviewReopenArtifactIdentity(t *testing.T) {
	req := ReviewReopenRequest{RunID: "run-1", TaskID: "task-1", ProjectID: "vp-1", StageName: "script", ExpectedArtifactID: "v1", NewArtifactID: "v2"}
	valid := reviewReopenArtifactIdentity{ProjectID: "vp-1", WorkflowRunID: "run-1", TaskID: "task-1", StageName: "script", ParentID: "v1", IsCurrent: true, ProducedByNode: "compiled-source", UniqueTaskRun: true}
	if err := validateReviewReopenArtifact(req, valid); err != nil {
		t.Fatalf("valid identity: %v", err)
	}
	for name, mutate := range map[string]func(*reviewReopenArtifactIdentity){
		"project":    func(v *reviewReopenArtifactIdentity) { v.ProjectID = "foreign" },
		"no link":    func(v *reviewReopenArtifactIdentity) { v.WorkflowRunID, v.TaskID = "foreign", "" },
		"cross task": func(v *reviewReopenArtifactIdentity) { v.TaskID = "foreign" },
		"stage":      func(v *reviewReopenArtifactIdentity) { v.StageName = "preview" },
		"parent":     func(v *reviewReopenArtifactIdentity) { v.ParentID = "other" },
		"current":    func(v *reviewReopenArtifactIdentity) { v.IsCurrent = false },
	} {
		t.Run(name, func(t *testing.T) {
			copy := valid
			mutate(&copy)
			if err := validateReviewReopenArtifact(req, copy); !errors.Is(err, ErrReviewReferenceMismatch) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestValidateReviewReopenArtifactAcceptsUniqueTaskLinkWhenWorkflowReferenceIsUnrelated(t *testing.T) {
	req := ReviewReopenRequest{RunID: "run-1", TaskID: "task-1", ProjectID: "vp-1", StageName: "script", ExpectedArtifactID: "v1"}
	identity := reviewReopenArtifactIdentity{ProjectID: "vp-1", WorkflowRunID: "other-reference", TaskID: "task-1", StageName: "script", ParentID: "v1", IsCurrent: true, ProducedByNode: "source", UniqueTaskRun: true}
	if err := validateReviewReopenArtifact(req, identity); err != nil {
		t.Fatalf("unique task link rejected: %v", err)
	}
	identity.UniqueTaskRun = false
	if err := validateReviewReopenArtifact(req, identity); !errors.Is(err, ErrReviewReferenceMismatch) {
		t.Fatalf("ambiguous task link error=%v", err)
	}
	identity.TaskID, identity.UniqueTaskRun = req.RunID, true
	if err := validateReviewReopenArtifact(req, identity); err != nil {
		t.Fatalf("resolved run-id task link rejected: %v", err)
	}
	identity.UniqueTaskRun = false
	if err := validateReviewReopenArtifact(req, identity); !errors.Is(err, ErrReviewReferenceMismatch) {
		t.Fatalf("ambiguous run-id task link error=%v", err)
	}
}

func TestValidateReviewReopenArtifactRejectsCrossRunAndCrossTask(t *testing.T) {
	req := ReviewReopenRequest{RunID: "run-1", TaskID: "task-1", ProjectID: "vp-1", StageName: "script", ExpectedArtifactID: "v1"}
	base := reviewReopenArtifactIdentity{ProjectID: "vp-1", WorkflowRunID: "run-1", TaskID: "task-1", StageName: "script", ParentID: "v1", IsCurrent: true, ProducedByNode: "source", UniqueTaskRun: true}
	for name, mutate := range map[string]func(*reviewReopenArtifactIdentity){
		"cross run no task link":        func(v *reviewReopenArtifactIdentity) { v.WorkflowRunID, v.TaskID = "run-2", "task-2" },
		"cross task despite direct run": func(v *reviewReopenArtifactIdentity) { v.TaskID = "task-2" },
	} {
		t.Run(name, func(t *testing.T) {
			identity := base
			mutate(&identity)
			if err := validateReviewReopenArtifact(req, identity); !errors.Is(err, ErrReviewReferenceMismatch) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
