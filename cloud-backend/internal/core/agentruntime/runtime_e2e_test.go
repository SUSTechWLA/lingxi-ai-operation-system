package agentruntime

import (
	"context"
	"slices"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/artifact"
	"github.com/tangying-ai/aios-core/internal/core/localrunner"
	workerService "github.com/tangying-ai/aios-core/internal/core/worker/service"
)

func TestRuntime_TangyingDirector_FirstBeta(t *testing.T) {
	t.Run("preview_unapproved_blocks_render", func(t *testing.T) {
		checker := runtimeRenderChecker(
			map[string]*workerService.ArtifactState{
				"composition": {ID: "art_composition", Kind: "VIDEO_COMPOSITION_SPEC", Status: "valid", HumanApproved: true},
				"preview":     {ID: "art_preview", Kind: "PREVIEW_SNAPSHOTS", Status: "valid", HumanApproved: false},
			},
			false,
			true,
		)

		err := checker.CheckRenderDependencies(context.Background(), runtimeRenderRequest())
		depErr, ok := err.(*workerService.RenderDependencyError)
		if !ok || depErr.Code != "RENDER_DEPENDENCY_MISSING" {
			t.Fatalf("expected render dependency error, got %T: %v", err, err)
		}
	})

	t.Run("preview_approved_allows_render", func(t *testing.T) {
		checker := runtimeRenderChecker(
			map[string]*workerService.ArtifactState{
				"composition": {ID: "art_composition", Kind: "VIDEO_COMPOSITION_SPEC", Status: "valid", HumanApproved: true},
				"preview":     {ID: "art_preview", Kind: "PREVIEW_SNAPSHOTS", Status: "valid", HumanApproved: true},
			},
			true,
			true,
		)

		if err := checker.CheckRenderDependencies(context.Background(), runtimeRenderRequest()); err != nil {
			t.Fatalf("expected render to pass after preview approval, got %v", err)
		}
	})

	t.Run("script_edit_marks_preview_video_final_review_and_package_stale", func(t *testing.T) {
		downstream := artifact.DownstreamStaleArtifactKinds("VIDEO_SCRIPT")
		for _, stageName := range []string{"preview", "render", "quality", "package"} {
			if !slices.Contains(downstream, stageName) {
				t.Fatalf("script edit should stale stage %s, got %v", stageName, downstream)
			}
		}
		if slices.Contains(downstream, "script") {
			t.Fatalf("script edit must not stale the changed stage itself: %v", downstream)
		}
	})

	t.Run("local_runner_offline_blocks_render", func(t *testing.T) {
		checker := runtimeRenderChecker(
			map[string]*workerService.ArtifactState{
				"composition": {ID: "art_composition", Kind: "VIDEO_COMPOSITION_SPEC", Status: "valid", HumanApproved: true},
				"preview":     {ID: "art_preview", Kind: "PREVIEW_SNAPSHOTS", Status: "valid", HumanApproved: true},
			},
			true,
			false,
		)

		err := checker.CheckRenderDependencies(context.Background(), runtimeRenderRequest())
		depErr, ok := err.(*workerService.RenderDependencyError)
		if !ok || depErr.Code != "RENDER_DEPENDENCY_MISSING" {
			t.Fatalf("expected offline runner to block render, got %T: %v", err, err)
		}
	})

	t.Run("final_review_failed_blocks_package", func(t *testing.T) {
		checker := runtimeRenderChecker(
			map[string]*workerService.ArtifactState{
				"render":  {ID: "art_video", Kind: "VIDEO", Status: "valid"},
				"quality": {ID: "art_final_review", Kind: "FINAL_REVIEW", Status: "valid", Metadata: map[string]interface{}{"passed": false}},
			},
			true,
			true,
		)

		err := checker.CheckRenderDependencies(context.Background(), runtimePackageRequest())
		depErr, ok := err.(*workerService.RenderDependencyError)
		if !ok || depErr.Code != "PACKAGE_DEPENDENCY_MISSING" {
			t.Fatalf("expected failed final review to block package, got %T: %v", err, err)
		}
	})

	t.Run("final_review_passed_allows_package", func(t *testing.T) {
		checker := runtimeRenderChecker(
			map[string]*workerService.ArtifactState{
				"render":  {ID: "art_video", Kind: "VIDEO", Status: "valid"},
				"quality": {ID: "art_final_review", Kind: "FINAL_REVIEW", Status: "valid", Metadata: map[string]interface{}{"passed": true}},
			},
			true,
			true,
		)

		if err := checker.CheckRenderDependencies(context.Background(), runtimePackageRequest()); err != nil {
			t.Fatalf("expected package to pass after final review passed, got %v", err)
		}
	})
}

func runtimeRenderChecker(
	states map[string]*workerService.ArtifactState,
	reviewApproved bool,
	runnerOnline bool,
) *workerService.RepositoryBackedRenderDependencyChecker {
	provider := runtimeArtifactProvider{states: states}
	return workerService.NewRepositoryBackedRenderDependencyChecker(
		provider,
		runtimeReviewChecker{approved: reviewApproved},
		runtimeRunnerChecker{supported: runnerOnline},
	)
}

func runtimeRenderRequest() workerService.RenderDependencyCheckRequest {
	return workerService.RenderDependencyCheckRequest{
		ProjectID: "project-1",
		TaskID:    "task-1",
		NodeID:    "render_exec",
		ToolName:  "hyperframes_renderer",
		Command:   localrunner.CommandHyperFramesRender,
	}
}

func runtimePackageRequest() workerService.RenderDependencyCheckRequest {
	return workerService.RenderDependencyCheckRequest{
		ProjectID: "project-1",
		TaskID:    "task-1",
		NodeID:    "package_exec",
		ToolName:  "artifact_packager",
		Command:   localrunner.CommandArtifactPackage,
	}
}

type runtimeArtifactProvider struct {
	states map[string]*workerService.ArtifactState
}

func (p runtimeArtifactProvider) FindCurrentByKind(_ context.Context, _ string, stageName string) (*workerService.ArtifactState, error) {
	return p.states[stageName], nil
}

type runtimeReviewChecker struct {
	approved bool
}

func (c runtimeReviewChecker) IsReviewApproved(_ context.Context, _, _ string) (bool, error) {
	return c.approved, nil
}

type runtimeRunnerChecker struct {
	supported bool
}

func (c runtimeRunnerChecker) SupportsCommand(_ context.Context, _ string) (bool, error) {
	return c.supported, nil
}
