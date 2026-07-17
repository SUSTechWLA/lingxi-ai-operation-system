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
				runtimeArtifactKey("composition", "VIDEO_COMPOSITION_SPEC"): {ID: "art_composition", StageName: "composition", Kind: "VIDEO_COMPOSITION_SPEC", Status: "valid", HumanApproved: true},
				runtimeArtifactKey("preview", "HYPERFRAMES_PROJECT"):        {ID: "art_project", StageName: "preview", Kind: "HYPERFRAMES_PROJECT", Status: "valid"},
				runtimeArtifactKey("preview", "PREVIEW_SNAPSHOTS"):          {ID: "art_preview", StageName: "preview", Kind: "PREVIEW_SNAPSHOTS", Status: "valid", HumanApproved: false},
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
				runtimeArtifactKey("composition", "VIDEO_COMPOSITION_SPEC"): {ID: "art_composition", StageName: "composition", Kind: "VIDEO_COMPOSITION_SPEC", Status: "valid", HumanApproved: true},
				runtimeArtifactKey("preview", "HYPERFRAMES_PROJECT"):        {ID: "art_project", StageName: "preview", Kind: "HYPERFRAMES_PROJECT", Status: "valid"},
				runtimeArtifactKey("preview", "PREVIEW_SNAPSHOTS"):          {ID: "art_preview", StageName: "preview", Kind: "PREVIEW_SNAPSHOTS", Status: "valid", HumanApproved: true},
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
				runtimeArtifactKey("composition", "VIDEO_COMPOSITION_SPEC"): {ID: "art_composition", StageName: "composition", Kind: "VIDEO_COMPOSITION_SPEC", Status: "valid", HumanApproved: true},
				runtimeArtifactKey("preview", "HYPERFRAMES_PROJECT"):        {ID: "art_project", StageName: "preview", Kind: "HYPERFRAMES_PROJECT", Status: "valid"},
				runtimeArtifactKey("preview", "PREVIEW_SNAPSHOTS"):          {ID: "art_preview", StageName: "preview", Kind: "PREVIEW_SNAPSHOTS", Status: "valid", HumanApproved: true},
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
				runtimeArtifactKey("render", "VIDEO"):                {ID: "art_video", StageName: "render", Kind: "VIDEO", Status: "valid"},
				runtimeArtifactKey("quality", "FFMPEG_PROBE_REPORT"): {ID: "art_probe", StageName: "quality", Kind: "FFMPEG_PROBE_REPORT", Status: "valid"},
				runtimeArtifactKey("quality", "FINAL_REVIEW"):        {ID: "art_final_review", StageName: "quality", Kind: "FINAL_REVIEW", Status: "valid", Metadata: map[string]interface{}{"passed": false}},
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
				runtimeArtifactKey("render", "VIDEO"):                {ID: "art_video", StageName: "render", Kind: "VIDEO", Status: "valid"},
				runtimeArtifactKey("quality", "FFMPEG_PROBE_REPORT"): {ID: "art_probe", StageName: "quality", Kind: "FFMPEG_PROBE_REPORT", Status: "valid"},
				runtimeArtifactKey("quality", "FINAL_REVIEW"):        {ID: "art_final_review", StageName: "quality", Kind: "FINAL_REVIEW", Status: "valid", Metadata: map[string]interface{}{"passed": true}},
			},
			true,
			true,
		)

		if err := checker.CheckRenderDependencies(context.Background(), runtimePackageRequest()); err != nil {
			t.Fatalf("expected package to pass after final review passed, got %v", err)
		}
	})

	t.Run("unrelated_preview_artifact_does_not_satisfy_render", func(t *testing.T) {
		checker := runtimeRenderChecker(
			map[string]*workerService.ArtifactState{
				runtimeArtifactKey("composition", "VIDEO_COMPOSITION_SPEC"): {ID: "art_composition", StageName: "composition", Kind: "VIDEO_COMPOSITION_SPEC", Status: "valid", HumanApproved: true},
				runtimeArtifactKey("preview", "PREVIEW_REPORT"):             {ID: "art_preview_report", StageName: "preview", Kind: "PREVIEW_REPORT", Status: "valid", HumanApproved: true},
				runtimeArtifactKey("preview", "PREVIEW_SNAPSHOTS"):          {ID: "art_preview", StageName: "preview", Kind: "PREVIEW_SNAPSHOTS", Status: "valid", HumanApproved: true},
			},
			true,
			true,
		)

		err := checker.CheckRenderDependencies(context.Background(), runtimeRenderRequest())
		depErr, ok := err.(*workerService.RenderDependencyError)
		if !ok || depErr.Code != "RENDER_DEPENDENCY_MISSING" {
			t.Fatalf("expected missing hyperframes project to block render, got %T: %v", err, err)
		}
	})

	t.Run("unrelated_quality_artifact_does_not_satisfy_package", func(t *testing.T) {
		checker := runtimeRenderChecker(
			map[string]*workerService.ArtifactState{
				runtimeArtifactKey("render", "VIDEO"):           {ID: "art_video", StageName: "render", Kind: "VIDEO", Status: "valid"},
				runtimeArtifactKey("quality", "PREVIEW_REPORT"): {ID: "art_quality_other", StageName: "quality", Kind: "PREVIEW_REPORT", Status: "valid", Metadata: map[string]interface{}{"passed": true}},
				runtimeArtifactKey("quality", "FINAL_REVIEW"):   {ID: "art_final_review", StageName: "quality", Kind: "FINAL_REVIEW", Status: "valid", Metadata: map[string]interface{}{"passed": true}},
			},
			true,
			true,
		)

		err := checker.CheckRenderDependencies(context.Background(), runtimePackageRequest())
		depErr, ok := err.(*workerService.RenderDependencyError)
		if !ok || depErr.Code != "PACKAGE_DEPENDENCY_MISSING" {
			t.Fatalf("expected missing ffmpeg probe to block package, got %T: %v", err, err)
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

func (p runtimeArtifactProvider) FindCurrentByStageAndKind(_ context.Context, _ string, stageName string, artifactKind string) (*workerService.ArtifactState, error) {
	return p.states[runtimeArtifactKey(stageName, artifactKind)], nil
}

func runtimeArtifactKey(stageName, artifactKind string) string {
	return stageName + "|" + artifactKind
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
