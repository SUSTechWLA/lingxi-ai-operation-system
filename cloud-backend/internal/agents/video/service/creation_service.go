package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

type CreationProjectStore interface {
	FindByIDForUser(ctx context.Context, userID string, id string) (*model.VideoProject, error)
	UpdateForUser(ctx context.Context, userID string, p *model.VideoProject) error
	CompareAndSwapForUser(ctx context.Context, userID string, p *model.VideoProject, expectedRevision int64) (bool, error)
}

type CreationService struct {
	store      CreationProjectStore
	dispatcher ShotGenerationDispatcher
}

type GenerateSpecRequest struct {
	SourceMessage string `json:"sourceMessage"`
	Platform      string `json:"platform,omitempty"`
}

type RegenerateShotRequest struct {
	BaseVersion        int      `json:"baseVersion"`
	Scope              string   `json:"scope"`
	Locks              []string `json:"locks,omitempty"`
	Instruction        string   `json:"instruction,omitempty"`
	IdempotencyKey     string   `json:"-"`
	requestFingerprint string
}

type RegenerateShotResult struct {
	Shot model.ShotUnit             `json:"shot"`
	Task model.ShotRegenerationTask `json:"task"`
}

type ShotGenerationDispatcher interface {
	EnqueueShotRegeneration(ctx context.Context, userID, projectID string, task model.ShotRegenerationTask) (runID string, err error)
}

type RejectRequest struct {
	Reason string `json:"reason"`
}

func NewCreationService(store CreationProjectStore, dispatchers ...ShotGenerationDispatcher) *CreationService {
	svc := &CreationService{store: store}
	if len(dispatchers) > 0 {
		svc.dispatcher = dispatchers[0]
	}
	return svc
}

func (s *CreationService) GetSpec(ctx context.Context, userID, projectID string) (*model.VideoCreationSpec, error) {
	_, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	if state.Spec == nil {
		return nil, fmt.Errorf("video creation spec not found")
	}
	return state.Spec, nil
}

func (s *CreationService) UpsertSpec(ctx context.Context, userID, projectID string, spec *model.VideoCreationSpec) (*model.VideoCreationSpec, error) {
	if spec == nil {
		spec = &model.VideoCreationSpec{}
	}
	project, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	spec = applySpecDefaults(project, spec)
	state.Spec = spec
	if err := s.save(ctx, userID, project, state); err != nil {
		return nil, err
	}
	return spec, nil
}

func (s *CreationService) GenerateSpec(ctx context.Context, userID, projectID string, req GenerateSpecRequest) (*model.VideoCreationSpec, error) {
	spec := model.NewVideoCreationSpec(projectID, strings.TrimSpace(req.SourceMessage))
	spec.Platform = req.Platform
	spec.Topic = deriveTopic(req.SourceMessage)
	spec.VideoType = deriveVideoType(req.SourceMessage)
	return s.UpsertSpec(ctx, userID, projectID, spec)
}

func (s *CreationService) ApproveSpec(ctx context.Context, userID, projectID string) (*model.VideoCreationSpec, error) {
	project, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	if state.Spec == nil {
		return nil, fmt.Errorf("video creation spec not found")
	}
	state.Spec.Status = model.ReviewStatusApproved
	state.Spec.UpdatedAt = time.Now()
	return state.Spec, s.save(ctx, userID, project, state)
}

func (s *CreationService) RejectSpec(ctx context.Context, userID, projectID string, reason string) (*model.VideoCreationSpec, error) {
	project, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	if state.Spec == nil {
		return nil, fmt.Errorf("video creation spec not found")
	}
	state.Spec.Status = model.ReviewStatusRejected
	state.Spec.UpdatedAt = time.Now()
	return state.Spec, s.save(ctx, userID, project, state)
}

func (s *CreationService) ListShots(ctx context.Context, userID, projectID string) ([]model.ShotUnit, error) {
	_, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	return state.Shots, nil
}

func (s *CreationService) GetShot(ctx context.Context, userID, projectID, shotID string) (*model.ShotUnit, error) {
	_, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	shot, _, ok := findShot(state.Shots, shotID)
	if !ok {
		return nil, fmt.Errorf("shot %s not found", shotID)
	}
	return &shot, nil
}

func (s *CreationService) UpsertShot(ctx context.Context, userID, projectID string, shot *model.ShotUnit) (*model.ShotUnit, error) {
	if shot == nil {
		return nil, fmt.Errorf("shot is required")
	}
	project, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	normalized := normalizeShot(projectID, *shot, len(state.Shots)+1)
	if err := model.ValidateShotDuration(normalized); err != nil {
		return nil, err
	}
	if current, index, ok := findShot(state.Shots, normalized.ID); ok {
		if current.Locked {
			return nil, fmt.Errorf("shot %s is locked", normalized.ID)
		}
		normalized.Version = current.Version + 1
		normalized.CreatedAt = current.CreatedAt
		state.Shots[index] = normalized
	} else {
		state.Shots = append(state.Shots, normalized)
	}
	if err := s.save(ctx, userID, project, state); err != nil {
		return nil, err
	}
	return &normalized, nil
}

func (s *CreationService) GenerateShots(ctx context.Context, userID, projectID string) ([]model.ShotUnit, error) {
	project, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	spec := state.Spec
	if spec == nil {
		spec = applySpecDefaults(project, model.NewVideoCreationSpec(projectID, ""))
		state.Spec = spec
	}
	total := spec.TargetDurationSec
	if total <= 0 {
		total = 60
	}
	prefer := spec.ShotPolicy.PreferDurationSec
	if prefer <= 0 {
		prefer = 6
	}
	count := int(math.Round(float64(total) / float64(prefer)))
	if count < 1 {
		count = 1
	}
	duration := int(math.Ceil(float64(total) / float64(count)))
	for duration >= model.MaxShotDurationExclusiveSec {
		count++
		duration = int(math.Ceil(float64(total) / float64(count)))
	}
	for duration < spec.ShotPolicy.MinDurationSec && count > 1 {
		count--
		duration = int(math.Ceil(float64(total) / float64(count)))
	}
	exactTexts := extractExactTexts(spec.SourceMessage)
	shots := make([]model.ShotUnit, 0, count)
	for i := 0; i < count; i++ {
		shotDuration := duration
		remaining := total - i*duration
		if remaining < duration && remaining >= spec.ShotPolicy.MinDurationSec {
			shotDuration = remaining
		}
		shot := normalizeShot(projectID, model.ShotUnit{
			ID:                fmt.Sprintf("shot-%02d", i+1),
			SequenceIndex:     i + 1,
			Title:             fmt.Sprintf("SHOT %02d", i+1),
			VideoType:         spec.VideoType,
			DurationSec:       shotDuration,
			SceneID:           "scene-main",
			SceneSummary:      "单一主场景内的信息表达",
			SingleScene:       true,
			VisualChangeLevel: model.VisualChangeLow,
			Narration:         spec.Topic,
			MainAction:        "围绕主题做连续口播或视觉解释",
			Camera:            "稳定镜头，低变化",
			TransitionIn:      "承接上一镜信息",
			TransitionOut:     "进入下一镜信息",
		}, i+1)
		if i == 1 && len(exactTexts) > 0 {
			shot.ScreenText = append([]string{}, exactTexts...)
		}
		shots = append(shots, shot)
	}
	state.Shots = shots
	if err := s.save(ctx, userID, project, state); err != nil {
		return nil, err
	}
	return shots, nil
}

func (s *CreationService) ApproveShot(ctx context.Context, userID, projectID, shotID string) (*model.ShotUnit, error) {
	return s.updateShot(ctx, userID, projectID, shotID, func(shot *model.ShotUnit) error {
		shot.ReviewStatus = model.ReviewStatusApproved
		shot.Stale = false
		return nil
	})
}

func (s *CreationService) RejectShot(ctx context.Context, userID, projectID, shotID string, reason string) (*model.ShotUnit, error) {
	return s.updateShot(ctx, userID, projectID, shotID, func(shot *model.ShotUnit) error {
		shot.ReviewStatus = model.ReviewStatusRejected
		shot.LastRejectReason = strings.TrimSpace(reason)
		return nil
	})
}

func (s *CreationService) LockShot(ctx context.Context, userID, projectID, shotID string) (*model.ShotUnit, error) {
	return s.updateShot(ctx, userID, projectID, shotID, func(shot *model.ShotUnit) error {
		shot.Locked = true
		return nil
	})
}

func (s *CreationService) UnlockShot(ctx context.Context, userID, projectID, shotID string) (*model.ShotUnit, error) {
	return s.updateShot(ctx, userID, projectID, shotID, func(shot *model.ShotUnit) error {
		shot.Locked = false
		return nil
	})
}

func (s *CreationService) RegenerateShot(ctx context.Context, userID, projectID, shotID string, req RegenerateShotRequest) (*model.ShotUnit, error) {
	current, err := s.GetShot(ctx, userID, projectID, shotID)
	if err != nil {
		return nil, err
	}
	if !allowedRegenerationScopes[req.Scope] {
		switch req.Scope {
		case "text_layers":
			req.Scope = "overlay"
		default:
			req.Scope = "full_shot"
		}
	}
	if req.IdempotencyKey == "" {
		req.requestFingerprint = shotRegenerationFingerprint(shotID, req)
		req.IdempotencyKey = legacyShotRegenerationKey(projectID, shotID, req)
	}
	if req.BaseVersion == 0 {
		req.BaseVersion = current.Version
	}
	result, err := s.RegenerateShotV2(ctx, userID, projectID, shotID, req)
	if err != nil {
		return nil, err
	}
	return &result.Shot, nil
}

func legacyShotRegenerationKey(projectID, shotID string, req RegenerateShotRequest) string {
	payload := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s\x00%s", projectID, shotID, req.BaseVersion, req.Scope, strings.Join(req.Locks, ","), req.Instruction)
	return fmt.Sprintf("legacy-%x", sha256.Sum256([]byte(payload)))
}

func (s *CreationService) GenerateVisualPlan(ctx context.Context, userID, projectID, shotID string) (*model.VisualPlan, error) {
	shot, err := s.updateShot(ctx, userID, projectID, shotID, func(shot *model.ShotUnit) error {
		shot.VisualPlan = visualPlanForShot(*shot)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &shot.VisualPlan, nil
}

func (s *CreationService) GenerateTextLayers(ctx context.Context, userID, projectID, shotID string) ([]model.TextLayerSpec, error) {
	plan, err := s.GenerateVisualPlan(ctx, userID, projectID, shotID)
	if err != nil {
		return nil, err
	}
	return plan.TextLayers, nil
}

func (s *CreationService) DecideShotRenderStrategy(ctx context.Context, userID, projectID, shotID string) (*model.RenderStrategy, error) {
	shot, err := s.updateShot(ctx, userID, projectID, shotID, func(shot *model.ShotUnit) error {
		if shot.VisualPlan.Canvas.DurationSec == 0 {
			shot.VisualPlan = visualPlanForShot(*shot)
		}
		shot.RenderStrategy = DecideRenderStrategy(*shot, shot.VisualPlan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &shot.RenderStrategy, nil
}

func (s *CreationService) Assemble(ctx context.Context, userID, projectID string) ([]ValidationIssue, error) {
	_, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	return CheckFinalAssembly(state.Shots), nil
}

func (s *CreationService) GeneratePublishPackage(ctx context.Context, userID, projectID string) (map[string]interface{}, error) {
	spec, err := s.GetSpec(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"projectId": projectID,
		"topic":     spec.Topic,
		"status":    model.ReviewStatusPending,
	}, nil
}

func (s *CreationService) updateShot(ctx context.Context, userID, projectID, shotID string, update func(*model.ShotUnit) error) (*model.ShotUnit, error) {
	project, state, err := s.load(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	shot, index, ok := findShot(state.Shots, shotID)
	if !ok {
		return nil, fmt.Errorf("shot %s not found", shotID)
	}
	if err := update(&shot); err != nil {
		return nil, err
	}
	shot.UpdatedAt = time.Now()
	state.Shots[index] = shot
	if err := s.save(ctx, userID, project, state); err != nil {
		return nil, err
	}
	return &shot, nil
}

func (s *CreationService) load(ctx context.Context, userID, projectID string) (*model.VideoProject, model.ShotDrivenState, error) {
	project, err := s.store.FindByIDForUser(ctx, userID, projectID)
	if err != nil {
		return nil, model.ShotDrivenState{}, err
	}
	state, err := DecodeShotDrivenState(project.Config)
	if err != nil {
		return nil, model.ShotDrivenState{}, err
	}
	return project, state, nil
}

func (s *CreationService) save(ctx context.Context, userID string, project *model.VideoProject, state model.ShotDrivenState) error {
	expectedRevision := project.ConfigRevision
	raw, err := EncodeShotDrivenState(project.Config, state)
	if err != nil {
		return err
	}
	project.Config = raw
	swapped, err := s.store.CompareAndSwapForUser(ctx, userID, project, expectedRevision)
	if err != nil {
		return err
	}
	if !swapped {
		return errProjectRevisionConflict
	}
	return nil
}

func applySpecDefaults(project *model.VideoProject, spec *model.VideoCreationSpec) *model.VideoCreationSpec {
	if spec == nil {
		spec = &model.VideoCreationSpec{}
	}
	if spec.ProjectID == "" {
		spec.ProjectID = project.ID
	}
	if spec.ID == "" {
		spec.ID = spec.ProjectID + "-spec"
	}
	if spec.AspectRatio == "" {
		spec.AspectRatio = project.AspectRatio
		if spec.AspectRatio == "" {
			spec.AspectRatio = "16:9"
		}
	}
	if spec.Language == "" {
		spec.Language = project.Language
		if spec.Language == "" {
			spec.Language = "zh-CN"
		}
	}
	if spec.TargetDurationSec == 0 {
		spec.TargetDurationSec = project.TargetDuration
	}
	if spec.TargetDurationSec == 0 {
		spec.TargetDurationSec = 60
	}
	if spec.ReviewMode == "" {
		spec.ReviewMode = model.ReviewModeShotLevel
	}
	if spec.ShotPolicy.MinDurationSec == 0 {
		spec.ShotPolicy = model.DefaultShotPolicy()
	}
	if spec.RenderPreference.DefaultRenderStrategy == "" {
		spec.RenderPreference = model.DefaultRenderPreference()
	}
	if spec.Status == "" {
		spec.Status = model.ReviewStatusPending
	}
	now := time.Now()
	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = now
	}
	spec.UpdatedAt = now
	return spec
}

func normalizeShot(projectID string, shot model.ShotUnit, fallbackIndex int) model.ShotUnit {
	now := time.Now()
	if shot.ProjectID == "" {
		shot.ProjectID = projectID
	}
	if shot.ID == "" {
		shot.ID = fmt.Sprintf("shot-%02d", fallbackIndex)
	}
	if shot.SequenceIndex == 0 {
		shot.SequenceIndex = fallbackIndex
	}
	if shot.DurationSec == 0 {
		shot.DurationSec = 6
	}
	if !shot.SingleScene {
		shot.SingleScene = true
	}
	if shot.VisualChangeLevel == "" {
		shot.VisualChangeLevel = model.VisualChangeLow
	}
	if shot.ReviewStatus == "" {
		shot.ReviewStatus = model.ReviewStatusPending
	}
	if shot.Version == 0 {
		shot.Version = 1
	}
	if shot.CreatedAt.IsZero() {
		shot.CreatedAt = now
	}
	shot.UpdatedAt = now
	return shot
}

func visualPlanForShot(shot model.ShotUnit) model.VisualPlan {
	textLayers := make([]model.TextLayerSpec, 0, len(shot.ScreenText))
	for i, text := range shot.ScreenText {
		textLayers = append(textLayers, model.TextLayerSpec{
			ID:          fmt.Sprintf("%s-text-%02d", shot.ID, i+1),
			Text:        text,
			Language:    "zh-CN",
			Role:        model.TextRoleKeyword,
			Position:    "center",
			FontSize:    56,
			FontWeight:  "700",
			Color:       "#ffffff",
			Background:  "rgba(0,0,0,0.4)",
			StartSec:    float64(i) * float64(shot.DurationSec) / math.Max(float64(len(shot.ScreenText)), 1),
			EndSec:      math.Min(float64(shot.DurationSec), float64(i+1)*float64(shot.DurationSec)/math.Max(float64(len(shot.ScreenText)), 1)),
			Animation:   "fade-in",
			MustBeExact: true,
		})
	}
	return model.VisualPlan{
		Canvas: model.CanvasSpec{
			AspectRatio: "16:9",
			Width:       1920,
			Height:      1080,
			FPS:         30,
			DurationSec: shot.DurationSec,
		},
		Background: model.BackgroundSpec{Description: shot.SceneSummary, RequiresAIGC: shot.MainAction != ""},
		Characters: []model.CharacterVisualSpec{{
			ID:      "speaker",
			Motion:  shot.MainAction,
			Emotion: "focused",
		}},
		TextLayers:    textLayers,
		TransitionIn:  shot.TransitionIn,
		TransitionOut: shot.TransitionOut,
		Style:         model.VisualStyleSpec{Description: "非真人风格化动画的电影质感"},
		Constraints:   model.VisualConstraints{MustAvoid: []string{"准确文字交给 AIGC 生成"}},
	}
}

func findShot(shots []model.ShotUnit, shotID string) (model.ShotUnit, int, bool) {
	for i, shot := range shots {
		if shot.ID == shotID {
			return shot, i, true
		}
	}
	return model.ShotUnit{}, -1, false
}

func deriveTopic(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	for _, marker := range []string{"主题是：", "主题是:", "主题：", "主题:"} {
		if idx := strings.Index(message, marker); idx >= 0 {
			topic := message[idx+len(marker):]
			for _, sep := range []string{"。", "，", ",", "."} {
				if end := strings.Index(topic, sep); end >= 0 {
					return strings.TrimSpace(topic[:end])
				}
			}
			return strings.TrimSpace(topic)
		}
	}
	return message
}

func deriveVideoType(message string) string {
	if strings.Contains(message, "口播") {
		return "口播视频"
	}
	if strings.Contains(message, "新闻") {
		return "新闻解读视频"
	}
	if strings.Contains(message, "产品") {
		return "产品介绍视频"
	}
	return "知识科普视频"
}

func extractExactTexts(message string) []string {
	seen := map[string]bool{}
	result := make([]string, 0)
	re := regexp.MustCompile(`[“"]([^”"]+)[”"]`)
	for _, match := range re.FindAllStringSubmatch(message, -1) {
		if len(match) > 1 && !seen[match[1]] {
			seen[match[1]] = true
			result = append(result, match[1])
		}
	}
	for _, phrase := range []string{"几个表格", "几份文档", "十几条聊天记录"} {
		if strings.Contains(message, phrase) && !seen[phrase] {
			seen[phrase] = true
			result = append(result, phrase)
		}
	}
	return result
}
