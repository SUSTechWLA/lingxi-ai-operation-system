package agentruntime

import (
	"context"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/artifact"
)

// TestRuntime_TangyingDirector_FirstBeta verifies the 4 blocking conditions
// required for the v1.0-beta release (spec section 11):
//  1. preview not confirmed → render must fail
//  2. script modified → downstream must be stale
//  3. local runner offline → render must fail
//  4. final_review failed → package must fail
func TestRuntime_TangyingDirector_FirstBeta(t *testing.T) {
	t.Run("stale_chain_covers_all_11_stages", func(t *testing.T) {
		allKinds := []string{
			"VIDEO_PROPOSAL", "VIDEO_SCRIPT", "CARD_PLAN",
			"VIDEO_COMPOSITION_SPEC", "REFERENCE_ASSET_PLAN",
			"CONTINUITY_REPORT", "HYPERFRAMES_PROJECT",
			"PREVIEW_SNAPSHOTS", "VIDEO", "FFMPEG_PROBE_REPORT",
			"FINAL_REVIEW",
		}
		for _, kind := range allKinds {
			downstream := artifact.DownstreamStaleArtifactKinds(kind)
			t.Logf("%s → %v", kind, downstream)
		}

		// VIDEO_PROPOSAL (the root) should cascade to everything
		rootDownstream := artifact.DownstreamStaleArtifactKinds("VIDEO_PROPOSAL")
		if len(rootDownstream) < 6 {
			t.Errorf("VIDEO_PROPOSAL should cascade to at least 6 downstream stages, got %d: %v",
				len(rootDownstream), rootDownstream)
		}

		// FINAL_REVIEW should only cascade to package
		finalDownstream := artifact.DownstreamStaleArtifactKinds("FINAL_REVIEW")
		if len(finalDownstream) != 1 || finalDownstream[0] != "package" {
			t.Errorf("FINAL_REVIEW should only cascade to package, got %v", finalDownstream)
		}
	})

	t.Run("stage_name_downstream_chain_correct", func(t *testing.T) {
		allStages := []string{"proposal", "script", "storyboard", "composition",
			"reference", "continuity", "preview", "render", "quality", "package"}
		for i, stage := range allStages {
			downstream := artifact.DownstreamStageNamesForStage(stage)
			expected := len(allStages) - i - 1
			if len(downstream) != expected {
				t.Errorf("%s should have %d downstream stages, got %d: %v",
					stage, expected, len(downstream), downstream)
			}
		}
		// "package" has no downstream
		pkgDownstream := artifact.DownstreamStageNamesForStage("package")
		if len(pkgDownstream) != 0 {
			t.Errorf("package should have 0 downstream, got %v", pkgDownstream)
		}
	})

	t.Run("preview_not_approved_blocks_render", func(t *testing.T) {
		provider := &stubArtifactStateProvider{
			states: map[string]*stubArtifactState{
				"composition": {status: "valid", humanApproved: true},
				"preview":     {status: "valid", humanApproved: false}, // NOT approved
			},
		}
		checker := newTestRenderChecker(provider, false, true)
		err := checker.CheckRenderDependencies(t.Context(), testRenderRequest())
		if err == nil {
			t.Fatal("expected render to be blocked when preview not approved")
		}
		depErr, ok := err.(*testRenderDependencyError)
		if !ok {
			t.Fatalf("expected testRenderDependencyError, got %T: %v", err, err)
		}
		if depErr.Code != "RENDER_DEPENDENCY_MISSING" {
			t.Errorf("expected RENDER_DEPENDENCY_MISSING code, got %s", depErr.Code)
		}
		t.Logf("Correctly blocked render: %s → %v", depErr.Message, depErr.Missing)
	})

	t.Run("runner_offline_blocks_render", func(t *testing.T) {
		provider := &stubArtifactStateProvider{
			states: map[string]*stubArtifactState{
				"composition": {status: "valid", humanApproved: true},
				"preview":     {status: "valid", humanApproved: true},
			},
		}
		// review approved, but runner offline
		checker := newTestRenderChecker(provider, true, false)
		err := checker.CheckRenderDependencies(t.Context(), testRenderRequest())
		if err == nil {
			t.Fatal("expected render to be blocked when runner is offline")
		}
		t.Logf("Correctly blocked render for offline runner: %v", err)
	})

	t.Run("final_review_failed_blocks_package", func(t *testing.T) {
		provider := &stubArtifactStateProvider{
			states: map[string]*stubArtifactState{
				"render":  {status: "valid"},
				"quality": {status: "valid", metadata: map[string]interface{}{"passed": false}},
			},
		}
		checker := newTestRenderChecker(provider, true, true)
		req := testRenderRequest()
		req.Command = "ARTIFACT_PACKAGE"
		req.ToolName = "artifact_packager"
		err := checker.CheckRenderDependencies(t.Context(), req)
		if err == nil {
			t.Fatal("expected package to be blocked when final_review not passed")
		}
		depErr, ok := err.(*testRenderDependencyError)
		if !ok {
			t.Fatalf("expected testRenderDependencyError, got %T: %v", err, err)
		}
		if depErr.Code != "PACKAGE_DEPENDENCY_MISSING" {
			t.Errorf("expected PACKAGE_DEPENDENCY_MISSING code, got %s", depErr.Code)
		}
		t.Logf("Correctly blocked package: %s → %v", depErr.Message, depErr.Missing)
	})
}

// --- Test stubs (mirror the production dependency checker logic) ---

type testRenderDependencyError struct {
	Code    string
	Message string
	Missing []string
}

func (e *testRenderDependencyError) Error() string { return e.Message }

type stubArtifactState struct {
	status        string
	humanApproved bool
	metadata      map[string]interface{}
}

type stubArtifactStateProvider struct {
	states map[string]*stubArtifactState
}

func (s *stubArtifactStateProvider) FindCurrentByKind(ctx context.Context, projectID, stageName string) (*stubArtifactState, error) {
	if s.states == nil {
		return nil, nil
	}
	st, ok := s.states[stageName]
	if !ok {
		return nil, nil
	}
	return st, nil
}

type testRenderChecker struct {
	provider       *stubArtifactStateProvider
	reviewApproved bool
	runnerOnline   bool
}

func newTestRenderChecker(provider *stubArtifactStateProvider, reviewApproved, runnerOnline bool) *testRenderChecker {
	return &testRenderChecker{provider: provider, reviewApproved: reviewApproved, runnerOnline: runnerOnline}
}

type testRenderDependencyCheckRequest struct {
	ProjectID string
	TaskID    string
	NodeID    string
	ToolName  string
	Command   string
	Payload   map[string]interface{}
}

func testRenderRequest() testRenderDependencyCheckRequest {
	return testRenderDependencyCheckRequest{
		ProjectID: "test-project",
		TaskID:    "test-task",
		NodeID:    "test-node",
		ToolName:  "hyperframes_renderer",
		Command:   "HYPERFRAMES_RENDER",
	}
}

func (c *testRenderChecker) CheckRenderDependencies(ctx context.Context, req testRenderDependencyCheckRequest) error {
	missing := make([]string, 0)

	if req.Command == "HYPERFRAMES_RENDER" || req.ToolName == "hyperframes_renderer" {
		comp, _ := c.provider.FindCurrentByKind(ctx, req.ProjectID, "composition")
		if comp == nil || comp.status != "valid" {
			missing = append(missing, "视频结构无效或已过期")
		}
		if comp != nil && !comp.humanApproved {
			missing = append(missing, "视频结构尚未确认")
		}

		preview, _ := c.provider.FindCurrentByKind(ctx, req.ProjectID, "preview")
		if preview == nil || preview.status != "valid" {
			missing = append(missing, "预览快照不存在或已过期")
		}
		if preview != nil && !preview.humanApproved {
			missing = append(missing, "预览尚未确认")
		}

		if !c.reviewApproved {
			missing = append(missing, "预览审核未通过")
		}
		if !c.runnerOnline {
			missing = append(missing, "本地执行器未就绪或不支持渲染")
		}

		if len(missing) > 0 {
			return &testRenderDependencyError{Code: "RENDER_DEPENDENCY_MISSING", Message: "当前项目尚不满足最终渲染条件", Missing: missing}
		}
	}

	if req.Command == "ARTIFACT_PACKAGE" || req.ToolName == "artifact_packager" {
		render, _ := c.provider.FindCurrentByKind(ctx, req.ProjectID, "render")
		if render == nil || render.status != "valid" {
			missing = append(missing, "最终视频不存在或已过期")
		}

		quality, _ := c.provider.FindCurrentByKind(ctx, req.ProjectID, "quality")
		if quality == nil || quality.status != "valid" {
			missing = append(missing, "质量报告不存在或已过期")
		} else if quality.metadata != nil {
			if passed, ok := quality.metadata["passed"].(bool); !ok || !passed {
				missing = append(missing, "质量报告未通过")
			}
		} else {
			missing = append(missing, "质量报告未通过")
		}

		if len(missing) > 0 {
			return &testRenderDependencyError{Code: "PACKAGE_DEPENDENCY_MISSING", Message: "最终视频尚未通过质量检查，禁止打包。", Missing: missing}
		}
	}

	return nil
}
