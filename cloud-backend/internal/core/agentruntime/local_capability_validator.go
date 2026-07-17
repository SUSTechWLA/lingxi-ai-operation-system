package agentruntime

import (
	"context"
	"fmt"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

// LocalCapabilityProvider is the interface that PlanGuard uses to check
// whether a user's local execution plane can satisfy the requirements of
// a plan step before the plan is committed. Implementations typically
// query the runner registry to verify online runner capabilities.
type LocalCapabilityProvider interface {
	// HasOnlineRunner checks whether the given user has at least one online
	// local runner that can accept jobs.
	HasOnlineRunner(ctx context.Context, userID string) (bool, error)

	// SupportsCommand checks whether any online runner for the given user
	// has registered the specified local command as available.
	SupportsCommand(ctx context.Context, userID string, command string) (bool, error)

	// SatisfiesRequirements checks whether the user's online runners satisfy
	// the given local requirements. Returns a list of unsatisfied requirements
	// (human-readable descriptions) and an error.
	SatisfiesRequirements(ctx context.Context, userID string, req *tool.LocalRequirements) (bool, []string, error)
}

// LocalCapabilityValidator checks whether the local execution plane can satisfy
// the requirements of a plan step before the plan is committed.
type LocalCapabilityValidator struct {
	provider LocalCapabilityProvider
}

// NewLocalCapabilityValidator creates a validator backed by the given provider.
// If provider is nil, local capability checks are skipped (graceful degradation).
func NewLocalCapabilityValidator(provider LocalCapabilityProvider) *LocalCapabilityValidator {
	return &LocalCapabilityValidator{provider: provider}
}

// ValidateForLocalExecution checks whether the given step can be executed on
// the local plane. Returns a human-readable error if the step cannot be executed
// locally, or nil if it can (or if no provider is configured).
func (v *LocalCapabilityValidator) ValidateForLocalExecution(ctx context.Context, userID string, step AgentStep, manifest *tool.ToolManifest) error {
	if v == nil || v.provider == nil {
		return nil // graceful degradation: skip local checks when provider is unavailable
	}
	if manifest == nil {
		return nil
	}
	if manifest.ExecutionPlane != tool.ExecutionPlaneLocal && manifest.ExecutionPlane != tool.ExecutionPlaneHybrid {
		return nil // not a local-execution tool
	}

	// 1. Check if the user has an online runner.
	hasRunner, err := v.provider.HasOnlineRunner(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to check local runner availability: %w", err)
	}
	if !hasRunner {
		return fmt.Errorf(
			"step %q uses tool %q which requires local execution (executionPlane=%s), "+
				"but no online local runner was found for this user. "+
				"请启动桌面端 local-backend 并确保已注册 runner。",
			step.ID, step.Tool, manifest.ExecutionPlane,
		)
	}

	// 2. Check if the runner supports the required local command.
	if manifest.LocalCommand != "" {
		supports, err := v.provider.SupportsCommand(ctx, userID, manifest.LocalCommand)
		if err != nil {
			return fmt.Errorf("failed to check local command support for %q: %w", manifest.LocalCommand, err)
		}
		if !supports {
			return fmt.Errorf(
				"step %q uses tool %q which requires local command %q, "+
					"but no online runner supports this command. "+
					"请确保 local-backend 已注册 %s executor。",
				step.ID, step.Tool, manifest.LocalCommand, manifest.LocalCommand,
			)
		}
	}

	// 3. Check local requirements (e.g. ffmpeg, node availability).
	if len(manifest.LocalRequirements.Commands) > 0 || len(manifest.LocalRequirements.OS) > 0 {
		satisfied, missing, err := v.provider.SatisfiesRequirements(ctx, userID, &manifest.LocalRequirements)
		if err != nil {
			return fmt.Errorf("failed to check local requirements for %q: %w", step.Tool, err)
		}
		if !satisfied {
			return fmt.Errorf(
				"step %q uses tool %q which has local requirements that are not met: %v. "+
					"请确保本地环境满足工具依赖。",
				step.ID, step.Tool, missing,
			)
		}
	}

	return nil
}
