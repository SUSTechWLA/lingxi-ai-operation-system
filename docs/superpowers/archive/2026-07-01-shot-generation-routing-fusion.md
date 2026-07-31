# Shot Generation Routing and Asset Fusion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first production-grade shot generation routing layer so each video shot declares whether it is AIGC, HyperFrames, hybrid, user-asset, or preview-driven, and so approved media can be fused into the final client-viewable MP4.

**Architecture:** Add a deterministic planning core in the video service/model layer, expose it through builtin video creation tools, insert the planner into the dynamic agent DAG before prompt and HyperFrames generation, then teach the local HyperFrames project generator to consume shot asset packages as timed image/video media. Keep the current fallback behavior intact: missing providers create reviewable external generation requests or visible preview placeholders instead of blocking the full run.

**Tech Stack:** Go backend (`cloud-backend`, `local-backend`), TypeScript frontend (`frontend`), HyperFrames HTML/GSAP local render service, existing artifact materializer and local artifact APIs.

---

## File Structure

- Modify `cloud-backend/internal/agents/video/model/creation.go`
  - Add `ShotGenerationPlan`, `ShotAssetNeed`, `FusionPlan`, and mode/source constants.
- Create `cloud-backend/internal/agents/video/service/generation_plan.go`
  - Convert `ShotUnit` + `VisualPlan` + `RenderPreference` + provider capabilities into a concrete `ShotGenerationPlan`.
- Test `cloud-backend/internal/agents/video/service/generation_plan_test.go`
  - Cover exact text, AIGC scene, hybrid, user asset, and missing provider fallback.
- Modify `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools.go`
  - Add `shot_generation_planner` manifest, executor, prompt integration, artifact output, and deterministic fallback package shaping.
- Test `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools_test.go`
  - Verify planner output and external request behavior.
- Modify `cloud-backend/internal/core/agentruntime/plan_compiler.go`
  - Insert `shot_generation_planner` after `shot_splitter` and pass `shotGenerationPlans` into downstream video prompt and HyperFrames project generation.
- Test `cloud-backend/internal/core/agentruntime/plan_compiler_test.go`
  - Verify DAG step order, references, and PlanGuard validity.
- Modify `local-backend/internal/localtool/hyperframes_project.go`
  - Parse `shotAssetPackages` and render timed `<video>` / `<img>` media layers plus exact text overlays.
- Test `local-backend/internal/localtool/hyperframes_project_test.go`
  - Verify generated HTML includes media clips, overlay layers, visible missing-media placeholders, and still satisfies the HyperFrames composition contract.
- Modify `frontend/src/pages/directorStudioLogic.ts` and `frontend/src/pages/DirectorStudioPage.tsx`
  - Surface per-shot strategy labels, reasons, and hybrid base/overlay slots.
- Test by TypeScript production build: `cd frontend && npm run build`.

---

### Task 1: Model And Service Generation Plan

**Files:**
- Modify: `cloud-backend/internal/agents/video/model/creation.go`
- Create: `cloud-backend/internal/agents/video/service/generation_plan.go`
- Test: `cloud-backend/internal/agents/video/service/generation_plan_test.go`

- [ ] **Step 1: Write failing generation plan tests**

Add `cloud-backend/internal/agents/video/service/generation_plan_test.go`:

```go
package service

import (
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestBuildShotGenerationPlanExactTextUsesHTMLOnly(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_01", DurationSec: 6, ScreenText: []string{"增长 42%"}}
	plan := model.VisualPlan{
		TextLayers: []model.TextLayerSpec{{ID: "txt-1", Text: "增长 42%", Role: model.TextRoleDataText, MustBeExact: true}},
	}

	got := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})

	if got.Mode != model.GenerationModeHTMLOnly {
		t.Fatalf("mode = %q, want %q; plan=%+v", got.Mode, model.GenerationModeHTMLOnly, got)
	}
	if got.PrimaryTool != "hyperframes_renderer" {
		t.Fatalf("primary tool = %q", got.PrimaryTool)
	}
	if len(got.RequiredAssets) != 1 || got.RequiredAssets[0].Source != model.AssetSourceHyperFrames {
		t.Fatalf("required assets = %+v", got.RequiredAssets)
	}
	if got.FusionPlan.Assembler != "hyperframes" {
		t.Fatalf("fusion assembler = %q", got.FusionPlan.Assembler)
	}
}

func TestBuildShotGenerationPlanMotionSceneUsesAIGCVideo(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_02", DurationSec: 8, MainAction: "角色穿过办公室并回头"}
	plan := model.VisualPlan{
		Background: model.BackgroundSpec{Description: "非真人风格化办公室", RequiresAIGC: true},
		Characters: []model.CharacterVisualSpec{{ID: "host", Motion: "walks through office", Emotion: "hesitant"}},
		CameraPlan: model.CameraPlan{Movement: "slow push-in", RequiresAIGC: true},
	}

	got := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})

	if got.Mode != model.GenerationModeAIGCVideo {
		t.Fatalf("mode = %q, want %q; plan=%+v", got.Mode, model.GenerationModeAIGCVideo, got)
	}
	if got.PrimaryTool != "text_image_to_video_generator" {
		t.Fatalf("primary tool = %q", got.PrimaryTool)
	}
}

func TestBuildShotGenerationPlanExactTextWithAIGCSceneUsesHybrid(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_03", DurationSec: 7, ScreenText: []string{"几个表格"}}
	plan := model.VisualPlan{
		Background: model.BackgroundSpec{Description: "文件飞散的办公室", RequiresAIGC: true},
		Characters: []model.CharacterVisualSpec{{ID: "host", Motion: "raises both hands"}},
		TextLayers: []model.TextLayerSpec{{ID: "txt-1", Text: "几个表格", Role: model.TextRoleKeyword, MustBeExact: true}},
	}

	got := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})

	if got.Mode != model.GenerationModeHybridAIGCBGHTMLOverlay {
		t.Fatalf("mode = %q, want hybrid; plan=%+v", got.Mode, got)
	}
	if !containsString(got.SecondaryTools, "hyperframes_renderer") {
		t.Fatalf("secondary tools = %+v", got.SecondaryTools)
	}
	if got.FusionPlan.BaseLayer.Kind != "video" || len(got.FusionPlan.OverlayLayers) == 0 {
		t.Fatalf("fusion plan = %+v", got.FusionPlan)
	}
}

func TestBuildShotGenerationPlanLogoRoutesToUserAsset(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_04", DurationSec: 5, SceneSummary: "展示客户上传 logo 和产品截图"}
	plan := model.VisualPlan{
		Props: []model.PropVisualSpec{{ID: "brand-logo", Description: "客户上传 logo"}},
	}

	got := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: true, HTMLAvailable: true})

	if got.Mode != model.GenerationModeExternalOrUserAsset {
		t.Fatalf("mode = %q, want user asset; plan=%+v", got.Mode, got)
	}
	if len(got.RequiredAssets) == 0 || got.RequiredAssets[0].Source != model.AssetSourceUserUpload {
		t.Fatalf("required assets = %+v", got.RequiredAssets)
	}
}

func TestBuildShotGenerationPlanMissingVideoProviderCreatesExternalNeed(t *testing.T) {
	shot := model.ShotUnit{ID: "SHOT_05", DurationSec: 6, MainAction: "人物跑过街口"}
	plan := model.VisualPlan{
		Background: model.BackgroundSpec{Description: "街口", RequiresAIGC: true},
		Characters: []model.CharacterVisualSpec{{ID: "runner", Motion: "runs through frame"}},
	}

	got := BuildShotGenerationPlan(shot, plan, model.DefaultRenderPreference(), RenderCapabilities{AIGCAvailable: false, HTMLAvailable: true})

	if got.Mode != model.GenerationModePlaceholderPreview {
		t.Fatalf("mode = %q, want placeholder preview; plan=%+v", got.Mode, got)
	}
	if len(got.RequiredAssets) == 0 || got.RequiredAssets[0].Source != model.AssetSourceExternalGeneration {
		t.Fatalf("required assets = %+v", got.RequiredAssets)
	}
	if got.FallbackPlan.Mode != model.GenerationModePlaceholderPreview {
		t.Fatalf("fallback plan = %+v", got.FallbackPlan)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run the new test to verify it fails**

Run:

```bash
cd cloud-backend
go test ./internal/agents/video/service -run 'TestBuildShotGenerationPlan' -count=1
```

Expected: FAIL with undefined symbols such as `BuildShotGenerationPlan`, `GenerationModeHTMLOnly`, or `ShotGenerationPlan`.

- [ ] **Step 3: Add model types and constants**

In `cloud-backend/internal/agents/video/model/creation.go`, add constants next to existing render mode constants:

```go
const (
	GenerationModeHTMLOnly                = "html_only"
	GenerationModeAIGCVideo               = "aigc_video"
	GenerationModeAIGCImageThenHyperFrames = "aigc_image_then_hyperframes"
	GenerationModeHybridAIGCBGHTMLOverlay = "hybrid_aigc_bg_html_overlay"
	GenerationModeExternalOrUserAsset     = "external_or_user_asset"
	GenerationModePlaceholderPreview      = "placeholder_preview"

	AssetSourceAIGCImage          = "aigc_image"
	AssetSourceAIGCVideo          = "aigc_video"
	AssetSourceHyperFrames        = "hyperframes"
	AssetSourceUserUpload         = "user_upload"
	AssetSourceExternalGeneration = "external_generation"
	AssetSourceOpenAsset          = "open_asset"
	AssetSourcePlaceholder        = "placeholder"
)
```

Add structs after `RenderStrategy`:

```go
type ShotGenerationPlan struct {
	ShotID          string          `json:"shotId"`
	Mode            string          `json:"mode"`
	PrimaryTool     string          `json:"primaryTool,omitempty"`
	SecondaryTools  []string        `json:"secondaryTools,omitempty"`
	Reason          string          `json:"reason,omitempty"`
	Confidence      float64         `json:"confidence,omitempty"`
	RiskLevel       string          `json:"riskLevel,omitempty"`
	RequiredAssets  []ShotAssetNeed `json:"requiredAssets,omitempty"`
	RenderInputs    map[string]interface{} `json:"renderInputs,omitempty"`
	FusionPlan      FusionPlan      `json:"fusionPlan,omitempty"`
	FallbackPlan    *ShotGenerationFallback `json:"fallbackPlan,omitempty"`
	ReviewFocus     []string        `json:"reviewFocus,omitempty"`
}

type ShotGenerationFallback struct {
	Mode   string `json:"mode"`
	Reason string `json:"reason,omitempty"`
}

type ShotAssetNeed struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	Role           string   `json:"role"`
	Source         string   `json:"source"`
	Required       bool     `json:"required"`
	ApprovalStatus string   `json:"approvalStatus,omitempty"`
	StorageRef     string   `json:"storageRef,omitempty"`
	RelatedShotID  string   `json:"relatedShotId,omitempty"`
	Locks          []string `json:"locks,omitempty"`
}

type FusionPlan struct {
	ShotID             string            `json:"shotId,omitempty"`
	BaseLayer          FusionLayer       `json:"baseLayer,omitempty"`
	OverlayLayers      []FusionLayer     `json:"overlayLayers,omitempty"`
	TimedMedia         []TimedMediaLayer `json:"timedMedia,omitempty"`
	Assembler          string            `json:"assembler,omitempty"`
	OutputArtifactKind string            `json:"outputArtifactKind,omitempty"`
}

type FusionLayer struct {
	ID          string  `json:"id,omitempty"`
	Kind        string  `json:"kind,omitempty"`
	Role        string  `json:"role,omitempty"`
	StorageRef  string  `json:"storageRef,omitempty"`
	StartSec    float64 `json:"startSec,omitempty"`
	DurationSec float64 `json:"durationSec,omitempty"`
}

type TimedMediaLayer struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	Role        string  `json:"role,omitempty"`
	StorageRef  string  `json:"storageRef,omitempty"`
	StartSec    float64 `json:"startSec"`
	DurationSec float64 `json:"durationSec"`
	TrackIndex  int     `json:"trackIndex"`
	Fit         string  `json:"fit,omitempty"`
	Opacity     float64 `json:"opacity,omitempty"`
}
```

- [ ] **Step 4: Implement `BuildShotGenerationPlan`**

Create `cloud-backend/internal/agents/video/service/generation_plan.go`:

```go
package service

import (
	"fmt"
	"strings"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func BuildShotGenerationPlan(shot model.ShotUnit, visual model.VisualPlan, pref model.RenderPreference, caps RenderCapabilities) model.ShotGenerationPlan {
	if visual.Canvas.DurationSec == 0 {
		visual.Canvas.DurationSec = shot.DurationSec
	}
	htmlScore := scoreHTMLNeed(shot, visual, pref)
	aigcScore := scoreAIGCNeed(shot, visual, pref)
	userAssetScore := scoreUserAssetNeed(shot, visual)

	switch {
	case userAssetScore >= 2:
		return userAssetPlan(shot, visual)
	case htmlScore >= 2 && aigcScore >= 2 && caps.AIGCAvailable && caps.HTMLAvailable:
		return hybridPlan(shot, visual)
	case htmlScore >= 2 && caps.HTMLAvailable:
		return htmlOnlyPlan(shot, visual)
	case aigcScore >= 2 && caps.AIGCAvailable:
		return aigcVideoPlan(shot, visual)
	case aigcScore >= 2 && !caps.AIGCAvailable:
		return externalAIGCPreviewPlan(shot, visual)
	case htmlScore > 0 && caps.HTMLAvailable:
		return htmlOnlyPlan(shot, visual)
	default:
		return placeholderPlan(shot, "画面信息不足，先生成可审核低成本预览。")
	}
}

func scoreHTMLNeed(shot model.ShotUnit, visual model.VisualPlan, pref model.RenderPreference) int {
	score := 0
	if pref.PreferHTMLForText && len(shot.ScreenText) > 0 {
		score += 2
	}
	for _, layer := range visual.TextLayers {
		if layer.MustBeExact || exactTextRole(layer.Role) {
			score += 2
		}
	}
	if pref.PreferHTMLForCharts && len(visual.DataVisuals) > 0 {
		score += 2
	}
	if pref.PreferHTMLForUI && len(visual.UILayers) > 0 {
		score += 2
	}
	return score
}

func scoreAIGCNeed(shot model.ShotUnit, visual model.VisualPlan, pref model.RenderPreference) int {
	score := 0
	if visual.Background.RequiresAIGC && pref.PreferAIGCForScene {
		score += 2
	}
	if visual.MotionPlan.RequiresAIGC || visual.CameraPlan.RequiresAIGC {
		score += 2
	}
	if strings.TrimSpace(visual.CameraPlan.Movement) != "" {
		score++
	}
	if strings.TrimSpace(shot.MainAction) != "" {
		score++
	}
	for _, character := range visual.Characters {
		if character.RequiresAIGC {
			score += 2
		}
		if pref.PreferAIGCForPeople && (strings.TrimSpace(character.Motion) != "" || strings.TrimSpace(character.Emotion) != "") {
			score++
		}
	}
	return score
}

func scoreUserAssetNeed(shot model.ShotUnit, visual model.VisualPlan) int {
	haystack := strings.ToLower(strings.Join([]string{shot.SceneSummary, shot.MainAction, strings.Join(shot.ScreenText, " ")}, " "))
	for _, prop := range visual.Props {
		haystack += " " + strings.ToLower(prop.ID+" "+prop.Description)
	}
	score := 0
	for _, marker := range []string{"logo", "标志", "上传", "用户素材", "本地素材", "截图", "screenshot", "新闻图", "产品图"} {
		if strings.Contains(haystack, marker) {
			score++
		}
	}
	return score
}

func htmlOnlyPlan(shot model.ShotUnit, visual model.VisualPlan) model.ShotGenerationPlan {
	return model.ShotGenerationPlan{
		ShotID:      shot.ID,
		Mode:        model.GenerationModeHTMLOnly,
		PrimaryTool: "hyperframes_renderer",
		Reason:      "镜头以准确文字、UI、图表或低变化信息结构为主，适合 HyperFrames 确定性渲染。",
		Confidence:  0.86,
		RiskLevel:   "low",
		RequiredAssets: []model.ShotAssetNeed{{
			ID:            shot.ID + "-html-overlay",
			Kind:          "html_overlay",
			Role:          "overlay",
			Source:        model.AssetSourceHyperFrames,
			Required:      true,
			RelatedShotID: shot.ID,
			Locks:         []string{"exact_text", "timing"},
		}},
		FusionPlan: model.FusionPlan{
			ShotID:             shot.ID,
			Assembler:          "hyperframes",
			OutputArtifactKind: "HYPERFRAMES_SHOT",
			OverlayLayers:      textFusionLayers(visual),
		},
		ReviewFocus: []string{"文字是否准确", "字幕和卡片是否可读", "节奏是否匹配口播"},
	}
}

func aigcVideoPlan(shot model.ShotUnit, visual model.VisualPlan) model.ShotGenerationPlan {
	return model.ShotGenerationPlan{
		ShotID:      shot.ID,
		Mode:        model.GenerationModeAIGCVideo,
		PrimaryTool: "text_image_to_video_generator",
		Reason:      "镜头主要依赖人物、场景、自然运动或电影感运镜，适合 AIGC 视频生成。",
		Confidence:  0.8,
		RiskLevel:   "medium",
		RequiredAssets: []model.ShotAssetNeed{{
			ID:            shot.ID + "-aigc-video",
			Kind:          "video",
			Role:          "background_video",
			Source:        model.AssetSourceAIGCVideo,
			Required:      true,
			RelatedShotID: shot.ID,
			Locks:         []string{"style", "motion", "scene"},
		}},
		FusionPlan: model.FusionPlan{
			ShotID:             shot.ID,
			Assembler:          "ffmpeg",
			OutputArtifactKind: "SHOT_VIDEO_CLIP",
			BaseLayer:          model.FusionLayer{ID: shot.ID + "-aigc-video", Kind: "video", Role: "background_video", DurationSec: float64(shot.DurationSec)},
		},
		ReviewFocus: []string{"人物和场景一致性", "运镜是否稳定", "动作是否自然", "是否适合拼接"},
	}
}

func hybridPlan(shot model.ShotUnit, visual model.VisualPlan) model.ShotGenerationPlan {
	return model.ShotGenerationPlan{
		ShotID:         shot.ID,
		Mode:           model.GenerationModeHybridAIGCBGHTMLOverlay,
		PrimaryTool:    "text_image_to_video_generator",
		SecondaryTools: []string{"hyperframes_renderer", "ffmpeg_compositor"},
		Reason:         "AIGC 负责动态背景/人物/场景，HyperFrames 负责准确文字、字幕或 UI 覆盖层。",
		Confidence:     0.88,
		RiskLevel:      "medium",
		RequiredAssets: []model.ShotAssetNeed{
			{ID: shot.ID + "-aigc-base", Kind: "video", Role: "background_video", Source: model.AssetSourceAIGCVideo, Required: true, RelatedShotID: shot.ID, Locks: []string{"style", "scene", "motion"}},
			{ID: shot.ID + "-html-overlay", Kind: "html_overlay", Role: "overlay", Source: model.AssetSourceHyperFrames, Required: true, RelatedShotID: shot.ID, Locks: []string{"exact_text", "timing"}},
		},
		FusionPlan: model.FusionPlan{
			ShotID:             shot.ID,
			Assembler:          "hybrid",
			OutputArtifactKind: "COMPOSITED_SHOT_VIDEO",
			BaseLayer:          model.FusionLayer{ID: shot.ID + "-aigc-base", Kind: "video", Role: "background_video", DurationSec: float64(shot.DurationSec)},
			OverlayLayers:      textFusionLayers(visual),
		},
		ReviewFocus: []string{"AIGC 背景是否可用", "精确文字是否由 overlay 承担", "合成后是否遮挡主体"},
	}
}

func userAssetPlan(shot model.ShotUnit, visual model.VisualPlan) model.ShotGenerationPlan {
	return model.ShotGenerationPlan{
		ShotID:      shot.ID,
		Mode:        model.GenerationModeExternalOrUserAsset,
		PrimaryTool: "manual_upload",
		Reason:      "镜头依赖 logo、截图、产品图、新闻图或用户本地素材，需要上传或外部授权素材。",
		Confidence:  0.78,
		RiskLevel:   "medium",
		RequiredAssets: []model.ShotAssetNeed{{
			ID:            shot.ID + "-user-media",
			Kind:          "user_asset",
			Role:          "uploaded_media",
			Source:        model.AssetSourceUserUpload,
			Required:      true,
			RelatedShotID: shot.ID,
			Locks:         []string{"asset_identity", "license_boundary"},
		}},
		FusionPlan: model.FusionPlan{ShotID: shot.ID, Assembler: "hyperframes", OutputArtifactKind: "HYPERFRAMES_SHOT"},
		ReviewFocus: []string{"素材是否已上传", "素材授权是否明确", "画面裁切是否正确"},
	}
}

func externalAIGCPreviewPlan(shot model.ShotUnit, visual model.VisualPlan) model.ShotGenerationPlan {
	plan := placeholderPlan(shot, "视频 Provider 未配置，创建外部生成请求并先用 HyperFrames 占位预览。")
	plan.RequiredAssets = []model.ShotAssetNeed{{
		ID:            shot.ID + "-external-video",
		Kind:          "video",
		Role:          "background_video",
		Source:        model.AssetSourceExternalGeneration,
		Required:      true,
		RelatedShotID: shot.ID,
		Locks:         []string{"style", "motion"},
	}}
	return plan
}

func placeholderPlan(shot model.ShotUnit, reason string) model.ShotGenerationPlan {
	return model.ShotGenerationPlan{
		ShotID:      shot.ID,
		Mode:        model.GenerationModePlaceholderPreview,
		PrimaryTool: "hyperframes_renderer",
		Reason:      reason,
		Confidence:  0.55,
		RiskLevel:   "low",
		FallbackPlan: &model.ShotGenerationFallback{
			Mode:   model.GenerationModePlaceholderPreview,
			Reason: reason,
		},
		FusionPlan:  model.FusionPlan{ShotID: shot.ID, Assembler: "hyperframes", OutputArtifactKind: "HYPERFRAMES_SHOT"},
		ReviewFocus: []string{"结构和节奏是否可接受", "缺失素材是否清楚"},
	}
}

func textFusionLayers(visual model.VisualPlan) []model.FusionLayer {
	layers := make([]model.FusionLayer, 0, len(visual.TextLayers))
	for _, layer := range visual.TextLayers {
		layers = append(layers, model.FusionLayer{
			ID:          layer.ID,
			Kind:        "html_overlay",
			Role:        layer.Role,
			StartSec:    layer.StartSec,
			DurationSec: layer.EndSec - layer.StartSec,
		})
	}
	return layers
}

func generationPlanID(shotID string) string {
	if strings.TrimSpace(shotID) == "" {
		return "shot-generation-plan"
	}
	return fmt.Sprintf("%s-generation-plan", shotID)
}
```

- [ ] **Step 5: Run service tests**

Run:

```bash
cd cloud-backend
go test ./internal/agents/video/service -run 'TestBuildShotGenerationPlan|TestRenderStrategy|TestTextLayer|TestAIGCPrompt|TestFinalAssembly|TestOralVideo' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 1**

```bash
git add cloud-backend/internal/agents/video/model/creation.go cloud-backend/internal/agents/video/service/generation_plan.go cloud-backend/internal/agents/video/service/generation_plan_test.go
git commit -m "feat: add shot generation planning model"
```

---

### Task 2: Builtin `shot_generation_planner`

**Files:**
- Modify: `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools.go`
- Test: `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools_test.go`

- [ ] **Step 1: Write failing builtin tool tests**

Append to `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools_test.go`:

```go
func TestShotGenerationPlannerRoutesHybridAndExternalNeeds(t *testing.T) {
	result := executeLocalVideoCreationTool("shot_generation_planner", map[string]interface{}{
		"stage": "generation_strategy",
		"shotList": []interface{}{
			map[string]interface{}{
				"shotId":        "SHOT_01",
				"durationSec":   float64(6),
				"visual":        "非真人风格化办公室里人物被文件包围，同时画面必须显示“几个表格”",
				"narrationText": "几个表格就能改变判断。",
				"screenText":    []interface{}{"几个表格"},
			},
			map[string]interface{}{
				"shotId":      "SHOT_02",
				"durationSec": float64(5),
				"visual":      "展示客户上传 logo 和产品截图",
			},
		},
		"aigcAvailable": true,
		"htmlAvailable": true,
	}, tool.ToolContext{TaskID: "task-generation-plan", NodeID: "shot_generation_planner_exec"})

	if !result.Success {
		t.Fatalf("shot_generation_planner failed: %s", result.Error)
	}
	plans, ok := result.Data["shotGenerationPlans"].([]map[string]interface{})
	if !ok || len(plans) != 2 {
		t.Fatalf("plans = %#v", result.Data["shotGenerationPlans"])
	}
	if plans[0]["mode"] != "hybrid_aigc_bg_html_overlay" {
		t.Fatalf("SHOT_01 mode = %#v", plans[0])
	}
	if plans[1]["mode"] != "external_or_user_asset" {
		t.Fatalf("SHOT_02 mode = %#v", plans[1])
	}
	if _, ok := result.Data["shotAssetPackages"].([]map[string]interface{}); !ok {
		t.Fatalf("missing shotAssetPackages: %#v", result.Data)
	}
}

func TestShotGenerationPlannerMissingProviderCreatesExternalRequest(t *testing.T) {
	result := executeLocalVideoCreationTool("shot_generation_planner", map[string]interface{}{
		"stage": "generation_strategy",
		"shotList": []interface{}{
			map[string]interface{}{
				"shotId":      "SHOT_01",
				"durationSec": float64(6),
				"visual":      "人物跑过街口，镜头跟随，电影感运动",
			},
		},
		"aigcAvailable": false,
		"htmlAvailable": true,
	}, tool.ToolContext{TaskID: "task-generation-plan", NodeID: "shot_generation_planner_exec"})

	if !result.Success {
		t.Fatalf("shot_generation_planner failed: %s", result.Error)
	}
	requests, ok := result.Data["externalGenerationRequests"].([]map[string]interface{})
	if !ok || len(requests) != 1 {
		t.Fatalf("external requests = %#v", result.Data["externalGenerationRequests"])
	}
	if requests[0]["kind"] != "video" || requests[0]["shotId"] != "SHOT_01" {
		t.Fatalf("request = %#v", requests[0])
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

```bash
cd cloud-backend
go test ./internal/core/worker/tool/builtin -run 'TestShotGenerationPlanner' -count=1
```

Expected: FAIL because `shot_generation_planner` is not implemented.

- [ ] **Step 3: Add manifest metadata**

In `configureVideoCreationManifest`, add a new `case "shot_generation_planner":` near `shot_splitter`:

```go
case "shot_generation_planner":
	manifest.Description = "Plan per-shot generation mode, asset needs, and fusion strategy."
	manifest.Type = "builtin_prompt_tool"
	manifest.CostLevel = tool.CostLow
	manifest.RiskLevel = tool.RiskLow
	manifest.SideEffect = false
	manifest.Idempotent = true
	manifest.Capabilities = []string{"video_creation", "render_strategy", "asset_routing", "shot_planning"}
	manifest.Parameters = map[string]tool.ParamDef{
		"shotList":            {Type: "array", Description: "Approved shot list", Required: true},
		"visualPlans":         {Type: "array", Description: "Optional per-shot visual plans", Required: false},
		"renderPreference":    {Type: "object", Description: "Render preference", Required: false},
		"aigcAvailable":       {Type: "boolean", Description: "AIGC provider availability", Required: false},
		"htmlAvailable":       {Type: "boolean", Description: "HyperFrames availability", Required: false},
		"providerCapabilities": {Type: "object", Description: "Client provider capabilities", Required: false},
	}
	manifest.Output = map[string]tool.ParamDef{
		"shotGenerationPlans":      {Type: "array", Description: "Per-shot generation plans"},
		"shotAssetPackages":        {Type: "array", Description: "Per-shot asset packages enriched with generation plans"},
		"externalGenerationRequests": {Type: "array", Description: "External generation requests for missing providers"},
		"summary":                  {Type: "string", Description: "Planner summary"},
		"content":                  {Type: "string", Description: "Human-readable review content"},
		"artifacts":                {Type: "object", Description: "Reviewable artifact manifest"},
	}
```

- [ ] **Step 4: Dispatch the builtin executor**

In `executeLocalVideoCreationTool`, add:

```go
case "shot_generation_planner":
	return executeShotGenerationPlanner(stage, skillName, params)
```

- [ ] **Step 5: Implement executor helpers**

In `video_creation_external_tools.go`, add near `executeRenderStrategyPlanner`:

```go
func executeShotGenerationPlanner(stage, skillName string, params map[string]interface{}) tool.ToolResult {
	shots := normalizeShotItemsForAssetDecision(params["shotList"])
	if len(shots) == 0 {
		return tool.FailureResult("shot_generation_planner requires shotList")
	}

	aigcAvailable := boolParam(params, "aigcAvailable", capabilitySnapshot(params).SeedanceAvailable)
	htmlAvailable := boolParam(params, "htmlAvailable", capabilitySnapshot(params).HyperFramesAvailable)
	if _, ok := params["htmlAvailable"]; !ok {
		htmlAvailable = true
	}

	plans := make([]map[string]interface{}, 0, len(shots))
	packages := make([]map[string]interface{}, 0, len(shots))
	requests := make([]map[string]interface{}, 0)
	for i, shotMap := range shots {
		shot := shotUnitFromToolMap(shotMap, i+1)
		visual := visualPlanFromToolMap(shot, shotMap)
		plan := videoservice.BuildShotGenerationPlan(shot, visual, videomodel.DefaultRenderPreference(), videoservice.RenderCapabilities{AIGCAvailable: aigcAvailable, HTMLAvailable: htmlAvailable})
		planMap := structToMap(plan)
		plans = append(plans, planMap)
		packages = append(packages, shotAssetPackageFromGenerationPlan(shotMap, plan))
		requests = append(requests, externalRequestsFromGenerationPlan(plan)...)
	}

	content := buildShotGenerationPlanReviewContent(plans)
	return tool.SuccessResult(map[string]interface{}{
		"content":                    content,
		"shotGenerationPlans":        plans,
		"shotAssetPackages":          packages,
		"externalGenerationRequests": requests,
		"summary":                    fmt.Sprintf("已为 %d 个 shot 生成 AIGC/HyperFrames/Hybrid 素材合成策略。", len(plans)),
		"artifacts": []map[string]interface{}{
			jsonArtifact(stage, "shot_generation_plans.json", skillName, "SHOT_GENERATION_PLAN", true),
		},
	})
}
```

Add these imports to `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools.go`:

```go
videomodel "github.com/tangying-ai/aios-core/internal/agents/video/model"
videoservice "github.com/tangying-ai/aios-core/internal/agents/video/service"
```

The code inside this file should call `videoservice.BuildShotGenerationPlan`, `videoservice.RenderCapabilities`, and `videomodel.DefaultRenderPreference`. Do not import these packages without aliases because the file already has several domain packages and the aliases make generated-plan code easier to scan.

Add helper functions in the same file:

```go
func shotUnitFromToolMap(values map[string]interface{}, fallbackIndex int) videomodel.ShotUnit {
	shotID := firstNonEmptyString(values, "shotId", "id", "cardId")
	if shotID == "" {
		shotID = fmt.Sprintf("SHOT_%02d", fallbackIndex)
	}
	duration := int(normalizedDurationSec(firstExistingValue(values, "durationSec", "duration", "seconds")))
	if duration <= 0 {
		duration = 6
	}
	return videomodel.ShotUnit{
		ID:            shotID,
		SequenceIndex: fallbackIndex,
		DurationSec:   duration,
		Title:         firstNonEmptyString(values, "title", "name"),
		SceneSummary:  firstNonEmptyString(values, "sceneSummary", "visual", "description"),
		MainAction:    firstNonEmptyString(values, "mainAction", "action", "visual"),
		Narration:     firstNonEmptyString(values, "narrationText", "scriptText", "text"),
		ScreenText:    stringListFromInterface(values["screenText"]),
		SingleScene:   true,
		VisualChangeLevel: videomodel.VisualChangeLow,
	}
}

func visualPlanFromToolMap(shot videomodel.ShotUnit, values map[string]interface{}) videomodel.VisualPlan {
	visual := firstNonEmptyString(values, "visual", "visualIntent", "description", "sceneSummary")
	textLayers := make([]videomodel.TextLayerSpec, 0, len(shot.ScreenText))
	for i, text := range shot.ScreenText {
		textLayers = append(textLayers, videomodel.TextLayerSpec{
			ID:          fmt.Sprintf("%s-text-%02d", shot.ID, i+1),
			Text:        text,
			Role:        videomodel.TextRoleKeyword,
			MustBeExact: true,
			StartSec:    float64(i) * float64(shot.DurationSec) / float64(maxInt(1, len(shot.ScreenText))),
			EndSec:      float64(i+1) * float64(shot.DurationSec) / float64(maxInt(1, len(shot.ScreenText))),
		})
	}
	return videomodel.VisualPlan{
		Canvas: videomodel.CanvasSpec{AspectRatio: "16:9", Width: 1920, Height: 1080, FPS: 30, DurationSec: shot.DurationSec},
		Background: videomodel.BackgroundSpec{Description: visual, RequiresAIGC: textLooksAIGC(visual)},
		Characters: []videomodel.CharacterVisualSpec{{ID: "subject", Motion: shot.MainAction}},
		Props: propsFromVisualText(visual),
		TextLayers: textLayers,
		CameraPlan: videomodel.CameraPlan{Movement: firstNonEmptyString(values, "camera", "cameraMove", "cameraMotion")},
	}
}
```

Also add `maxInt`, `textLooksAIGC`, `propsFromVisualText`, `shotAssetPackageFromGenerationPlan`, `externalRequestsFromGenerationPlan`, and `buildShotGenerationPlanReviewContent` with direct deterministic behavior:

```go
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func textLooksAIGC(text string) bool {
	lower := strings.ToLower(text)
	return containsAny(lower, "人物", "character", "场景", "scene", "动作", "motion", "镜头", "camera", "电影", "cinematic", "跑", "走", "转身")
}

func propsFromVisualText(text string) []videomodel.PropVisualSpec {
	if containsAny(strings.ToLower(text), "logo", "标志", "上传", "截图", "产品图") {
		return []videomodel.PropVisualSpec{{ID: "user-media", Description: text}}
	}
	return nil
}
```

- [ ] **Step 6: Run builtin tests**

```bash
cd cloud-backend
go test ./internal/core/worker/tool/builtin -run 'TestShotGenerationPlanner|TestAssetDecisionAgent|TestTextImageToVideoGeneratorRequiresClientProvider|TestOneSentenceChainStopsAtMissingVideoProvider' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit Task 2**

```bash
git add cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools.go cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools_test.go
git commit -m "feat: add shot generation planner tool"
```

---

### Task 3: Dynamic Agent DAG Insertion

**Files:**
- Modify: `cloud-backend/internal/core/agentruntime/plan_compiler.go`
- Test: `cloud-backend/internal/core/agentruntime/plan_compiler_test.go`

- [ ] **Step 1: Write failing PlanCompiler test**

Append to `cloud-backend/internal/core/agentruntime/plan_compiler_test.go`:

```go
func TestPlanCompiler_PreparePlanInsertsShotGenerationPlanner(t *testing.T) {
	catalog := staticToolCatalog{
		"video_script_generator": {Name: "video_script_generator", Output: map[string]tool.ParamDef{"script": {Type: "string"}}},
		"shot_splitter": {
			Name: "shot_splitter",
			Parameters: map[string]tool.ParamDef{"script": {Type: "string", Required: true}},
			Output: map[string]tool.ParamDef{"shotList": {Type: "array"}, "shotAssetPackages": {Type: "array"}},
		},
		"shot_generation_planner": {
			Name: "shot_generation_planner",
			Parameters: map[string]tool.ParamDef{"shotList": {Type: "array", Required: true}},
			Output: map[string]tool.ParamDef{
				"shotGenerationPlans": {Type: "array"},
				"shotAssetPackages":   {Type: "array"},
			},
		},
		"video_prompt_generator": {
			Name: "video_prompt_generator",
			Parameters: map[string]tool.ParamDef{
				"shotList":            {Type: "array", Required: true},
				"shotGenerationPlans": {Type: "array", Required: false},
				"shotAssetPackages":   {Type: "array", Required: false},
			},
			Output: map[string]tool.ParamDef{"videoPrompts": {Type: "array"}, "shotAssetPackages": {Type: "array"}},
		},
		"hyperframes_project_generator": {
			Name: "hyperframes_project_generator",
			Parameters: map[string]tool.ParamDef{
				"topic":               {Type: "string", Required: true},
				"script":              {Type: "string", Required: true},
				"shotList":            {Type: "array", Required: true},
				"videoPrompts":        {Type: "array", Required: false},
				"shotGenerationPlans": {Type: "array", Required: false},
				"shotAssetPackages":   {Type: "array", Required: false},
			},
			Output: map[string]tool.ParamDef{"projectDir": {Type: "string"}, "entry": {Type: "string"}},
		},
		"hyperframes_renderer": {Name: "hyperframes_renderer", Parameters: map[string]tool.ParamDef{"projectDir": {Type: "string", Required: true}}, Output: map[string]tool.ParamDef{"outputPath": {Type: "string"}}},
		"publish_copy_generator": {Name: "publish_copy_generator", Output: map[string]tool.ParamDef{"title": {Type: "string"}}},
	}
	compiler := NewPlanCompiler(catalog)
	plan := &AgentPlan{
		Goal: "做一个混合 AIGC 和字幕的短视频",
		Domain: "video_creation",
		Mode: "dynamic_agent",
		Steps: []AgentStep{{
			ID: "script_generation", Tool: "video_script_generator",
			Arguments: map[string]interface{}{"topic": "AI运营"},
			ExpectedOutput: []string{"script"},
			ProduceArtifact: true,
		}},
	}

	prepared := compiler.PreparePlan(plan)

	strategy := findStep(t, prepared, "shot_generation")
	if got := strategy.Tool; got != "shot_generation_planner" {
		t.Fatalf("tool = %q", got)
	}
	if got := strategy.Arguments["shotList"]; got != "{{beat_plan.output.shotList}}" {
		t.Fatalf("shot_generation should consume beat_plan shotList, got %#v", strategy.Arguments)
	}
	prompt := findStep(t, prepared, "video_prompt")
	if got := prompt.Arguments["shotGenerationPlans"]; got != "{{shot_generation.output.shotGenerationPlans}}" {
		t.Fatalf("video_prompt should consume shot generation plans, got %#v", prompt.Arguments)
	}
	preview := findStep(t, prepared, "preview")
	if got := preview.Arguments["shotGenerationPlans"]; got != "{{shot_generation.output.shotGenerationPlans}}" {
		t.Fatalf("preview should consume shot generation plans, got %#v", preview.Arguments)
	}
	if got := preview.Arguments["shotAssetPackages"]; got != "{{video_prompt.output.shotAssetPackages}}" {
		t.Fatalf("preview should prefer video_prompt packages, got %#v", preview.Arguments)
	}
	if err := NewPlanGuard(catalog, nil).Validate(prepared); err != nil {
		t.Fatalf("prepared plan should pass PlanGuard: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify failure**

```bash
cd cloud-backend
go test ./internal/core/agentruntime -run TestPlanCompiler_PreparePlanInsertsShotGenerationPlanner -count=1
```

Expected: FAIL because no `shot_generation` step exists.

- [ ] **Step 3: Update required beta tool detection**

In `hasVideoBetaCompletionTools`, include `shot_generation_planner` only as optional so older catalogs still compile. Add:

```go
func (c *PlanCompiler) hasOptionalTool(name string) bool {
	return c.manifestFor(name) != nil
}
```

Do not add `shot_generation_planner` to the required list in `hasVideoBetaCompletionTools`; this preserves compatibility with tests and deployments that do not expose the tool yet.

- [ ] **Step 4: Insert the planner after shot split**

In `completeVideoBetaPlan`, after `shotAnchor` is resolved and before `promptAnchor` lookup, add:

```go
generationAnchor, generationField := c.lastProducerStepForFields(plan,
	[]string{"shotGenerationPlans"},
	[]string{"shot_generation_planner"},
)
if generationAnchor == "" && c.hasOptionalTool("shot_generation_planner") {
	generationAnchor = appendPlanStep(plan, AgentStep{
		ID:        uniqueStepID(plan, "shot_generation"),
		Intent:    "按 shot 判断 AIGC、HyperFrames、Hybrid、用户素材和占位预览生成策略",
		Tool:      "shot_generation_planner",
		DependsOn: dependencyList(shotAnchor),
		Arguments: map[string]interface{}{
			"stage":    "generation_strategy",
			"brief":    plan.Goal,
			"shotList": stepOutputRef(shotAnchor, shotField),
		},
		ExpectedOutput:  []string{"shotGenerationPlans", "shotAssetPackages", "externalGenerationRequests"},
		ProduceArtifact: true,
	})
	generationField = preferredOutputField(c.manifestFor("shot_generation_planner"), "shotGenerationPlans")
}
```

- [ ] **Step 5: Pass planner output downstream**

When creating `video_prompt` args, add:

```go
if generationAnchor != "" && generationField != "" {
	args["shotGenerationPlans"] = stepOutputRef(generationAnchor, generationField)
	if packageField := c.outputFieldForStep(plan, generationAnchor, "shotAssetPackages"); packageField != "" {
		args["shotAssetPackages"] = stepOutputRef(generationAnchor, packageField)
	}
}
```

When creating `previewArgs`, add:

```go
if generationAnchor != "" && generationField != "" {
	previewArgs["shotGenerationPlans"] = stepOutputRef(generationAnchor, generationField)
}
```

Keep the existing preference where `video_prompt.output.shotAssetPackages` wins if present. Use the generation planner packages only when `video_prompt_generator` does not expose package output.

- [ ] **Step 6: Run compiler tests**

```bash
cd cloud-backend
go test ./internal/core/agentruntime -run 'TestPlanCompiler_PreparePlan' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit Task 3**

```bash
git add cloud-backend/internal/core/agentruntime/plan_compiler.go cloud-backend/internal/core/agentruntime/plan_compiler_test.go
git commit -m "feat: route dynamic video plans through shot strategy"
```

---

### Task 4: Local HyperFrames Timed Media Fusion

**Files:**
- Modify: `local-backend/internal/localtool/hyperframes_project.go`
- Test: `local-backend/internal/localtool/hyperframes_project_test.go`

- [ ] **Step 1: Write failing localtool tests**

Append to `local-backend/internal/localtool/hyperframes_project_test.go`:

```go
func TestHyperFramesProjectExecutorUsesShotAssetPackageMedia(t *testing.T) {
	root := t.TempDir()
	contentDir := filepath.Join(root, "artifacts", "project_001", "shot-video-01")
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "content"), []byte("fake mp4 bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	executor := NewHyperFramesProjectExecutor(root)
	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandHyperFramesProjectGenerate,
		Payload: map[string]interface{}{
			"topic":  "混合视频",
			"script": "这是一个混合素材视频。",
			"shotAssetPackages": []interface{}{
				map[string]interface{}{
					"shotId":      "SHOT_01",
					"durationSec": float64(4),
					"generationPlan": map[string]interface{}{
						"mode": "hybrid_aigc_bg_html_overlay",
						"fusionPlan": map[string]interface{}{
							"baseLayer": map[string]interface{}{"kind": "video", "storageRef": "local://projects/project_001/artifacts/shot-video-01/hash/clip.mp4"},
							"overlayLayers": []interface{}{
								map[string]interface{}{"id": "title", "kind": "html_overlay", "role": "title", "text": "精确文字"},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, "projects", "project_001", "hyperframes", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	for _, expected := range []string{
		`<video`,
		`muted`,
		`playsinline`,
		`data-track-index="0"`,
		`data-duration="4.0"`,
		`data-shot-id="SHOT_01"`,
		`精确文字`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("index.html missing %q:\n%s", expected, html)
		}
	}
}

func TestHyperFramesProjectExecutorShowsMissingMediaPlaceholder(t *testing.T) {
	root := t.TempDir()
	executor := NewHyperFramesProjectExecutor(root)
	_, err := executor.Execute(context.Background(), Job{
		ID:        "job-1",
		ProjectID: "project_001",
		Command:   CommandHyperFramesProjectGenerate,
		Payload: map[string]interface{}{
			"topic": "缺素材",
			"shotAssetPackages": []interface{}{
				map[string]interface{}{
					"shotId":      "SHOT_01",
					"durationSec": float64(3),
					"generationPlan": map[string]interface{}{
						"mode": "hybrid_aigc_bg_html_overlay",
						"fusionPlan": map[string]interface{}{
							"baseLayer": map[string]interface{}{"kind": "video", "storageRef": "local://projects/project_001/artifacts/missing/hash/clip.mp4"},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, "projects", "project_001", "hyperframes", "index.html"))
	if !strings.Contains(string(raw), "Missing media for SHOT_01") {
		t.Fatalf("missing placeholder not rendered:\n%s", string(raw))
	}
}
```

- [ ] **Step 2: Run localtool tests to verify failure**

```bash
cd local-backend
go test ./internal/localtool -run 'TestHyperFramesProjectExecutorUsesShotAssetPackageMedia|TestHyperFramesProjectExecutorShowsMissingMediaPlaceholder' -count=1
```

Expected: FAIL because `shotAssetPackages` are preserved in data but not rendered as media.

- [ ] **Step 3: Add parsing helpers**

In `local-backend/internal/localtool/hyperframes_project.go`, add types:

```go
type shotMediaPackage struct {
	ShotID      string
	DurationSec float64
	Mode        string
	BaseLayer   mediaLayer
	Overlays    []mediaOverlay
}

type mediaLayer struct {
	Kind       string
	StorageRef string
	ResolvedSrc string
	Missing    bool
}

type mediaOverlay struct {
	ID    string
	Kind  string
	Role  string
	Text  string
}
```

Add parser:

```go
func shotMediaPackagesFromPayload(dataDir, projectID string, payload map[string]interface{}) []shotMediaPackage {
	items, _ := payload["shotAssetPackages"].([]interface{})
	result := make([]shotMediaPackage, 0, len(items))
	for _, item := range items {
		pkg, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		shotID := stringFromMap(pkg, "shotId")
		if shotID == "" {
			shotID = fmt.Sprintf("SHOT_%02d", len(result)+1)
		}
		duration := floatFromMap(pkg, "durationSec")
		if duration <= 0 {
			duration = 4
		}
		gen, _ := pkg["generationPlan"].(map[string]interface{})
		mode := stringFromMap(gen, "mode")
		fusion, _ := gen["fusionPlan"].(map[string]interface{})
		baseMap, _ := fusion["baseLayer"].(map[string]interface{})
		base := mediaLayer{Kind: stringFromMap(baseMap, "kind"), StorageRef: stringFromMap(baseMap, "storageRef")}
		base.ResolvedSrc, base.Missing = resolveMediaStorageRef(dataDir, projectID, base.StorageRef)
		result = append(result, shotMediaPackage{
			ShotID: shotID, DurationSec: duration, Mode: mode, BaseLayer: base,
			Overlays: overlaysFromFusion(fusion),
		})
	}
	return result
}
```

Implement `floatFromMap`, `overlaysFromFusion`, and `resolveMediaStorageRef`. `resolveMediaStorageRef` should map `local://projects/<projectID>/artifacts/<artifactID>/<hash>/<name>` to `/api/local/artifacts/<artifactID>?projectId=<projectID>` for browser/client use when content exists at `dataDir/artifacts/<projectID>/<artifactID>/content`; otherwise return missing.

- [ ] **Step 4: Render media-aware fallback index**

In `Execute`, after building `data`, compute:

```go
mediaPackages := shotMediaPackagesFromPayload(e.dataDir, projectID, job.Payload)
index := buildHyperFramesIndexWithMedia(topic, script, mediaPackages)
```

Implement `buildHyperFramesIndexWithMedia` by extending the existing fallback composition:

```go
func buildHyperFramesIndexWithMedia(topic, script string, mediaPackages []shotMediaPackage) string {
	if len(mediaPackages) == 0 {
		return buildHyperFramesIndex(topic, script)
	}
	spec := fallbackCompositionSpec()
	spec.DurationSec = totalMediaDuration(mediaPackages)
	cards := make([]cardInfo, 0, len(mediaPackages))
	captions := make([]captionInfo, 0, len(mediaPackages))
	for _, pkg := range mediaPackages {
		cards = append(cards, cardInfo{ID: pkg.ShotID + "-card", Kind: "summary_card", StartSec: startForPackage(mediaPackages, pkg.ShotID), EndSec: startForPackage(mediaPackages, pkg.ShotID) + pkg.DurationSec, Title: pkg.ShotID, Body: mediaPackageBody(pkg)})
		for _, overlay := range pkg.Overlays {
			if strings.TrimSpace(overlay.Text) != "" {
				captions = append(captions, captionInfo{ID: overlay.ID, StartSec: startForPackage(mediaPackages, pkg.ShotID), EndSec: startForPackage(mediaPackages, pkg.ShotID) + pkg.DurationSec, Text: overlay.Text})
			}
		}
	}
	return injectTimedMedia(buildCompositionIndex(topic, spec, cards, captions, spec.Style), mediaPackages)
}
```

`injectTimedMedia` should insert media elements inside `.bg-layer` before `.bg-field`:

```html
<video id="media-SHOT_01" class="shot-media" data-shot-id="SHOT_01" data-start="0.0" data-duration="4.0" data-track-index="0" src="/api/local/artifacts/shot-video-01?projectId=project_001" muted playsinline crossorigin="anonymous"></video>
```

For missing media, insert:

```html
<div class="missing-media" data-shot-id="SHOT_01">Missing media for SHOT_01</div>
```

Update `buildCompositionStyle` with `.shot-media` and `.missing-media` rules.

- [ ] **Step 5: Run localtool tests**

```bash
cd local-backend
go test ./internal/localtool -run 'TestHyperFramesProjectExecutor' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 4**

```bash
git add local-backend/internal/localtool/hyperframes_project.go local-backend/internal/localtool/hyperframes_project_test.go
git commit -m "feat: fuse shot media into hyperframes previews"
```

---

### Task 5: Frontend Strategy Display

**Files:**
- Modify: `frontend/src/pages/directorStudioLogic.ts`
- Modify: `frontend/src/pages/DirectorStudioPage.tsx`

- [ ] **Step 1: Extend shot review group types**

In `frontend/src/pages/directorStudioLogic.ts`, add fields to `DirectorShotReviewGroup`:

```ts
generationStrategy?: {
  mode: string
  label: string
  reason: string
  riskLevel?: string
}
```

Add helper:

```ts
function shotGenerationStrategy(artifacts: DirectorArtifactRecord[]): DirectorShotReviewGroup['generationStrategy'] {
  const candidate = artifacts
    .map((artifact) => objectValue(artifact.metadata?.generationPlan) || objectValue(artifact.metadata?.renderStrategy))
    .find(Boolean)
  const mode = stringValue(candidate?.mode)
  if (!mode) return undefined
  const labels: Record<string, string> = {
    html_only: 'HyperFrames',
    aigc_video: 'AIGC',
    aigc_image_then_hyperframes: 'Image + HyperFrames',
    hybrid_aigc_bg_html_overlay: 'Hybrid',
    external_or_user_asset: 'User Asset',
    placeholder_preview: 'Preview',
  }
  return {
    mode,
    label: labels[mode] || mode,
    reason: stringValue(candidate?.reason),
    riskLevel: stringValue(candidate?.riskLevel),
  }
}
```

In `buildShotReviewGroups`, set:

```ts
generationStrategy: shotGenerationStrategy(shotArtifacts),
```

- [ ] **Step 2: Split hybrid slots**

In `buildShotAssetSlots`, add slot specs:

```ts
{
  kind: 'base-media',
  label: '基础画面',
  description: 'AIGC 背景视频、首帧图片或用户上传素材。',
  uploadKind: 'video',
},
{
  kind: 'overlay',
  label: '文字叠层',
  description: 'HyperFrames 字幕、卡片、UI、图表和精确文字层。',
},
```

If `DirectorShotAssetSlotKind` is a union, extend it with `'base-media' | 'overlay'`. Update `shotAssetSlotForArtifact` to route `SHOT_GENERATION_PLAN` and `SHOT_MEDIA_FUSION_PLAN` to `prompt`, `SHOT_VIDEO_CLIP` to `base-media`, and `HYPERFRAMES_SHOT` / overlay metadata to `overlay`.

- [ ] **Step 3: Render strategy label in shot header**

In `DirectorStudioPage.tsx`, find the open shot header around the section showing `openGroup.shotId`. Add:

```tsx
{openGroup.generationStrategy && (
  <div className="mt-2 flex flex-wrap items-center gap-2">
    <span className="rounded-full bg-primary-soft px-3 py-1 text-xs font-black text-primary-dark">
      {openGroup.generationStrategy.label}
    </span>
    {openGroup.generationStrategy.reason && (
      <span className="text-xs leading-5 text-ink-muted">{openGroup.generationStrategy.reason}</span>
    )}
  </div>
)}
```

- [ ] **Step 4: Run frontend build**

```bash
cd frontend
npm run build
```

Expected: PASS.

- [ ] **Step 5: Commit Task 5**

```bash
git add frontend/src/pages/directorStudioLogic.ts frontend/src/pages/DirectorStudioPage.tsx
git commit -m "feat: show shot generation strategies"
```

---

### Task 6: End-To-End Smoke Harness

**Files:**
- Create: `local-backend/internal/localtool/testdata/shot-media/README.md`
- Test via generated temp data only. Do not commit generated MP4s or `tmp/`.

- [ ] **Step 1: Run full backend/frontend verification**

```bash
cd cloud-backend && go test ./...
cd ../local-backend && go test ./...
cd ../frontend && npm run build
cd ../hyperframes-render-service && npm run build
```

Expected: all commands exit 0.

- [ ] **Step 2: Create a local synthetic media fixture in temp space**

Run from repo root:

```bash
mkdir -p tmp/shot-routing-e2e/data/artifacts/video_agent_routing/shot-video-01
ffmpeg -y -f lavfi -i color=c=0x17202a:s=1920x1080:d=4:r=30 -vf "drawtext=text='AIGC BASE PLACEHOLDER':fontcolor=white:fontsize=72:x=(w-text_w)/2:y=(h-text_h)/2" tmp/shot-routing-e2e/data/artifacts/video_agent_routing/shot-video-01/content
```

Expected: `tmp/shot-routing-e2e/data/artifacts/video_agent_routing/shot-video-01/content` exists and is a short MP4. If local ffmpeg lacks `drawtext`, rerun with only the color source:

```bash
ffmpeg -y -f lavfi -i color=c=0x17202a:s=1920x1080:d=4:r=30 tmp/shot-routing-e2e/data/artifacts/video_agent_routing/shot-video-01/content
```

- [ ] **Step 3: Run a small localtool probe without committing it**

Use `go test` or a temporary probe under `tmp/` to call `NewHyperFramesProjectExecutor` with three shot packages:

```json
[
  {"shotId":"SHOT_01","durationSec":4,"generationPlan":{"mode":"html_only","fusionPlan":{"overlayLayers":[{"id":"title","kind":"html_overlay","role":"title","text":"HyperFrames 精确文字"}]}}},
  {"shotId":"SHOT_02","durationSec":4,"generationPlan":{"mode":"aigc_video","fusionPlan":{"baseLayer":{"kind":"video","storageRef":"local://projects/video_agent_routing/artifacts/shot-video-01/hash/clip.mp4"}}}},
  {"shotId":"SHOT_03","durationSec":4,"generationPlan":{"mode":"hybrid_aigc_bg_html_overlay","fusionPlan":{"baseLayer":{"kind":"video","storageRef":"local://projects/video_agent_routing/artifacts/shot-video-01/hash/clip.mp4"},"overlayLayers":[{"id":"hybrid-title","kind":"html_overlay","role":"title","text":"Hybrid 精确叠字"}]}}}
]
```

Expected generated `index.html` contains one root `data-composition-id`, timed video media, and both exact text overlays.

- [ ] **Step 4: Render through HyperFrames service**

Start service from `hyperframes-render-service` with temp roots:

```bash
PORT=8787 HOST=127.0.0.1 HYPERFRAMES_PROJECT_ROOT=/Users/wanglian/Projects/tangying-ai-operation-system/tmp/shot-routing-e2e/data/projects HYPERFRAMES_OUTPUT_ROOT=/Users/wanglian/Projects/tangying-ai-operation-system/tmp/shot-routing-e2e/data/projects npm run dev
```

Render with local backend executor or direct local agent. Expected final video:

```text
/Users/wanglian/Projects/tangying-ai-operation-system/tmp/shot-routing-e2e/data/projects/video_agent_routing/renders/final.mp4
```

- [ ] **Step 5: Verify video metadata**

```bash
ffprobe -v error -show_entries format=duration,size -show_entries stream=codec_type,codec_name,width,height,r_frame_rate -of json /Users/wanglian/Projects/tangying-ai-operation-system/tmp/shot-routing-e2e/data/projects/video_agent_routing/renders/final.mp4
```

Expected: H.264 video, 1920x1080, 30fps, duration around 12 seconds.

- [ ] **Step 6: Browser verify final client playback surface**

Use the browser skill with the in-app browser:

1. Start local agent against `tmp/shot-routing-e2e/data`.
2. Open a local client page that fetches `final-video` through `/api/local/artifacts/final-video?projectId=video_agent_routing`.
3. Verify the single `<video data-testid="final-video-preview">` has `readyState >= 1`, `videoWidth === 1920`, `videoHeight === 1080`, and duration near 12 seconds.

- [ ] **Step 7: Stop all test servers**

Stop HyperFrames render service, local agent, and any static client server. Confirm ports are closed:

```bash
lsof -nP -iTCP:8787 -sTCP:LISTEN
lsof -nP -iTCP:18080 -sTCP:LISTEN
lsof -nP -iTCP:4321 -sTCP:LISTEN
```

Expected: each command exits 1 with no listeners.

- [ ] **Step 8: Commit after smoke cleanup**

Only commit source and tests:

```bash
git status --short
git add cloud-backend local-backend frontend docs/superpowers/plans/2026-07-01-shot-generation-routing-fusion.md
git commit -m "test: verify shot routing fusion smoke path"
```

Do not add `tmp/`.

---

### Final Verification

- [ ] Run all required commands:

```bash
cd cloud-backend && go test ./...
cd ../local-backend && go test ./...
cd ../frontend && npm run build
cd ../hyperframes-render-service && npm run build
```

- [ ] Confirm worktree:

```bash
git status --short --branch
```

Expected: only intentional source commits ahead of `develop_go/develop_go`; `tmp/` may remain untracked and must not be staged.

- [ ] Push only when requested or when continuing the existing `develop_go` integration path:

```bash
git push develop_go develop_go
```
