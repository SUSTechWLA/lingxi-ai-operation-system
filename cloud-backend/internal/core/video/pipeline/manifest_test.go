package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDirectoryLoadsAndValidatesKnowledgePipeline(t *testing.T) {
	root := t.TempDir()
	writePipelineFile(t, filepath.Join(root, "knowledge-video.yaml"), knowledgePipelineYAML)

	registry, err := LoadDirectory(root)
	if err != nil {
		t.Fatalf("LoadDirectory returned error: %v", err)
	}

	manifest, ok := registry.Get("knowledge-video")
	if !ok {
		t.Fatalf("knowledge-video pipeline was not loaded")
	}
	if manifest.Name != "知识口播视频" {
		t.Fatalf("pipeline name not loaded: %#v", manifest)
	}
	if len(manifest.Stages) != 10 {
		t.Fatalf("expected 10 stages, got %d", len(manifest.Stages))
	}
	if manifest.Stages[1].ID != "proposal" || !manifest.Stages[1].ReviewRequired {
		t.Fatalf("proposal stage should be the first review gate: %#v", manifest.Stages[1])
	}
	if manifest.Stages[4].ID != "render_strategy" || manifest.Stages[4].Produces != ArtifactRenderStrategy {
		t.Fatalf("render_strategy stage not loaded correctly: %#v", manifest.Stages[4])
	}
}

func TestSelectorChoosesKnowledgeVideoForBriefOnlyKnowledgeRequest(t *testing.T) {
	root := t.TempDir()
	writePipelineFile(t, filepath.Join(root, "knowledge-video.yaml"), knowledgePipelineYAML)
	registry, err := LoadDirectory(root)
	if err != nil {
		t.Fatalf("LoadDirectory returned error: %v", err)
	}

	selection, err := registry.Select(SelectionRequest{
		Message: "请帮我根据端午节的来历创作一个 60 秒知识分享视频",
		Inputs:  []InputKind{InputBrief},
	})
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}

	if selection.PipelineID != "knowledge-video" {
		t.Fatalf("expected knowledge-video, got %#v", selection)
	}
	if selection.Confidence < 0.8 {
		t.Fatalf("expected high confidence selection, got %#v", selection)
	}
	if selection.FirstApprovalStage != "proposal" {
		t.Fatalf("proposal should be the first approval stage: %#v", selection)
	}
}

func TestBuildProposalPacketCreatesReviewableOptions(t *testing.T) {
	manifest := mustPipeline(t, knowledgePipelineYAML)

	packet := BuildProposalPacket(ProposalRequest{
		Pipeline:          manifest,
		Message:           "请帮我根据端午节的来历创作一个 60 秒知识分享视频",
		TargetDurationSec: 60,
		Capabilities: CapabilitySnapshot{
			TextModelAvailable:   true,
			HyperFramesAvailable: true,
			SeedanceAvailable:    true,
		},
	})

	if packet.ArtifactKind != ArtifactProposalPacket {
		t.Fatalf("unexpected artifact kind: %#v", packet)
	}
	if len(packet.Options) != 3 {
		t.Fatalf("expected three proposal options, got %#v", packet.Options)
	}
	if packet.RecommendedOptionID != "option_b" {
		t.Fatalf("expected hybrid recommendation when Seedance is available: %#v", packet)
	}
	if !packet.RequiresApproval || packet.DecisionLog.DecisionType != DecisionProposalSelection {
		t.Fatalf("proposal must require approval and carry a decision log: %#v", packet)
	}
}

func TestBuildRenderStrategyAssignsEnginesAndDecisionLog(t *testing.T) {
	strategy := BuildRenderStrategy(RenderStrategyRequest{
		Shots: []ShotPlan{
			{
				ShotID:      "SHOT_01",
				DurationSec: 5,
				Elements: []VisualElement{
					{ID: "title", Type: "text", RequiresExactText: true},
				},
			},
			{
				ShotID:      "SHOT_02",
				DurationSec: 8,
				Elements: []VisualElement{
					{ID: "dragon_boat_background", Type: "natural_motion", RequiresComplexMotion: true},
					{ID: "caption", Type: "text", RequiresExactText: true},
				},
			},
		},
		Capabilities: CapabilitySnapshot{
			HyperFramesAvailable: true,
			SeedanceAvailable:    true,
		},
	})

	if strategy.ArtifactKind != ArtifactRenderStrategy || strategy.OverallMode != RenderModeHybrid {
		t.Fatalf("unexpected render strategy summary: %#v", strategy)
	}
	if strategy.Shots[0].Engine != EngineHyperFrames {
		t.Fatalf("exact text shot should use HyperFrames: %#v", strategy.Shots[0])
	}
	if strategy.Shots[1].Engine != EngineHybrid {
		t.Fatalf("complex motion plus exact text should use Hybrid: %#v", strategy.Shots[1])
	}
	if strategy.DecisionLog.DecisionType != DecisionRenderStrategySelection || len(strategy.DecisionLog.OptionsConsidered) != 3 {
		t.Fatalf("render strategy should include decision log options: %#v", strategy.DecisionLog)
	}
}

func TestLoadDirectoryRejectsDuplicateStages(t *testing.T) {
	root := t.TempDir()
	writePipelineFile(t, filepath.Join(root, "broken.yaml"), `
id: broken
name: Broken
stages:
  - id: brief
    director: brief_director
    produces: video_brief
  - id: brief
    director: other_director
    produces: script
`)

	if _, err := LoadDirectory(root); err == nil {
		t.Fatalf("expected duplicate stage validation error")
	}
}

func mustPipeline(t *testing.T, content string) *Manifest {
	t.Helper()
	root := t.TempDir()
	writePipelineFile(t, filepath.Join(root, "pipeline.yaml"), content)
	registry, err := LoadDirectory(root)
	if err != nil {
		t.Fatalf("LoadDirectory returned error: %v", err)
	}
	manifest, ok := registry.Get("knowledge-video")
	if !ok {
		t.Fatalf("pipeline not loaded")
	}
	return manifest
}

func writePipelineFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

const knowledgePipelineYAML = `
id: knowledge-video
name: 知识口播视频
description: 适合从一句话生成知识分享类视频，包含脚本、分镜、字幕、图文组件和最终 MP4。
inputKinds:
  - brief
keywords:
  - 知识
  - 科普
  - 口播
  - 分享
stages:
  - id: brief
    director: brief_director
    produces: video_brief
    reviewRequired: false
  - id: proposal
    director: proposal_director
    produces: proposal_packet
    reviewRequired: true
  - id: script
    director: script_director
    produces: script
    reviewRequired: true
  - id: visual_plan
    director: visual_plan_director
    produces: visual_plan
    reviewRequired: true
  - id: render_strategy
    director: render_strategy_director
    produces: render_strategy
    reviewRequired: true
  - id: assets
    director: asset_director
    produces: asset_manifest
    reviewRequired: false
  - id: edit
    director: edit_director
    produces: video_composition_spec
    reviewRequired: true
  - id: compose
    director: compose_director
    produces: render_report
    reviewRequired: false
  - id: final_review
    director: final_review_director
    produces: final_review
    reviewRequired: false
  - id: publish
    director: publish_director
    produces: publish_package
    reviewRequired: true
`
