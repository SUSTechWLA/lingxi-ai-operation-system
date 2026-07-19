package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

var (
	ErrShotVersionConflict     = errors.New("shot version conflict")
	ErrShotIdempotencyConflict = errors.New("shot regeneration idempotency conflict")
	errProjectRevisionConflict = errors.New("video project config revision conflict")
)

type ShotRegenerationProvenance struct {
	TaskID string
	RunID  string
	ShotID string
}

const (
	maxProjectCASAttempts         = 4
	shotRegenerationDispatchLease = 30 * time.Second
	ShotRegenerationQueued        = "queued"
	ShotRegenerationDispatching   = "dispatching"
	ShotRegenerationRunning       = "running"
	ShotRegenerationCompleted     = "completed"
	ShotRegenerationFailed        = "failed"
)

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
	if err := validateRegenerateShotRequest(req); err != nil {
		return RegenerateShotResult{}, err
	}
	fingerprint := shotRegenerationFingerprint(shotID, req)
	idempotencyScope := shotRegenerationIdempotencyScope(shotID, req.IdempotencyKey)
	for attempt := 0; attempt < maxProjectCASAttempts; attempt++ {
		project, state, err := s.load(ctx, userID, projectID)
		if err != nil {
			return RegenerateShotResult{}, err
		}
		taskID := state.IdempotencyTasks[idempotencyScope]
		if taskID == "" {
			legacyTaskID := state.IdempotencyTasks[req.IdempotencyKey]
			if legacyTask := state.RegenerationTasks[legacyTaskID]; legacyTaskID != "" && legacyTask.ShotID == shotID {
				taskID = legacyTaskID
			}
		}
		if taskID != "" {
			result, err := regenerationResult(state, taskID)
			if err != nil {
				return RegenerateShotResult{}, err
			}
			if result.Task.ShotID != shotID || durableTaskFingerprint(result.Task) != fingerprint {
				return RegenerateShotResult{}, fmt.Errorf("%w: key %q was used with different regeneration input", ErrShotIdempotencyConflict, req.IdempotencyKey)
			}
			return s.dispatchDurableTask(ctx, userID, projectID, result)
		}
		shot, index, ok := findShot(state.Shots, shotID)
		if !ok {
			return RegenerateShotResult{}, fmt.Errorf("shot %s not found", shotID)
		}
		if shot.Locked {
			return RegenerateShotResult{}, fmt.Errorf("shot %s is locked; unlock before regeneration", shot.ID)
		}
		if shot.Version != req.BaseVersion {
			return RegenerateShotResult{}, shotVersionConflict(shot, req.BaseVersion)
		}
		if err := model.ValidateShotDuration(shot); err != nil {
			return RegenerateShotResult{}, err
		}
		now := time.Now()
		state.ShotHistory[shot.ID] = append(state.ShotHistory[shot.ID], model.ShotRevision{
			RevisionID: "shot-revision-" + uuid.NewString(), ShotID: shot.ID, Version: shot.Version,
			Reason: regenerationRevisionReason(req), Snapshot: cloneShot(shot), CreatedAt: now,
		})
		if shot.LastRejectReason != "" {
			shot.PromptConstraints.MustInclude = append([]string{"regenerate using reject reason: " + shot.LastRejectReason}, shot.PromptConstraints.MustInclude...)
		}
		if instruction := strings.TrimSpace(req.Instruction); instruction != "" {
			shot.PromptConstraints.MustInclude = append(shot.PromptConstraints.MustInclude, instruction)
		}
		shot.Version++
		shot.AcceptedCandidateID = ""
		shot.ReviewStatus = model.ReviewStatusPending
		shot.Stale = true
		shot.UpdatedAt = now
		state.Shots[index] = shot
		task := model.ShotRegenerationTask{
			TaskID: "shot-regeneration-" + uuid.NewString(), ShotID: shot.ID,
			BaseVersion: req.BaseVersion, Scope: req.Scope, Locks: append([]string(nil), req.Locks...),
			Instruction: strings.TrimSpace(req.Instruction), IdempotencyKey: req.IdempotencyKey,
			RequestFingerprint: fingerprint, Status: ShotRegenerationQueued, CreatedAt: now, UpdatedAt: now,
		}
		state.RegenerationTasks[task.TaskID] = task
		state.IdempotencyTasks[idempotencyScope] = task.TaskID
		state.AssemblyDirty = true
		if err := s.save(ctx, userID, project, state); errors.Is(err, errProjectRevisionConflict) {
			continue
		} else if err != nil {
			return RegenerateShotResult{}, err
		}
		return s.dispatchDurableTask(ctx, userID, projectID, RegenerateShotResult{Shot: shot, Task: task})
	}
	return RegenerateShotResult{}, fmt.Errorf("%w: project changed while claiming regeneration for shot %s", ErrShotVersionConflict, shotID)
}

func (s *CreationService) dispatchDurableTask(ctx context.Context, userID, projectID string, result RegenerateShotResult) (RegenerateShotResult, error) {
	if s.dispatcher == nil || isShotRegenerationTerminal(result.Task.Status) || result.Task.Status == ShotRegenerationRunning {
		return result, nil
	}
	claimed, claimedTask, shot, err := s.claimTaskForDispatch(ctx, userID, projectID, result.Task.TaskID)
	if err != nil {
		return result, err
	}
	result.Task, result.Shot = claimedTask, shot
	if !claimed {
		return result, nil
	}
	runID, dispatchErr := s.dispatcher.EnqueueShotRegeneration(ctx, userID, projectID, claimedTask)
	if dispatchErr != nil {
		failed, persistErr := s.transitionTask(ctx, userID, projectID, claimedTask.TaskID, nil, func(task *model.ShotRegenerationTask) bool {
			if isShotRegenerationTerminal(task.Status) {
				return false
			}
			task.Status = ShotRegenerationFailed
			task.FailureReason = dispatchErr.Error()
			return true
		})
		result.Task = failed
		if persistErr != nil {
			return result, fmt.Errorf("dispatch shot regeneration: %v; persist failure: %w", dispatchErr, persistErr)
		}
		return result, fmt.Errorf("dispatch shot regeneration: %w", dispatchErr)
	}
	if strings.TrimSpace(runID) != claimedTask.RunID {
		return result, fmt.Errorf("shot regeneration dispatcher returned run %q, durable run is %q", runID, claimedTask.RunID)
	}
	running, err := s.transitionTask(ctx, userID, projectID, claimedTask.TaskID, nil, func(task *model.ShotRegenerationTask) bool {
		if task.Status != ShotRegenerationDispatching || task.RunID != claimedTask.RunID {
			return false
		}
		task.Status = ShotRegenerationRunning
		task.DispatchLeaseUntil = time.Time{}
		return true
	})
	if err != nil {
		return result, err
	}
	result.Task = running
	return result, nil
}

func (s *CreationService) claimTaskForDispatch(ctx context.Context, userID, projectID, taskID string) (bool, model.ShotRegenerationTask, model.ShotUnit, error) {
	for attempt := 0; attempt < maxProjectCASAttempts; attempt++ {
		project, state, err := s.load(ctx, userID, projectID)
		if err != nil {
			return false, model.ShotRegenerationTask{}, model.ShotUnit{}, err
		}
		task, ok := state.RegenerationTasks[taskID]
		if !ok {
			return false, model.ShotRegenerationTask{}, model.ShotUnit{}, fmt.Errorf("shot regeneration task %s not found", taskID)
		}
		shot, _, ok := findShot(state.Shots, task.ShotID)
		if !ok {
			return false, task, model.ShotUnit{}, fmt.Errorf("shot %s not found for task %s", task.ShotID, taskID)
		}
		if isShotRegenerationTerminal(task.Status) || task.Status == ShotRegenerationRunning ||
			(task.Status == ShotRegenerationDispatching && task.DispatchLeaseUntil.After(time.Now())) {
			return false, task, shot, nil
		}
		task.Status = ShotRegenerationDispatching
		if task.RunID == "" {
			task.RunID = shotRegenerationRunID(task.TaskID)
		}
		task.DispatchAttempts++
		task.DispatchLeaseUntil = time.Now().Add(shotRegenerationDispatchLease)
		task.UpdatedAt = time.Now()
		state.RegenerationTasks[taskID] = task
		if err := s.save(ctx, userID, project, state); errors.Is(err, errProjectRevisionConflict) {
			continue
		} else if err != nil {
			return false, task, shot, err
		}
		return true, task, shot, nil
	}
	return false, model.ShotRegenerationTask{}, model.ShotUnit{}, fmt.Errorf("%w: project changed while claiming task %s", ErrShotVersionConflict, taskID)
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
	userID, projectID string,
	provenance ShotRegenerationProvenance,
	candidate model.ShotCandidate,
) error {
	taskID := strings.TrimSpace(provenance.TaskID)
	for attempt := 0; attempt < maxProjectCASAttempts; attempt++ {
		project, state, err := s.load(ctx, userID, projectID)
		if err != nil {
			return err
		}
		task, ok := state.RegenerationTasks[taskID]
		if !ok {
			return fmt.Errorf("shot regeneration task %s not found", taskID)
		}
		if err := validateShotRegenerationProvenance(task, provenance); err != nil {
			return err
		}
		if strings.TrimSpace(candidate.ShotID) != provenance.ShotID {
			return fmt.Errorf("shot regeneration candidate targets %s, durable task targets %s", candidate.ShotID, task.ShotID)
		}
		if isShotRegenerationTerminal(task.Status) {
			return nil
		}
		if candidate.DurationSec <= 0 || candidate.DurationSec >= float64(model.MaxShotDurationExclusiveSec) {
			return fmt.Errorf("shot candidate duration must be greater than 0 and less than 15 seconds")
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
				return nil
			}
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
		task.Status = ShotRegenerationCompleted
		task.FailureReason = ""
		task.DispatchLeaseUntil = time.Time{}
		task.UpdatedAt = time.Now()
		state.RegenerationTasks[taskID] = task
		if err := s.save(ctx, userID, project, state); errors.Is(err, errProjectRevisionConflict) {
			continue
		} else {
			return err
		}
	}
	return fmt.Errorf("%w: project changed while completing task %s", ErrShotVersionConflict, taskID)
}

func (s *CreationService) FailShotRegeneration(ctx context.Context, userID, projectID string, provenance ShotRegenerationProvenance, reason string) error {
	_, err := s.transitionTask(ctx, userID, projectID, strings.TrimSpace(provenance.TaskID), func(task model.ShotRegenerationTask) error {
		return validateShotRegenerationProvenance(task, provenance)
	}, func(task *model.ShotRegenerationTask) bool {
		if isShotRegenerationTerminal(task.Status) {
			return false
		}
		task.Status = ShotRegenerationFailed
		task.FailureReason = strings.TrimSpace(reason)
		task.DispatchLeaseUntil = time.Time{}
		return true
	})
	return err
}

func (s *CreationService) transitionTask(
	ctx context.Context,
	userID, projectID, taskID string,
	validate func(model.ShotRegenerationTask) error,
	update func(*model.ShotRegenerationTask) bool,
) (model.ShotRegenerationTask, error) {
	for attempt := 0; attempt < maxProjectCASAttempts; attempt++ {
		project, state, err := s.load(ctx, userID, projectID)
		if err != nil {
			return model.ShotRegenerationTask{}, err
		}
		task, ok := state.RegenerationTasks[taskID]
		if !ok {
			return model.ShotRegenerationTask{}, fmt.Errorf("shot regeneration task %s not found", taskID)
		}
		if validate != nil {
			if err := validate(task); err != nil {
				return model.ShotRegenerationTask{}, err
			}
		}
		if !update(&task) {
			return task, nil
		}
		task.UpdatedAt = time.Now()
		state.RegenerationTasks[taskID] = task
		if err := s.save(ctx, userID, project, state); errors.Is(err, errProjectRevisionConflict) {
			continue
		} else if err != nil {
			return model.ShotRegenerationTask{}, err
		}
		return task, nil
	}
	return model.ShotRegenerationTask{}, fmt.Errorf("%w: project changed while updating task %s", ErrShotVersionConflict, taskID)
}

func validateShotRegenerationProvenance(task model.ShotRegenerationTask, provenance ShotRegenerationProvenance) error {
	if strings.TrimSpace(provenance.TaskID) == "" || strings.TrimSpace(provenance.RunID) == "" || strings.TrimSpace(provenance.ShotID) == "" {
		return fmt.Errorf("shot regeneration provenance requires task, run, and shot ids")
	}
	if task.TaskID != provenance.TaskID || task.RunID != provenance.RunID || task.ShotID != provenance.ShotID {
		return fmt.Errorf("shot regeneration provenance does not match durable task %s", task.TaskID)
	}
	return nil
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

func shotVersionConflict(shot model.ShotUnit, baseVersion int) error {
	return fmt.Errorf("%w: shot %s is version %d, request is based on version %d", ErrShotVersionConflict, shot.ID, shot.Version, baseVersion)
}

func shotRegenerationIdempotencyScope(shotID, idempotencyKey string) string {
	return "shot_regeneration\x00" + shotID + "\x00" + strings.TrimSpace(idempotencyKey)
}

func shotRegenerationFingerprint(shotID string, req RegenerateShotRequest) string {
	if req.requestFingerprint != "" {
		return req.requestFingerprint
	}
	locks := append([]string(nil), req.Locks...)
	sort.Strings(locks)
	payload := fmt.Sprintf("shot_regeneration\x00%s\x00%d\x00%s\x00%s\x00%s", shotID, req.BaseVersion, req.Scope, strings.Join(locks, ","), strings.TrimSpace(req.Instruction))
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func durableTaskFingerprint(task model.ShotRegenerationTask) string {
	if task.RequestFingerprint != "" {
		return task.RequestFingerprint
	}
	return shotRegenerationFingerprint(task.ShotID, RegenerateShotRequest{
		BaseVersion: task.BaseVersion, Scope: task.Scope, Locks: task.Locks,
		Instruction: task.Instruction, IdempotencyKey: task.IdempotencyKey,
	})
}

func shotRegenerationRunID(taskID string) string {
	sum := sha256.Sum256([]byte(taskID))
	return "agent_run_shot_" + hex.EncodeToString(sum[:16])
}

func isShotRegenerationTerminal(status string) bool {
	return status == ShotRegenerationCompleted || status == ShotRegenerationFailed
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
