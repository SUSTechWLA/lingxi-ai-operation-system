package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
	"github.com/tangying-ai/aios-core/internal/core/artifact"
)

// The mapping is deliberately closed: developer-only or future stages cannot
// appear as an extra creator step.
var creatorStageSteps = map[string]model.CreatorStepID{
	"requirements":     model.CreatorStepRequirements,
	"requirement":      model.CreatorStepRequirements,
	"brief":            model.CreatorStepRequirements,
	"source_materials": model.CreatorStepRequirements,

	"direction":          model.CreatorStepDirection,
	"creative_direction": model.CreatorStepDirection,
	"proposal":           model.CreatorStepDirection,
	"research":           model.CreatorStepDirection,
	"style":              model.CreatorStepDirection,
	"character":          model.CreatorStepDirection,
	"characters":         model.CreatorStepDirection,
	"feasibility":        model.CreatorStepDirection,

	"script":       model.CreatorStepScript,
	"voiceover":    model.CreatorStepScript,
	"audio_master": model.CreatorStepScript,
	"timing":       model.CreatorStepScript,

	"shots":           model.CreatorStepShots,
	"shot":            model.CreatorStepShots,
	"storyboard":      model.CreatorStepShots,
	"composition":     model.CreatorStepShots,
	"reference":       model.CreatorStepShots,
	"continuity":      model.CreatorStepShots,
	"visual_plan":     model.CreatorStepShots,
	"render_strategy": model.CreatorStepShots,
	"assets":          model.CreatorStepShots,
	"materials":       model.CreatorStepShots,

	"preview":      model.CreatorStepPreview,
	"assembly":     model.CreatorStepPreview,
	"captions":     model.CreatorStepPreview,
	"subtitle":     model.CreatorStepPreview,
	"audio_mix":    model.CreatorStepPreview,
	"quality":      model.CreatorStepPreview,
	"final_review": model.CreatorStepPreview,
	"final_qa":     model.CreatorStepPreview,
	"render":       model.CreatorStepPreview,

	"delivery": model.CreatorStepDelivery,
	"package":  model.CreatorStepDelivery,
	"publish":  model.CreatorStepDelivery,
	"export":   model.CreatorStepDelivery,
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

// CreatorViewService combines only persisted project, artifact, and Shot data.
// It has no client-derived state or workflow topology dependency.
type CreatorViewService struct {
	projects  creatorProjectReader
	artifacts creatorArtifactReader
	shots     creatorShotReader
	history   creatorArtifactHistoryReader
	revisions creatorRevisionService
	reviews   creatorReviewMutations
}

type creatorArtifactHistoryReader interface {
	GetByID(context.Context, string) (*artifact.Artifact, error)
	GetCurrent(context.Context, string, string, string) (*artifact.Artifact, error)
	GetHistory(context.Context, string, string, string) ([]*artifact.Artifact, error)
}

type creatorRevisionService interface {
	Revise(context.Context, artifact.ReviseRequest) (*artifact.RevisionResult, error)
	Restore(context.Context, artifact.RestoreRequest) (*artifact.RevisionResult, error)
}

type creatorReviewMutations interface {
	ResolveReviewGate(context.Context, *artifact.Artifact, string, string) (string, string, error)
	Confirm(context.Context, string, string, string, string) error
	ReopenWithArtifact(context.Context, string, string, string, string, string) error
}

var (
	ErrCreatorStepInvalid         = errors.New("creator step is invalid")
	ErrCreatorArtifactNotFound    = errors.New("creator artifact not found")
	ErrCreatorVersionConflict     = errors.New("creator artifact version conflict")
	ErrCreatorImpactMismatch      = errors.New("creator impact confirmation mismatch")
	ErrCreatorMutationUnavailable = errors.New("creator step mutation is unavailable")
	ErrCreatorInvalidRequest      = errors.New("creator step request is invalid")
)

func NewCreatorViewService(projects creatorProjectReader, shots creatorShotReader, artifacts creatorArtifactReader) *CreatorViewService {
	return &CreatorViewService{projects: projects, artifacts: artifacts, shots: shots}
}

func (s *CreatorViewService) WithStepMutations(revisions creatorRevisionService, reviews creatorReviewMutations) *CreatorViewService {
	s.revisions, s.reviews = revisions, reviews
	if history, ok := s.artifacts.(creatorArtifactHistoryReader); ok {
		s.history = history
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
	artifacts, err := s.artifacts.ListCurrentByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
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
		stepID, ok := creatorStepForStage(current.StageName)
		if !ok {
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

	return &model.CreationView{
		Project: project, ActiveStep: activeCreatorStep(steps), Steps: steps,
		ShotSummary: shotState.Summary, ActiveTasks: activeTasks, AssemblyDirty: shotState.AssemblyDirty,
	}, nil
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
	if req.BaseVersion <= 0 {
		return nil, ErrCreatorVersionConflict
	}
	base, err := s.history.GetByID(ctx, req.ArtifactID)
	if err != nil || base == nil || base.ProjectID != projectID {
		return nil, ErrCreatorArtifactNotFound
	}
	current, err := s.history.GetCurrent(ctx, projectID, base.StageName, base.UnitID)
	if err != nil || current == nil || current.ID != base.ID || current.Version != req.BaseVersion {
		return nil, ErrCreatorVersionConflict
	}
	authoritative, err := s.currentArtifactForStep(ctx, projectID, stepID)
	if err != nil || authoritative.ID != base.ID {
		return nil, ErrCreatorVersionConflict
	}
	if mapped, ok := creatorStepForStage(base.StageName); !ok || mapped != stepID {
		return nil, ErrCreatorArtifactNotFound
	}
	mode := req.Mode
	var directContent []byte
	message := ""
	switch mode {
	case "direct":
		if strings.TrimSpace(req.DirectContent) == "" || strings.TrimSpace(req.Instruction) != "" {
			return nil, ErrCreatorInvalidRequest
		}
		directContent = []byte(req.DirectContent)
	case "instruction":
		if strings.TrimSpace(req.Instruction) == "" || strings.TrimSpace(req.DirectContent) != "" {
			return nil, ErrCreatorInvalidRequest
		}
		message = strings.TrimSpace(req.Instruction)
	default:
		return nil, ErrCreatorInvalidRequest
	}
	selection, err := normalizeArtifactSelection(req.Selection)
	if err != nil {
		return nil, err
	}
	impact, err := s.stepImpact(ctx, userID, projectID, stepID, base)
	if err != nil {
		return nil, err
	}
	if !sameExactIDs(req.ConfirmedAffectedShotIDs, impact.AffectedShotIDs) {
		return nil, ErrCreatorImpactMismatch
	}
	runID, reviewID, err := s.reviews.ResolveReviewGate(ctx, base, req.RunID, req.ReviewID)
	if err != nil {
		return nil, err
	}
	provenance := map[string]interface{}{"mode": mode, "baseVersion": req.BaseVersion}
	if selection != nil {
		provenance["selection"] = selection
	}
	revised, err := s.revisions.Revise(ctx, artifact.ReviseRequest{
		ArtifactID: base.ID, Message: message, DirectContent: directContent, Provenance: provenance,
	})
	if err != nil {
		return nil, err
	}
	if revised == nil || revised.Artifact == nil {
		return nil, fmt.Errorf("revision produced no artifact")
	}
	if !validCreatorRevisionChild(revised.Artifact, base, projectID, stepID) {
		return nil, ErrCreatorArtifactNotFound
	}
	if err := s.reviews.ReopenWithArtifact(ctx, runID, reviewID, revised.Artifact.ID, userID, "内容已修改，请重新确认"); err != nil {
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
	if mapped, ok := creatorStepForStage(base.StageName); !ok || mapped != stepID {
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
	if !knownCreatorStep(stepID) || version <= 0 {
		return nil, ErrCreatorStepInvalid
	}
	current, err := s.currentArtifactForStep(ctx, projectID, stepID)
	if err != nil {
		return nil, err
	}
	if req.BaseVersion <= 0 || current.Version != req.BaseVersion {
		return nil, ErrCreatorVersionConflict
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
	restored, err := s.revisions.Restore(ctx, artifact.RestoreRequest{ArtifactID: historical.ID, ReviewerID: userID, Reason: strings.TrimSpace(req.Reason)})
	if err != nil {
		return nil, err
	}
	if restored == nil || restored.Artifact == nil {
		return nil, fmt.Errorf("restore produced no artifact")
	}
	if !validCreatorRevisionChild(restored.Artifact, current, projectID, stepID) {
		return nil, ErrCreatorArtifactNotFound
	}
	if err := s.reviews.ReopenWithArtifact(ctx, runID, reviewID, restored.Artifact.ID, userID, "历史版本已恢复，请重新确认"); err != nil {
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
	if err := s.reviews.Confirm(ctx, runID, reviewID, userID, strings.TrimSpace(req.Comment)); err != nil {
		return nil, err
	}
	return s.GetCreationView(ctx, userID, projectID)
}

func (s *CreatorViewService) currentArtifactForStep(ctx context.Context, projectID string, stepID model.CreatorStepID) (*artifact.Artifact, error) {
	items, err := s.artifacts.ListCurrentByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	selection := model.CreatorStep{ID: stepID, State: model.CreatorStepNotStarted}
	byID := make(map[string]*artifact.Artifact, len(items))
	for _, candidate := range items {
		if candidate == nil {
			continue
		}
		mapped, ok := creatorStepForStage(candidate.StageName)
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
	mapped, ok := creatorStepForStage(candidate.StageName)
	return ok && mapped == stepID && candidate.StageName == parent.StageName && candidate.UnitID == parent.UnitID
}

func normalizeArtifactSelection(selection *model.ArtifactSelection) (map[string]interface{}, error) {
	if selection == nil {
		return nil, nil
	}
	kind := strings.ToLower(strings.TrimSpace(selection.Kind))
	switch kind {
	case "rect":
		if selection.X == nil || selection.Y == nil || selection.Width == nil || selection.Height == nil || selection.StartMs != nil || selection.EndMs != nil {
			return nil, ErrCreatorInvalidRequest
		}
		x, y, width, height := *selection.X, *selection.Y, *selection.Width, *selection.Height
		if x < 0 || y < 0 || width <= 0 || height <= 0 || x > 1 || y > 1 || x+width > 1 || y+height > 1 {
			return nil, ErrCreatorInvalidRequest
		}
		return map[string]interface{}{"kind": kind, "x": x, "y": y, "width": width, "height": height}, nil
	case "time":
		if selection.StartMs == nil || selection.EndMs == nil || selection.X != nil || selection.Y != nil || selection.Width != nil || selection.Height != nil {
			return nil, ErrCreatorInvalidRequest
		}
		if *selection.StartMs < 0 || *selection.EndMs <= *selection.StartMs {
			return nil, ErrCreatorInvalidRequest
		}
		return map[string]interface{}{"kind": kind, "startMs": *selection.StartMs, "endMs": *selection.EndMs}, nil
	default:
		return nil, ErrCreatorInvalidRequest
	}
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
		return []string{"view", "review", "revise"}
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
