package builtin

import (
	"fmt"

	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

var videoCreationExternalTools = []string{
	"skill_stage_agent",
	"image_asset_generator",
	"hyperframes_project_builder",
	"hyperframes_renderer",
	"material_library_matcher",
	"video_keyframe_prompt_builder",
	"storyboard_assembler",
	"text_image_to_video_generator",
	"video_final_assembler",
	"video_shot_extractor",
	"shot_motion_analyzer",
	"asset_guard",
	"material_library_importer",
	"voice_post_process",
	"audio_artifact_packager",
}

// RegisterVideoCreationExternalTools registers local tool manifests for video
// skills while provider APIs are not connected. They return reviewable assets
// through the same external-tool bridge used by real HTTP tools.
func RegisterVideoCreationExternalTools(registry *tool.ToolRegistry) {
	for _, name := range videoCreationExternalTools {
		registry.RegisterExternal(&tool.ToolManifest{
			Name:        name,
			Description: "Local video creation tool for reviewable intermediate assets",
			Version:     "local-1.0.0",
			Type:        "builtin",
			Endpoint:    "builtin://video-creation/" + name,
			Timeout:     60,
			Parameters: map[string]tool.ParamDef{
				"skill_name":      {Type: "string", Description: "Skill name", Required: false},
				"skill_version":   {Type: "string", Description: "Skill version", Required: false},
				"stage":           {Type: "string", Description: "Workflow stage", Required: false},
				"instruction_ref": {Type: "string", Description: "Stage instruction reference", Required: false},
			},
			Output: map[string]tool.ParamDef{
				"content":   {Type: "string", Description: "Human-reviewable markdown content"},
				"artifacts": {Type: "object", Description: "Structured intermediate assets"},
			},
			Sandbox: false,
		})
	}
}

func executeLocalVideoCreationTool(toolName string, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	stage := stringParam(params, "stage", toolName)
	skillName := stringParam(params, "skill_name", "video-skill")
	brief := stringParam(params, "brief", "")
	instructionRef := stringParam(params, "instruction_ref", "")

	content := fmt.Sprintf(
		"## %s\n\nSkill: %s\nTask: %s\nBrief: %s\nInstruction: %s\n\n该阶段已生成可审核中间态。当前视频生成 API 未接入，输出为可导入网页工具的素材包和提示词。",
		stage,
		skillName,
		toolCtx.TaskID,
		brief,
		instructionRef,
	)

	data := map[string]interface{}{
		"content": content,
		"artifacts": map[string]interface{}{
			"stage":           stage,
			"skillName":       skillName,
			"instructionRef":  instructionRef,
			"source":          "local-video-creation-tool",
			"requiresReview":  true,
			"canReviseByChat": true,
		},
	}

	switch toolName {
	case "image_asset_generator":
		data["imageRequests"] = []map[string]interface{}{
			{
				"provider": "codex_imagegen",
				"prompt":   fmt.Sprintf("为「%s」生成 9:16 自媒体视频关键参考帧，画面清晰，主体明确，可用于图生视频。", brief),
				"stage":    stage,
			},
		}
	case "hyperframes_project_builder", "storyboard_assembler":
		data["projectFiles"] = []string{"storyboard.md", "shot-list.md", "publish-pack.md"}
	case "hyperframes_renderer", "text_image_to_video_generator", "video_final_assembler":
		data["videoImportPackage"] = map[string]interface{}{
			"videoPrompt":       fmt.Sprintf("基于「%s」生成竖屏短视频，节奏紧凑，保留口播信息密度。", brief),
			"referenceFrames":   []string{"使用 imageRequests 生成的关键帧作为首帧/风格参考"},
			"negativePrompt":    "低清晰度、字幕遮挡、人物畸变、口型错位",
			"finalDeliverables": []string{"video.mp4", "title.txt", "description.md", "keywords.json"},
		}
	case "skill_stage_agent":
		data["publishCopy"] = map[string]interface{}{
			"title":       "待优化标题",
			"description": "基于口播内容生成的发布简介草稿。",
			"keywords":    []string{"口播", "知识视频", "自媒体"},
		}
	}

	return tool.SuccessResult(data)
}

func stringParam(params map[string]interface{}, key string, fallback string) string {
	if value, ok := params[key].(string); ok && value != "" {
		return value
	}
	return fallback
}
