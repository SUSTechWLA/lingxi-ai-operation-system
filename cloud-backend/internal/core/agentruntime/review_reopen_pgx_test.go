package agentruntime

import (
	"errors"
	"testing"
)

func TestValidateReviewReopenArtifactIdentity(t *testing.T) {
	req := ReviewReopenRequest{RunID: "run-1", TaskID: "task-1", ProjectID: "vp-1", StageName: "script", ExpectedArtifactID: "v1", NewArtifactID: "v2"}
	valid := reviewReopenArtifactIdentity{ProjectID: "vp-1", WorkflowRunID: "run-1", TaskID: "task-1", StageName: "script", ParentID: "v1", IsCurrent: true, ProducedByNode: "compiled-source"}
	if err := validateReviewReopenArtifact(req, valid); err != nil {
		t.Fatalf("valid identity: %v", err)
	}
	for name, mutate := range map[string]func(*reviewReopenArtifactIdentity){
		"project": func(v *reviewReopenArtifactIdentity) { v.ProjectID = "foreign" },
		"run":     func(v *reviewReopenArtifactIdentity) { v.WorkflowRunID = "foreign" },
		"task":    func(v *reviewReopenArtifactIdentity) { v.TaskID = "foreign" },
		"stage":   func(v *reviewReopenArtifactIdentity) { v.StageName = "preview" },
		"parent":  func(v *reviewReopenArtifactIdentity) { v.ParentID = "other" },
		"current": func(v *reviewReopenArtifactIdentity) { v.IsCurrent = false },
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
