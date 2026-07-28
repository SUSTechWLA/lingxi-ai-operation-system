package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	videoModel "github.com/tangying-ai/aios-core/internal/agents/video/model"
	"github.com/tangying-ai/aios-core/internal/core/artifact"
	coreModel "github.com/tangying-ai/aios-core/internal/core/model"
)

// CreatorRunAudit is the minimum persisted run identity needed by the creator
// projection. Keeping it independent from agentruntime prevents a domain cycle.
type CreatorRunAudit struct {
	ID     string
	TaskID string
	UserID string
}

type creatorRunAuditLookup func(context.Context, string) (*CreatorRunAudit, error)

type creatorNodeAuditReader interface {
	FindByTaskID(context.Context, string) ([]*coreModel.Node, error)
}

func (s *CreatorViewService) WithProcessAudit(findRun creatorRunAuditLookup, nodes creatorNodeAuditReader) *CreatorViewService {
	if s == nil {
		return s
	}
	s.auditRun, s.auditNodes = findRun, nodes
	return s
}

func (s *CreatorViewService) creatorAuditProjection(
	ctx context.Context,
	project *videoModel.VideoProject,
	currentArtifacts []*artifact.Artifact,
	steps []videoModel.CreatorStep,
) ([]videoModel.CreatorProcessEvent, map[videoModel.CreatorStepID][]videoModel.CreatorArtifactDescriptor, error) {
	stepArtifacts := make(map[videoModel.CreatorStepID][]videoModel.CreatorArtifactDescriptor, len(creatorStepDefinitions))
	stepIndexes := make(map[videoModel.CreatorStepID]int, len(steps))
	for index := range steps {
		stepIndexes[steps[index].ID] = index
		stepArtifacts[steps[index].ID] = []videoModel.CreatorArtifactDescriptor{}
	}

	timeline := make([]videoModel.CreatorProcessEvent, 0)
	if requirementsIndex, ok := stepIndexes[videoModel.CreatorStepRequirements]; ok && creatorProjectHasBrief(project) {
		created := project.CreatedAt
		updated := project.UpdatedAt
		if updated.IsZero() {
			updated = created
		}
		state := videoModel.CreatorStepConfirmed
		timelineState := "confirmed"
		if project.Status == videoModel.StatusDraft {
			state = videoModel.CreatorStepNeedsReview
			timelineState = "generated"
		}
		steps[requirementsIndex].State = state
		steps[requirementsIndex].HasHistory = true
		steps[requirementsIndex].AttemptCount = 1
		steps[requirementsIndex].RunID = project.CurrentRunID
		setCreatorStepStarted(&steps[requirementsIndex], created)
		setCreatorStepUpdated(&steps[requirementsIndex], updated)
		timeline = append(timeline, videoModel.CreatorProcessEvent{
			ID: "project:" + project.ID, StepID: videoModel.CreatorStepRequirements, Attempt: 1,
			State: timelineState, SourceType: "project", SourceID: project.ID,
			Title: "创作需求已保存", Summary: "项目主题和创作要求已记录",
			StartedAt: &created, CompletedAt: &updated, ArtifactIDs: []string{},
		})
	}
	seenArtifacts := make(map[string]bool)
	for _, current := range currentArtifacts {
		if current == nil || current.ProjectID != project.ID {
			continue
		}
		stepID, ok := creatorStepForArtifact(current)
		if !ok {
			continue
		}
		history := []*artifact.Artifact{current}
		if s.history != nil {
			items, err := s.history.GetHistory(ctx, project.ID, current.StageName, current.UnitID)
			if err != nil {
				return nil, nil, err
			}
			history = append(history, items...)
		}
		for _, item := range history {
			if item == nil || item.ProjectID != project.ID || item.StageName != current.StageName || item.UnitID != current.UnitID || seenArtifacts[item.ID] {
				continue
			}
			seenArtifacts[item.ID] = true
			isCurrent := item.ID == current.ID
			isStale := !isCurrent || artifactIsStale(item)
			artifactType, generationKind, relatedShotID := creatorArtifactClassificationHints(item)
			descriptor := videoModel.CreatorArtifactDescriptor{
				ArtifactID: item.ID, StepID: stepID, Name: item.Name, Kind: string(item.Kind), MimeType: item.MimeType,
				ArtifactType: artifactType, GenerationKind: generationKind, RelatedShotID: relatedShotID,
				Version: item.Version, Attempt: maxCreatorAttempt(item.Version), IsCurrent: isCurrent, IsStale: isStale,
				SizeBytes: item.SizeBytes, CreatedAt: item.CreatedAt,
			}
			stepArtifacts[stepID] = append(stepArtifacts[stepID], descriptor)
			created := item.CreatedAt
			timeline = append(timeline, videoModel.CreatorProcessEvent{
				ID: "artifact:" + item.ID, StepID: stepID, Attempt: descriptor.Attempt, State: artifactTimelineState(item, isCurrent),
				SourceType: "artifact", SourceID: item.ID, Title: creatorArtifactTitle(item),
				Summary: creatorArtifactSummary(item, isCurrent), StartedAt: &created, CompletedAt: &created,
				ArtifactIDs: []string{item.ID},
			})
		}
	}

	for stepID, items := range stepArtifacts {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].IsCurrent != items[j].IsCurrent {
				return items[i].IsCurrent
			}
			if items[i].Version != items[j].Version {
				return items[i].Version > items[j].Version
			}
			return items[i].ArtifactID > items[j].ArtifactID
		})
		stepArtifacts[stepID] = items
		if len(items) == 0 {
			continue
		}
		index := stepIndexes[stepID]
		steps[index].HasHistory = true
		steps[index].ArtifactCount = len(items)
		for _, item := range items {
			steps[index].AttemptCount = maxInt(steps[index].AttemptCount, item.Attempt)
			created := item.CreatedAt
			setCreatorStepStarted(&steps[index], created)
			setCreatorStepUpdated(&steps[index], created)
			if item.IsCurrent && item.IsStale {
				steps[index].IsStale = true
			}
		}
	}

	if s.auditRun != nil && s.auditNodes != nil && strings.TrimSpace(project.CurrentRunID) != "" {
		run, err := s.auditRun(ctx, project.CurrentRunID)
		if err != nil {
			return nil, nil, err
		}
		if run != nil && run.ID == project.CurrentRunID && run.UserID == project.UserID && run.TaskID != "" {
			nodes, err := s.auditNodes.FindByTaskID(ctx, run.TaskID)
			if err != nil {
				return nil, nil, err
			}
			sort.SliceStable(nodes, func(i, j int) bool {
				if nodes[i] == nil || nodes[j] == nil {
					return nodes[j] == nil
				}
				return creatorNodeTime(nodes[i]).Before(creatorNodeTime(nodes[j]))
			})
			for _, node := range nodes {
				event, stepID, state, ok := creatorEventFromNode(project, run, node)
				if !ok {
					continue
				}
				index := stepIndexes[stepID]
				steps[index].HasHistory = true
				steps[index].AttemptCount = maxInt(steps[index].AttemptCount, event.Attempt)
				steps[index].RunID = run.ID
				if event.StartedAt != nil {
					setCreatorStepStarted(&steps[index], *event.StartedAt)
					setCreatorStepUpdated(&steps[index], *event.StartedAt)
				}
				if event.CompletedAt != nil {
					setCreatorStepUpdated(&steps[index], *event.CompletedAt)
				}
				if state != videoModel.CreatorStepNotStarted {
					isApprovedReview := (node.Type == coreModel.NodeTypeControl || node.Type == coreModel.NodeTypeReviewGate) && state == videoModel.CreatorStepConfirmed
					if isApprovedReview {
						steps[index].State = state
					} else {
						steps[index].State = mergeCreatorState(steps[index].State, state)
					}
				}
				if node.Type == coreModel.NodeTypeControl || node.Type == coreModel.NodeTypeReviewGate {
					steps[index].ReviewID = node.ID
				}
				timeline = append(timeline, event)
			}
		}
	}

	sort.SliceStable(timeline, func(i, j int) bool {
		left, right := creatorEventTime(timeline[i]), creatorEventTime(timeline[j])
		if !left.Equal(right) {
			return left.Before(right)
		}
		return timeline[i].ID < timeline[j].ID
	})
	return timeline, stepArtifacts, nil
}

func creatorArtifactClassificationHints(item *artifact.Artifact) (artifactType, generationKind, relatedShotID string) {
	if item == nil || item.Metadata == nil {
		return "", "", ""
	}
	classificationString := func(key string) string {
		value, ok := item.Metadata[key].(string)
		if !ok {
			return ""
		}
		value = strings.TrimSpace(value)
		runes := []rune(value)
		if len(runes) > 120 {
			return string(runes[:120])
		}
		return value
	}
	artifactType = classificationString("artifactType")
	generationKind = classificationString("generationKind")
	relatedShotID = classificationString("relatedShotId")
	if relatedShotID == "" {
		relatedShotID = classificationString("shotId")
	}
	return artifactType, generationKind, relatedShotID
}

func creatorProjectHasBrief(project *videoModel.VideoProject) bool {
	if project == nil || strings.TrimSpace(project.ID) == "" {
		return false
	}
	return !project.CreatedAt.IsZero() || strings.TrimSpace(project.Name) != "" || strings.TrimSpace(project.Description) != ""
}

func creatorEventFromNode(
	project *videoModel.VideoProject,
	run *CreatorRunAudit,
	node *coreModel.Node,
) (videoModel.CreatorProcessEvent, videoModel.CreatorStepID, videoModel.CreatorStepState, bool) {
	if node == nil || node.TaskID != run.TaskID || node.Status == coreModel.NodeSkipped {
		return videoModel.CreatorProcessEvent{}, "", videoModel.CreatorStepNotStarted, false
	}
	stepID, ok := creatorStepForNode(node)
	if !ok {
		return videoModel.CreatorProcessEvent{}, "", videoModel.CreatorStepNotStarted, false
	}
	state, timelineState, summary, visible := creatorNodePresentation(project.Status, node)
	if !visible {
		return videoModel.CreatorProcessEvent{}, "", videoModel.CreatorStepNotStarted, false
	}
	started := node.StartedAt
	if started == nil && !node.CreatedAt.IsZero() {
		created := node.CreatedAt
		started = &created
	}
	return videoModel.CreatorProcessEvent{
		ID: "node:" + node.ID, StepID: stepID, Attempt: maxCreatorAttempt(node.RetryCount + 1), State: timelineState,
		SourceType: creatorNodeSourceType(node), SourceID: node.ID, Title: creatorSafeTitle(node.Name, stepID), Summary: summary,
		StartedAt: started, CompletedAt: node.CompletedAt, ArtifactIDs: creatorNodeArtifactIDs(node),
	}, stepID, state, true
}

func creatorStepForNode(node *coreModel.Node) (videoModel.CreatorStepID, bool) {
	if node == nil {
		return "", false
	}
	for _, key := range []string{"stage", "stepId", "stageName", "creatorStepId"} {
		if node.Input == nil {
			break
		}
		if value, ok := node.Input[key].(string); ok {
			if step, found := creatorStepForStage(value); found {
				return step, true
			}
		}
	}
	return "", false
}

func creatorNodePresentation(projectStatus videoModel.ProjectStatus, node *coreModel.Node) (videoModel.CreatorStepState, string, string, bool) {
	isReview := node.Type == coreModel.NodeTypeControl || node.Type == coreModel.NodeTypeReviewGate
	switch node.Status {
	case coreModel.NodeSuccess:
		if isReview || projectStatus == videoModel.StatusCompleted || projectStatus == videoModel.StatusArchived {
			return videoModel.CreatorStepConfirmed, "confirmed", "已完成并保留为可审阅记录", true
		}
		return videoModel.CreatorStepNeedsReview, "generated", "内容已生成，等待审阅", true
	case coreModel.NodeReady:
		if isReview {
			return videoModel.CreatorStepNeedsReview, "needs_review", "等待用户审阅", true
		}
		return videoModel.CreatorStepGenerating, "started", "已进入执行队列", true
	case coreModel.NodeRunning, coreModel.NodeWaitingLocal, coreModel.NodeLocalClaimed, coreModel.NodeLocalRunning, coreModel.NodeLocalCompleted, coreModel.NodeRetrying:
		return videoModel.CreatorStepGenerating, "started", "正在生成", true
	case coreModel.NodeFailed, coreModel.NodeLocalFailed, coreModel.NodeHeartbeatTimeout, coreModel.NodeCancelled:
		return videoModel.CreatorStepFailed, "failed", "本次执行未完成，可查看并重试", true
	case coreModel.NodeCreated:
		if node.StartedAt != nil || strings.TrimSpace(node.IdempotencyKey) != "" {
			return videoModel.CreatorStepGenerating, "started", "准备生成", true
		}
	}
	return videoModel.CreatorStepNotStarted, "", "", false
}

func creatorNodeSourceType(node *coreModel.Node) string {
	if node.Type == coreModel.NodeTypeControl || node.Type == coreModel.NodeTypeReviewGate {
		return "review"
	}
	return "agent_node"
}

func creatorNodeArtifactIDs(node *coreModel.Node) []string {
	if node == nil || node.Input == nil {
		return []string{}
	}
	if artifactID, ok := node.Input["artifactId"].(string); ok && strings.TrimSpace(artifactID) != "" {
		return []string{strings.TrimSpace(artifactID)}
	}
	return []string{}
}

func creatorSafeTitle(name string, stepID videoModel.CreatorStepID) string {
	name = strings.Join(strings.Fields(strings.TrimSpace(name)), " ")
	if name == "" {
		name = creatorStepLabelForAudit(stepID)
	}
	const maxRunes = 120
	runes := []rune(name)
	if len(runes) > maxRunes {
		name = string(runes[:maxRunes]) + "…"
	}
	return name
}

func creatorStepLabelForAudit(stepID videoModel.CreatorStepID) string {
	for _, definition := range creatorStepDefinitions {
		if definition.id == stepID {
			return definition.label
		}
	}
	return "Creation step"
}

func creatorArtifactTitle(item *artifact.Artifact) string {
	if strings.TrimSpace(item.Name) != "" {
		return creatorSafeTitle(item.Name, "")
	}
	return fmt.Sprintf("%s artifact", item.Kind)
}

func creatorArtifactSummary(item *artifact.Artifact, isCurrent bool) string {
	if !isCurrent {
		return fmt.Sprintf("历史版本 v%d", item.Version)
	}
	if artifactIsStale(item) {
		return fmt.Sprintf("当前版本 v%d，需要根据上游更新", item.Version)
	}
	return fmt.Sprintf("当前版本 v%d", item.Version)
}

func artifactTimelineState(item *artifact.Artifact, isCurrent bool) string {
	if !isCurrent || artifactIsStale(item) {
		return "stale"
	}
	if stateForArtifact(item) == videoModel.CreatorStepFailed {
		return "failed"
	}
	if stateForArtifact(item) == videoModel.CreatorStepNeedsReview {
		return "needs_review"
	}
	if stateForArtifact(item) == videoModel.CreatorStepConfirmed {
		return "confirmed"
	}
	return "generated"
}

func artifactIsStale(item *artifact.Artifact) bool {
	if item == nil {
		return false
	}
	switch normalizeCreatorStage(item.Status) {
	case "stale", "rejected", "invalidated", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func creatorNodeTime(node *coreModel.Node) time.Time {
	if node == nil {
		return time.Time{}
	}
	if node.StartedAt != nil {
		return *node.StartedAt
	}
	return node.CreatedAt
}

func creatorEventTime(event videoModel.CreatorProcessEvent) time.Time {
	if event.StartedAt != nil {
		return *event.StartedAt
	}
	if event.CompletedAt != nil {
		return *event.CompletedAt
	}
	return time.Time{}
}

func setCreatorStepStarted(step *videoModel.CreatorStep, candidate time.Time) {
	if candidate.IsZero() {
		return
	}
	if step.StartedAt == nil || candidate.Before(*step.StartedAt) {
		value := candidate
		step.StartedAt = &value
	}
}

func setCreatorStepUpdated(step *videoModel.CreatorStep, candidate time.Time) {
	if candidate.IsZero() {
		return
	}
	if step.UpdatedAt == nil || candidate.After(*step.UpdatedAt) {
		value := candidate
		step.UpdatedAt = &value
	}
}

func maxCreatorAttempt(value int) int {
	if value < 1 {
		return 1
	}
	return value
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
