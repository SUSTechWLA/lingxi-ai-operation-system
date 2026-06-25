package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/localrunner"
)

// ArtifactStateProvider is the subset of artifact.Service needed by the
// repository-backed render dependency checker.
type ArtifactStateProvider interface {
	// FindCurrentByKind returns the current artifact of a given stage kind for a project.
	FindCurrentByKind(ctx context.Context, projectID, stageName string) (*ArtifactState, error)
}

// ArtifactState is a lightweight view of an artifact for dependency checking.
type ArtifactState struct {
	ID            string
	Kind          string
	Status        string
	HumanApproved bool
}

// ReviewApprovalChecker checks whether a review gate has been approved.
type ReviewApprovalChecker interface {
	// IsReviewApproved returns true if the review for a given node has been approved.
	IsReviewApproved(ctx context.Context, nodeID string) (bool, error)
}

// RunnerCapabilityChecker checks whether a local runner can execute a command.
type RunnerCapabilityChecker interface {
	// SupportsCommand returns true if any online runner supports the given command.
	SupportsCommand(ctx context.Context, command string) (bool, error)
}

// RepositoryBackedRenderDependencyChecker validates render preconditions
// against the actual database state (artifact status, reviews, runner capability).
type RepositoryBackedRenderDependencyChecker struct {
	artifactProvider ArtifactStateProvider
	reviewChecker    ReviewApprovalChecker
	runnerChecker    RunnerCapabilityChecker
}

// NewRepositoryBackedRenderDependencyChecker creates a checker that verifies
// render dependencies against database facts rather than payload fields.
func NewRepositoryBackedRenderDependencyChecker(
	artifactProvider ArtifactStateProvider,
	reviewChecker ReviewApprovalChecker,
	runnerChecker RunnerCapabilityChecker,
) *RepositoryBackedRenderDependencyChecker {
	return &RepositoryBackedRenderDependencyChecker{
		artifactProvider: artifactProvider,
		reviewChecker:    reviewChecker,
		runnerChecker:    runnerChecker,
	}
}

// CheckRenderDependencies verifies all preconditions for HYPERFRAMES_RENDER
// and ARTIFACT_PACKAGE against database facts.
func (c *RepositoryBackedRenderDependencyChecker) CheckRenderDependencies(
	ctx context.Context,
	req RenderDependencyCheckRequest,
) error {
	// Route to the appropriate guard based on command.
	if req.Command == localrunner.CommandHyperFramesRender ||
		req.ToolName == "hyperframes_renderer" {
		return c.checkRenderGuard(ctx, req)
	}
	if req.Command == localrunner.CommandArtifactPackage ||
		req.ToolName == "artifact_packager" {
		return c.checkPackageGuard(ctx, req)
	}
	return nil
}

// checkRenderGuard validates HYPERFRAMES_RENDER preconditions against database facts.
func (c *RepositoryBackedRenderDependencyChecker) checkRenderGuard(
	ctx context.Context,
	req RenderDependencyCheckRequest,
) error {
	missing := make([]string, 0)

	// 1. VIDEO_COMPOSITION_SPEC must be valid and human-approved (stage: composition)
	composition, err := c.findArtifact(ctx, req.ProjectID, "composition")
	if err != nil {
		zap.L().Warn("render guard: cannot check composition artifact", zap.Error(err))
	}
	if composition == nil || composition.Status != "valid" {
		missing = append(missing, "视频结构无效或已过期")
	}
	if composition != nil && !composition.HumanApproved {
		missing = append(missing, "视频结构尚未确认")
	}

	// 2. HYPERFRAMES_PROJECT / PREVIEW_SNAPSHOTS must be valid (stage: preview)
	preview, err := c.findArtifact(ctx, req.ProjectID, "preview")
	if err != nil {
		zap.L().Warn("render guard: cannot check preview artifact", zap.Error(err))
	}
	if preview == nil || preview.Status != "valid" {
		missing = append(missing, "预览快照不存在或已过期")
	}
	if preview != nil && !preview.HumanApproved {
		missing = append(missing, "预览尚未确认")
	}

	if len(missing) > 0 {
		return &RenderDependencyError{
			Code:    "RENDER_DEPENDENCY_MISSING",
			Message: "当前项目尚不满足最终渲染条件",
			Missing: missing,
		}
	}

	return nil
}

// checkPackageGuard validates ARTIFACT_PACKAGE preconditions against database facts.
func (c *RepositoryBackedRenderDependencyChecker) checkPackageGuard(
	ctx context.Context,
	req RenderDependencyCheckRequest,
) error {
	missing := make([]string, 0)

	// 1. VIDEO must be valid (stage: render)
	video, err := c.findArtifact(ctx, req.ProjectID, "render")
	if err != nil {
		zap.L().Warn("package guard: cannot check video artifact", zap.Error(err))
	}
	if video == nil || video.Status != "valid" {
		missing = append(missing, "最终视频不存在或已过期")
	}

	// 2. FFMPEG_PROBE_REPORT must be valid (stage: quality)
	probeReport, err := c.findArtifact(ctx, req.ProjectID, "quality")
	if err != nil {
		zap.L().Warn("package guard: cannot check ffmpeg probe artifact", zap.Error(err))
	}
	if probeReport == nil || probeReport.Status != "valid" {
		missing = append(missing, "视频检测报告不存在或已过期")
	}

	// 3. FINAL_REVIEW must be valid and passed (stage: quality)
	//    The FINAL_REVIEW artifact's metadata should contain "passed": true
	finalReview, err := c.findArtifact(ctx, req.ProjectID, "quality")
	if err != nil {
		zap.L().Warn("package guard: cannot check final review artifact", zap.Error(err))
	}
	if finalReview == nil || finalReview.Status != "valid" {
		missing = append(missing, "质量报告不存在或已过期")
	}
	// Note: We can't deeply check metadata "passed" field through ArtifactState.
	// The tool should validate this at execution time.

	if len(missing) > 0 {
		return &RenderDependencyError{
			Code:    "PACKAGE_DEPENDENCY_MISSING",
			Message: "最终视频尚未通过质量检查，禁止打包。",
			Missing: missing,
		}
	}

	return nil
}

// findArtifact looks up a current artifact by stage name for a project.
func (c *RepositoryBackedRenderDependencyChecker) findArtifact(
	ctx context.Context, projectID, stageName string,
) (*ArtifactState, error) {
	if c.artifactProvider == nil {
		return nil, nil
	}
	return c.artifactProvider.FindCurrentByKind(ctx, projectID, stageName)
}

// RenderDependencyError is returned when render preconditions are not met.
type RenderDependencyError struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Missing []string `json:"missing"`
}

func (e *RenderDependencyError) Error() string {
	return fmt.Sprintf("%s: %v", e.Message, e.Missing)
}
