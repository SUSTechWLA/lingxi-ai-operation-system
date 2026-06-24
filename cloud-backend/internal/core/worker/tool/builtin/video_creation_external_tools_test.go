package builtin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

func TestRegisterVideoCreationExternalToolsInstallsOpinionVideoDependencies(t *testing.T) {
	registry := tool.NewToolRegistry()
	RegisterVideoCreationExternalTools(registry)

	for _, name := range []string{
		"skill_stage_agent",
		"image_asset_generator",
		"hyperframes_project_builder",
		"hyperframes_renderer",
		"hypergen_keyframes",
	} {
		if registry.GetExternalManifest(name) == nil {
			t.Fatalf("expected default video tool %q to be registered", name)
		}
	}
}

func TestRegisterVideoCreationExternalToolsInstallsVideoForgeDependencies(t *testing.T) {
	registry := tool.NewToolRegistry()
	RegisterVideoCreationExternalTools(registry)

	for _, name := range []string{
		"pipeline_selector",
		"capability_preflight",
		"proposal_generator",
		"visual_feasibility_analyzer",
		"render_strategy_planner",
	} {
		if registry.GetExternalManifest(name) == nil {
			t.Fatalf("expected VideoForge tool %q to be registered", name)
		}
	}
}

func TestExecutePipelineSelectorReturnsProposalFirstSelection(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "knowledge-video.yaml"), `
id: knowledge-video
name: 知识口播视频
inputKinds:
  - brief
keywords:
  - 知识
  - 分享
stages:
  - id: brief
    director: brief_director
    produces: video_brief
  - id: proposal
    director: proposal_director
    produces: proposal_packet
    reviewRequired: true
`)

	result := executeLocalVideoCreationTool("pipeline_selector", map[string]interface{}{
		"brief":        "请帮我根据端午节的来历创作一个 60 秒知识分享视频",
		"pipelineRoot": root,
	}, tool.ToolContext{TaskID: "task-1", NodeID: "node-1"})

	if !result.Success {
		t.Fatalf("pipeline_selector failed: %s", result.Error)
	}
	selection, ok := result.Data["pipelineSelection"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing pipelineSelection: %#v", result.Data)
	}
	if selection["pipelineId"] != "knowledge-video" || selection["firstApprovalStage"] != "proposal" {
		t.Fatalf("unexpected selection: %#v", selection)
	}
}

func TestExecuteProposalGeneratorReturnsDecisionLoggedPacket(t *testing.T) {
	result := executeLocalVideoCreationTool("proposal_generator", map[string]interface{}{
		"brief":             "请帮我根据端午节的来历创作一个 60 秒知识分享视频",
		"targetDurationSec": float64(60),
		"seedanceAvailable": true,
	}, tool.ToolContext{TaskID: "task-1", NodeID: "node-1"})

	if !result.Success {
		t.Fatalf("proposal_generator failed: %s", result.Error)
	}
	packet, ok := result.Data["proposalPacket"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing proposalPacket: %#v", result.Data)
	}
	if packet["artifactKind"] != "proposal_packet" || packet["recommendedOptionId"] != "option_b" {
		t.Fatalf("unexpected proposal packet: %#v", packet)
	}
	if result.Data["decisionLog"] == nil {
		t.Fatalf("proposal_generator should expose decisionLog: %#v", result.Data)
	}
}

func TestExecuteRenderStrategyPlannerReturnsHybridStrategy(t *testing.T) {
	result := executeLocalVideoCreationTool("render_strategy_planner", map[string]interface{}{
		"shots": []interface{}{
			map[string]interface{}{
				"shotId": "SHOT_01",
				"elements": []interface{}{
					map[string]interface{}{"id": "title", "type": "text", "requiresExactText": true},
				},
			},
			map[string]interface{}{
				"shotId": "SHOT_02",
				"elements": []interface{}{
					map[string]interface{}{"id": "background", "type": "natural_motion", "requiresComplexMotion": true},
					map[string]interface{}{"id": "caption", "type": "text", "requiresExactText": true},
				},
			},
		},
		"seedanceAvailable": true,
	}, tool.ToolContext{TaskID: "task-1", NodeID: "node-1"})

	if !result.Success {
		t.Fatalf("render_strategy_planner failed: %s", result.Error)
	}
	strategy, ok := result.Data["renderStrategy"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing renderStrategy: %#v", result.Data)
	}
	if strategy["artifactKind"] != "render_strategy" || strategy["overallMode"] != "hybrid" {
		t.Fatalf("unexpected render strategy: %#v", strategy)
	}
	if result.Data["decisionLog"] == nil {
		t.Fatalf("render_strategy_planner should expose decisionLog: %#v", result.Data)
	}
}

func TestBuildSkillStageArtifactsDoesNotEmitPublishCopyForNonPublishStage(t *testing.T) {
	artifacts := buildSkillStageArtifacts("viewpoint_dossier", "create-opinion-videos", false, true)
	for _, artifact := range artifacts {
		if artifact["unitId"] == "publish-copy" {
			t.Fatalf("non-publish stages should not emit publish-copy artifacts: %+v", artifact)
		}
	}
}

func TestBuildSkillStageArtifactsEmitsPublishCopyForPublishPackageStage(t *testing.T) {
	artifacts := buildSkillStageArtifacts("publish_package", "create-opinion-videos", true, true)
	found := false
	for _, artifact := range artifacts {
		if artifact["unitId"] == "publish-copy" {
			found = true
		}
	}
	if !found {
		t.Fatalf("publish_package stage should emit a publish-copy artifact: %+v", artifacts)
	}
}

func TestSkillStageContinuationDetectsLengthFinishReason(t *testing.T) {
	if !needsSkillStageContinuation("hyperframes_reference", "## BEAT 03\n证据", "length") {
		t.Fatal("length finish reason should request continuation")
	}
}

func TestSkillStageContinuationDetectsIncompleteHyperframesReference(t *testing.T) {
	partial := `# HyperFrames 视频参考

### 四、逐段画面脚本

## BEAT 03｜防疫实证
**参考时长**：20—36秒

### 口播
证据一：挂艾草、菖蒲。这两种植物在古代就是天然驱虫剂。证据`

	if !needsSkillStageContinuation("hyperframes_reference", partial, "stop") {
		t.Fatal("hyperframes_reference missing later chapters and ending mid-sentence should request continuation")
	}
}

func TestSkillStageContinuationDoesNotTriggerForCompleteHyperframesReference(t *testing.T) {
	complete := `# HyperFrames 视频参考

### 一、观点档案
ok
### 二、视频总体设定
ok
### 三、重要制作原则
ok
### 四、逐段画面脚本
## BEAT 01｜开场
### 转场
ok
### 五、imagegen 图片清单
ok
### 六、图片在视频中的处理方式
ok
### 七、字幕和文字规则
ok
### 八、整体节奏控制
ok
### 九、推荐项目素材目录
ok。`

	if needsSkillStageContinuation("hyperframes_reference", complete, "stop") {
		t.Fatal("complete hyperframes_reference should not request continuation")
	}
}

func TestAppendContinuationContinuesMidSentenceInline(t *testing.T) {
	got := appendContinuation("证据一：挂艾草、菖蒲。这两种植物在古代就是天然驱虫剂。证据", "二：雄黄酒。")
	want := "证据一：挂艾草、菖蒲。这两种植物在古代就是天然驱虫剂。证据二：雄黄酒。"
	if got != want {
		t.Fatalf("unexpected continuation join:\ngot  %q\nwant %q", got, want)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
