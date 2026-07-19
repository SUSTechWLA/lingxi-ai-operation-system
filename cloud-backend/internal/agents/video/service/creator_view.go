package service

import (
	"context"
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
}

func NewCreatorViewService(projects creatorProjectReader, shots creatorShotReader, artifacts creatorArtifactReader) *CreatorViewService {
	return &CreatorViewService{projects: projects, artifacts: artifacts, shots: shots}
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
	return creatorShotReadState{Summary: summary, Tasks: tasks, AssemblyDirty: state.AssemblyDirty}, nil
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
