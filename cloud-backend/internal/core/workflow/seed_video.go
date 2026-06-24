package workflow

// GuidedImageTextVideoWorkflow returns the default workflow template for
// guided image-text video creation. Unlike the one-click variant, every
// creative stage requires human approval before the next stage can begin.
//
// Pipeline: proposal → review → script → review → composition → review →
//
//	preview → review → render → package → final_review
func GuidedImageTextVideoWorkflow() struct {
	ID, Name, Desc, Cat, DAG string
} {
	return struct {
		ID, Name, Desc, Cat, DAG string
	}{
		ID:   "wf-guided-image-text-video",
		Name: "引导式图文视频",
		Desc: "一句话启动项目 → 确认方案 → 确认脚本 → 确认结构 → 确认预览 → 渲染MP4 → 打包导出。每个创作阶段需人工确认。",
		Cat:  "video",
		DAG: `{
  "nodes": [
    {
      "id": "proposal_exec",
      "type": "TOOL",
      "name": "external",
      "input": {
        "tool": "video_script_generator",
        "topic": "{{topic}}",
        "durationSec": "{{durationSec}}",
        "language": "zh-CN",
        "style": "中文平台图文口播",
        "format": "image_text_video",
        "stage": "proposal",
        "outputMode": "proposal_only"
      }
    },
    {
      "id": "proposal_review_gate",
      "type": "CONTROL",
      "name": "审核-创作方案",
      "input": {
        "phase": "after_artifact",
        "reviewFocus": ["主题是否准确", "目标时长是否合理", "视频结构是否清楚", "是否适合图文视频"],
        "stage": "proposal"
      }
    },
    {
      "id": "script_exec",
      "type": "TOOL",
      "name": "external",
      "input": {
        "tool": "video_script_generator",
        "topic": "{{topic}}",
        "durationSec": "{{durationSec}}",
        "language": "zh-CN",
        "style": "中文平台图文口播",
        "format": "image_text_video",
        "stage": "script",
        "proposalRef": "{{proposal_exec.output}}"
      }
    },
    {
      "id": "script_review_gate",
      "type": "CONTROL",
      "name": "审核-口播脚本",
      "input": {
        "phase": "after_artifact",
        "reviewFocus": ["开头是否有吸引力", "口播是否自然", "内容是否准确", "时长是否匹配"],
        "stage": "script"
      }
    },
    {
      "id": "composition_exec",
      "type": "TOOL",
      "name": "external",
      "input": {
        "tool": "video_composition_builder",
        "script": "{{script_exec.output.script}}",
        "sections": "{{script_exec.output.sections}}",
        "durationSec": "{{durationSec}}",
        "resolution": {"width": 1920, "height": 1080},
        "fps": 30,
        "theme": "clean_card",
        "stage": "composition"
      }
    },
    {
      "id": "composition_review_gate",
      "type": "CONTROL",
      "name": "审核-视频结构",
      "input": {
        "phase": "after_artifact",
        "reviewFocus": ["卡片顺序是否合理", "每页文字是否过长", "时间轴是否覆盖全片", "是否适合 HyperFrames 图文渲染"],
        "stage": "composition"
      }
    },
    {
      "id": "preview_exec",
      "type": "TOOL",
      "name": "external",
      "input": {
        "tool": "hyperframes_project_generator",
        "projectId": "{{task.id}}",
        "topic": "{{topic}}",
        "compositionSpec": "{{composition_exec.output}}",
        "outputDir": "local://projects/{{task.id}}/hyperframes",
        "stage": "preview"
      }
    },
    {
      "id": "preview_review_gate",
      "type": "CONTROL",
      "name": "审核-画面预览",
      "input": {
        "phase": "after_artifact",
        "reviewFocus": ["画面是否可读", "文字是否溢出", "卡片顺序是否正确", "是否允许进入最终渲染"],
        "stage": "preview"
      }
    },
    {
      "id": "render_exec",
      "type": "TOOL",
      "name": "external",
      "input": {
        "tool": "hyperframes_renderer",
        "projectId": "{{task.id}}",
        "projectDir": "local://projects/{{task.id}}/hyperframes",
        "entry": "index.html",
        "outputPath": "local://projects/{{task.id}}/renders/final.mp4",
        "fps": 30,
        "quality": "standard",
        "timeoutSec": 1800,
        "stage": "render"
      }
    },
    {
      "id": "package_exec",
      "type": "TOOL",
      "name": "external",
      "input": {
        "tool": "artifact_packager",
        "projectId": "{{task.id}}",
        "include": [
          "local://projects/{{task.id}}/renders/final.mp4",
          "local://projects/{{task.id}}/hyperframes/manifest.json",
          "local://projects/{{task.id}}/hyperframes/assets/data.json"
        ],
        "output": "local://projects/{{task.id}}/packages/project_package.zip",
        "stage": "package"
      }
    },
    {
      "id": "final_review_exec",
      "type": "TOOL",
      "name": "external",
      "input": {
        "tool": "ffmpeg_probe",
        "input": "local://projects/{{task.id}}/renders/final.mp4",
        "stage": "final_review"
      }
    }
  ],
  "edges": [
    {"from": "proposal_exec", "to": "proposal_review_gate"},
    {"from": "proposal_review_gate", "to": "script_exec"},
    {"from": "script_exec", "to": "script_review_gate"},
    {"from": "script_review_gate", "to": "composition_exec"},
    {"from": "composition_exec", "to": "composition_review_gate"},
    {"from": "composition_review_gate", "to": "preview_exec"},
    {"from": "preview_exec", "to": "preview_review_gate"},
    {"from": "preview_review_gate", "to": "render_exec"},
    {"from": "render_exec", "to": "package_exec"},
    {"from": "package_exec", "to": "final_review_exec"}
  ],
  "metadata": {
    "requireReviewGates": true,
    "allowFullAuto": false,
    "defaultCheckpointPolicy": "guided",
    "pipelineVersion": "1.0",
    "humanApprovalStages": ["proposal", "script", "composition", "preview"]
  }
}`,
	}
}

// OneClickImageTextVideoWorkflow returns the legacy one-click workflow.
// This workflow is archived — use GuidedImageTextVideoWorkflow instead.
func OneClickImageTextVideoWorkflow() struct {
	ID, Name, Desc, Cat, DAG string
} {
	return struct {
		ID, Name, Desc, Cat, DAG string
	}{
		ID:   "wf-one-click-image-text-video",
		Name: "一句话图文视频（已归档）",
		Desc: "[已归档] 输入一句话主题 → 生成脚本 → 构建图文结构 → 本地生成HyperFrames项目 → 渲染MP4 → 打包导出。请使用 wf-guided-image-text-video。",
		Cat:  "video",
		DAG: `{
  "nodes": [
    {"id":"script_gen","type":"TOOL","name":"external","input":{"tool":"video_script_generator","topic":"{{topic}}","durationSec":"{{durationSec}}","language":"zh-CN","style":"中文平台图文口播","format":"image_text_video"}},
    {"id":"composition_build","type":"TOOL","name":"external","input":{"tool":"video_composition_builder","script":"{{script_gen.output.script}}","sections":"{{script_gen.output.sections}}","durationSec":"{{durationSec}}","resolution":{"width":1920,"height":1080},"fps":30,"theme":"clean_card"}},
    {"id":"hf_project_gen","type":"TOOL","name":"external","input":{"tool":"hyperframes_project_generator","projectId":"{{task.id}}","topic":"{{script_gen.output.title}}","compositionSpec":"{{composition_build.output}}","outputDir":"local://projects/{{task.id}}/hyperframes"}},
    {"id":"hf_render","type":"TOOL","name":"external","input":{"tool":"hyperframes_renderer","projectId":"{{task.id}}","projectDir":"local://projects/{{task.id}}/hyperframes","entry":"index.html","outputPath":"local://projects/{{task.id}}/renders/final.mp4","fps":30,"quality":"standard","timeoutSec":1800}},
    {"id":"package","type":"TOOL","name":"external","input":{"tool":"artifact_packager","projectId":"{{task.id}}","include":["local://projects/{{task.id}}/renders/final.mp4","local://projects/{{task.id}}/hyperframes/manifest.json","local://projects/{{task.id}}/hyperframes/assets/data.json"],"output":"local://projects/{{task.id}}/packages/project_package.zip"}},
    {"id":"final_review","type":"TOOL","name":"external","input":{"tool":"ffmpeg_probe","input":"local://projects/{{task.id}}/renders/final.mp4"}}
  ],
  "edges":[
    {"from":"script_gen","to":"composition_build"},
    {"from":"composition_build","to":"hf_project_gen"},
    {"from":"hf_project_gen","to":"hf_render"},
    {"from":"hf_render","to":"package"},
    {"from":"package","to":"final_review"}
  ]
}`,
	}
}
