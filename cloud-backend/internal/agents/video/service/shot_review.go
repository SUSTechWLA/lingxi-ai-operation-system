package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strconv"
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
	ShotRegenerationCancelled     = "cancelled"
	CandidateAcceptScope          = "candidate_accept"
	CandidateRestoreScope         = "candidate_restore"
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

type ShotRegenerationImpact = model.ShotImpact

type CandidateMutationRequest struct {
	BaseVersion    int      `json:"baseVersion"`
	Scope          string   `json:"scope"`
	Locks          []string `json:"locks,omitempty"`
	IdempotencyKey string   `json:"-"`
}

func (s *CreationService) ListShotPage(ctx context.Context, userID, projectID string, query model.ShotPageQuery) (model.ShotPage, error) {
	_, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return model.ShotPage{}, err
	}
	limit := query.Limit
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}
	lastSequenceIndex := 0
	lastShotID := ""
	if query.Cursor != "" {
		lastSequenceIndex, lastShotID, err = decodeShotCursor(query.Cursor)
		if err != nil || lastSequenceIndex < 1 || lastShotID == "" {
			return model.ShotPage{}, fmt.Errorf("invalid shot page cursor %q", query.Cursor)
		}
	}
	shots := append([]model.ShotUnit(nil), state.Shots...)
	sort.SliceStable(shots, func(i, j int) bool {
		if shots[i].SequenceIndex == shots[j].SequenceIndex {
			return shots[i].ID < shots[j].ID
		}
		return shots[i].SequenceIndex < shots[j].SequenceIndex
	})
	filtered := make([]model.ShotUnit, 0, len(shots))
	for _, shot := range shots {
		if shotMatchesPageQuery(shot, query) {
			filtered = append(filtered, shot)
		}
	}
	page := model.ShotPage{Items: make([]model.ShotListItem, 0, limit), Total: len(filtered)}
	for _, shot := range filtered {
		if shot.SequenceIndex < lastSequenceIndex || (shot.SequenceIndex == lastSequenceIndex && shot.ID <= lastShotID) {
			continue
		}
		if len(page.Items) == limit {
			last := page.Items[len(page.Items)-1]
			page.NextCursor = encodeShotCursor(last.SequenceIndex, last.ID)
			break
		}
		page.Items = append(page.Items, shotListItem(state, shot))
	}
	return page, nil
}

func (s *CreationService) GetShotSummary(ctx context.Context, userID, projectID string) (model.ShotSummary, error) {
	_, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return model.ShotSummary{}, err
	}
	summary := model.ShotSummary{Total: len(state.Shots)}
	for _, shot := range state.Shots {
		if shotHasTerminalFailure(state, shot.ID) {
			summary.NeedsAction++
			continue
		}
		if isShotGenerating(state, shot.ID) {
			summary.Generating++
		}
		switch shot.ReviewStatus {
		case model.ReviewStatusApproved:
			summary.Confirmed++
		case model.ReviewStatusRejected, model.ReviewStatusStale:
			summary.NeedsAction++
		default:
			if shot.QAStatus == model.ShotHumanReviewRequired || hasHumanReviewCandidate(shot) {
				summary.NeedsAction++
			} else {
				summary.AwaitingReview++
			}
		}
	}
	return summary, nil
}

func (s *CreationService) GetShotWorkspace(ctx context.Context, userID, projectID, shotID string) (model.ShotWorkspace, error) {
	_, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return model.ShotWorkspace{}, err
	}
	shot, _, ok := findShot(state.Shots, shotID)
	if !ok {
		return model.ShotWorkspace{}, fmt.Errorf("shot %s not found", shotID)
	}
	history := append([]model.ShotRevision(nil), state.ShotHistory[shotID]...)
	history = append(history, model.ShotRevision{RevisionID: "current", ShotID: shot.ID, Version: shot.Version, Reason: "current", Snapshot: cloneShot(shot), CreatedAt: shot.UpdatedAt})
	return model.ShotWorkspace{
		Shot: shot, History: history,
		Impact: creatorShotImpact(shot),
	}, nil
}

func (s *CreationService) AcceptShotCandidate(ctx context.Context, userID, projectID, shotID, candidateID string, req CandidateMutationRequest) (*model.ShotUnit, error) {
	return s.mutateShotCandidate(ctx, userID, projectID, shotID, candidateID, req, CandidateAcceptScope, func(shot *model.ShotUnit, candidate model.ShotCandidate, now time.Time) error {
		shot.AcceptedCandidateID = candidate.CandidateID
		shot.ReviewStatus = model.ReviewStatusApproved
		shot.QAStatus = candidateQAStatus(candidate)
		shot.Stale = false
		return nil
	})
}

func (s *CreationService) RestoreShotCandidate(ctx context.Context, userID, projectID, shotID, candidateID string, req CandidateMutationRequest) (*model.ShotUnit, error) {
	return s.mutateShotCandidate(ctx, userID, projectID, shotID, candidateID, req, CandidateRestoreScope, func(shot *model.ShotUnit, historical model.ShotCandidate, now time.Time) error {
		restored := cloneShotCandidate(historical)
		restored.CandidateID = "candidate-restore-" + uuid.NewString()
		restored.AttemptIndex = len(shot.Candidates) + 1
		restored.Status = model.CandidateAcceptedForAssembly
		restored.Stale = false
		restored.StaleReason = ""
		restored.CreatedAt = now
		if restored.QAReport != nil {
			restored.QAReport.CandidateID = restored.CandidateID
			restored.QAReport.ShotID = shot.ID
		}
		shot.Candidates = append(shot.Candidates, restored)
		shot.AcceptedCandidateID = restored.CandidateID
		shot.ReviewStatus = model.ReviewStatusApproved
		shot.QAStatus = candidateQAStatus(restored)
		shot.Stale = false
		return nil
	})
}

func (s *CreationService) mutateShotCandidate(
	ctx context.Context, userID, projectID, shotID, candidateID string, req CandidateMutationRequest, operation string,
	mutate func(*model.ShotUnit, model.ShotCandidate, time.Time) error,
) (*model.ShotUnit, error) {
	if err := validateCandidateMutationRequest(req, operation); err != nil {
		return nil, err
	}
	fingerprint := candidateMutationFingerprint(operation, shotID, candidateID, req)
	receiptKey := candidateMutationReceiptKey(operation, shotID, req.IdempotencyKey)
	for attempt := 0; attempt < maxProjectCASAttempts; attempt++ {
		project, state, err := s.load(ctx, userID, projectID)
		if err != nil {
			return nil, err
		}
		shot, index, ok := findShot(state.Shots, shotID)
		if !ok {
			return nil, fmt.Errorf("shot %s not found", shotID)
		}
		if err := model.ValidateShotDuration(shot); err != nil {
			return nil, err
		}
		if receipt, ok := state.ShotMutationReceipts[receiptKey]; ok {
			if receipt.RequestFingerprint != fingerprint {
				return nil, fmt.Errorf("%w: key %q was used with different candidate mutation input", ErrShotIdempotencyConflict, req.IdempotencyKey)
			}
			result := cloneShot(receipt.ResultShot)
			return &result, nil
		}
		if shot.Version != req.BaseVersion {
			return nil, shotVersionConflict(shot, req.BaseVersion)
		}
		_, candidate, ok := findShotCandidate(shot, candidateID)
		if !ok {
			return nil, fmt.Errorf("candidate %s not found for shot %s", candidateID, shotID)
		}
		if candidate.ShotID != shot.ID {
			return nil, fmt.Errorf("candidate %s belongs to shot %s, not shot %s", candidateID, candidate.ShotID, shot.ID)
		}
		if err := validateCandidateAcceptance(candidate); err != nil {
			return nil, err
		}
		now := time.Now().Round(0)
		state.ShotHistory[shot.ID] = append(state.ShotHistory[shot.ID], model.ShotRevision{RevisionID: "shot-revision-" + uuid.NewString(), ShotID: shot.ID, Version: shot.Version, Reason: operation + " " + candidateID, Snapshot: cloneShot(shot), CreatedAt: now})
		if err := mutate(&shot, candidate, now); err != nil {
			return nil, err
		}
		shot.Version++
		shot.UpdatedAt = now
		state.Shots[index] = shot
		state.AssemblyDirty = true
		state.ShotMutationReceipts[receiptKey] = model.ShotMutationReceipt{
			Operation: operation, ShotID: shotID, CandidateID: candidateID, BaseVersion: req.BaseVersion,
			Scope: req.Scope, Locks: append([]string(nil), req.Locks...), IdempotencyKey: req.IdempotencyKey,
			RequestFingerprint: fingerprint, ResultShot: cloneShot(shot), CreatedAt: now,
		}
		if err := s.save(ctx, userID, project, state); errors.Is(err, errProjectRevisionConflict) {
			continue
		} else if err != nil {
			return nil, err
		}
		result := cloneShot(state.ShotMutationReceipts[receiptKey].ResultShot)
		return &result, nil
	}
	return nil, fmt.Errorf("%w: project changed while applying %s for shot %s", ErrShotVersionConflict, operation, shotID)
}

func encodeShotCursor(sequenceIndex int, shotID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(sequenceIndex) + "\x00" + shotID))
}

func decodeShotCursor(cursor string) (int, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, "", err
	}
	parts := strings.Split(string(raw), "\x00")
	if len(parts) != 2 || parts[1] == "" {
		return 0, "", fmt.Errorf("invalid cursor")
	}
	sequenceIndex, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", err
	}
	return sequenceIndex, parts[1], nil
}

func validateCandidateMutationRequest(req CandidateMutationRequest, operation string) error {
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return fmt.Errorf("idempotency key is required")
	}
	wantScope := CandidateAcceptScope
	if operation == CandidateRestoreScope {
		wantScope = CandidateRestoreScope
	}
	if req.Scope != wantScope {
		return fmt.Errorf("invalid candidate mutation scope %q", req.Scope)
	}
	seen := map[string]bool{}
	for _, lock := range req.Locks {
		if !allowedShotLocks[lock] {
			return fmt.Errorf("invalid shot mutation lock %q", lock)
		}
		if seen[lock] {
			return fmt.Errorf("duplicate shot mutation lock %q", lock)
		}
		seen[lock] = true
	}
	return nil
}

func candidateMutationReceiptKey(operation, shotID, idempotencyKey string) string {
	return "shot_mutation\x00" + operation + "\x00" + shotID + "\x00" + strings.TrimSpace(idempotencyKey)
}

func candidateMutationFingerprint(operation, shotID, candidateID string, req CandidateMutationRequest) string {
	locks := append([]string(nil), req.Locks...)
	sort.Strings(locks)
	payload := fmt.Sprintf("shot_mutation\x00%s\x00%s\x00%s\x00%d\x00%s\x00%s", operation, shotID, candidateID, req.BaseVersion, req.Scope, strings.Join(locks, ","))
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func shotMatchesPageQuery(shot model.ShotUnit, query model.ShotPageQuery) bool {
	if status := strings.TrimSpace(query.Status); status != "" && !strings.EqualFold(shot.ReviewStatus, status) {
		return false
	}
	chapter := shotChapter(shot)
	if wanted := strings.TrimSpace(query.Chapter); wanted != "" && !strings.EqualFold(chapter, wanted) {
		return false
	}
	needle := strings.ToLower(strings.TrimSpace(query.Query))
	if needle == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join(append([]string{shot.Title, shot.Narration, shot.SceneSummary}, shot.ScreenText...), "\n"))
	return strings.Contains(haystack, needle)
}

func shotListItem(state model.ShotDrivenState, shot model.ShotUnit) model.ShotListItem {
	return model.ShotListItem{
		ID: shot.ID, SequenceIndex: shot.SequenceIndex, Title: shot.Title, Chapter: shotChapter(shot),
		DurationSec: shot.DurationSec, Version: shot.Version, ReviewStatus: shot.ReviewStatus,
		QAStatus: shot.QAStatus, GenerationStatus: generationStatusForShot(state, shot),
		AcceptedCandidateID: shot.AcceptedCandidateID, ThumbnailRef: shotThumbnailRef(shot),
	}
}

func shotChapter(shot model.ShotUnit) string {
	if shot.Scene != "" {
		return shot.Scene
	}
	return shot.SceneID
}

func generationStatusForShot(state model.ShotDrivenState, shot model.ShotUnit) string {
	latest := latestShotRegenerationTask(state, shot.ID)
	if latest != nil && (latest.Status == ShotRegenerationFailed || latest.Status == ShotRegenerationCancelled) {
		return latest.Status
	}
	if latest != nil && !isShotRegenerationTerminal(latest.Status) {
		return latest.Status
	}
	if shot.AcceptedCandidateID != "" {
		return model.CandidateAcceptedForAssembly
	}
	if shot.QAStatus != "" {
		return shot.QAStatus
	}
	if len(shot.Candidates) > 0 {
		return shot.Candidates[len(shot.Candidates)-1].Status
	}
	return model.ShotPlanned
}

func isShotGenerating(state model.ShotDrivenState, shotID string) bool {
	latest := latestShotRegenerationTask(state, shotID)
	return latest != nil && !isShotRegenerationTerminal(latest.Status)
}

func shotHasTerminalFailure(state model.ShotDrivenState, shotID string) bool {
	latest := latestShotRegenerationTask(state, shotID)
	return latest != nil && (latest.Status == ShotRegenerationFailed || latest.Status == ShotRegenerationCancelled)
}

func latestShotRegenerationTask(state model.ShotDrivenState, shotID string) *model.ShotRegenerationTask {
	var latest *model.ShotRegenerationTask
	for _, task := range state.RegenerationTasks {
		if task.ShotID != shotID {
			continue
		}
		if latest == nil || taskIsLater(task, *latest) {
			taskCopy := task
			latest = &taskCopy
		}
	}
	return latest
}

func taskIsLater(left, right model.ShotRegenerationTask) bool {
	if !left.UpdatedAt.Equal(right.UpdatedAt) {
		return left.UpdatedAt.After(right.UpdatedAt)
	}
	if !left.CreatedAt.Equal(right.CreatedAt) {
		return left.CreatedAt.After(right.CreatedAt)
	}
	return left.TaskID > right.TaskID
}

func hasHumanReviewCandidate(shot model.ShotUnit) bool {
	for _, candidate := range shot.Candidates {
		if candidate.Status == model.CandidateHumanReviewRequired {
			return true
		}
	}
	return false
}

func shotThumbnailRef(shot model.ShotUnit) string {
	candidate, ok := acceptedCandidateForShot(shot)
	if !ok {
		return ""
	}
	refs := candidate.ArtifactRefs
	for _, ref := range []string{refs.KeyframeImageArtifactID, refs.VideoClipArtifactID, refs.CompositedShotVideoArtifactID, refs.HTMLPreviewVideoArtifactID} {
		if ref != "" {
			return ref
		}
	}
	return ""
}

func creatorShotImpact(shot model.ShotUnit) model.ShotImpact {
	return model.ShotImpact{
		ShotID: shot.ID, AffectedShotIDs: []string{shot.ID}, InvalidatesFinalAssembly: true,
		RegeneratesOtherShots: false, EstimatedDurationSec: shot.DurationSec, RequiresConfirmation: true,
	}
}

func findShotCandidate(shot model.ShotUnit, candidateID string) (int, model.ShotCandidate, bool) {
	for index, candidate := range shot.Candidates {
		if candidate.CandidateID == candidateID {
			return index, candidate, true
		}
	}
	return -1, model.ShotCandidate{}, false
}

func validateCandidateAcceptance(candidate model.ShotCandidate) error {
	if candidate.DurationSec <= 0 || candidate.DurationSec >= float64(model.MaxShotDurationExclusiveSec) {
		return fmt.Errorf("shot candidate duration must be greater than 0 and less than 15 seconds")
	}
	if candidate.Status == model.CandidateHumanReviewRequired {
		return nil
	}
	if candidate.Status != model.CandidateShotQAPassed || candidate.QAReport == nil || !candidate.QAReport.Passed {
		return fmt.Errorf("candidate %s must pass QA or require human review before acceptance", candidate.CandidateID)
	}
	return nil
}

func candidateQAStatus(candidate model.ShotCandidate) string {
	if candidate.QAReport != nil && candidate.QAReport.Status != "" {
		return candidate.QAReport.Status
	}
	return candidate.Status
}

func cloneShotCandidate(candidate model.ShotCandidate) model.ShotCandidate {
	cloned := candidate
	cloned.LayerRevisions = maps.Clone(candidate.LayerRevisions)
	if candidate.QAReport != nil {
		report := *candidate.QAReport
		report.Scores = maps.Clone(candidate.QAReport.Scores)
		report.PassedDimensions = append([]string(nil), candidate.QAReport.PassedDimensions...)
		report.FailedDimensions = append([]string(nil), candidate.QAReport.FailedDimensions...)
		cloned.QAReport = &report
	}
	if candidate.RepairPlan != nil {
		plan := *candidate.RepairPlan
		plan.LockedDimensions = append([]string(nil), candidate.RepairPlan.LockedDimensions...)
		plan.RepairTargets = append([]string(nil), candidate.RepairPlan.RepairTargets...)
		plan.PromptPatch = cloneInterfaceMap(candidate.RepairPlan.PromptPatch)
		plan.RenderStrategyPatch = cloneInterfaceMap(candidate.RepairPlan.RenderStrategyPatch)
		cloned.RepairPlan = &plan
	}
	return cloned
}

func cloneInterfaceMap(values map[string]interface{}) map[string]interface{} {
	if values == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(values))
	for key, value := range values {
		cloned[key] = cloneInterfaceValue(value)
	}
	return cloned
}

func cloneInterfaceValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return cloneInterfaceMap(typed)
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for index, item := range typed {
			cloned[index] = cloneInterfaceValue(item)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	case map[string]string:
		return maps.Clone(typed)
	case map[string]int:
		return maps.Clone(typed)
	default:
		return value
	}
}

func (s *CreationService) RegenerateShotV2(
	ctx context.Context,
	userID, projectID, shotID string,
	req RegenerateShotRequest,
) (RegenerateShotResult, error) {
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return RegenerateShotResult{}, fmt.Errorf("idempotency key is required")
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
			if result.Task.ShotID != shotID || !durableTaskFingerprintMatches(result.Task, fingerprint) {
				return RegenerateShotResult{}, fmt.Errorf("%w: key %q was used with different regeneration input", ErrShotIdempotencyConflict, req.IdempotencyKey)
			}
			if result.Task.RequestFingerprint == "" || state.IdempotencyTasks[idempotencyScope] == "" {
				result.Task.RequestFingerprint = fingerprint
				state.RegenerationTasks[taskID] = result.Task
				state.IdempotencyTasks[idempotencyScope] = taskID
				if err := s.save(ctx, userID, project, state); errors.Is(err, errProjectRevisionConflict) {
					continue
				} else if err != nil {
					return RegenerateShotResult{}, err
				}
			}
			return s.dispatchDurableTask(ctx, userID, projectID, result)
		}
		if err := validateRegenerateShotRequest(req); err != nil {
			return RegenerateShotResult{}, err
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

func durableTaskFingerprintMatches(task model.ShotRegenerationTask, fingerprint string) bool {
	if durableTaskFingerprint(task) == fingerprint {
		return true
	}
	if task.RequestFingerprint != "" {
		return false
	}
	legacyZeroBase := task
	legacyZeroBase.BaseVersion = 0
	return durableTaskFingerprint(legacyZeroBase) == fingerprint
}

func shotRegenerationRunID(taskID string) string {
	sum := sha256.Sum256([]byte(taskID))
	return "agent_run_shot_" + hex.EncodeToString(sum[:16])
}

func isShotRegenerationTerminal(status string) bool {
	return status == ShotRegenerationCompleted || status == ShotRegenerationFailed || status == ShotRegenerationCancelled
}

func cloneShot(shot model.ShotUnit) model.ShotUnit {
	cloned := shot
	cloned.ScreenText = append([]string(nil), shot.ScreenText...)
	cloned.Candidates = make([]model.ShotCandidate, len(shot.Candidates))
	for index, candidate := range shot.Candidates {
		cloned.Candidates[index] = cloneShotCandidate(candidate)
	}
	cloned.RepairPlans = make([]model.RepairPlan, len(shot.RepairPlans))
	for index, plan := range shot.RepairPlans {
		cloned.RepairPlans[index] = *cloneRepairPlan(&plan)
	}
	cloned.PromptConstraints.MustInclude = append([]string(nil), shot.PromptConstraints.MustInclude...)
	cloned.PromptConstraints.MustAvoid = append([]string(nil), shot.PromptConstraints.MustAvoid...)
	return cloned
}

func cloneRepairPlan(plan *model.RepairPlan) *model.RepairPlan {
	if plan == nil {
		return nil
	}
	cloned := *plan
	cloned.LockedDimensions = append([]string(nil), plan.LockedDimensions...)
	cloned.RepairTargets = append([]string(nil), plan.RepairTargets...)
	cloned.PromptPatch = cloneInterfaceMap(plan.PromptPatch)
	cloned.RenderStrategyPatch = cloneInterfaceMap(plan.RenderStrategyPatch)
	return &cloned
}
