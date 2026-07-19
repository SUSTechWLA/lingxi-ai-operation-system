package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

var ErrShotVersionConflict = errors.New("shot version conflict")

var allowedRegenerationScopes = map[string]bool{
	"prompt": true, "reference": true, "base_media": true,
	"overlay": true, "audio_alignment": true, "full_shot": true,
}

var allowedShotLocks = map[string]bool{
	"duration": true, "narration": true, "character": true,
	"wardrobe": true, "scene": true, "camera": true,
	"first_frame": true, "last_frame": true,
	"reference_set": true, "accepted_overlay": true,
}

type ShotRegenerationImpact struct {
	ShotID                   string   `json:"shotId"`
	AffectedShotIDs          []string `json:"affectedShotIds"`
	InvalidatesFinalAssembly bool     `json:"invalidatesFinalAssembly"`
	RegeneratesOtherShots    bool     `json:"regeneratesOtherShots"`
	EstimatedDurationSec     int      `json:"estimatedDurationSec"`
	RequiresConfirmation     bool     `json:"requiresConfirmation"`
}

func (s *CreationService) RegenerateShotV2(
	ctx context.Context,
	userID, projectID, shotID string,
	req RegenerateShotRequest,
) (RegenerateShotResult, error) {
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return RegenerateShotResult{}, fmt.Errorf("idempotency key is required")
	}

	s.mutationMutex.Lock()
	project, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		s.mutationMutex.Unlock()
		return RegenerateShotResult{}, err
	}
	if taskID := state.IdempotencyTasks[req.IdempotencyKey]; taskID != "" {
		result, resultErr := regenerationResult(state, taskID)
		s.mutationMutex.Unlock()
		return result, resultErr
	}
	if err := validateRegenerateShotRequest(req); err != nil {
		s.mutationMutex.Unlock()
		return RegenerateShotResult{}, err
	}

	shot, index, ok := findShot(state.Shots, shotID)
	if !ok {
		s.mutationMutex.Unlock()
		return RegenerateShotResult{}, fmt.Errorf("shot %s not found", shotID)
	}
	if shot.Locked {
		s.mutationMutex.Unlock()
		return RegenerateShotResult{}, fmt.Errorf("shot %s is locked; unlock before regeneration", shot.ID)
	}
	if shot.Version != req.BaseVersion {
		s.mutationMutex.Unlock()
		return RegenerateShotResult{}, fmt.Errorf("%w: shot %s is version %d, request is based on version %d", ErrShotVersionConflict, shot.ID, shot.Version, req.BaseVersion)
	}
	if err := model.ValidateShotDuration(shot); err != nil {
		s.mutationMutex.Unlock()
		return RegenerateShotResult{}, err
	}

	now := time.Now()
	revision := model.ShotRevision{
		RevisionID: "shot-revision-" + uuid.NewString(),
		ShotID:     shot.ID,
		Version:    shot.Version,
		Reason:     regenerationRevisionReason(req),
		Snapshot:   cloneShot(shot),
		CreatedAt:  now,
	}
	state.ShotHistory[shot.ID] = append(state.ShotHistory[shot.ID], revision)

	if shot.LastRejectReason != "" {
		shot.PromptConstraints.MustInclude = append([]string{"regenerate using reject reason: " + shot.LastRejectReason}, shot.PromptConstraints.MustInclude...)
	}
	if strings.TrimSpace(req.Instruction) != "" {
		shot.PromptConstraints.MustInclude = append(shot.PromptConstraints.MustInclude, strings.TrimSpace(req.Instruction))
	}
	shot.Version++
	shot.AcceptedCandidateID = ""
	shot.ReviewStatus = model.ReviewStatusPending
	shot.Stale = true
	shot.UpdatedAt = now
	state.Shots[index] = shot

	task := model.ShotRegenerationTask{
		TaskID:         "shot-regeneration-" + uuid.NewString(),
		ShotID:         shot.ID,
		BaseVersion:    req.BaseVersion,
		Scope:          req.Scope,
		Locks:          append([]string(nil), req.Locks...),
		Instruction:    strings.TrimSpace(req.Instruction),
		IdempotencyKey: req.IdempotencyKey,
		Status:         "queued",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	state.RegenerationTasks[task.TaskID] = task
	state.IdempotencyTasks[req.IdempotencyKey] = task.TaskID
	state.AssemblyDirty = true
	if err := s.save(ctx, userID, project, state); err != nil {
		s.mutationMutex.Unlock()
		return RegenerateShotResult{}, err
	}
	s.mutationMutex.Unlock()

	result := RegenerateShotResult{Shot: shot, Task: task}
	if s.dispatcher == nil {
		return result, nil
	}
	runID, dispatchErr := s.dispatcher.EnqueueShotRegeneration(ctx, userID, projectID, task)
	if dispatchErr != nil {
		failed, persistErr := s.updateRegenerationTask(ctx, userID, projectID, task.TaskID, func(durable *model.ShotRegenerationTask) {
			durable.Status = "failed"
			durable.FailureReason = dispatchErr.Error()
		})
		result.Task = failed
		if persistErr != nil {
			return result, fmt.Errorf("dispatch shot regeneration: %v; persist failure: %w", dispatchErr, persistErr)
		}
		return result, fmt.Errorf("dispatch shot regeneration: %w", dispatchErr)
	}
	if strings.TrimSpace(runID) != "" {
		durable, persistErr := s.updateRegenerationTask(ctx, userID, projectID, task.TaskID, func(current *model.ShotRegenerationTask) {
			current.RunID = strings.TrimSpace(runID)
		})
		if persistErr != nil {
			return result, persistErr
		}
		result.Task = durable
	}
	return result, nil
}

func (s *CreationService) GetShotHistory(ctx context.Context, userID, projectID, shotID string) ([]model.ShotRevision, error) {
	_, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	shot, _, ok := findShot(state.Shots, shotID)
	if !ok {
		return nil, fmt.Errorf("shot %s not found", shotID)
	}
	history := append([]model.ShotRevision(nil), state.ShotHistory[shotID]...)
	history = append(history, model.ShotRevision{
		RevisionID: "current", ShotID: shot.ID, Version: shot.Version,
		Reason: "current", Snapshot: cloneShot(shot), CreatedAt: shot.UpdatedAt,
	})
	return history, nil
}

func (s *CreationService) PreviewShotRegeneration(ctx context.Context, userID, projectID, shotID string) (ShotRegenerationImpact, error) {
	shot, err := s.GetShot(ctx, userID, projectID, shotID)
	if err != nil {
		return ShotRegenerationImpact{}, err
	}
	return ShotRegenerationImpact{
		ShotID: shot.ID, AffectedShotIDs: []string{shot.ID},
		InvalidatesFinalAssembly: true, RegeneratesOtherShots: false,
		EstimatedDurationSec: shot.DurationSec, RequiresConfirmation: true,
	}, nil
}

func (s *CreationService) CompleteShotRegeneration(
	ctx context.Context,
	userID, projectID, taskID string,
	candidate model.ShotCandidate,
) error {
	s.mutationMutex.Lock()
	defer s.mutationMutex.Unlock()
	project, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return err
	}
	task, ok := state.RegenerationTasks[taskID]
	if !ok {
		return fmt.Errorf("shot regeneration task %s not found", taskID)
	}
	if strings.TrimSpace(candidate.ShotID) != task.ShotID {
		return fmt.Errorf("shot regeneration candidate targets %s, durable task targets %s", candidate.ShotID, task.ShotID)
	}
	if strings.TrimSpace(candidate.CandidateID) == "" {
		return fmt.Errorf("shot regeneration candidate id is required")
	}
	shot, index, ok := findShot(state.Shots, task.ShotID)
	if !ok {
		return fmt.Errorf("shot %s not found for regeneration task %s", task.ShotID, taskID)
	}
	for _, existing := range shot.Candidates {
		if existing.CandidateID == candidate.CandidateID {
			if task.Status == "completed" {
				return nil
			}
			return fmt.Errorf("candidate %s already belongs to shot %s", candidate.CandidateID, shot.ID)
		}
	}
	if task.Status == "completed" {
		return fmt.Errorf("shot regeneration task %s already completed", taskID)
	}
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = time.Now()
	}
	if candidate.Status == "" {
		candidate.Status = model.CandidateRendered
	}
	shot.Candidates = append(shot.Candidates, candidate)
	shot.UpdatedAt = time.Now()
	state.Shots[index] = shot
	task.Status = "completed"
	task.FailureReason = ""
	task.UpdatedAt = time.Now()
	state.RegenerationTasks[taskID] = task
	return s.save(ctx, userID, project, state)
}

func (s *CreationService) FailShotRegeneration(ctx context.Context, userID, projectID, taskID, reason string) error {
	_, err := s.updateRegenerationTask(ctx, userID, projectID, taskID, func(task *model.ShotRegenerationTask) {
		if task.Status == "completed" {
			return
		}
		task.Status = "failed"
		task.FailureReason = strings.TrimSpace(reason)
	})
	return err
}

func (s *CreationService) updateRegenerationTask(
	ctx context.Context,
	userID, projectID, taskID string,
	update func(*model.ShotRegenerationTask),
) (model.ShotRegenerationTask, error) {
	s.mutationMutex.Lock()
	defer s.mutationMutex.Unlock()
	project, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return model.ShotRegenerationTask{}, err
	}
	task, ok := state.RegenerationTasks[taskID]
	if !ok {
		return model.ShotRegenerationTask{}, fmt.Errorf("shot regeneration task %s not found", taskID)
	}
	update(&task)
	task.UpdatedAt = time.Now()
	state.RegenerationTasks[taskID] = task
	if err := s.save(ctx, userID, project, state); err != nil {
		return model.ShotRegenerationTask{}, err
	}
	return task, nil
}

func regenerationResult(state model.ShotDrivenState, taskID string) (RegenerateShotResult, error) {
	task, ok := state.RegenerationTasks[taskID]
	if !ok {
		return RegenerateShotResult{}, fmt.Errorf("idempotency task %s not found", taskID)
	}
	shot, _, ok := findShot(state.Shots, task.ShotID)
	if !ok {
		return RegenerateShotResult{}, fmt.Errorf("shot %s not found for task %s", task.ShotID, taskID)
	}
	return RegenerateShotResult{Shot: shot, Task: task}, nil
}

func validateRegenerateShotRequest(req RegenerateShotRequest) error {
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return fmt.Errorf("idempotency key is required")
	}
	if !allowedRegenerationScopes[req.Scope] {
		return fmt.Errorf("invalid shot regeneration scope %q", req.Scope)
	}
	seen := map[string]bool{}
	for _, lock := range req.Locks {
		if !allowedShotLocks[lock] {
			return fmt.Errorf("invalid shot regeneration lock %q", lock)
		}
		if seen[lock] {
			return fmt.Errorf("duplicate shot regeneration lock %q", lock)
		}
		seen[lock] = true
	}
	return nil
}

func regenerationRevisionReason(req RegenerateShotRequest) string {
	if instruction := strings.TrimSpace(req.Instruction); instruction != "" {
		return instruction
	}
	return "regenerate scope: " + req.Scope
}

func cloneShot(shot model.ShotUnit) model.ShotUnit {
	cloned := shot
	cloned.ScreenText = append([]string(nil), shot.ScreenText...)
	cloned.Candidates = append([]model.ShotCandidate(nil), shot.Candidates...)
	cloned.RepairPlans = append([]model.RepairPlan(nil), shot.RepairPlans...)
	cloned.PromptConstraints.MustInclude = append([]string(nil), shot.PromptConstraints.MustInclude...)
	cloned.PromptConstraints.MustAvoid = append([]string(nil), shot.PromptConstraints.MustAvoid...)
	return cloned
}
