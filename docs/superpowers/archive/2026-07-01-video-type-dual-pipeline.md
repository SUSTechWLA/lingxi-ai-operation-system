# Video Type Dual Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build profile-aware video creation so talking-head videos follow a script-timed material alignment chain and cinematic videos follow a continuity-first coarse-to-fine chain with AIGC-safe 3-15 second generation windows.

**Architecture:** Add `VideoCreationProfile` and `TimeWindowPlan` as shared planning contracts in the video model/service layer. Expose them through builtin tools, then teach `PlanCompiler` to select a talking-head or cinematic DAG template before invoking per-shot generation planning. Keep `shot_generation_planner` shared, but feed it profile and time-window context so external AIGC requests support direct API execution and manual client generation with references.

**Tech Stack:** Go backend (`cloud-backend` model/service/tool/DAG tests), TypeScript frontend logic tests, existing local HyperFrames preview path.

---

## File Structure

- Modify `cloud-backend/internal/agents/video/model/creation.go`
  - Add profile constants, `VideoCreationProfile`, `TimeWindowPlan`, `TimeWindowUnit`, `ScriptSpan`, and external generation delivery fields.
- Create `cloud-backend/internal/agents/video/service/profile.go`
  - Deterministic `BuildVideoCreationProfile` from route, deliverable, brief, and optional context.
- Create `cloud-backend/internal/agents/video/service/profile_test.go`
  - Unit tests for profile selection.
- Create `cloud-backend/internal/agents/video/service/time_window_plan.go`
  - Deterministic 3-15 second time-window planning for talking-head and cinematic flows.
- Create `cloud-backend/internal/agents/video/service/time_window_plan_test.go`
  - Unit tests for script-timed talking-head windows and cinematic coarse-to-fine splitting.
- Modify `cloud-backend/internal/agents/video/service/generation_plan.go`
  - Preserve time-window metadata and AIGC delivery constraints in render inputs and asset needs.
- Modify `cloud-backend/internal/agents/video/service/generation_plan_test.go`
  - Add duration and reference propagation tests.
- Modify `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools.go`
  - Add `video_profile_classifier`, `time_window_planner`, `visual_alignment_planner`, `cinematic_shot_designer`, and `sound_design_planner`.
  - Enrich `externalGenerationRequests` with duration, reference images, direct API eligibility, and manual upload binding.
- Modify `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools_test.go`
  - Add builtin tool contract tests and request payload tests.
- Modify `cloud-backend/internal/core/agentruntime/plan_compiler.go`
  - Add profile-aware plan completion and keep current completion as fallback.
- Modify `cloud-backend/internal/core/agentruntime/plan_compiler_test.go`
  - Add talking-head and cinematic DAG template tests.
- Modify `frontend/src/pages/directorStudioLogic.ts`
  - Surface `VIDEO_CREATION_PROFILE`, `TIME_WINDOW_PLAN`, and AIGC delivery information in review groups.
- Modify `frontend/scripts/director-studio-logic-check.mjs`
  - Add frontend logic assertions for profile and time-window artifacts.

---

### Task 1: Video Creation Profile Contract

**Files:**
- Modify: `cloud-backend/internal/agents/video/model/creation.go`
- Create: `cloud-backend/internal/agents/video/service/profile.go`
- Create: `cloud-backend/internal/agents/video/service/profile_test.go`

- [ ] **Step 1: Write profile service failing tests**

Add `cloud-backend/internal/agents/video/service/profile_test.go`:

```go
package service

import (
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestBuildVideoCreationProfileTalkingHead(t *testing.T) {
	profile := BuildVideoCreationProfile(ProfileRequest{
		Route:       "talking_head",
		Deliverable: "publish_pack",
		Brief:       "做一期60秒口播知识视频，讲AI工作流",
	})

	if profile.ProfileID != model.VideoProfileTalkingHead {
		t.Fatalf("profile = %s, want %s", profile.ProfileID, model.VideoProfileTalkingHead)
	}
	if profile.PrimaryArtifact != "VIDEO_SCRIPT" {
		t.Fatalf("primary artifact = %s", profile.PrimaryArtifact)
	}
	if !containsString(profile.QualityContract, "script_timeline_alignment") {
		t.Fatalf("talking-head quality contract should include script timeline alignment: %#v", profile.QualityContract)
	}
	if profile.DAGTemplateID != "talking_head_v1" {
		t.Fatalf("dag template = %s", profile.DAGTemplateID)
	}
}

func TestBuildVideoCreationProfileCinematic(t *testing.T) {
	profile := BuildVideoCreationProfile(ProfileRequest{
		Route:       "cinematic_short",
		Deliverable: "video_prompt",
		Brief:       "做一个有角色、场景和道具连续性的影视短片",
	})

	if profile.ProfileID != model.VideoProfileCinematicStory {
		t.Fatalf("profile = %s, want %s", profile.ProfileID, model.VideoProfileCinematicStory)
	}
	if profile.PrimaryArtifact != "CONTINUITY_BIBLE" {
		t.Fatalf("primary artifact = %s", profile.PrimaryArtifact)
	}
	if !containsString(profile.QualityContract, "aigc_time_windows_3_15s") {
		t.Fatalf("cinematic quality contract should include AIGC time-window limit: %#v", profile.QualityContract)
	}
	if profile.DAGTemplateID != "cinematic_story_v1" {
		t.Fatalf("dag template = %s", profile.DAGTemplateID)
	}
}
```

- [ ] **Step 2: Run profile tests to verify they fail**

Run:

```bash
cd cloud-backend && go test ./internal/agents/video/service -run 'TestBuildVideoCreationProfile' -count=1
```

Expected: FAIL with `undefined: BuildVideoCreationProfile` or missing model constants.

- [ ] **Step 3: Add profile model types**

Append these definitions near the existing video creation constants in `cloud-backend/internal/agents/video/model/creation.go`:

```go
const (
	VideoProfileTalkingHead    = "talking_head"
	VideoProfileCinematicStory = "cinematic_story"

	ArtifactKindVideoCreationProfile = "VIDEO_CREATION_PROFILE"
)

type VideoCreationProfile struct {
	ProfileID       string            `json:"profileId"`
	SourceRoute     string            `json:"sourceRoute,omitempty"`
	PrimaryArtifact string            `json:"primaryArtifact"`
	QualityContract []string          `json:"qualityContract,omitempty"`
	DAGTemplateID    string            `json:"dagTemplateId"`
	ReviewGatePolicy []string         `json:"reviewGatePolicy,omitempty"`
	ToolBias         map[string]string `json:"toolBias,omitempty"`
	FallbackProfile  string            `json:"fallbackProfile,omitempty"`
	Confidence       float64           `json:"confidence,omitempty"`
	Reason           string            `json:"reason,omitempty"`
	NeedsUserReview  bool              `json:"needsUserReview,omitempty"`
}
```

- [ ] **Step 4: Add deterministic profile service**

Create `cloud-backend/internal/agents/video/service/profile.go`:

```go
package service

import (
	"strings"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

type ProfileRequest struct {
	Route       string
	Deliverable string
	Brief       string
}

func BuildVideoCreationProfile(req ProfileRequest) model.VideoCreationProfile {
	route := strings.TrimSpace(req.Route)
	brief := strings.ToLower(req.Brief)
	if route == "cinematic_short" || route == "director_pipeline" || looksCinematicBrief(brief) {
		if req.Deliverable == "publish_pack" && looksTalkingHeadBrief(brief) && !looksCinematicBrief(brief) {
			return talkingHeadProfile(route, 0.74, "publish pack brief is script-led")
		}
		return cinematicProfile(route, 0.82, "brief requires cinematic continuity")
	}
	if route == "talking_head" || looksTalkingHeadBrief(brief) {
		return talkingHeadProfile(route, 0.84, "brief is script-led")
	}
	profile := talkingHeadProfile(route, 0.56, "low-confidence fallback to script-led preview")
	profile.NeedsUserReview = true
	profile.FallbackProfile = model.VideoProfileCinematicStory
	return profile
}

func talkingHeadProfile(route string, confidence float64, reason string) model.VideoCreationProfile {
	return model.VideoCreationProfile{
		ProfileID:       model.VideoProfileTalkingHead,
		SourceRoute:     route,
		PrimaryArtifact: "VIDEO_SCRIPT",
		QualityContract: []string{
			"script_timeline_alignment",
			"caption_coverage",
			"visuals_support_script",
			"exact_text_in_hyperframes",
			"aigc_time_windows_3_15s",
		},
		DAGTemplateID: "talking_head_v1",
		ReviewGatePolicy: []string{
			"script_review",
			"visual_alignment_review",
			"preview_review",
		},
		ToolBias: map[string]string{
			"primaryRenderer": "hyperframes",
			"aigcUse":         "optional_broll_or_concept_visual",
		},
		Confidence: confidence,
		Reason:     reason,
	}
}

func cinematicProfile(route string, confidence float64, reason string) model.VideoCreationProfile {
	return model.VideoCreationProfile{
		ProfileID:       model.VideoProfileCinematicStory,
		SourceRoute:     route,
		PrimaryArtifact: "CONTINUITY_BIBLE",
		QualityContract: []string{
			"character_scene_prop_consistency",
			"director_reasoning_per_shot",
			"aigc_time_windows_3_15s",
			"time_window_first_frame_lock",
			"sound_design_per_shot",
		},
		DAGTemplateID: "cinematic_story_v1",
		ReviewGatePolicy: []string{
			"story_review",
			"continuity_bible_review",
			"reference_asset_review",
			"shot_design_review",
			"keyframe_storyboard_review",
			"director_cut_review",
		},
		ToolBias: map[string]string{
			"primaryRenderer": "aigc_then_assembly",
			"hyperframesUse":  "deterministic_text_overlay",
		},
		Confidence: confidence,
		Reason:     reason,
	}
}

func looksTalkingHeadBrief(text string) bool {
	return containsAnyString(text, "口播", "观点", "讲", "解释", "知识", "解说", "图文", "小红书", "b站")
}

func looksCinematicBrief(text string) bool {
	return containsAnyString(text, "影视", "剧情", "短片", "角色", "场景", "道具", "导演", "镜头", "故事", "连续性")
}

func containsAnyString(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Run profile tests**

Run:

```bash
cd cloud-backend && go test ./internal/agents/video/service -run 'TestBuildVideoCreationProfile' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit profile contract**

Run:

```bash
git add cloud-backend/internal/agents/video/model/creation.go cloud-backend/internal/agents/video/service/profile.go cloud-backend/internal/agents/video/service/profile_test.go
git commit -m "feat: add video creation profile contract"
```

---

### Task 2: Time-Window Planner Contract

**Files:**
- Modify: `cloud-backend/internal/agents/video/model/creation.go`
- Create: `cloud-backend/internal/agents/video/service/time_window_plan.go`
- Create: `cloud-backend/internal/agents/video/service/time_window_plan_test.go`

- [ ] **Step 1: Write failing time-window tests**

Create `cloud-backend/internal/agents/video/service/time_window_plan_test.go`:

```go
package service

import (
	"testing"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

func TestBuildTimeWindowPlanSplitsCinematicLongShot(t *testing.T) {
	profile := model.VideoCreationProfile{ProfileID: model.VideoProfileCinematicStory}
	plan := BuildTimeWindowPlan(TimeWindowRequest{
		Profile: profile,
		Shots: []model.ShotUnit{
			{ID: "SHOT_01", DurationSec: 40, SceneSummary: "夜晚街道追逐", MainAction: "角色穿过街道并躲入巷子"},
		},
	})

	if len(plan.Windows) != 4 {
		t.Fatalf("window count = %d, want 4: %#v", len(plan.Windows), plan.Windows)
	}
	for _, window := range plan.Windows {
		if window.DurationSec < 3 || window.DurationSec > 15 {
			t.Fatalf("window duration outside 3-15s: %#v", window)
		}
		if window.ParentShotID != "SHOT_01" {
			t.Fatalf("parent shot mismatch: %#v", window)
		}
		if !window.AIGCEligible {
			t.Fatalf("cinematic windows should be AIGC eligible: %#v", window)
		}
	}
}

func TestBuildTimeWindowPlanKeepsTalkingHeadScriptTiming(t *testing.T) {
	profile := model.VideoCreationProfile{ProfileID: model.VideoProfileTalkingHead}
	plan := BuildTimeWindowPlan(TimeWindowRequest{
		Profile: profile,
		ScriptSpans: []model.ScriptSpan{
			{ID: "seg-1", StartSec: 0, EndSec: 6, Text: "第一句口播。"},
			{ID: "seg-2", StartSec: 6, EndSec: 14, Text: "第二句解释。"},
		},
	})

	if len(plan.Windows) != 2 {
		t.Fatalf("window count = %d, want 2", len(plan.Windows))
	}
	if plan.Windows[0].ScriptText != "第一句口播。" || plan.Windows[1].ScriptText != "第二句解释。" {
		t.Fatalf("script text should be preserved: %#v", plan.Windows)
	}
	if plan.Windows[0].StartSec != 0 || plan.Windows[1].StartSec != 6 {
		t.Fatalf("script timing should be preserved: %#v", plan.Windows)
	}
}
```

- [ ] **Step 2: Run time-window tests to verify they fail**

Run:

```bash
cd cloud-backend && go test ./internal/agents/video/service -run 'TestBuildTimeWindowPlan' -count=1
```

Expected: FAIL with `undefined: BuildTimeWindowPlan` or missing model types.

- [ ] **Step 3: Add time-window model types**

Append to `cloud-backend/internal/agents/video/model/creation.go` after `TimedMediaLayer`:

```go
const (
	ArtifactKindTimeWindowPlan = "TIME_WINDOW_PLAN"
)

type ScriptSpan struct {
	ID       string  `json:"id"`
	StartSec float64 `json:"startSec"`
	EndSec   float64 `json:"endSec"`
	Text     string  `json:"text"`
}

type TimeWindowPlan struct {
	ProfileID string           `json:"profileId"`
	Windows   []TimeWindowUnit `json:"windows"`
	Warnings  []string         `json:"warnings,omitempty"`
}

type TimeWindowUnit struct {
	ID              string  `json:"id"`
	ShotID          string  `json:"shotId"`
	ParentShotID    string  `json:"parentShotId,omitempty"`
	SequenceIndex   int     `json:"sequenceIndex"`
	StartSec        float64 `json:"startSec"`
	EndSec          float64 `json:"endSec"`
	DurationSec     float64 `json:"durationSec"`
	ScriptSpanID    string  `json:"scriptSpanId,omitempty"`
	ScriptText      string  `json:"scriptText,omitempty"`
	SceneSummary    string  `json:"sceneSummary,omitempty"`
	MainAction      string  `json:"mainAction,omitempty"`
	AIGCEligible    bool    `json:"aigcEligible"`
	RecommendedMode string  `json:"recommendedMode,omitempty"`
	Reason          string  `json:"reason,omitempty"`
}
```

- [ ] **Step 4: Add deterministic time-window service**

Create `cloud-backend/internal/agents/video/service/time_window_plan.go`:

```go
package service

import (
	"fmt"
	"math"

	"github.com/tangying-ai/aios-core/internal/agents/video/model"
)

type TimeWindowRequest struct {
	Profile     model.VideoCreationProfile
	Shots       []model.ShotUnit
	ScriptSpans []model.ScriptSpan
}

func BuildTimeWindowPlan(req TimeWindowRequest) model.TimeWindowPlan {
	if req.Profile.ProfileID == model.VideoProfileCinematicStory {
		return buildCinematicTimeWindowPlan(req)
	}
	return buildTalkingHeadTimeWindowPlan(req)
}

func buildTalkingHeadTimeWindowPlan(req TimeWindowRequest) model.TimeWindowPlan {
	windows := make([]model.TimeWindowUnit, 0, len(req.ScriptSpans))
	for i, span := range req.ScriptSpans {
		duration := span.EndSec - span.StartSec
		if duration <= 0 {
			duration = 6
		}
		window := model.TimeWindowUnit{
			ID:              fmt.Sprintf("TW_%02d", i+1),
			ShotID:          fmt.Sprintf("SHOT_%02d", i+1),
			SequenceIndex:   i,
			StartSec:        span.StartSec,
			EndSec:          span.StartSec + duration,
			DurationSec:     duration,
			ScriptSpanID:    span.ID,
			ScriptText:      span.Text,
			AIGCEligible:    duration >= 3 && duration <= 15,
			RecommendedMode: model.GenerationModeHTMLOnly,
			Reason:          "script-timed visual support window",
		}
		if duration > 15 {
			window.AIGCEligible = false
			window.RecommendedMode = model.GenerationModeHTMLOnly
			window.Reason = "long script span should stay HyperFrames unless a material planner splits it"
		}
		windows = append(windows, window)
	}
	return model.TimeWindowPlan{ProfileID: model.VideoProfileTalkingHead, Windows: windows}
}

func buildCinematicTimeWindowPlan(req TimeWindowRequest) model.TimeWindowPlan {
	windows := []model.TimeWindowUnit{}
	for _, shot := range req.Shots {
		duration := float64(shot.DurationSec)
		if duration <= 0 {
			duration = 6
		}
		count := int(math.Ceil(duration / 12.0))
		if count < 1 {
			count = 1
		}
		windowDuration := duration / float64(count)
		if windowDuration < 3 {
			windowDuration = 3
		}
		start := 0.0
		for i := 0; i < count; i++ {
			end := start + windowDuration
			if i == count-1 {
				end = duration
			}
			actual := end - start
			if actual < 3 && len(windows) > 0 {
				previous := &windows[len(windows)-1]
				previous.EndSec = end
				previous.DurationSec = previous.EndSec - previous.StartSec
				break
			}
			windows = append(windows, model.TimeWindowUnit{
				ID:              fmt.Sprintf("%s_TW_%02d", shot.ID, i+1),
				ShotID:          fmt.Sprintf("%s_TW_%02d", shot.ID, i+1),
				ParentShotID:    shot.ID,
				SequenceIndex:   len(windows),
				StartSec:        start,
				EndSec:          end,
				DurationSec:     actual,
				SceneSummary:    shot.SceneSummary,
				MainAction:      shot.MainAction,
				AIGCEligible:    true,
				RecommendedMode: model.GenerationModeAIGCVideo,
				Reason:          "cinematic coarse shot split into AIGC-safe 3-15s window",
			})
			start = end
		}
	}
	return model.TimeWindowPlan{ProfileID: model.VideoProfileCinematicStory, Windows: windows}
}
```

- [ ] **Step 5: Run time-window tests**

Run:

```bash
cd cloud-backend && go test ./internal/agents/video/service -run 'TestBuildTimeWindowPlan' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit time-window contract**

Run:

```bash
git add cloud-backend/internal/agents/video/model/creation.go cloud-backend/internal/agents/video/service/time_window_plan.go cloud-backend/internal/agents/video/service/time_window_plan_test.go
git commit -m "feat: add video time window planning"
```

---

### Task 3: Builtin Profile and Time-Window Tools

**Files:**
- Modify: `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools.go`
- Modify: `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools_test.go`

- [ ] **Step 1: Write failing builtin tool tests**

Append to `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools_test.go`:

```go
func TestVideoProfileClassifierReturnsTalkingHeadProfile(t *testing.T) {
	result := executeLocalVideoCreationTool("video_profile_classifier", map[string]interface{}{
		"stage":       "profile_selection",
		"route":       "talking_head",
		"deliverable": "publish_pack",
		"brief":       "做一期60秒口播知识视频",
	}, tool.ToolContext{TaskID: "task-profile", NodeID: "profile_exec"})

	if !result.Success {
		t.Fatalf("video_profile_classifier failed: %s", result.Error)
	}
	profile, ok := result.Data["creationProfile"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing creationProfile: %#v", result.Data)
	}
	if profile["profileId"] != "talking_head" {
		t.Fatalf("profileId = %#v", profile["profileId"])
	}
	if result.Data["artifacts"] == nil {
		t.Fatalf("profile classifier should create reviewable artifacts")
	}
}

func TestTimeWindowPlannerSplitsCinematicShot(t *testing.T) {
	result := executeLocalVideoCreationTool("time_window_planner", map[string]interface{}{
		"stage": "time_window",
		"creationProfile": map[string]interface{}{
			"profileId": "cinematic_story",
		},
		"shotList": []interface{}{
			map[string]interface{}{
				"shotId":      "SHOT_01",
				"durationSec": float64(40),
				"visual":      "夜晚街道追逐",
				"mainAction":  "角色穿过街道并躲入巷子",
			},
		},
	}, tool.ToolContext{TaskID: "task-time-window", NodeID: "time_window_exec"})

	if !result.Success {
		t.Fatalf("time_window_planner failed: %s", result.Error)
	}
	windows, ok := result.Data["timeWindows"].([]map[string]interface{})
	if !ok || len(windows) != 4 {
		t.Fatalf("expected four time windows, got %#v", result.Data["timeWindows"])
	}
	for _, window := range windows {
		duration := intFromInterface(window["durationSec"], 0)
		if duration < 3 || duration > 15 {
			t.Fatalf("duration outside 3-15s: %#v", window)
		}
	}
}
```

- [ ] **Step 2: Run builtin tool tests to verify they fail**

Run:

```bash
cd cloud-backend && go test ./internal/core/worker/tool/builtin -run 'TestVideoProfileClassifier|TestTimeWindowPlanner' -count=1
```

Expected: FAIL because the new tools are not registered or dispatched.

- [ ] **Step 3: Register tool manifests**

In `supportedVideoCreationTools`, add:

```go
"video_profile_classifier",
"time_window_planner",
"visual_alignment_planner",
"cinematic_shot_designer",
"sound_design_planner",
```

In `configureVideoCreationManifest`, add cases:

```go
case "video_profile_classifier":
	manifest.Description = "Classify the video brief into a creation profile and DAG template."
	manifest.Type = "builtin_prompt_tool"
	manifest.CostLevel = tool.CostLow
	manifest.RiskLevel = tool.RiskLow
	manifest.SideEffect = false
	manifest.Idempotent = true
	manifest.Capabilities = []string{"video_creation", "workflow_routing"}
	manifest.Parameters = map[string]tool.ParamDef{
		"brief":       {Type: "string", Description: "Original user brief", Required: true},
		"route":       {Type: "string", Description: "Optional route from skill runtime", Required: false},
		"deliverable": {Type: "string", Description: "Optional deliverable from skill runtime", Required: false},
	}
	manifest.Output = map[string]tool.ParamDef{
		"creationProfile": {Type: "object", Description: "Selected video creation profile"},
		"summary":         {Type: "string", Description: "Human-readable profile summary"},
		"content":         {Type: "string", Description: "Reviewable markdown content"},
		"artifacts":       {Type: "object", Description: "Reviewable artifact manifest"},
	}
case "time_window_planner":
	manifest.Description = "Plan script-timed or cinematic 3-15 second AIGC-safe time windows."
	manifest.Type = "builtin_prompt_tool"
	manifest.CostLevel = tool.CostLow
	manifest.RiskLevel = tool.RiskLow
	manifest.SideEffect = false
	manifest.Idempotent = true
	manifest.Capabilities = []string{"video_creation", "shot_planning", "timing"}
	manifest.Parameters = map[string]tool.ParamDef{
		"creationProfile": {Type: "object", Description: "Video creation profile", Required: true},
		"shotList":        {Type: "array", Description: "Coarse shots", Required: false},
		"scriptSpans":     {Type: "array", Description: "Timed script spans", Required: false},
	}
	manifest.Output = map[string]tool.ParamDef{
		"timeWindowPlan": {Type: "object", Description: "Full time-window plan"},
		"timeWindows":    {Type: "array", Description: "3-15 second time windows"},
		"summary":        {Type: "string", Description: "Human-readable summary"},
		"content":        {Type: "string", Description: "Reviewable markdown content"},
		"artifacts":      {Type: "object", Description: "Reviewable artifact manifest"},
	}
case "visual_alignment_planner":
	manifest.Description = "Align talking-head visuals, materials, captions, and AIGC needs to script time windows."
	manifest.Type = "builtin_prompt_tool"
	manifest.CostLevel = tool.CostLow
	manifest.RiskLevel = tool.RiskLow
	manifest.SideEffect = false
	manifest.Idempotent = true
	manifest.Capabilities = []string{"video_creation", "visual_alignment"}
	manifest.Parameters = map[string]tool.ParamDef{
		"script":        {Type: "string", Description: "Approved script", Required: false},
		"timeWindows":   {Type: "array", Description: "Script-timed windows", Required: true},
		"assetStrategy": {Type: "string", Description: "Optional asset strategy", Required: false},
	}
	manifest.Output = map[string]tool.ParamDef{
		"visualAlignmentPlan": {Type: "object", Description: "Visual-to-script alignment plan"},
		"shotList":            {Type: "array", Description: "Visual shot list for downstream generation"},
		"content":             {Type: "string", Description: "Reviewable markdown content"},
		"artifacts":           {Type: "object", Description: "Reviewable artifact manifest"},
	}
case "cinematic_shot_designer", "sound_design_planner":
	manifest.Type = "builtin_prompt_tool"
	manifest.CostLevel = tool.CostLow
	manifest.RiskLevel = tool.RiskMedium
	manifest.SideEffect = false
	manifest.Idempotent = true
	manifest.Capabilities = []string{"video_creation", "cinematic_planning"}
	manifest.Parameters = map[string]tool.ParamDef{
		"brief":           {Type: "string", Description: "Original user brief", Required: false},
		"creationProfile": {Type: "object", Description: "Video creation profile", Required: false},
	}
	manifest.Output = map[string]tool.ParamDef{
		"content":   {Type: "string", Description: "Reviewable markdown content"},
		"artifacts": {Type: "object", Description: "Reviewable artifact manifest"},
	}
```

- [ ] **Step 4: Add tool dispatch**

In `executeLocalVideoCreationTool`, add cases before `shot_generation_planner`:

```go
case "video_profile_classifier":
	return executeVideoProfileClassifier(stage, skillName, brief, params)
case "time_window_planner":
	return executeTimeWindowPlanner(stage, skillName, params)
case "visual_alignment_planner":
	return executeVisualAlignmentPlanner(stage, skillName, params)
case "cinematic_shot_designer":
	return executeCinematicShotDesigner(stage, skillName, brief, params)
case "sound_design_planner":
	return executeSoundDesignPlanner(stage, skillName, brief, params)
```

- [ ] **Step 5: Add deterministic tool executors**

Add helper functions near `executeShotGenerationPlanner`:

```go
func executeVideoProfileClassifier(stage, skillName, brief string, params map[string]interface{}) tool.ToolResult {
	profile := videoservice.BuildVideoCreationProfile(videoservice.ProfileRequest{
		Route:       stringParam(params, "route", ""),
		Deliverable: stringParam(params, "deliverable", ""),
		Brief:       firstNonEmptyString(params, "brief", "topic", "goal"),
	})
	profileMap := structToMap(profile)
	content := fmt.Sprintf("# Video Creation Profile\n\nProfile: `%s`\n\nReason: %s\n", profile.ProfileID, profile.Reason)
	return tool.SuccessResult(map[string]interface{}{
		"creationProfile": profileMap,
		"summary":         fmt.Sprintf("已选择 `%s` 创作主线。", profile.ProfileID),
		"content":         content,
		"artifacts": []map[string]interface{}{
			jsonArtifact(stage, "video_creation_profile.json", skillName, videomodel.ArtifactKindVideoCreationProfile, true),
		},
	})
}

func executeTimeWindowPlanner(stage, skillName string, params map[string]interface{}) tool.ToolResult {
	profile := creationProfileFromToolValue(params["creationProfile"])
	shots := shotUnitsFromToolValue(params["shotList"])
	spans := scriptSpansFromToolValue(params["scriptSpans"])
	plan := videoservice.BuildTimeWindowPlan(videoservice.TimeWindowRequest{
		Profile:     profile,
		Shots:       shots,
		ScriptSpans: spans,
	})
	planMap := structToMap(plan)
	windowMaps := make([]map[string]interface{}, 0, len(plan.Windows))
	for _, window := range plan.Windows {
		windowMaps = append(windowMaps, structToMap(window))
	}
	return tool.SuccessResult(map[string]interface{}{
		"timeWindowPlan": planMap,
		"timeWindows":    windowMaps,
		"summary":        fmt.Sprintf("已生成 %d 个 3-15 秒时间窗。", len(windowMaps)),
		"content":        buildTimeWindowReviewContent(windowMaps),
		"artifacts": []map[string]interface{}{
			jsonArtifact(stage, "time_window_plan.json", skillName, videomodel.ArtifactKindTimeWindowPlan, true),
		},
	})
}
```

For `visual_alignment_planner`, `cinematic_shot_designer`, and `sound_design_planner`, return deterministic reviewable artifacts that do not claim unavailable media was generated:

```go
func executeVisualAlignmentPlanner(stage, skillName string, params map[string]interface{}) tool.ToolResult {
	windows := normalizeShotItemsForAssetDecision(params["timeWindows"])
	shotList := make([]map[string]interface{}, 0, len(windows))
	for i, window := range windows {
		shotID := firstNonEmptyString(window, "shotId", "id")
		if shotID == "" {
			shotID = fmt.Sprintf("SHOT_%02d", i+1)
		}
		shotList = append(shotList, map[string]interface{}{
			"shotId":        shotID,
			"durationSec":   normalizedDurationSec(window["durationSec"]),
			"narrationText": firstNonEmptyString(window, "scriptText"),
			"visual":        "围绕该口播时间窗补充可视化素材、字幕、图表或B-roll。",
			"timeWindowId":  firstNonEmptyString(window, "id"),
		})
	}
	return tool.SuccessResult(map[string]interface{}{
		"visualAlignmentPlan": map[string]interface{}{"shotCount": len(shotList), "source": "time_window_planner"},
		"shotList":            shotList,
		"content":             buildShotListMarkdown("Visual Alignment", shotList),
		"artifacts": []map[string]interface{}{
			jsonArtifact(stage, "visual_alignment_plan.json", skillName, "VISUAL_ALIGNMENT_PLAN", true),
		},
	})
}
```

- [ ] **Step 6: Run builtin tool tests**

Run:

```bash
cd cloud-backend && go test ./internal/core/worker/tool/builtin -run 'TestVideoProfileClassifier|TestTimeWindowPlanner' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit builtin tools**

Run:

```bash
git add cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools.go cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools_test.go
git commit -m "feat: add profile and time window planner tools"
```

---

### Task 4: AIGC Request Contract and Shot Generation Inputs

**Files:**
- Modify: `cloud-backend/internal/agents/video/model/creation.go`
- Modify: `cloud-backend/internal/agents/video/service/generation_plan.go`
- Modify: `cloud-backend/internal/agents/video/service/generation_plan_test.go`
- Modify: `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools.go`
- Modify: `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools_test.go`

- [ ] **Step 1: Write failing AIGC request contract test**

Append to `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools_test.go`:

```go
func TestShotGenerationPlannerExternalRequestIncludesReferencesAndDelivery(t *testing.T) {
	result := executeLocalVideoCreationTool("shot_generation_planner", map[string]interface{}{
		"stage": "generation_strategy",
		"shotList": []interface{}{
			map[string]interface{}{
				"shotId":      "SHOT_01_TW_01",
				"durationSec": float64(8),
				"visual":      "角色在雨夜街口回头，镜头缓慢靠近",
				"referenceImages": []interface{}{
					map[string]interface{}{"role": "character_reference", "storageRef": "local://projects/p/artifacts/char/hash/char.png"},
					map[string]interface{}{"role": "scene_reference", "storageRef": "local://projects/p/artifacts/street/hash/street.png"},
				},
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
		t.Fatalf("expected one external request, got %#v", result.Data["externalGenerationRequests"])
	}
	req := requests[0]
	if intFromInterface(req["durationSec"], 0) != 8 {
		t.Fatalf("request should expose durationSec=8: %#v", req)
	}
	if req["directApiEligible"] != false {
		t.Fatalf("direct API should be false when AIGC provider unavailable: %#v", req)
	}
	if req["manualUploadRequired"] != true {
		t.Fatalf("manual upload should be required: %#v", req)
	}
	refs, ok := req["referenceImages"].([]interface{})
	if !ok || len(refs) != 2 {
		t.Fatalf("reference images should be preserved: %#v", req["referenceImages"])
	}
	if strings.TrimSpace(ensureStringValue(req["promptPackage"])) == "" {
		t.Fatalf("promptPackage should be copyable for external clients: %#v", req)
	}
}
```

- [ ] **Step 2: Run request contract test to verify it fails**

Run:

```bash
cd cloud-backend && go test ./internal/core/worker/tool/builtin -run TestShotGenerationPlannerExternalRequestIncludesReferencesAndDelivery -count=1
```

Expected: FAIL because request delivery fields are missing.

- [ ] **Step 3: Add request delivery model fields**

Append to `cloud-backend/internal/agents/video/model/creation.go`:

```go
type ExternalGenerationReference struct {
	Role       string `json:"role,omitempty"`
	StorageRef string `json:"storageRef,omitempty"`
}

type ExternalGenerationDelivery struct {
	DirectAPIEligible    bool                          `json:"directApiEligible"`
	ManualUploadRequired bool                          `json:"manualUploadRequired"`
	ReferenceImages      []ExternalGenerationReference `json:"referenceImages,omitempty"`
	PromptPackage        string                        `json:"promptPackage,omitempty"`
}
```

- [ ] **Step 4: Preserve references in shot generation values**

In `mergeShotGenerationToolValues`, include reference-related fields:

```go
for _, key := range []string{
	"visualPlan", "visual", "visualIntent", "description", "sceneSummary", "mainAction", "action",
	"screenText", "textLayers", "background", "characters", "props", "motionPlan", "cameraPlan",
	"referenceImages", "references", "timeWindowId", "parentShotId",
} {
	if _, exists := values[key]; !exists {
		values[key] = selected[key]
	}
}
```

In `visualPlanFromToolMap` or `shotUnitFromToolMap`, keep the existing visual inference, but add `RenderInputs` enrichment in `shotAssetPackageFromGenerationPlan`:

```go
if refs, ok := interfaceSliceValue(firstValueInMap(shotMap, "referenceImages", "references")); ok && len(refs) > 0 {
	planMap["referenceImages"] = refs
}
if timeWindowID := firstNonEmptyString(shotMap, "timeWindowId", "id"); timeWindowID != "" {
	planMap["timeWindowId"] = timeWindowID
}
```

- [ ] **Step 5: Enrich external generation requests**

Modify `externalRequestsFromGenerationPlan`:

```go
durationSec := intFromInterface(plan.RenderInputs["durationSec"], 0)
if durationSec == 0 && plan.FusionPlan.BaseLayer.DurationSec > 0 {
	durationSec = int(plan.FusionPlan.BaseLayer.DurationSec)
}
durationSec = normalizedDurationSec(durationSec)
request := map[string]interface{}{
	"requestId":            fmt.Sprintf("%s-%s-external-request", shotID, asset.ID),
	"shotId":               shotID,
	"relatedShotId":        shotID,
	"assetId":              asset.ID,
	"kind":                 asset.Kind,
	"role":                 asset.Role,
	"mode":                 plan.Mode,
	"reason":               plan.Reason,
	"status":               videomodel.ReviewStatusPending,
	"prompt":               strings.TrimSpace(ensureStringValue(plan.RenderInputs["prompt"])),
	"durationSec":          durationSec,
	"directApiEligible":    false,
	"manualUploadRequired": true,
	"target": map[string]interface{}{
		"durationSec": durationSec,
	},
}
if refs := interfaceSliceFromAny(plan.RenderInputs["referenceImages"]); len(refs) > 0 {
	request["referenceImages"] = refs
}
request["promptPackage"] = buildExternalPromptPackage(request)
```

Add helpers:

```go
func interfaceSliceFromAny(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		return typed
	case []map[string]interface{}:
		items := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
		return items
	default:
		return nil
	}
}

func buildExternalPromptPackage(request map[string]interface{}) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Shot: %s\n", ensureStringValue(request["shotId"]))
	fmt.Fprintf(&b, "Kind: %s\n", ensureStringValue(request["kind"]))
	fmt.Fprintf(&b, "Duration: %d seconds\n", intFromInterface(request["durationSec"], 0))
	fmt.Fprintf(&b, "Prompt:\n%s\n", ensureStringValue(request["prompt"]))
	if negative := strings.TrimSpace(ensureStringValue(request["negativePrompt"])); negative != "" {
		fmt.Fprintf(&b, "Negative prompt:\n%s\n", negative)
	}
	return b.String()
}
```

- [ ] **Step 6: Run request tests**

Run:

```bash
cd cloud-backend && go test ./internal/core/worker/tool/builtin -run 'TestShotGenerationPlanner.*External' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit AIGC request contract**

Run:

```bash
git add cloud-backend/internal/agents/video/model/creation.go cloud-backend/internal/agents/video/service/generation_plan.go cloud-backend/internal/agents/video/service/generation_plan_test.go cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools.go cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools_test.go
git commit -m "feat: enrich aigc generation request contracts"
```

---

### Task 5: Profile-Aware DAG Completion

**Files:**
- Modify: `cloud-backend/internal/core/agentruntime/plan_compiler.go`
- Modify: `cloud-backend/internal/core/agentruntime/plan_compiler_test.go`

- [ ] **Step 1: Write failing talking-head compiler test**

Append to `cloud-backend/internal/core/agentruntime/plan_compiler_test.go`:

```go
func TestPlanCompiler_PreparePlanUsesTalkingHeadProfileTemplate(t *testing.T) {
	catalog := videoProfileTemplateCatalog()
	compiler := NewPlanCompiler(catalog)
	plan := &AgentPlan{
		Goal:   "做一期60秒口播知识视频，讲AI工作流",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{ID: "script_generation", Tool: "video_script_generator", ExpectedOutput: []string{"script"}, ProduceArtifact: true},
		},
	}

	prepared := compiler.PreparePlan(plan)

	assertStepOrder(t, prepared, []string{
		"profile_selection",
		"script_generation",
		"time_window",
		"visual_alignment",
		"shot_generation",
		"video_prompt",
		"preview",
		"render",
	})
	timeWindow := findStep(t, prepared, "time_window")
	if got := timeWindow.Arguments["creationProfile"]; got != "{{profile_selection.output.creationProfile}}" {
		t.Fatalf("time_window should consume profile: %#v", timeWindow.Arguments)
	}
	visual := findStep(t, prepared, "visual_alignment")
	if got := visual.Arguments["timeWindows"]; got != "{{time_window.output.timeWindows}}" {
		t.Fatalf("visual_alignment should consume time windows: %#v", visual.Arguments)
	}
}
```

- [ ] **Step 2: Write failing cinematic compiler test**

Append:

```go
func TestPlanCompiler_PreparePlanUsesCinematicProfileTemplate(t *testing.T) {
	catalog := videoProfileTemplateCatalog()
	compiler := NewPlanCompiler(catalog)
	plan := &AgentPlan{
		Goal:   "做一个有角色、场景和道具连续性的影视短片",
		Domain: "video_creation",
		Mode:   "dynamic_agent",
		Steps: []AgentStep{
			{ID: "story_foundation", Tool: "proposal_generator", ExpectedOutput: []string{"storyBrief"}, ProduceArtifact: true},
		},
	}

	prepared := compiler.PreparePlan(plan)

	assertStepOrder(t, prepared, []string{
		"profile_selection",
		"story_foundation",
		"cinematic_script",
		"continuity_bible",
		"reference_assets",
		"cinematic_shot_design",
		"time_window",
		"keyframes_storyboards",
		"shot_generation",
	})
	timeWindow := findStep(t, prepared, "time_window")
	if got := timeWindow.Arguments["shotList"]; got != "{{cinematic_shot_design.output.shotList}}" {
		t.Fatalf("cinematic time windows should consume coarse shot design: %#v", timeWindow.Arguments)
	}
	shotGeneration := findStep(t, prepared, "shot_generation")
	if got := shotGeneration.Arguments["timeWindows"]; got != "{{time_window.output.timeWindows}}" {
		t.Fatalf("shot_generation should consume time windows: %#v", shotGeneration.Arguments)
	}
}
```

Add test helpers:

```go
func videoProfileTemplateCatalog() staticToolCatalog {
	catalog := staticToolCatalog{
		"video_profile_classifier": {Name: "video_profile_classifier", Output: map[string]tool.ParamDef{"creationProfile": {Type: "object"}}},
		"time_window_planner":      {Name: "time_window_planner", Parameters: map[string]tool.ParamDef{"creationProfile": {Type: "object"}}, Output: map[string]tool.ParamDef{"timeWindows": {Type: "array"}, "timeWindowPlan": {Type: "object"}}},
		"visual_alignment_planner": {Name: "visual_alignment_planner", Parameters: map[string]tool.ParamDef{"timeWindows": {Type: "array"}}, Output: map[string]tool.ParamDef{"shotList": {Type: "array"}, "visualAlignmentPlan": {Type: "object"}}},
		"cinematic_shot_designer":  {Name: "cinematic_shot_designer", Output: map[string]tool.ParamDef{"shotList": {Type: "array"}, "directorDesign": {Type: "object"}}},
		"sound_design_planner":     {Name: "sound_design_planner", Output: map[string]tool.ParamDef{"soundDesignPlan": {Type: "object"}}},
	}
	for k, v := range videoBetaCompletionCatalog() {
		catalog[k] = v
	}
	catalog["keyframe_prompt_generator"] = tool.ToolManifest{Name: "keyframe_prompt_generator", Output: map[string]tool.ParamDef{"keyframePrompts": {Type: "array"}}}
	catalog["reference_asset_planner"] = tool.ToolManifest{Name: "reference_asset_planner", Output: map[string]tool.ParamDef{"referenceAssetPlan": {Type: "object"}}}
	catalog["continuity_checker"] = tool.ToolManifest{Name: "continuity_checker", Output: map[string]tool.ParamDef{"continuityBible": {Type: "object"}}}
	return catalog
}
```

- [ ] **Step 3: Run compiler tests to verify they fail**

Run:

```bash
cd cloud-backend && go test ./internal/core/agentruntime -run 'TestPlanCompiler_PreparePlanUses.*ProfileTemplate' -count=1
```

Expected: FAIL because profile-aware completion does not exist.

- [ ] **Step 4: Add profile-aware dispatcher**

Modify `PreparePlan`:

```go
func (c *PlanCompiler) PreparePlan(plan *AgentPlan) *AgentPlan {
	if plan == nil {
		return nil
	}
	c.injectKnowledgeContext(plan)
	if !c.completeVideoPlanByProfile(plan) {
		c.completeVideoBetaPlan(plan)
	}
	repairInvalidOutputReferences(plan.Steps, c.manifestsByPlan(plan))
	c.expandPreparedPlanBudget(plan)
	return plan
}
```

Add:

```go
func (c *PlanCompiler) completeVideoPlanByProfile(plan *AgentPlan) bool {
	if plan == nil || plan.Domain != "video_creation" {
		return false
	}
	if c.manifestFor("video_profile_classifier") == nil || c.manifestFor("time_window_planner") == nil {
		return false
	}
	profile := inferVideoCreationProfile(plan.Goal)
	profileAnchor := c.ensureProfileSelectionStep(plan, profile)
	if profile == "cinematic_story" {
		return c.completeCinematicStoryPlan(plan, profileAnchor)
	}
	return c.completeTalkingHeadPlan(plan, profileAnchor)
}

func inferVideoCreationProfile(goal string) string {
	lower := strings.ToLower(goal)
	if strings.Contains(lower, "影视") || strings.Contains(lower, "剧情") || strings.Contains(lower, "角色") || strings.Contains(lower, "场景") || strings.Contains(lower, "道具") || strings.Contains(lower, "导演") || strings.Contains(lower, "短片") {
		return "cinematic_story"
	}
	return "talking_head"
}
```

- [ ] **Step 5: Add profile selection and dependency helpers**

Add:

```go
func (c *PlanCompiler) ensureProfileSelectionStep(plan *AgentPlan, profile string) string {
	if existing, _ := c.lastProducerStepForFields(plan, []string{"creationProfile"}, []string{"video_profile_classifier"}); existing != "" {
		return existing
	}
	step := AgentStep{
		ID:     uniqueStepID(plan, "profile_selection"),
		Intent: "选择口播或影视创作主线",
		Tool:   "video_profile_classifier",
		Arguments: map[string]interface{}{
			"stage": "profile_selection",
			"brief": plan.Goal,
			"route": profile,
		},
		ExpectedOutput:  []string{"creationProfile"},
		ProduceArtifact: true,
	}
	plan.Steps = append([]AgentStep{step}, plan.Steps...)
	return step.ID
}
```

When a downstream step should use the profile, set:

```go
step.Arguments["creationProfile"] = stepOutputRef(profileAnchor, "creationProfile")
appendDependencyIfMissing(step, profileAnchor)
```

- [ ] **Step 6: Add talking-head template completion**

Add `completeTalkingHeadPlan` that:

1. Finds or keeps `video_script_generator`.
2. Inserts `time_window_planner` after script generation.
3. Inserts `visual_alignment_planner` after time-window planning.
4. Sets the downstream shot anchor to `visual_alignment.output.shotList`.
5. Reuses the existing `shot_generation_planner`, `video_prompt_generator`, `hyperframes_project_generator`, `hyperframes_renderer`, and `publish_copy_generator` logic.

Core insertion code:

```go
timeWindowAnchor := insertPlanStepAfter(plan, scriptAnchor, AgentStep{
	ID:        uniqueStepID(plan, "time_window"),
	Intent:    "按口播稿时间轴规划素材时间窗",
	Tool:      "time_window_planner",
	DependsOn: dependencyListUnique(profileAnchor, scriptAnchor),
	Arguments: map[string]interface{}{
		"stage":           "time_window",
		"creationProfile": stepOutputRef(profileAnchor, "creationProfile"),
		"scriptSpans":     stepOutputRef(scriptAnchor, scriptField),
	},
	ExpectedOutput:  []string{"timeWindowPlan", "timeWindows"},
	ProduceArtifact: true,
})
visualAnchor := insertPlanStepAfter(plan, timeWindowAnchor, AgentStep{
	ID:        uniqueStepID(plan, "visual_alignment"),
	Intent:    "按口播时间窗补齐画面、素材、字幕和AIGC需求",
	Tool:      "visual_alignment_planner",
	DependsOn: dependencyListUnique(profileAnchor, scriptAnchor, timeWindowAnchor),
	Arguments: map[string]interface{}{
		"stage":           "visual_alignment",
		"creationProfile": stepOutputRef(profileAnchor, "creationProfile"),
		"script":          stepOutputRef(scriptAnchor, scriptField),
		"timeWindows":     stepOutputRef(timeWindowAnchor, "timeWindows"),
	},
	ExpectedOutput:  []string{"visualAlignmentPlan", "shotList"},
	ProduceArtifact: true,
})
```

- [ ] **Step 7: Add cinematic template completion**

Add `completeCinematicStoryPlan` that inserts or reuses:

```text
profile_selection -> story_foundation -> cinematic_script -> continuity_bible -> reference_assets -> cinematic_shot_design -> time_window -> keyframes_storyboards -> shot_generation
```

Use existing generic tools where full cinematic tools do not exist:

- `proposal_generator` for `story_foundation`
- `video_script_generator` for `cinematic_script`, with `creationProfile`
- `continuity_checker` for `continuity_bible`
- `reference_asset_planner` for `reference_assets`
- `cinematic_shot_designer` for `cinematic_shot_design`
- `time_window_planner` for 3-15 second windows
- `keyframe_prompt_generator` for keyframe prompts

- [ ] **Step 8: Run compiler tests**

Run:

```bash
cd cloud-backend && go test ./internal/core/agentruntime -run 'TestPlanCompiler_PreparePlanUses.*ProfileTemplate|TestPlanCompiler_PreparePlanInsertsShotGenerationPlanner' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit profile-aware compiler**

Run:

```bash
git add cloud-backend/internal/core/agentruntime/plan_compiler.go cloud-backend/internal/core/agentruntime/plan_compiler_test.go
git commit -m "feat: route video plans by creation profile"
```

---

### Task 6: Frontend Review Surface for Profiles and Time Windows

**Files:**
- Modify: `frontend/src/pages/directorStudioLogic.ts`
- Modify: `frontend/scripts/director-studio-logic-check.mjs`

- [ ] **Step 1: Write failing frontend logic assertions**

In `frontend/scripts/director-studio-logic-check.mjs`, add a fixture artifact:

```js
const profileArtifacts = [
  {
    id: 'profile-1',
    kind: 'VIDEO_CREATION_PROFILE',
    name: '创作主线',
    status: 'valid',
    owner: '创作路由',
    version: '第1版',
    updatedAt: '-',
    humanApproved: true,
    storageRef: 'inline://profile',
    metadata: {
      profileId: 'cinematic_story',
      primaryArtifact: 'CONTINUITY_BIBLE',
      qualityContract: ['aigc_time_windows_3_15s'],
    },
  },
  {
    id: 'time-window-1',
    kind: 'TIME_WINDOW_PLAN',
    name: '时间窗计划',
    status: 'review',
    owner: '时间窗规划',
    version: '第1版',
    updatedAt: '-',
    humanApproved: false,
    storageRef: 'inline://time-window',
    metadata: {
      windows: [
        { id: 'SHOT_01_TW_01', shotId: 'SHOT_01_TW_01', parentShotId: 'SHOT_01', durationSec: 8, aigcEligible: true },
      ],
    },
  },
]
```

Then assert exported helpers:

```js
const profileSummary = creationProfileSummary(profileArtifacts)
assert.equal(profileSummary.profileId, 'cinematic_story')
assert.equal(profileSummary.label, '影视剧情')
const windowSummary = timeWindowPlanSummary(profileArtifacts)
assert.equal(windowSummary.totalWindows, 1)
assert.equal(windowSummary.aigcWindowCount, 1)
```

- [ ] **Step 2: Run frontend logic test to verify it fails**

Run:

```bash
cd frontend && npm run test:director
```

Expected: FAIL because `creationProfileSummary` and `timeWindowPlanSummary` are missing.

- [ ] **Step 3: Add frontend helper types and functions**

In `frontend/src/pages/directorStudioLogic.ts`, export:

```ts
export interface DirectorCreationProfileSummary {
  profileId?: string
  label: string
  primaryArtifact?: string
  qualityContract: string[]
}

export interface DirectorTimeWindowSummary {
  totalWindows: number
  aigcWindowCount: number
  invalidDurationCount: number
}

export function creationProfileSummary(artifacts: DirectorArtifactRecord[]): DirectorCreationProfileSummary {
  const artifact = artifacts.find((item) => item.kind === 'VIDEO_CREATION_PROFILE')
  const metadata = artifact?.metadata || {}
  const profileId = stringValue(metadata.profileId)
  return {
    profileId,
    label: profileId === 'cinematic_story' ? '影视剧情' : profileId === 'talking_head' ? '口播解说' : '未选择',
    primaryArtifact: stringValue(metadata.primaryArtifact) || undefined,
    qualityContract: normalizeStringList(metadata.qualityContract),
  }
}

export function timeWindowPlanSummary(artifacts: DirectorArtifactRecord[]): DirectorTimeWindowSummary {
  const artifact = artifacts.find((item) => item.kind === 'TIME_WINDOW_PLAN')
  const windows = Array.isArray(artifact?.metadata?.windows) ? artifact?.metadata?.windows as unknown[] : []
  let aigcWindowCount = 0
  let invalidDurationCount = 0
  for (const item of windows) {
    const record = objectValue(item)
    if (!record) continue
    const duration = numberValue(record.durationSec) || 0
    if (booleanValue(record.aigcEligible)) aigcWindowCount += 1
    if (duration > 0 && (duration < 3 || duration > 15)) invalidDurationCount += 1
  }
  return { totalWindows: windows.length, aigcWindowCount, invalidDurationCount }
}
```

- [ ] **Step 4: Import helpers in director logic check**

Add to the import destructuring:

```js
creationProfileSummary,
timeWindowPlanSummary,
```

- [ ] **Step 5: Run frontend tests**

Run:

```bash
cd frontend && npm run test:director && npm run build
```

Expected: PASS.

- [ ] **Step 6: Commit frontend review logic**

Run:

```bash
git add frontend/src/pages/directorStudioLogic.ts frontend/scripts/director-studio-logic-check.mjs
git commit -m "feat: surface video profile timing review data"
```

---

### Task 7: Full Verification and E2E Smoke

**Files:**
- No committed source files unless tests expose a gap.
- Temporary E2E files may live under `tmp/video-type-dual-pipeline-e2e/`.

- [ ] **Step 1: Run full backend and frontend verification**

Run:

```bash
cd cloud-backend && go test ./...
cd ../local-backend && go test ./...
cd ../frontend && npm run test:director && npm run build
cd ../hyperframes-render-service && npm run build
```

Expected: every command exits 0.

- [ ] **Step 2: Run talking-head compiler smoke**

Use a focused Go test from Task 5:

```bash
cd cloud-backend && go test ./internal/core/agentruntime -run TestPlanCompiler_PreparePlanUsesTalkingHeadProfileTemplate -count=1
```

Expected: PASS and plan includes `profile_selection`, `time_window`, `visual_alignment`, `shot_generation`, `preview`, and `render`.

- [ ] **Step 3: Run cinematic compiler smoke**

Run:

```bash
cd cloud-backend && go test ./internal/core/agentruntime -run TestPlanCompiler_PreparePlanUsesCinematicProfileTemplate -count=1
```

Expected: PASS and plan includes `continuity_bible`, `reference_assets`, `cinematic_shot_design`, `time_window`, `keyframes_storyboards`, and `shot_generation`.

- [ ] **Step 4: Run external request smoke**

Run:

```bash
cd cloud-backend && go test ./internal/core/worker/tool/builtin -run TestShotGenerationPlannerExternalRequestIncludesReferencesAndDelivery -count=1
```

Expected: PASS and request contains `durationSec`, `referenceImages`, `promptPackage`, `directApiEligible`, and `manualUploadRequired`.

- [ ] **Step 5: Browser E2E**

Create a temporary talking-head preview using the existing local HyperFrames path. The preview must contain:

- `VIDEO_CREATION_PROFILE` for `talking_head`.
- `TIME_WINDOW_PLAN` with 3-15 second windows.
- `SHOT_GENERATION_PLAN` with at least one HyperFrames segment and one AIGC/external request segment.
- Final local MP4 loaded by client `<video>`.

Use the existing browser plugin flow from the prior E2E:

```text
Open http://127.0.0.1:<client-port>/index.html
Verify videoWidth = 1920, videoHeight = 1080, duration > 0, readyState >= 2.
Trigger playback with Space and verify currentTime advances.
```

Expected: playable final video in the user client.

- [ ] **Step 6: Commit verification fixes if needed**

If verification exposes source changes, commit only those changes:

```bash
git status --short
git add <changed-source-files>
git commit -m "fix: stabilize dual pipeline verification"
```

- [ ] **Step 7: Push develop_go**

Run:

```bash
git push develop_go develop_go
```

Expected: remote `develop_go` advances to the final implementation commit.

---

## Self-Review Checklist

- Spec coverage:
  - `VideoCreationProfile` is covered by Task 1 and Task 3.
  - Talking-head script-timed visual alignment is covered by Task 2, Task 3, and Task 5.
  - Cinematic coarse-to-fine splitting is covered by Task 2 and Task 5.
  - AIGC 3-15 second generation limit is covered by Task 2, Task 4, and Task 7.
  - Direct API/manual client generation with references is covered by Task 4.
  - Frontend review visibility is covered by Task 6.
- Type consistency:
  - `VideoCreationProfile`, `TimeWindowPlan`, `TimeWindowUnit`, and `ScriptSpan` are defined before tool and compiler tasks use them.
  - `timeWindows` is the array output passed from `time_window_planner` to `visual_alignment_planner` and `shot_generation_planner`.
  - `creationProfile` is the object output passed from `video_profile_classifier` into profile-aware stages.
- Verification:
  - Every task has a focused test command.
  - Task 7 includes full repo verification and browser playback.
