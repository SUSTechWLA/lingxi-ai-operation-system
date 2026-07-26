package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/tangying-ai/aios-core/internal/agents/video/assets"
	"github.com/tangying-ai/aios-core/internal/agents/video/model"
	"github.com/tangying-ai/aios-core/internal/core/artifact"
)

// The mapping is deliberately closed: developer-only or future stages cannot
// appear as an extra creator step.
var creatorStageSteps = map[string]model.CreatorStepID{
	"requirements":         model.CreatorStepRequirements,
	"requirement":          model.CreatorStepRequirements,
	"requirement_analysis": model.CreatorStepRequirements,
	"brief":                model.CreatorStepRequirements,
	"source_materials":     model.CreatorStepRequirements,

	"direction":                     model.CreatorStepDirection,
	"creative_direction":            model.CreatorStepDirection,
	"creative_direction_generation": model.CreatorStepDirection,
	"proposal":                      model.CreatorStepDirection,
	"proposal_generator":            model.CreatorStepDirection,
	"research":                      model.CreatorStepDirection,
	"knowledge_researcher":          model.CreatorStepDirection,
	"style":                         model.CreatorStepDirection,
	"character":                     model.CreatorStepDirection,
	"characters":                    model.CreatorStepDirection,
	"feasibility":                   model.CreatorStepDirection,

	"script":                         model.CreatorStepScript,
	"script_generation":              model.CreatorStepScript,
	"script_generation_quality_gate": model.CreatorStepScript,
	"voiceover":                      model.CreatorStepScript,
	"audio_master":                   model.CreatorStepScript,
	"timing":                         model.CreatorStepScript,
	"time_window":                    model.CreatorStepScript,

	"shots":                     model.CreatorStepShots,
	"shot":                      model.CreatorStepShots,
	"shot_design":               model.CreatorStepShots,
	"shot_split":                model.CreatorStepShots,
	"shot_split_quality_gate":   model.CreatorStepShots,
	"storyboard":                model.CreatorStepShots,
	"composition":               model.CreatorStepShots,
	"reference":                 model.CreatorStepShots,
	"continuity":                model.CreatorStepShots,
	"visual_plan":               model.CreatorStepShots,
	"visual_alignment":          model.CreatorStepShots,
	"video_prompt":              model.CreatorStepShots,
	"video_prompt_quality_gate": model.CreatorStepShots,
	"ip_aroll_generation":       model.CreatorStepShots,
	"render_strategy":           model.CreatorStepShots,
	"assets":                    model.CreatorStepShots,
	"materials":                 model.CreatorStepShots,

	"preview":        model.CreatorStepPreview,
	"assembly":       model.CreatorStepPreview,
	"video_assembly": model.CreatorStepPreview,
	"final_render":   model.CreatorStepPreview,
	"captions":       model.CreatorStepPreview,
	"subtitle":       model.CreatorStepPreview,
	"audio_mix":      model.CreatorStepPreview,
	"quality":        model.CreatorStepPreview,
	"final_review":   model.CreatorStepPreview,
	"final_qa":       model.CreatorStepPreview,
	"render":         model.CreatorStepPreview,
	"visual_qa":      model.CreatorStepPreview,

	"delivery":     model.CreatorStepDelivery,
	"package":      model.CreatorStepDelivery,
	"publish":      model.CreatorStepDelivery,
	"publish_copy": model.CreatorStepDelivery,
	"export":       model.CreatorStepDelivery,
}

var creatorStepDefinitions = []struct {
	id    model.CreatorStepID
	label string
}{
	{model.CreatorStepRequirements, "Requirements"},
	{model.CreatorStepDirection, "Creative direction"},
	{model.CreatorStepScript, "Script"},
	{model.CreatorStepShots, "Shots and materials"},
	{model.CreatorStepPreview, "Video preview"},
	{model.CreatorStepDelivery, "Delivery"},
}

type creatorProjectReader interface {
	GetProject(ctx context.Context, userID, projectID string) (*model.VideoProject, error)
}

type creatorProjectLifecycle interface {
	MarkAgentRunStarted(ctx context.Context, userID, projectID, runID string) error
}

type creatorArtifactReader interface {
	ListCurrentByProject(ctx context.Context, projectID string) ([]*artifact.Artifact, error)
}

type creatorShotReadState struct {
	Summary       model.ShotSummary
	Tasks         []model.ShotRegenerationTask
	AssemblyDirty bool
	ShotIDs       []string
}

type creatorShotReader interface {
	getCreatorShotReadState(ctx context.Context, userID, projectID string) (creatorShotReadState, error)
}

type creatorShotInvalidationService interface {
	InvalidateShotsForUpstreamRevision(context.Context, string, string, string, []string, string) error
}

type creatorArtifactReconciler interface {
	ReconcileProjectArtifacts(context.Context, string) error
}

type creatorArtifactTextResolver interface {
	ResolveReviewableText(context.Context, *artifact.Artifact) (string, error)
}

// CreatorViewService combines only persisted project, artifact, and Shot data.
// It has no client-derived state or workflow topology dependency.
type CreatorViewService struct {
	projects             creatorProjectReader
	artifacts            creatorArtifactReader
	shots                creatorShotReader
	history              creatorArtifactHistoryReader
	revisions            creatorRevisionService
	reviews              creatorReviewMutations
	assembly             creatorAssemblyService
	shotInvalidations    creatorShotInvalidationService
	auditRun             creatorRunAuditLookup
	auditNodes           creatorNodeAuditReader
	projectLifecycle     creatorProjectLifecycle
	artifactReconciler   creatorArtifactReconciler
	artifactTextResolver creatorArtifactTextResolver
}

type creatorArtifactHistoryReader interface {
	GetByID(context.Context, string) (*artifact.Artifact, error)
	GetCurrent(context.Context, string, string, string) (*artifact.Artifact, error)
	GetHistory(context.Context, string, string, string) ([]*artifact.Artifact, error)
}

type creatorRevisionService interface {
	Revise(context.Context, artifact.ReviseRequest) (*artifact.RevisionResult, error)
	Replace(context.Context, artifact.ReplaceRequest) (*artifact.RevisionResult, error)
	Restore(context.Context, artifact.RestoreRequest) (*artifact.RevisionResult, error)
}

type creatorReviewMutations interface {
	ResolveReviewGate(context.Context, *artifact.Artifact, string, string) (string, string, error)
	Confirm(context.Context, string, string, string, string) error
	ConfirmForArtifact(context.Context, string, string, string, string, string) error
	ReopenWithArtifact(context.Context, string, string, string, string, string, string, string) error
	Regenerate(context.Context, string, string, string, string) ([]string, error)
	RegenerateIdempotent(context.Context, string, string, string, string, string) ([]string, error)
	RegenerationStatus(context.Context, string, string) (string, error)
}

type creatorAssemblyService interface {
	RebuildFinalAssembly(context.Context, string, string, string) (AssemblyRebuildResult, error)
	MarkFinalAssemblyQueued(context.Context, string, string, string, string, string) (AssemblyRebuildResult, error)
	ClaimFinalAssemblyDispatch(context.Context, string, string, string, string, string, string) (AssemblyRebuildResult, error)
	ClaimFinalAssemblyRedispatch(context.Context, string, string, string) (AssemblyRebuildResult, error)
	LatestAssemblyReceipt(context.Context, string, string) (model.AssemblyReceipt, bool, error)
	GetAssemblyReceipt(context.Context, string, string, string) (model.AssemblyReceipt, bool, error)
}

var (
	ErrCreatorStepInvalid              = errors.New("creator step is invalid")
	ErrCreatorArtifactNotFound         = errors.New("creator artifact not found")
	ErrCreatorVersionConflict          = errors.New("creator artifact version conflict")
	ErrCreatorImpactMismatch           = errors.New("creator impact confirmation mismatch")
	ErrCreatorMutationUnavailable      = errors.New("creator step mutation is unavailable")
	ErrCreatorInvalidRequest           = errors.New("creator step request is invalid")
	ErrCreatorIdempotencyConflict      = errors.New("creator idempotency key conflict")
	ErrCreatorModelProviderUnavailable = errors.New("creator model provider is unavailable")
	ErrCreatorSelectionConflict        = errors.New("creator artifact selection conflict")
)

const creatorMutationReceiptKey = "creatorStepMutationReceipt"

type creatorMutationReceipt struct {
	Operation            string                                `json:"operation"`
	IdempotencyKey       string                                `json:"idempotencyKey"`
	Fingerprint          string                                `json:"fingerprint"`
	ProjectID            string                                `json:"projectId"`
	StepID               string                                `json:"stepId"`
	BaseArtifactID       string                                `json:"baseArtifactId"`
	HistoricalArtifactID string                                `json:"historicalArtifactId,omitempty"`
	BaseVersion          int                                   `json:"baseVersion"`
	HistoricalVersion    int                                   `json:"historicalVersion,omitempty"`
	RunID                string                                `json:"runId"`
	ReviewID             string                                `json:"reviewId"`
	NewArtifactID        string                                `json:"newArtifactId"`
	ParentArtifactID     string                                `json:"parentArtifactId"`
	AffectedStepIDs      []model.CreatorStepID                 `json:"affectedStepIds"`
	AffectedShotIDs      []string                              `json:"affectedShotIds,omitempty"`
	Selection            map[string]interface{}                `json:"selection,omitempty"`
	ReplacementMaterial  *artifact.ReplacementMaterialIdentity `json:"replacementMaterial,omitempty"`
	RequestDigest        string                                `json:"requestDigest"`
}

func NewCreatorViewService(projects creatorProjectReader, shots creatorShotReader, artifacts creatorArtifactReader) *CreatorViewService {
	svc := &CreatorViewService{projects: projects, artifacts: artifacts, shots: shots}
	if lifecycle, ok := projects.(creatorProjectLifecycle); ok {
		svc.projectLifecycle = lifecycle
	}
	if history, ok := artifacts.(creatorArtifactHistoryReader); ok {
		svc.history = history
	}
	if assembly, ok := shots.(creatorAssemblyService); ok {
		svc.assembly = assembly
	}
	if invalidations, ok := shots.(creatorShotInvalidationService); ok {
		svc.shotInvalidations = invalidations
	}
	return svc
}

func (s *CreatorViewService) RebuildFinalAssembly(ctx context.Context, userID, projectID, idempotencyKey string) (AssemblyRebuildResult, error) {
	if s.assembly == nil || s.reviews == nil {
		return AssemblyRebuildResult{}, ErrCreatorMutationUnavailable
	}
	if strings.TrimSpace(idempotencyKey) == "" {
		return AssemblyRebuildResult{}, ErrCreatorInvalidRequest
	}
	result, err := s.assembly.RebuildFinalAssembly(ctx, userID, projectID, idempotencyKey)
	if err != nil || result.Status == "blocked" {
		return result, err
	}
	if result.Status == "dispatching" {
		receipt, found, receiptErr := s.assembly.GetAssemblyReceipt(ctx, userID, projectID, idempotencyKey)
		if receiptErr != nil || !found {
			return result, receiptErr
		}
		current, currentErr := s.currentArtifactForStep(ctx, projectID, model.CreatorStepPreview)
		if currentErr != nil || current == nil {
			return result, currentErr
		}
		if receipt.BasePreviewArtifactID == "" || receipt.PreviewTaskID == "" || receipt.PreviewReviewID == "" || receipt.DispatchAttempt <= 0 {
			return result, fmt.Errorf("assembled preview dispatch receipt is incomplete")
		}
		if current.ID != receipt.BasePreviewArtifactID {
			return s.assembly.MarkFinalAssemblyQueued(ctx, userID, projectID, idempotencyKey, receipt.BasePreviewArtifactID, receipt.PreviewTaskID)
		}
		runID, reviewID, resolveErr := s.reviews.ResolveReviewGate(ctx, current, receipt.PreviewTaskID, receipt.PreviewReviewID)
		if resolveErr != nil {
			return result, resolveErr
		}
		if _, retryErr := s.reviews.RegenerateIdempotent(ctx, runID, reviewID, userID, "resume durable assembled preview dispatch", assemblyDispatchKey(idempotencyKey, receipt.DispatchAttempt)); retryErr != nil {
			return result, retryErr
		}
		return s.assembly.MarkFinalAssemblyQueued(ctx, userID, projectID, idempotencyKey, current.ID, runID)
	}
	if result.Status == "queued" {
		receipt, found, receiptErr := s.assembly.GetAssemblyReceipt(ctx, userID, projectID, idempotencyKey)
		if receiptErr != nil || !found {
			return result, receiptErr
		}
		current, currentErr := s.currentArtifactForStep(ctx, projectID, model.CreatorStepPreview)
		if currentErr != nil || current == nil || current.ID != receipt.BasePreviewArtifactID {
			return result, currentErr
		}
		runID, reviewID, resolveErr := s.reviews.ResolveReviewGate(ctx, current, receipt.PreviewTaskID, receipt.PreviewReviewID)
		if resolveErr != nil {
			return result, resolveErr
		}
		status, statusErr := s.reviews.RegenerationStatus(ctx, runID, reviewID)
		if statusErr != nil {
			return result, fmt.Errorf("assembled preview status is unavailable; wait for the current preview or retry after it reports a failure")
		}
		if status == "CREATED" || status == "READY" || status == "RUNNING" || status == "WAITING_LOCAL" || status == "LOCAL_RUNNING" || status == "RETRYING" || status == "SUCCESS" || status == "COMPLETED" {
			return result, nil
		}
		if status != "FAILED" && status != "CANCELLED" && status != "LOCAL_FAILED" && status != "HEARTBEAT_TIMEOUT" {
			return result, fmt.Errorf("assembled preview is in unknown state %q", status)
		}
		if result, err = s.assembly.ClaimFinalAssemblyRedispatch(ctx, userID, projectID, idempotencyKey); err != nil || result.Status != "dispatching" {
			return result, err
		}
		receipt, found, err = s.assembly.GetAssemblyReceipt(ctx, userID, projectID, idempotencyKey)
		if err != nil || !found {
			return result, err
		}
		if _, err = s.reviews.RegenerateIdempotent(ctx, runID, reviewID, userID, "retry assembled preview after a failed attempt", assemblyDispatchKey(idempotencyKey, receipt.DispatchAttempt)); err != nil {
			return result, err
		}
		return s.assembly.MarkFinalAssemblyQueued(ctx, userID, projectID, idempotencyKey, current.ID, runID)
	}
	if result.Status != "validated" {
		return result, nil
	}
	current, err := s.currentArtifactForStep(ctx, projectID, model.CreatorStepPreview)
	if err != nil {
		return result, err
	}
	runID, reviewID, err := s.reviews.ResolveReviewGate(ctx, current, "", "")
	if err != nil {
		return result, err
	}
	if result, err = s.assembly.ClaimFinalAssemblyDispatch(ctx, userID, projectID, idempotencyKey, current.ID, runID, reviewID); err != nil || result.Status != "dispatching" {
		return result, err
	}
	receipt, found, err := s.assembly.GetAssemblyReceipt(ctx, userID, projectID, idempotencyKey)
	if err != nil || !found {
		return result, err
	}
	if _, err = s.reviews.RegenerateIdempotent(ctx, runID, reviewID, userID, "accepted Shot changes require a new assembled preview", assemblyDispatchKey(idempotencyKey, receipt.DispatchAttempt)); err != nil {
		return result, err
	}
	return s.assembly.MarkFinalAssemblyQueued(ctx, userID, projectID, idempotencyKey, current.ID, runID)
}

func assemblyDispatchKey(idempotencyKey string, attempt int) string {
	if attempt <= 0 {
		attempt = 1
	}
	return fmt.Sprintf("%s:dispatch:%d", idempotencyKey, attempt)
}

func (s *CreatorViewService) WithStepMutations(revisions creatorRevisionService, reviews creatorReviewMutations) *CreatorViewService {
	s.revisions, s.reviews = revisions, reviews
	if history, ok := s.artifacts.(creatorArtifactHistoryReader); ok {
		s.history = history
	}
	return s
}

func (s *CreatorViewService) WithArtifactReconciler(reconciler creatorArtifactReconciler) *CreatorViewService {
	s.artifactReconciler = reconciler
	if resolver, ok := reconciler.(creatorArtifactTextResolver); ok {
		s.artifactTextResolver = resolver
	}
	return s
}

func (s *CreatorViewService) GetCreationView(ctx context.Context, userID, projectID string) (*model.CreationView, error) {
	if s == nil || s.projects == nil || s.artifacts == nil || s.shots == nil {
		return nil, fmt.Errorf("creator view service is not configured")
	}
	project, err := s.projects.GetProject(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, fmt.Errorf("project not found")
	}
	if s.artifactReconciler != nil {
		if err := s.artifactReconciler.ReconcileProjectArtifacts(ctx, projectID); err != nil {
			return nil, fmt.Errorf("reconcile creator artifacts: %w", err)
		}
	}
	artifacts, err := s.artifacts.ListCurrentByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	finalDelivery := authoritativeFinalDeliveryArtifact(artifacts)
	shotState, err := s.shots.getCreatorShotReadState(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}

	steps := newCreatorSteps()
	stepIndexes := make(map[model.CreatorStepID]int, len(steps))
	for i, step := range steps {
		stepIndexes[step.ID] = i
	}
	activeTasks := make([]model.CreatorTask, 0)
	for _, current := range artifacts {
		if current == nil {
			continue
		}
		stepID, ok := creatorStepForArtifact(current)
		if !ok {
			continue
		}
		// Delivery authority belongs exclusively to the selected final video.
		// Publish copy, export bundles, and other delivery-stage records remain
		// available in audit history but must never become the current playable
		// artifact when the final video is absent.
		if stepID == model.CreatorStepDelivery && (finalDelivery == nil || current.ID != finalDelivery.ID) {
			continue
		}
		index := stepIndexes[stepID]
		if stepID == model.CreatorStepShots {
			// Shot readiness is derived solely from the persisted Shot summary and
			// durable regeneration tasks below. A storyboard artifact may provide
			// lineage, but it must not override that authoritative Shot state.
			if artifactPreferred(current, steps[index]) {
				applyArtifact(&steps[index], current, steps[index].State)
			}
		} else {
			candidate := stateForArtifact(current)
			if statePriority(candidate) > statePriority(steps[index].State) ||
				(statePriority(candidate) == statePriority(steps[index].State) && artifactPreferred(current, steps[index])) {
				applyArtifact(&steps[index], current, candidate)
			}
		}
		if isActiveArtifact(current) && current.TaskID != "" {
			activeTasks = append(activeTasks, model.CreatorTask{
				ID: current.TaskID, Scope: string(stepID), Status: current.Status,
				Label: creatorTaskLabel(stepID),
			})
		}
	}
	if finalDelivery != nil {
		index := stepIndexes[model.CreatorStepDelivery]
		applyArtifact(&steps[index], finalDelivery, stateForArtifact(finalDelivery))
	}

	shotsIndex := stepIndexes[model.CreatorStepShots]
	steps[shotsIndex].State = mergeCreatorState(steps[shotsIndex].State, stateForShots(shotState.Summary))
	for _, task := range latestCreatorShotTasks(shotState.Tasks) {
		if !isActiveShotRegeneration(task.Status) || task.TaskID == "" {
			continue
		}
		activeTasks = append(activeTasks, model.CreatorTask{
			ID: task.TaskID, Scope: string(model.CreatorStepShots), ShotID: task.ShotID,
			Status: task.Status, Label: "正在重新生成镜头",
		})
	}
	if shotState.AssemblyDirty {
		for _, stepID := range []model.CreatorStepID{model.CreatorStepPreview, model.CreatorStepDelivery} {
			index := stepIndexes[stepID]
			steps[index].State = mergeCreatorState(steps[index].State, model.CreatorStepNeedsAttention)
		}
	}
	viewAssemblyDirty := shotState.AssemblyDirty
	if s.assembly != nil {
		if receipt, found, receiptErr := s.assembly.LatestAssemblyReceipt(ctx, userID, projectID); receiptErr == nil && found && receipt.Status == "queued" {
			previewIndex := stepIndexes[model.CreatorStepPreview]
			if receipt.BasePreviewArtifactID != "" && steps[previewIndex].CurrentArtifactID == receipt.BasePreviewArtifactID {
				runID, reviewID, statusErr := s.reviews.ResolveReviewGate(ctx, artifactsByID(artifacts, receipt.BasePreviewArtifactID), receipt.PreviewTaskID, "")
				status := "UNKNOWN"
				if statusErr == nil {
					status, statusErr = s.reviews.RegenerationStatus(ctx, runID, reviewID)
				}
				if statusErr != nil || status == "FAILED" || status == "CANCELLED" || status == "LOCAL_FAILED" || status == "HEARTBEAT_TIMEOUT" {
					steps[previewIndex].State = model.CreatorStepFailed
					steps[stepIndexes[model.CreatorStepDelivery]].State = model.CreatorStepNeedsAttention
					viewAssemblyDirty = true
				} else {
					steps[previewIndex].State = model.CreatorStepGenerating
					steps[stepIndexes[model.CreatorStepDelivery]].State = model.CreatorStepGenerating
					if receipt.PreviewTaskID != "" {
						activeTasks = append(activeTasks, model.CreatorTask{ID: receipt.PreviewTaskID, Scope: string(model.CreatorStepPreview), Status: "running", Label: "正在重新拼接成片"})
					}
				}
			}
		}
	}
	for i := range steps {
		steps[i].AllowedActions = actionsForCreatorState(steps[i].State)
		if steps[i].CurrentArtifactID != "" && s.reviews != nil {
			for _, current := range artifacts {
				if current != nil && current.ID == steps[i].CurrentArtifactID {
					if runID, reviewID, resolveErr := s.reviews.ResolveReviewGate(ctx, current, "", ""); resolveErr == nil {
						steps[i].RunID, steps[i].ReviewID = runID, reviewID
					} else {
						steps[i].RunID, steps[i].ReviewID = "", ""
					}
					break
				}
			}
		}
	}
	activeTasks = deduplicateCreatorTasks(activeTasks)
	processTimeline, stepArtifacts, err := s.creatorAuditProjection(ctx, project, artifacts, steps)
	if err != nil {
		return nil, err
	}
	if finalDelivery == nil {
		index := stepIndexes[model.CreatorStepDelivery]
		steps[index].CurrentArtifactID = ""
		steps[index].CurrentVersion = 0
		steps[index].IsStale = false
		if project.Status == model.StatusCompleted || project.Status == model.StatusArchived {
			steps[index].State = model.CreatorStepNeedsAttention
		}
		if project.Status == model.StatusRunning && s.reviews != nil && steps[index].RunID != "" && steps[index].ReviewID != "" {
			status, statusErr := s.reviews.RegenerationStatus(ctx, steps[index].RunID, steps[index].ReviewID)
			status = strings.ToUpper(strings.TrimSpace(status))
			if statusErr == nil && isActiveCreatorRegenerationStatus(status) {
				steps[index].State = model.CreatorStepGenerating
				activeTasks = append(activeTasks, model.CreatorTask{
					ID: steps[index].ReviewID, Scope: string(model.CreatorStepDelivery), Status: status,
					Label: "正在重新生成交付文件",
				})
			}
		}
	}
	if finalDelivery != nil {
		items := stepArtifacts[model.CreatorStepDelivery]
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].ArtifactID == finalDelivery.ID {
				return true
			}
			if items[j].ArtifactID == finalDelivery.ID {
				return false
			}
			return false
		})
		stepArtifacts[model.CreatorStepDelivery] = items
	}
	for i := range steps {
		steps[i].AllowedActions = actionsForCreatorState(steps[i].State)
		if steps[i].HasHistory && steps[i].ReviewID != "" && steps[i].State != model.CreatorStepGenerating {
			steps[i].AllowedActions = append(steps[i].AllowedActions, "regenerate")
		}
	}
	activeTasks = deduplicateCreatorTasks(activeTasks)

	return &model.CreationView{
		Project: project, ActiveStep: activeCreatorStep(steps), FinalDeliveryArtifactID: artifactID(finalDelivery), Steps: steps,
		ShotSummary: shotState.Summary, ActiveTasks: activeTasks, AssemblyDirty: viewAssemblyDirty,
		ProcessTimeline: processTimeline, StepArtifacts: stepArtifacts,
	}, nil
}

func artifactsByID(items []*artifact.Artifact, id string) *artifact.Artifact {
	for _, item := range items {
		if item != nil && item.ID == id {
			return item
		}
	}
	return nil
}

func (s *CreationService) getCreatorShotReadState(ctx context.Context, userID, projectID string) (creatorShotReadState, error) {
	_, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return creatorShotReadState{}, err
	}
	summary, err := s.GetShotSummary(ctx, userID, projectID)
	if err != nil {
		return creatorShotReadState{}, err
	}
	tasks := make([]model.ShotRegenerationTask, 0, len(state.RegenerationTasks))
	for _, task := range state.RegenerationTasks {
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		if !tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
			return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
		}
		return tasks[i].TaskID < tasks[j].TaskID
	})
	shotIDs := make([]string, 0, len(state.Shots))
	for _, shot := range state.Shots {
		if shot.ID != "" {
			shotIDs = append(shotIDs, shot.ID)
		}
	}
	sort.Strings(shotIDs)
	return creatorShotReadState{Summary: summary, Tasks: tasks, AssemblyDirty: state.AssemblyDirty, ShotIDs: shotIDs}, nil
}

func (s *CreatorViewService) ReviseStep(ctx context.Context, userID, projectID string, stepID model.CreatorStepID, req model.StepRevisionRequest) (*model.StepMutationResult, error) {
	if s.revisions == nil || s.reviews == nil || s.history == nil {
		return nil, ErrCreatorMutationUnavailable
	}
	if !knownCreatorStep(stepID) {
		return nil, ErrCreatorStepInvalid
	}
	if req.BaseVersion <= 0 || strings.TrimSpace(req.IdempotencyKey) == "" {
		return nil, ErrCreatorVersionConflict
	}
	base, err := s.history.GetByID(ctx, req.ArtifactID)
	if err != nil || base == nil || base.ProjectID != projectID {
		return nil, ErrCreatorArtifactNotFound
	}
	if mapped, ok := creatorStepForArtifact(base); !ok || mapped != stepID {
		return nil, ErrCreatorArtifactNotFound
	}
	mode := req.Mode
	var directContent []byte
	var replacement *artifact.ReplacementMaterialIdentity
	message := ""
	switch mode {
	case "direct":
		if strings.TrimSpace(req.DirectContent) == "" || strings.TrimSpace(req.Instruction) != "" ||
			req.ReplacementMaterial != nil || len(req.ModelProviders) != 0 {
			return nil, ErrCreatorInvalidRequest
		}
		directContent = []byte(req.DirectContent)
	case "instruction":
		if strings.TrimSpace(req.Instruction) == "" || strings.TrimSpace(req.DirectContent) != "" ||
			req.ReplacementMaterial != nil {
			return nil, ErrCreatorInvalidRequest
		}
		message = strings.TrimSpace(req.Instruction)
	case "replace":
		if req.ReplacementMaterial == nil || strings.TrimSpace(req.Instruction) != "" ||
			strings.TrimSpace(req.DirectContent) != "" || len(req.ModelProviders) != 0 ||
			base.Kind != artifact.KindImage {
			return nil, ErrCreatorInvalidRequest
		}
		replacement, err = s.resolveCreatorReplacementMaterial(ctx, projectID, *req.ReplacementMaterial)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrCreatorInvalidRequest
	}
	selection, err := normalizeArtifactSelection(req.Selection)
	if err != nil {
		return nil, err
	}
	if mode == "replace" && selection != nil && selection["kind"] != "rect" {
		return nil, ErrCreatorInvalidRequest
	}
	if mode == "direct" && selection != nil && selection["kind"] == "text" {
		return nil, ErrCreatorInvalidRequest
	}
	impact, err := s.stepImpact(ctx, userID, projectID, stepID, base)
	if err != nil {
		return nil, err
	}
	if !sameExactIDs(req.ConfirmedAffectedShotIDs, impact.AffectedShotIDs) {
		return nil, ErrCreatorImpactMismatch
	}
	authoritative, err := s.currentArtifactForStep(ctx, projectID, stepID)
	if err != nil {
		return nil, err
	}
	requestIntent := map[string]interface{}{
		"mode": mode, "instruction": message, "directContent": string(directContent), "selection": selection,
	}
	if replacement != nil {
		requestIntent["replacementMaterial"] = replacement
	}
	requestDigest := creatorRequestDigest(requestIntent)
	if authoritative.ID != base.ID || authoritative.Version != req.BaseVersion {
		return s.retryCreatorMutation(ctx, userID, projectID, stepID, authoritative, req.IdempotencyKey, "revise", base.ID, req.BaseVersion, "", 0, requestDigest, selection, replacement, impact, req.RunID, req.ReviewID)
	}
	runID, reviewID, err := s.reviews.ResolveReviewGate(ctx, base, req.RunID, req.ReviewID)
	if err != nil {
		return nil, err
	}
	receipt := newCreatorMutationReceipt("revise", req.IdempotencyKey, projectID, stepID, base, nil, runID, reviewID, impact, selection, replacement, requestDigest)
	provenance := map[string]interface{}{"mode": mode, "baseVersion": req.BaseVersion, creatorMutationReceiptKey: receipt}
	if selection != nil {
		provenance["selection"] = selection
	}
	if replacement != nil {
		provenance["replacementMaterial"] = *replacement
	}
	var providers map[string]interface{}
	if mode == "instruction" {
		providers, err = s.creatorModelProviders(ctx, userID, projectID, req.ModelProviders)
		if err != nil {
			return nil, err
		}
	}
	var sourceContent []byte
	var expectedSourceHash string
	if selection != nil && selection["kind"] == "text" {
		if s.artifactTextResolver == nil {
			return nil, ErrCreatorSelectionConflict
		}
		reviewText, resolveErr := s.artifactTextResolver.ResolveReviewableText(ctx, base)
		if resolveErr != nil {
			return nil, ErrCreatorSelectionConflict
		}
		expectedSourceHash, _ = selection["sourceHash"].(string)
		if err := validateTextArtifactSelection(selection, reviewText, expectedSourceHash); err != nil {
			return nil, err
		}
		sourceContent = []byte(reviewText)
	}
	var revised *artifact.RevisionResult
	if replacement != nil {
		revised, err = s.revisions.Replace(ctx, artifact.ReplaceRequest{
			ArtifactID: base.ID, BaseVersion: req.BaseVersion, NewArtifactID: receipt.NewArtifactID,
			Material: *replacement, Provenance: provenance,
		})
	} else {
		revised, err = s.revisions.Revise(ctx, artifact.ReviseRequest{
			ArtifactID: base.ID, NewArtifactID: receipt.NewArtifactID, Message: message, DirectContent: directContent,
			SourceContent: sourceContent, ExpectedSourceHash: expectedSourceHash, SourceContentValidated: sourceContent != nil,
			ModelProviders: providers, Provenance: provenance,
		})
	}
	if err != nil {
		if errors.Is(err, artifact.ErrRevisionSourceConflict) {
			return nil, ErrCreatorSelectionConflict
		}
		if errors.Is(err, artifact.ErrArtifactVersionConflict) {
			current, reloadErr := s.currentArtifactForStep(ctx, projectID, stepID)
			if reloadErr == nil {
				return s.retryCreatorMutation(ctx, userID, projectID, stepID, current, req.IdempotencyKey, "revise", base.ID, req.BaseVersion, "", 0, requestDigest, selection, replacement, impact, req.RunID, req.ReviewID)
			}
		}
		return nil, err
	}
	if revised == nil || revised.Artifact == nil {
		return nil, fmt.Errorf("revision produced no artifact")
	}
	if !validCreatorRevisionChild(revised.Artifact, base, projectID, stepID) {
		return nil, ErrCreatorArtifactNotFound
	}
	if err := s.applyCreatorShotInvalidation(ctx, userID, projectID, revised.Artifact.ID, stepID, impact); err != nil {
		return nil, err
	}
	if err := s.reviews.ReopenWithArtifact(ctx, runID, reviewID, projectID, base.ID, revised.Artifact.ID, userID, "内容已修改，请重新确认"); err != nil {
		return nil, err
	}
	view, err := s.GetCreationView(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	return &model.StepMutationResult{Artifact: revised.Artifact, Impact: impact, View: view}, nil
}

func (s *CreatorViewService) PreviewStepRevision(ctx context.Context, userID, projectID string, stepID model.CreatorStepID, req model.StepRevisionRequest) (model.StepImpact, error) {
	if !knownCreatorStep(stepID) {
		return model.StepImpact{}, ErrCreatorStepInvalid
	}
	if s.history == nil || req.BaseVersion <= 0 {
		return model.StepImpact{}, ErrCreatorVersionConflict
	}
	base, err := s.history.GetByID(ctx, req.ArtifactID)
	if err != nil || base == nil || base.ProjectID != projectID {
		return model.StepImpact{}, ErrCreatorArtifactNotFound
	}
	if mapped, ok := creatorStepForArtifact(base); !ok || mapped != stepID {
		return model.StepImpact{}, ErrCreatorArtifactNotFound
	}
	current, err := s.history.GetCurrent(ctx, projectID, base.StageName, base.UnitID)
	if err != nil || current == nil || current.ID != base.ID || current.Version != req.BaseVersion {
		return model.StepImpact{}, ErrCreatorVersionConflict
	}
	authoritative, err := s.currentArtifactForStep(ctx, projectID, stepID)
	if err != nil || authoritative.ID != base.ID {
		return model.StepImpact{}, ErrCreatorVersionConflict
	}
	return s.stepImpact(ctx, userID, projectID, stepID, base)
}

func (s *CreatorViewService) RestoreStepVersion(ctx context.Context, userID, projectID string, stepID model.CreatorStepID, version int, req model.StepRestoreRequest) (*model.StepMutationResult, error) {
	if s.revisions == nil || s.reviews == nil || s.history == nil {
		return nil, ErrCreatorMutationUnavailable
	}
	if !knownCreatorStep(stepID) || version <= 0 || strings.TrimSpace(req.IdempotencyKey) == "" {
		return nil, ErrCreatorStepInvalid
	}
	current, err := s.currentArtifactForStep(ctx, projectID, stepID)
	if err != nil {
		return nil, err
	}
	if req.BaseVersion <= 0 {
		return nil, ErrCreatorVersionConflict
	}
	if current.Version != req.BaseVersion {
		impact, impactErr := s.stepImpact(ctx, userID, projectID, stepID, current)
		if impactErr != nil {
			return nil, impactErr
		}
		if !sameExactIDs(req.ConfirmedAffectedShotIDs, impact.AffectedShotIDs) {
			return nil, ErrCreatorImpactMismatch
		}
		return s.retryCreatorMutation(ctx, userID, projectID, stepID, current, req.IdempotencyKey, "restore", "", req.BaseVersion, "", version, creatorRequestDigest(map[string]interface{}{"reason": strings.TrimSpace(req.Reason)}), nil, nil, impact, req.RunID, req.ReviewID)
	}
	history, err := s.history.GetHistory(ctx, projectID, current.StageName, current.UnitID)
	if err != nil {
		return nil, err
	}
	var historical *artifact.Artifact
	for _, candidate := range history {
		if candidate != nil && candidate.Version == version && candidate.ProjectID == projectID && candidate.StageName == current.StageName && candidate.UnitID == current.UnitID {
			historical = candidate
			break
		}
	}
	if historical == nil {
		return nil, ErrCreatorArtifactNotFound
	}
	impact, err := s.stepImpact(ctx, userID, projectID, stepID, current)
	if err != nil {
		return nil, err
	}
	if !sameExactIDs(req.ConfirmedAffectedShotIDs, impact.AffectedShotIDs) {
		return nil, ErrCreatorImpactMismatch
	}
	runID, reviewID, err := s.reviews.ResolveReviewGate(ctx, current, req.RunID, req.ReviewID)
	if err != nil {
		return nil, err
	}
	requestDigest := creatorRequestDigest(map[string]interface{}{"reason": strings.TrimSpace(req.Reason)})
	receipt := newCreatorMutationReceipt("restore", req.IdempotencyKey, projectID, stepID, current, historical, runID, reviewID, impact, nil, nil, requestDigest)
	restored, err := s.revisions.Restore(ctx, artifact.RestoreRequest{
		ArtifactID: historical.ID, NewArtifactID: receipt.NewArtifactID, ReviewerID: userID,
		Reason: strings.TrimSpace(req.Reason), Provenance: map[string]interface{}{creatorMutationReceiptKey: receipt},
	})
	if err != nil {
		if errors.Is(err, artifact.ErrArtifactVersionConflict) {
			latest, reloadErr := s.currentArtifactForStep(ctx, projectID, stepID)
			if reloadErr == nil {
				return s.retryCreatorMutation(ctx, userID, projectID, stepID, latest, req.IdempotencyKey, "restore", current.ID, req.BaseVersion, historical.ID, version, requestDigest, nil, nil, impact, req.RunID, req.ReviewID)
			}
		}
		return nil, err
	}
	if restored == nil || restored.Artifact == nil {
		return nil, fmt.Errorf("restore produced no artifact")
	}
	if !validCreatorRevisionChild(restored.Artifact, current, projectID, stepID) {
		return nil, ErrCreatorArtifactNotFound
	}
	if err := s.applyCreatorShotInvalidation(ctx, userID, projectID, restored.Artifact.ID, stepID, impact); err != nil {
		return nil, err
	}
	if err := s.reviews.ReopenWithArtifact(ctx, runID, reviewID, projectID, current.ID, restored.Artifact.ID, userID, "历史版本已恢复，请重新确认"); err != nil {
		return nil, err
	}
	view, err := s.GetCreationView(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	return &model.StepMutationResult{Artifact: restored.Artifact, Impact: impact, View: view}, nil
}

func (s *CreatorViewService) GetStepVersions(ctx context.Context, userID, projectID string, stepID model.CreatorStepID) (*model.StepVersions, error) {
	if s.history == nil {
		return nil, ErrCreatorMutationUnavailable
	}
	if !knownCreatorStep(stepID) {
		return nil, ErrCreatorStepInvalid
	}
	current, err := s.currentArtifactForStep(ctx, projectID, stepID)
	if err != nil {
		return nil, err
	}
	history, err := s.history.GetHistory(ctx, projectID, current.StageName, current.UnitID)
	if err != nil {
		return nil, err
	}
	filtered := make([]*artifact.Artifact, 0, len(history))
	for _, item := range history {
		if item != nil && item.ProjectID == projectID && item.StageName == current.StageName && item.UnitID == current.UnitID {
			filtered = append(filtered, item)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Version != filtered[j].Version {
			return filtered[i].Version > filtered[j].Version
		}
		return filtered[i].ID > filtered[j].ID
	})
	versions := make([]model.CreatorArtifactVersion, 0, len(filtered))
	for _, item := range filtered {
		versions = append(versions, model.CreatorArtifactVersion{ArtifactID: item.ID, Version: item.Version, IsCurrent: item.ID == current.ID, CreatedAt: item.CreatedAt})
	}
	return &model.StepVersions{Versions: versions}, nil
}

func (s *CreatorViewService) ConfirmStep(ctx context.Context, userID, projectID string, stepID model.CreatorStepID, req model.StepConfirmRequest) (*model.CreationView, error) {
	if s.reviews == nil || s.history == nil {
		return nil, ErrCreatorMutationUnavailable
	}
	if !knownCreatorStep(stepID) {
		return nil, ErrCreatorStepInvalid
	}
	current, err := s.currentArtifactForStep(ctx, projectID, stepID)
	if err != nil {
		return nil, err
	}
	if req.ArtifactID == "" || req.ArtifactID != current.ID {
		return nil, ErrCreatorArtifactNotFound
	}
	runID, reviewID, err := s.reviews.ResolveReviewGate(ctx, current, req.RunID, req.ReviewID)
	if err != nil {
		return nil, err
	}
	if err := s.reviews.ConfirmForArtifact(ctx, runID, reviewID, current.ID, userID, strings.TrimSpace(req.Comment)); err != nil {
		return nil, err
	}
	return s.GetCreationView(ctx, userID, projectID)
}

func (s *CreatorViewService) currentArtifactForStep(ctx context.Context, projectID string, stepID model.CreatorStepID) (*artifact.Artifact, error) {
	items, err := s.artifacts.ListCurrentByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if stepID == model.CreatorStepDelivery {
		if finalDelivery := authoritativeFinalDeliveryArtifact(items); finalDelivery != nil {
			return finalDelivery, nil
		}
		return nil, ErrCreatorArtifactNotFound
	}
	selection := model.CreatorStep{ID: stepID, State: model.CreatorStepNotStarted}
	byID := make(map[string]*artifact.Artifact, len(items))
	for _, candidate := range items {
		if candidate == nil {
			continue
		}
		mapped, ok := creatorStepForArtifact(candidate)
		if !ok || mapped != stepID || candidate.ProjectID != projectID {
			continue
		}
		byID[candidate.ID] = candidate
		if stepID == model.CreatorStepShots {
			if artifactPreferred(candidate, selection) {
				applyArtifact(&selection, candidate, selection.State)
			}
			continue
		}
		state := stateForArtifact(candidate)
		if statePriority(state) > statePriority(selection.State) ||
			(statePriority(state) == statePriority(selection.State) && artifactPreferred(candidate, selection)) {
			applyArtifact(&selection, candidate, state)
		}
	}
	current := byID[selection.CurrentArtifactID]
	if current == nil {
		return nil, ErrCreatorArtifactNotFound
	}
	return current, nil
}

func knownCreatorStep(stepID model.CreatorStepID) bool {
	for _, definition := range creatorStepDefinitions {
		if definition.id == stepID {
			return true
		}
	}
	return false
}

func validCreatorRevisionChild(candidate, parent *artifact.Artifact, projectID string, stepID model.CreatorStepID) bool {
	if candidate == nil || parent == nil || candidate.ProjectID != projectID || candidate.ParentID != parent.ID || candidate.Version != parent.Version+1 {
		return false
	}
	mapped, ok := creatorStepForArtifact(candidate)
	return ok && mapped == stepID && candidate.StageName == parent.StageName && candidate.UnitID == parent.UnitID
}

func (s *CreatorViewService) resolveCreatorReplacementMaterial(
	ctx context.Context,
	projectID string,
	supplied model.ReplacementMaterial,
) (*artifact.ReplacementMaterialIdentity, error) {
	manifest, err := s.history.GetCurrent(ctx, projectID, "requirements", "source-materials")
	if err != nil || manifest == nil || manifest.ProjectID != projectID ||
		manifest.StageName != "requirements" || manifest.UnitID != "source-materials" {
		return nil, ErrCreatorInvalidRequest
	}
	materials, err := assets.ProjectMaterialsFromArtifact(manifest)
	if err != nil {
		return nil, ErrCreatorInvalidRequest
	}
	for _, candidate := range materials {
		canonical, normalizeErr := assets.NormalizeProjectMaterial(projectID, candidate)
		if normalizeErr != nil {
			return nil, ErrCreatorInvalidRequest
		}
		if canonical.ContentHash != supplied.ContentHash {
			continue
		}
		if canonical.Kind != assets.AssetTypeImage ||
			canonical.StorageRef != supplied.StorageRef ||
			canonical.MimeType != supplied.MimeType ||
			canonical.SizeBytes != supplied.SizeBytes {
			return nil, ErrCreatorInvalidRequest
		}
		return &artifact.ReplacementMaterialIdentity{
			ContentHash: canonical.ContentHash,
			StorageRef:  canonical.StorageRef,
			MimeType:    canonical.MimeType,
			SizeBytes:   canonical.SizeBytes,
		}, nil
	}
	return nil, ErrCreatorInvalidRequest
}

func normalizeArtifactSelection(selection *model.ArtifactSelection) (map[string]interface{}, error) {
	if selection == nil {
		return nil, nil
	}
	kind := strings.ToLower(strings.TrimSpace(selection.Kind))
	switch kind {
	case "rect":
		if selection.X == nil || selection.Y == nil || selection.Width == nil || selection.Height == nil ||
			selection.StartMs != nil || selection.EndMs != nil || selection.Start != nil || selection.End != nil || selection.Text != "" || selection.SourceHash != "" {
			return nil, ErrCreatorInvalidRequest
		}
		x, y, width, height := *selection.X, *selection.Y, *selection.Width, *selection.Height
		if x < 0 || y < 0 || width <= 0 || height <= 0 || x > 1 || y > 1 || x+width > 1 || y+height > 1 {
			return nil, ErrCreatorInvalidRequest
		}
		return map[string]interface{}{"kind": kind, "x": x, "y": y, "width": width, "height": height}, nil
	case "time":
		if selection.StartMs == nil || selection.EndMs == nil || selection.X != nil || selection.Y != nil ||
			selection.Width != nil || selection.Height != nil || selection.Start != nil || selection.End != nil || selection.Text != "" || selection.SourceHash != "" {
			return nil, ErrCreatorInvalidRequest
		}
		if *selection.StartMs < 0 || *selection.EndMs <= *selection.StartMs {
			return nil, ErrCreatorInvalidRequest
		}
		return map[string]interface{}{"kind": kind, "startMs": *selection.StartMs, "endMs": *selection.EndMs}, nil
	case "text":
		if selection.Start == nil || selection.End == nil || selection.Text == "" ||
			selection.X != nil || selection.Y != nil || selection.Width != nil || selection.Height != nil ||
			selection.StartMs != nil || selection.EndMs != nil {
			return nil, ErrCreatorInvalidRequest
		}
		start, end, text, sourceHash := *selection.Start, *selection.End, selection.Text, strings.TrimSpace(selection.SourceHash)
		if start < 0 || end <= start || end-start > 4000 || len(utf16.Encode([]rune(text))) > 4000 {
			return nil, ErrCreatorInvalidRequest
		}
		if len(sourceHash) != len("sha256:")+64 || !strings.HasPrefix(sourceHash, "sha256:") {
			return nil, ErrCreatorInvalidRequest
		}
		if _, decodeErr := hex.DecodeString(strings.TrimPrefix(sourceHash, "sha256:")); decodeErr != nil || sourceHash != strings.ToLower(sourceHash) {
			return nil, ErrCreatorInvalidRequest
		}
		return map[string]interface{}{"kind": kind, "start": start, "end": end, "text": text, "sourceHash": sourceHash}, nil
	default:
		return nil, ErrCreatorInvalidRequest
	}
}

func validateTextArtifactSelection(selection map[string]interface{}, source, expectedSourceHash string) error {
	start, startOK := selection["start"].(int)
	end, endOK := selection["end"].(int)
	text, textOK := selection["text"].(string)
	if !startOK || !endOK || !textOK {
		return ErrCreatorInvalidRequest
	}
	actualHash := sha256.Sum256([]byte(source))
	if expectedSourceHash != fmt.Sprintf("sha256:%x", actualHash) {
		return ErrCreatorSelectionConflict
	}
	sourceUnits := utf16.Encode([]rune(source))
	if end > len(sourceUnits) || splitsUTF16SurrogatePair(sourceUnits, start) || splitsUTF16SurrogatePair(sourceUnits, end) {
		return ErrCreatorSelectionConflict
	}
	selectedUnits := utf16.Encode([]rune(text))
	if len(selectedUnits) != end-start {
		return ErrCreatorSelectionConflict
	}
	for index, unit := range selectedUnits {
		if sourceUnits[start+index] != unit {
			return ErrCreatorSelectionConflict
		}
	}
	return nil
}

func splitsUTF16SurrogatePair(units []uint16, offset int) bool {
	return offset > 0 && offset < len(units) &&
		utf16.IsSurrogate(rune(units[offset-1])) && utf16.IsSurrogate(rune(units[offset]))
}

func (s *CreatorViewService) stepImpact(ctx context.Context, userID, projectID string, stepID model.CreatorStepID, current *artifact.Artifact) (model.StepImpact, error) {
	downstream := map[model.CreatorStepID][]model.CreatorStepID{
		model.CreatorStepRequirements: {model.CreatorStepDirection, model.CreatorStepScript, model.CreatorStepShots, model.CreatorStepPreview, model.CreatorStepDelivery},
		model.CreatorStepDirection:    {model.CreatorStepScript, model.CreatorStepShots, model.CreatorStepPreview, model.CreatorStepDelivery},
		model.CreatorStepScript:       {model.CreatorStepShots, model.CreatorStepPreview, model.CreatorStepDelivery},
		model.CreatorStepShots:        {model.CreatorStepPreview, model.CreatorStepDelivery},
		model.CreatorStepPreview:      {model.CreatorStepDelivery},
		model.CreatorStepDelivery:     {},
	}
	affected, ok := downstream[stepID]
	if !ok {
		return model.StepImpact{}, ErrCreatorStepInvalid
	}
	impact := model.StepImpact{AffectedStepIDs: append([]model.CreatorStepID(nil), affected...), RequiresConfirmation: len(affected) > 0}
	stage := ""
	if current != nil {
		stage = normalizeCreatorStage(current.StageName)
	}
	if stage == "script" || stage == "character" || stage == "characters" || stage == "audio_master" || stage == "style" {
		shotState, err := s.shots.getCreatorShotReadState(ctx, userID, projectID)
		if err != nil {
			return model.StepImpact{}, err
		}
		impact.AffectedShotIDs = append([]string(nil), shotState.ShotIDs...)
		sort.Strings(impact.AffectedShotIDs)
	}
	return impact, nil
}

func sameExactIDs(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	seen := make(map[string]bool, len(actual))
	for _, id := range actual {
		if id == "" || seen[id] {
			return false
		}
		seen[id] = true
	}
	for _, id := range expected {
		if !seen[id] {
			return false
		}
	}
	return true
}

func creatorRequestDigest(value interface{}) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func newCreatorMutationReceipt(operation, key, projectID string, stepID model.CreatorStepID, base, historical *artifact.Artifact, runID, reviewID string, impact model.StepImpact, selection map[string]interface{}, replacement *artifact.ReplacementMaterialIdentity, requestDigest string) creatorMutationReceipt {
	receipt := creatorMutationReceipt{
		Operation: operation, IdempotencyKey: key, ProjectID: projectID, StepID: string(stepID),
		BaseArtifactID: base.ID, BaseVersion: base.Version, RunID: runID, ReviewID: reviewID,
		ParentArtifactID: base.ID, AffectedStepIDs: append([]model.CreatorStepID(nil), impact.AffectedStepIDs...),
		AffectedShotIDs: append([]string(nil), impact.AffectedShotIDs...), Selection: selection, RequestDigest: requestDigest,
	}
	if replacement != nil {
		canonical := *replacement
		receipt.ReplacementMaterial = &canonical
	}
	if historical != nil {
		receipt.HistoricalArtifactID, receipt.HistoricalVersion = historical.ID, historical.Version
	}
	idSeed := sha256.Sum256([]byte(operation + "\x00" + projectID + "\x00" + string(stepID) + "\x00" + key))
	receipt.NewArtifactID = "art-creator-" + hex.EncodeToString(idSeed[:12])
	receipt.Fingerprint = creatorReceiptFingerprint(receipt)
	return receipt
}

func creatorReceiptFingerprint(receipt creatorMutationReceipt) string {
	receipt.IdempotencyKey, receipt.Fingerprint, receipt.NewArtifactID = "", "", ""
	data, _ := json.Marshal(receipt)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func creatorReceiptFromArtifact(current *artifact.Artifact) (creatorMutationReceipt, bool) {
	if current == nil || current.Metadata == nil {
		return creatorMutationReceipt{}, false
	}
	value, ok := current.Metadata[creatorMutationReceiptKey]
	if !ok {
		return creatorMutationReceipt{}, false
	}
	data, err := json.Marshal(value)
	if err != nil {
		return creatorMutationReceipt{}, false
	}
	var receipt creatorMutationReceipt
	if json.Unmarshal(data, &receipt) != nil || receipt.NewArtifactID != current.ID || receipt.Fingerprint == "" {
		return creatorMutationReceipt{}, false
	}
	return receipt, creatorReceiptFingerprint(receipt) == receipt.Fingerprint
}

func (s *CreatorViewService) retryCreatorMutation(ctx context.Context, userID, projectID string, stepID model.CreatorStepID, current *artifact.Artifact, key, operation, baseArtifactID string, baseVersion int, historicalArtifactID string, historicalVersion int, requestDigest string, selection map[string]interface{}, replacement *artifact.ReplacementMaterialIdentity, impact model.StepImpact, assertedRunID, assertedReviewID string) (*model.StepMutationResult, error) {
	receipt, ok := creatorReceiptFromArtifact(current)
	if !ok || receipt.ProjectID != projectID || receipt.StepID != string(stepID) || receipt.Operation != operation {
		return nil, ErrCreatorVersionConflict
	}
	if receipt.IdempotencyKey != key {
		return nil, ErrCreatorVersionConflict
	}
	if receipt.BaseVersion != baseVersion || (baseArtifactID != "" && receipt.BaseArtifactID != baseArtifactID) ||
		receipt.HistoricalVersion != historicalVersion || (historicalArtifactID != "" && receipt.HistoricalArtifactID != historicalArtifactID) ||
		receipt.RequestDigest != requestDigest || !reflectCreatorJSON(receipt.Selection, selection) ||
		!reflectCreatorJSON(receipt.ReplacementMaterial, replacement) ||
		!reflectCreatorJSON(receipt.AffectedStepIDs, impact.AffectedStepIDs) || !reflectCreatorJSON(receipt.AffectedShotIDs, impact.AffectedShotIDs) {
		return nil, ErrCreatorIdempotencyConflict
	}
	if (assertedRunID != "" && assertedRunID != receipt.RunID) || (assertedReviewID != "" && assertedReviewID != receipt.ReviewID) {
		return nil, ErrCreatorIdempotencyConflict
	}
	runID, reviewID, err := s.reviews.ResolveReviewGate(ctx, current, assertedRunID, assertedReviewID)
	if err != nil {
		return nil, err
	}
	if runID != receipt.RunID || reviewID != receipt.ReviewID {
		return nil, ErrCreatorIdempotencyConflict
	}
	if err := s.applyCreatorShotInvalidation(ctx, userID, projectID, current.ID, stepID, impact); err != nil {
		return nil, err
	}
	reason := "内容已修改，请重新确认"
	if operation == "restore" {
		reason = "历史版本已恢复，请重新确认"
	}
	if err := s.reviews.ReopenWithArtifact(ctx, runID, reviewID, projectID, receipt.ParentArtifactID, current.ID, userID, reason); err != nil {
		return nil, err
	}
	view, err := s.GetCreationView(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	return &model.StepMutationResult{Artifact: current, Impact: impact, View: view}, nil
}

func (s *CreatorViewService) applyCreatorShotInvalidation(ctx context.Context, userID, projectID, revisionID string, stepID model.CreatorStepID, impact model.StepImpact) error {
	if len(impact.AffectedShotIDs) == 0 {
		return nil
	}
	if s.shotInvalidations == nil {
		return ErrCreatorMutationUnavailable
	}
	return s.shotInvalidations.InvalidateShotsForUpstreamRevision(
		ctx, userID, projectID, revisionID, impact.AffectedShotIDs,
		fmt.Sprintf("%s changed; review affected Shots before assembly", stepID),
	)
}

func reflectCreatorJSON(left, right interface{}) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return string(a) == string(b)
}

func (s *CreatorViewService) creatorModelProviders(ctx context.Context, userID, projectID string, runtimeProviders map[string]interface{}) (map[string]interface{}, error) {
	project, err := s.projects.GetProject(ctx, userID, projectID)
	if err != nil || project == nil || project.ID != projectID {
		return nil, ErrCreatorModelProviderUnavailable
	}
	var config map[string]interface{}
	if len(project.Config) > 0 {
		_ = json.Unmarshal(project.Config, &config)
	}
	refs, _ := config["modelProviderRefs"].(map[string]interface{})
	if !creatorTextProviderMatchesReference(refs, runtimeProviders) {
		return nil, ErrCreatorModelProviderUnavailable
	}
	data, _ := json.Marshal(runtimeProviders)
	var cloned map[string]interface{}
	_ = json.Unmarshal(data, &cloned)
	return cloned, nil
}

func creatorTextProviderMatchesReference(refs, providers map[string]interface{}) bool {
	ref, _ := refs["text_to_text"].(map[string]interface{})
	provider, _ := providers["text_to_text"].(map[string]interface{})
	source, _ := ref["source"].(string)
	refBaseURL, _ := ref["baseUrl"].(string)
	refModel, _ := ref["model"].(string)
	baseURL, _ := provider["baseUrl"].(string)
	modelName, _ := provider["model"].(string)
	apiKey, _ := provider["apiKey"].(string)
	return strings.TrimSpace(source) == "local_agent" &&
		strings.TrimSpace(refBaseURL) != "" && strings.TrimSpace(refBaseURL) == strings.TrimSpace(baseURL) &&
		strings.TrimSpace(refModel) != "" && strings.TrimSpace(refModel) == strings.TrimSpace(modelName) &&
		strings.TrimSpace(apiKey) != ""
}

func newCreatorSteps() []model.CreatorStep {
	steps := make([]model.CreatorStep, len(creatorStepDefinitions))
	for i, definition := range creatorStepDefinitions {
		steps[i] = model.CreatorStep{ID: definition.id, Label: definition.label, State: model.CreatorStepNotStarted, AllowedActions: []string{"start"}}
	}
	return steps
}

func creatorStepForStage(stage string) (model.CreatorStepID, bool) {
	step, ok := creatorStageSteps[normalizeCreatorStage(stage)]
	return step, ok
}

func creatorStepForArtifact(item *artifact.Artifact) (model.CreatorStepID, bool) {
	if item == nil {
		return "", false
	}
	if _, ok := finalDeliveryVideoScore(item); ok {
		return model.CreatorStepDelivery, true
	}
	if step, ok := creatorStepForStage(item.StageName); ok {
		return step, true
	}
	return creatorStepForStage(item.UnitID)
}

func artifactID(item *artifact.Artifact) string {
	if item == nil {
		return ""
	}
	return item.ID
}

func authoritativeFinalDeliveryArtifact(items []*artifact.Artifact) *artifact.Artifact {
	var selected *artifact.Artifact
	selectedScore := -1
	for _, candidate := range items {
		score, ok := finalDeliveryVideoScore(candidate)
		if !ok {
			continue
		}
		if selected == nil || score > selectedScore ||
			(score == selectedScore && candidate.Version > selected.Version) ||
			(score == selectedScore && candidate.Version == selected.Version && candidate.CreatedAt.After(selected.CreatedAt)) ||
			(score == selectedScore && candidate.Version == selected.Version && candidate.CreatedAt.Equal(selected.CreatedAt) && candidate.ID > selected.ID) {
			selected, selectedScore = candidate, score
		}
	}
	return selected
}

func finalDeliveryVideoScore(item *artifact.Artifact) (int, bool) {
	if item == nil || item.Kind != artifact.KindVideo || !item.IsCurrent || artifactIsStale(item) {
		return 0, false
	}
	stage := normalizeCreatorStage(item.StageName)
	unit := normalizeCreatorStage(item.UnitID)
	name := normalizeCreatorStage(item.Name)
	artifactType, generationKind, relatedShotID := creatorArtifactClassificationHints(item)
	artifactType = normalizeCreatorStage(artifactType)
	if relatedShotID != "" || artifactType == "publish_copy" || stage == "publish" || stage == "publish_copy" || stage == "export" || stage == "package" {
		return 0, false
	}
	score := 0
	if unit == "final_video" {
		score += 80
	}
	if name == "final.mp4" || name == "final_video.mp4" {
		score += 40
	}
	if stage == "delivery" {
		score += 60
	}
	if stage == "render" || stage == "final_render" {
		score += 30
	}
	if normalizeCreatorStage(generationKind) == "video" && artifactType == "external_generation_result" {
		score += 20
	}
	if item.Metadata != nil {
		for _, key := range []string{"generationRequestId", "externalGenerationRequestId"} {
			if value, _ := item.Metadata[key].(string); normalizeCreatorStage(value) == "final_video" {
				score += 100
			}
		}
		if tags, ok := item.Metadata["tags"].([]interface{}); ok {
			for _, raw := range tags {
				if value, _ := raw.(string); normalizeCreatorStage(value) == "final_video" {
					score += 120
					break
				}
			}
		}
		if tags, ok := item.Metadata["tags"].([]string); ok {
			for _, value := range tags {
				if normalizeCreatorStage(value) == "final_video" {
					score += 120
					break
				}
			}
		}
	}
	return score, score > 0
}

func normalizeCreatorStage(stage string) string {
	stage = strings.ToLower(strings.TrimSpace(stage))
	stage = strings.ReplaceAll(stage, "-", "_")
	stage = strings.ReplaceAll(stage, " ", "_")
	return stage
}

func stateForArtifact(current *artifact.Artifact) model.CreatorStepState {
	switch normalizeCreatorStage(current.Status) {
	case "failed", "error":
		return model.CreatorStepFailed
	case "stale", "rejected", "invalidated", "cancelled", "canceled":
		return model.CreatorStepNeedsAttention
	case "pending", "waiting_approval", "needs_review", "review":
		return model.CreatorStepNeedsReview
	case "generating", "running", "queued", "processing":
		return model.CreatorStepGenerating
	case "valid", "succeeded", "success", "completed":
		if current.HumanApproved {
			return model.CreatorStepConfirmed
		}
		return model.CreatorStepNeedsReview
	default:
		return model.CreatorStepNotStarted
	}
}

func stateForShots(summary model.ShotSummary) model.CreatorStepState {
	if summary.NeedsAction > 0 {
		return model.CreatorStepNeedsAttention
	}
	if summary.AwaitingReview > 0 {
		return model.CreatorStepNeedsReview
	}
	if summary.Generating > 0 {
		return model.CreatorStepGenerating
	}
	if summary.Total > 0 && summary.Confirmed == summary.Total {
		return model.CreatorStepConfirmed
	}
	return model.CreatorStepNotStarted
}

func statePriority(state model.CreatorStepState) int {
	switch state {
	case model.CreatorStepFailed:
		return 6
	case model.CreatorStepNeedsAttention:
		return 5
	case model.CreatorStepNeedsReview:
		return 4
	case model.CreatorStepGenerating:
		return 3
	case model.CreatorStepConfirmed:
		return 2
	default:
		return 1
	}
}

func mergeCreatorState(current, candidate model.CreatorStepState) model.CreatorStepState {
	if statePriority(candidate) > statePriority(current) {
		return candidate
	}
	return current
}

func artifactPreferred(candidate *artifact.Artifact, step model.CreatorStep) bool {
	if step.CurrentArtifactID == "" || candidate.Version != step.CurrentVersion {
		return candidate.Version > step.CurrentVersion
	}
	return candidate.ID > step.CurrentArtifactID
}

func applyArtifact(step *model.CreatorStep, current *artifact.Artifact, state model.CreatorStepState) {
	step.State = state
	step.CurrentArtifactID = current.ID
	step.CurrentVersion = current.Version
	step.RunID = current.WorkflowRunID
	// Artifact records do not carry a review gate ID. Leave ReviewID empty rather
	// than guessing from a task or stage name.
	step.ReviewID = ""
}

func isActiveArtifact(current *artifact.Artifact) bool {
	switch normalizeCreatorStage(current.Status) {
	case "generating", "running", "processing":
		return true
	default:
		return false
	}
}

func isActiveCreatorRegenerationStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "CREATED", "READY", "RUNNING", "WAITING_LOCAL", "LOCAL_CLAIMED", "LOCAL_RUNNING", "LOCAL_COMPLETED", "RETRYING":
		return true
	default:
		return false
	}
}

func isActiveShotRegeneration(status string) bool {
	switch status {
	case ShotRegenerationQueued, ShotRegenerationDispatching, ShotRegenerationRunning:
		return true
	default:
		return false
	}
}

// latestCreatorShotTasks applies the same UpdatedAt, CreatedAt, TaskID ordering
// as Shot summary/workspace reads. A terminal newest task deliberately hides an
// older queued or running task for that Shot.
func latestCreatorShotTasks(tasks []model.ShotRegenerationTask) []model.ShotRegenerationTask {
	latestByShot := make(map[string]model.ShotRegenerationTask)
	for _, task := range tasks {
		if task.ShotID == "" {
			continue
		}
		latest, ok := latestByShot[task.ShotID]
		if !ok || taskIsLater(task, latest) {
			latestByShot[task.ShotID] = task
		}
	}
	latest := make([]model.ShotRegenerationTask, 0, len(latestByShot))
	for _, task := range latestByShot {
		latest = append(latest, task)
	}
	sort.Slice(latest, func(i, j int) bool {
		if latest[i].ShotID != latest[j].ShotID {
			return latest[i].ShotID < latest[j].ShotID
		}
		return latest[i].TaskID < latest[j].TaskID
	})
	return latest
}

func deduplicateCreatorTasks(tasks []model.CreatorTask) []model.CreatorTask {
	sortCreatorTasks(tasks)
	byID := make(map[string]model.CreatorTask, len(tasks))
	for _, task := range tasks {
		if existing, ok := byID[task.ID]; !ok || (task.ShotID != "" && existing.ShotID == "") {
			byID[task.ID] = task
		}
	}
	unique := make([]model.CreatorTask, 0, len(byID))
	for _, task := range byID {
		unique = append(unique, task)
	}
	sortCreatorTasks(unique)
	return unique
}

func sortCreatorTasks(tasks []model.CreatorTask) {
	sort.SliceStable(tasks, func(i, j int) bool {
		left, right := creatorTaskScopeOrder(tasks[i].Scope), creatorTaskScopeOrder(tasks[j].Scope)
		if left != right {
			return left < right
		}
		if tasks[i].ShotID != tasks[j].ShotID {
			return tasks[i].ShotID < tasks[j].ShotID
		}
		return tasks[i].ID < tasks[j].ID
	})
}

func creatorTaskScopeOrder(scope string) int {
	for i, definition := range creatorStepDefinitions {
		if scope == string(definition.id) {
			return i
		}
	}
	return len(creatorStepDefinitions)
}

func creatorTaskLabel(stepID model.CreatorStepID) string {
	switch stepID {
	case model.CreatorStepRequirements:
		return "正在准备需求"
	case model.CreatorStepDirection:
		return "正在准备创意方向"
	case model.CreatorStepScript:
		return "正在准备脚本"
	case model.CreatorStepShots:
		return "正在准备镜头和素材"
	case model.CreatorStepPreview:
		return "正在准备视频预览"
	case model.CreatorStepDelivery:
		return "正在准备交付文件"
	default:
		return "正在准备内容"
	}
}

func actionsForCreatorState(state model.CreatorStepState) []string {
	switch state {
	case model.CreatorStepFailed:
		return []string{"view", "retry"}
	case model.CreatorStepNeedsAttention:
		return []string{"view", "retry", "revise"}
	case model.CreatorStepNeedsReview:
		return []string{"view", "confirm", "revise"}
	case model.CreatorStepGenerating:
		return []string{"view"}
	case model.CreatorStepConfirmed:
		return []string{"view", "revise"}
	default:
		return []string{"start"}
	}
}

func activeCreatorStep(steps []model.CreatorStep) model.CreatorStepID {
	for _, state := range []model.CreatorStepState{
		model.CreatorStepFailed, model.CreatorStepNeedsAttention, model.CreatorStepNeedsReview,
		model.CreatorStepGenerating, model.CreatorStepNotStarted,
	} {
		for _, step := range steps {
			if step.State == state {
				return step.ID
			}
		}
	}
	return model.CreatorStepDelivery
}
