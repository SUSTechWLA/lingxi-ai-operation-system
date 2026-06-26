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
	// FindCurrentByStageAndKind returns the current artifact of an exact stage+kind pair for a project.
	FindCurrentByStageAndKind(ctx context.Context, projectID, stageName, artifactKind string) (*ArtifactState, error)
}

// ArtifactState is a lightweight view of an artifact for dependency checking.
type ArtifactState struct {
	ID            string
	StageName     string
	Kind          string
	Status        string
	HumanApproved bool
	Metadata      map[string]interface{}
}

// ReviewApprovalChecker checks whether a review gate has been approved.
type ReviewApprovalChecker interface {
	// IsReviewApproved returns true if the review for a given project+stage has been approved.
	// It checks that the stage's current artifact is valid and human-approved.
	IsReviewApproved(ctx context.Context, projectID, stageName string) (bool, error)
}

// RunnerCapabilityChecker checks whether a local runner can execute a command.
type RunnerCapabilityChecker interface {
	// SupportsCommand returns true if any online runner supports the given command.
	SupportsCommand(ctx context.Context, command string) (bool, error)
}

// RepositoryReviewApprovalChecker implements ReviewApprovalChecker by checking
// whether the current artifact for a project+stage is valid and human-approved.
type RepositoryReviewApprovalChecker struct {
	artifactProvider ArtifactStateProvider
}

// NewRepositoryReviewApprovalChecker creates a review checker backed by artifact state.
func NewRepositoryReviewApprovalChecker(artifactProvider ArtifactStateProvider) *RepositoryReviewApprovalChecker {
	return &RepositoryReviewApprovalChecker{artifactProvider: artifactProvider}
}

// IsReviewApproved checks whether the current artifact for the given project+stage
// has been reviewed and approved by a human.
func (c *RepositoryReviewApprovalChecker) IsReviewApproved(ctx context.Context, projectID, stageName string) (bool, error) {
	artifactKind := reviewArtifactKindForStage(stageName)
	if artifactKind == "" {
		return false, nil
	}
	artifact, err := c.artifactProvider.FindCurrentByStageAndKind(ctx, projectID, stageName, artifactKind)
	if err != nil || artifact == nil {
		return false, err
	}
	return artifact.Status == "valid" && artifact.HumanApproved, nil
}

// RunnerCapabilityCheckerImpl implements RunnerCapabilityChecker by delegating
// to the local runner service.
type RunnerCapabilityCheckerImpl struct {
	runnerSvc RunnerService
}

// RunnerService is the subset of localrunner.Service needed for capability checks.
type RunnerService interface {
	SupportsCommand(ctx context.Context, command string) (bool, error)
}

// NewRunnerCapabilityChecker creates a runner capability checker.
func NewRunnerCapabilityChecker(runnerSvc RunnerService) *RunnerCapabilityCheckerImpl {
	return &RunnerCapabilityCheckerImpl{runnerSvc: runnerSvc}
}

// SupportsCommand checks whether any online runner can execute the given command.
func (c *RunnerCapabilityCheckerImpl) SupportsCommand(ctx context.Context, command string) (bool, error) {
	if c.runnerSvc == nil {
		return false, nil
	}
	return c.runnerSvc.SupportsCommand(ctx, command)
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
// Per spec section 6.4, all of the following must be satisfied before rendering:
//
//  1. VIDEO_COMPOSITION_SPEC must be valid and human-approved
//  2. HYPERFRAMES_PROJECT must be valid
//  3. PREVIEW_SNAPSHOTS must be valid and human-approved
//  4. Preview review must be APPROVED
//  5. Local runner must be online and support HYPERFRAMES_RENDER
func (c *RepositoryBackedRenderDependencyChecker) checkRenderGuard(
	ctx context.Context,
	req RenderDependencyCheckRequest,
) error {
	missing := make([]string, 0)

	// 1. VIDEO_COMPOSITION_SPEC must be valid and human-approved (stage: composition)
	composition, err := c.findArtifact(ctx, req.ProjectID, "composition", "VIDEO_COMPOSITION_SPEC")
	if err != nil {
		zap.L().Warn("render guard: cannot check composition artifact", zap.Error(err))
	}
	if composition == nil || composition.Status != "valid" {
		missing = append(missing, "视频结构无效或已过期")
	}
	if composition != nil && !composition.HumanApproved {
		missing = append(missing, "视频结构尚未确认")
	}

	// 2. HYPERFRAMES_PROJECT must be valid (stage: preview)
	project, err := c.findArtifact(ctx, req.ProjectID, "preview", "HYPERFRAMES_PROJECT")
	if err != nil {
		zap.L().Warn("render guard: cannot check hyperframes project artifact", zap.Error(err))
	}
	if project == nil || project.Status != "valid" {
		missing = append(missing, "渲染项目不存在或已过期")
	}

	// 3. PREVIEW_SNAPSHOTS must be valid and human-approved (stage: preview)
	preview, err := c.findArtifact(ctx, req.ProjectID, "preview", "PREVIEW_SNAPSHOTS")
	if err != nil {
		zap.L().Warn("render guard: cannot check preview artifact", zap.Error(err))
	}
	if preview == nil || preview.Status != "valid" {
		missing = append(missing, "预览快照不存在或已过期")
	}
	if preview != nil && !preview.HumanApproved {
		missing = append(missing, "预览尚未确认")
	}

	// 4. Preview review must be APPROVED
	if c.reviewChecker != nil {
		approved, err := c.reviewChecker.IsReviewApproved(ctx, req.ProjectID, "preview")
		if err != nil {
			zap.L().Warn("render guard: cannot check preview review", zap.Error(err))
		}
		if !approved {
			missing = append(missing, "预览审核未通过")
		}
	}

	// 5. Local runner must be online and support HYPERFRAMES_RENDER
	if c.runnerChecker != nil {
		supported, err := c.runnerChecker.SupportsCommand(ctx, "HYPERFRAMES_RENDER")
		if err != nil {
			zap.L().Warn("render guard: cannot check runner capability", zap.Error(err))
		}
		if !supported {
			missing = append(missing, "本地执行器未就绪或不支持渲染")
		}
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
// Per spec section 7.3, all of the following must be satisfied before packaging:
//
//  1. VIDEO must exist and be valid
//  2. FFMPEG_PROBE_REPORT must exist and be valid
//  3. FINAL_REVIEW must exist, be valid, and have metadata.passed == true
func (c *RepositoryBackedRenderDependencyChecker) checkPackageGuard(
	ctx context.Context,
	req RenderDependencyCheckRequest,
) error {
	missing := make([]string, 0)

	// 1. VIDEO must be valid (stage: render)
	video, err := c.findArtifact(ctx, req.ProjectID, "render", "VIDEO")
	if err != nil {
		zap.L().Warn("package guard: cannot check video artifact", zap.Error(err))
	}
	if video == nil || video.Status != "valid" {
		missing = append(missing, "最终视频不存在或已过期")
	}

	// 2. FFMPEG_PROBE_REPORT must be valid (stage: quality)
	probe, err := c.findArtifact(ctx, req.ProjectID, "quality", "FFMPEG_PROBE_REPORT")
	if err != nil {
		zap.L().Warn("package guard: cannot check ffmpeg probe artifact", zap.Error(err))
	}
	if probe == nil || probe.Status != "valid" {
		missing = append(missing, "视频检测报告不存在或已过期")
	}

	// 3. FINAL_REVIEW must be valid and have metadata.passed == true.
	review, err := c.findArtifact(ctx, req.ProjectID, "quality", "FINAL_REVIEW")
	if err != nil {
		zap.L().Warn("package guard: cannot check final review artifact", zap.Error(err))
	}
	if review == nil || review.Status != "valid" {
		missing = append(missing, "质量报告不存在或已过期")
	} else {
		if review.Metadata != nil {
			if passed, ok := review.Metadata["passed"].(bool); !ok || !passed {
				missing = append(missing, "质量报告未通过")
			}
		} else {
			missing = append(missing, "质量报告未通过")
		}
	}

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
	ctx context.Context, projectID, stageName, artifactKind string,
) (*ArtifactState, error) {
	if c.artifactProvider == nil {
		return nil, nil
	}
	return c.artifactProvider.FindCurrentByStageAndKind(ctx, projectID, stageName, artifactKind)
}

func reviewArtifactKindForStage(stageName string) string {
	switch stageName {
	case "proposal":
		return "VIDEO_PROPOSAL"
	case "script":
		return "VIDEO_SCRIPT"
	case "storyboard":
		return "CARD_PLAN"
	case "composition":
		return "VIDEO_COMPOSITION_SPEC"
	case "reference":
		return "REFERENCE_ASSET_PLAN"
	case "continuity":
		return "CONTINUITY_REPORT"
	case "preview":
		return "PREVIEW_SNAPSHOTS"
	case "render":
		return "VIDEO"
	case "quality":
		return "FINAL_REVIEW"
	case "package":
		return "PROJECT_PACKAGE"
	default:
		return ""
	}
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
