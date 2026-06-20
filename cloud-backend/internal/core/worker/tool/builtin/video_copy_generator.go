package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/tangying-ai/aios-core/internal/core/common/jsonx"
	"github.com/tangying-ai/aios-core/internal/core/common/llmutil"
	"github.com/tangying-ai/aios-core/internal/core/config"
	"github.com/tangying-ai/aios-core/internal/core/worker/tool"
)

type VideoCopyGeneratorTool struct {
	cfg config.OpenAIConfig
}

func NewVideoCopyGeneratorTool(cfg config.OpenAIConfig) *VideoCopyGeneratorTool {
	return &VideoCopyGeneratorTool{cfg: cfg}
}

func (t *VideoCopyGeneratorTool) Name() string { return "video_copy_generator" }
func (t *VideoCopyGeneratorTool) Description() string {
	return "基于视频元数据、关键帧画面分析和音频转录文本，调用多模态大模型生成平台适配的短视频标题、文案和关键词。适用于抖音/小红书/B站/快手等平台。"
}
func (t *VideoCopyGeneratorTool) Type() tool.ToolType { return tool.ToolTypeCustom }

func (t *VideoCopyGeneratorTool) ValidateParameters(params map[string]interface{}) bool {
	_, ok := params["platform"].(string)
	return ok
}

// platformRules holds per-platform content formatting rules.
type platformRules struct {
	Style     string
	TitleMax  int
	DescStyle string
	TagPrefix string
}

var platformRulesMap = map[string]platformRules{
	"douyin": {Style: "抖音短视频", TitleMax: 15, DescStyle: "口语化、有互动感、多用反问和感叹句", TagPrefix: "#"},
	"抖音":     {Style: "抖音短视频", TitleMax: 15, DescStyle: "口语化、有互动感、多用反问和感叹句", TagPrefix: "#"},
	"小红书":    {Style: "小红书种草笔记", TitleMax: 20, DescStyle: "真实体验分享、分段清晰、适当Emoji", TagPrefix: "#"},
	"快手":     {Style: "快手短视频", TitleMax: 15, DescStyle: "接地气、真实感强、简单直接", TagPrefix: "#"},
	"B站":     {Style: "B站视频", TitleMax: 30, DescStyle: "有梗有趣、可加时间戳、适当玩梗", TagPrefix: "#"},
	"微博":     {Style: "微博视频", TitleMax: 20, DescStyle: "精炼有力、可用话题标签", TagPrefix: "#"},
	"视频号":    {Style: "微信视频号", TitleMax: 15, DescStyle: "简洁温馨、适合朋友圈传播", TagPrefix: "#"},
}

func (t *VideoCopyGeneratorTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
	platform, _ := params["platform"].(string)
	if platform == "" {
		platform = "douyin"
	}

	metadataStr, _ := params["metadata"].(string)
	transcription, _ := params["transcription"].(string)

	// Collect keyframe image URLs
	var imageURLs []string
	if urls, ok := params["keyframes_data_urls"].([]interface{}); ok {
		for _, u := range urls {
			if s, ok := u.(string); ok && s != "" {
				imageURLs = append(imageURLs, s)
			}
		}
	}

	// Parse metadata for duration/resolution
	var meta videoMeta
	if metadataStr != "" {
		json.Unmarshal([]byte(metadataStr), &meta)
	}
	// Also check top-level params
	if meta.DurationSec == 0 {
		if v, ok := params["duration_sec"].(float64); ok {
			meta.DurationSec = v
		}
	}
	if meta.Width == 0 {
		if v, ok := params["width"].(float64); ok {
			meta.Width = int(v)
		}
	}
	if meta.Height == 0 {
		if v, ok := params["height"].(float64); ok {
			meta.Height = int(v)
		}
	}

	zap.L().Info("VideoCopyGeneratorTool starting",
		zap.String("taskId", toolCtx.TaskID),
		zap.String("nodeId", toolCtx.NodeID),
		zap.String("platform", platform),
		zap.Int("imageCount", len(imageURLs)),
		zap.Bool("hasTranscription", transcription != ""),
	)

	// Stage 1: Visual analysis via multimodal LLM
	var visualAnalysis string
	if len(imageURLs) > 0 {
		va, err := t.callVisualAnalysis(ctx, imageURLs, meta)
		if err != nil {
			zap.L().Warn("Visual analysis failed, continuing without it", zap.Error(err))
		} else {
			visualAnalysis = va
		}
	}

	// Stage 2: Combined summary → platform-optimized copy
	reply, title, desc, keywords, err := t.generateCopy(ctx, meta, transcription, visualAnalysis, platform)
	if err != nil {
		zap.L().Error("VideoCopyGeneratorTool copy generation failed",
			zap.String("taskId", toolCtx.TaskID),
			zap.String("nodeId", toolCtx.NodeID),
			zap.Error(err),
		)
		return tool.FailureResult("大模型服务调用失败，请检查网络连接: " + err.Error())
	}

	zap.L().Info("VideoCopyGeneratorTool completed",
		zap.String("taskId", toolCtx.TaskID),
		zap.String("title", title),
		zap.Int("keywords", len(keywords)),
	)

	return tool.SuccessResult(map[string]interface{}{
		"reply":                reply,
		"title":                title,
		"description":          desc,
		"keywords":             keywords,
		"visual_analysis":      visualAnalysis,
		"platform":             platform,
		"keyframes_count":      len(imageURLs),
		"visual_analysis_chars": len([]rune(visualAnalysis)),
		"transcription_chars":  len([]rune(transcription)),
	})
}

func (t *VideoCopyGeneratorTool) Manifest() tool.ToolManifest {
	return tool.ToolManifest{
		Name:        t.Name(),
		Description: t.Description(),
		Type:        "builtin",
		Sandbox:     false,
		Parameters: map[string]tool.ParamDef{
			"platform": {
				Type:        "string",
				Description: "目标发布平台：douyin/抖音、小红书、快手、B站、微博、视频号",
				Required:    false,
				Default:     "douyin",
				Enum:        []string{"douyin", "抖音", "小红书", "快手", "B站", "微博", "视频号"},
			},
			"metadata": {
				Type:        "string",
				Description: "视频元数据JSON（来自video_metadata工具输出，含时长/分辨率/帧率等）",
				Required:    false,
			},
			"keyframes_data_urls": {
				Type:        "array",
				Description: "关键帧base64数据URL数组（来自video_analyzer工具输出）",
				Required:    false,
			},
			"transcription": {
				Type:        "string",
				Description: "Whisper音频转录文本（来自video_analyzer工具输出）",
				Required:    false,
			},
			"duration_sec": {
				Type:        "number",
				Description: "视频时长（秒），metadata为空时的备选",
				Required:    false,
			},
		},
		Output: map[string]tool.ParamDef{
			"reply":                {Type: "string", Description: "完整的发布文案（含标题+正文+话题标签，可直接复制发布）"},
			"title":                {Type: "string", Description: "平台优化的标题"},
			"description":          {Type: "string", Description: "视频简介（纯文案，不含话题标签）"},
			"keywords":             {Type: "array", Description: "5-8个与视频内容强相关的标签"},
			"visual_analysis":      {Type: "string", Description: "基于关键帧的多模态视觉分析结果"},
			"platform":             {Type: "string", Description: "实际使用的目标平台"},
			"keyframes_count":      {Type: "number", Description: "输入的关键帧数量"},
			"visual_analysis_chars": {Type: "number", Description: "视觉分析结果字符数"},
			"transcription_chars":  {Type: "number", Description: "转录文本字符数"},
		},
		Examples: []tool.ToolExample{
			{
				Input: map[string]interface{}{
					"platform": "douyin",
					"metadata": `{"duration_sec":30.5,"width":1920,"height":1080}`,
					"transcription": "今天给大家分享一个超实用的小技巧...",
				},
				Output: map[string]interface{}{
					"title": "超实用小技巧，学会省一半时间",
					"description": "今天发现这个方法真的太方便了...",
					"keywords": []interface{}{"实用技巧", "生活妙招", "干货分享"},
				},
			},
		},
	}
}

// --- Visual analysis via multimodal LLM ---

func (t *VideoCopyGeneratorTool) callVisualAnalysis(ctx context.Context, imageDataURLs []string, meta videoMeta) (string, error) {
	systemPrompt := `你是一个专业的短视频内容分析助手。你将看到从短视频中提取的关键帧截图（按时间顺序排列），请综合分析：

1. 画面主体：人物是谁？在做什么？有没有产品/物品特写？
2. 场景变化：几个不同场景？是否有转场效果？
3. 文字信息：画面中是否有字幕、标题、贴纸等关键文字？
4. 情绪氛围：画面的色调、光线、情绪基调是什么？
5. 内容类型：知识分享/vlog/产品种草/剧情/美食/旅行/美妆/健身/其他？

重点关注短时间内的内容爆点——是什么让观众在3秒内停下来观看？`

	userPrompt := fmt.Sprintf("这是一个%d秒的短视频，分辨率%dx%d，共提取了%d个关键帧。请按时间顺序分析，指出最吸引眼球的画面是第几帧。",
		int(meta.DurationSec), meta.Width, meta.Height, len(imageDataURLs))

	userMsg := llmutil.BuildUserMessage(userPrompt, imageDataURLs)

	reqBody := map[string]interface{}{
		"model":       t.cfg.Model,
		"temperature": 0.3,
		"max_tokens":  1024,
		"messages": []map[string]interface{}{
			{"role": "system", "content": systemPrompt},
			userMsg,
		},
	}

	return callLLMAPI(ctx, t.cfg, reqBody)
}

// --- Copy generation ---

func (t *VideoCopyGeneratorTool) generateCopy(ctx context.Context, meta videoMeta, transcription, visualAnalysis, platform string) (reply, title, desc string, keywords []string, err error) {
	rules, ok := platformRulesMap[platform]
	if !ok {
		rules = platformRulesMap["douyin"]
	}

	transcriptionBlock := "(视频无对话或仅有背景音乐)"
	if transcription != "" {
		transcriptionBlock = "【人物对话转录】\n" + transcription + "\n\n注意：以上是Whisper从视频中提取的人物对话内容，已自动过滤背景音乐。请基于对话内容理解视频要表达的信息。"
	}

	visualBlock := "(视觉分析不可用)"
	if visualAnalysis != "" {
		visualBlock = visualAnalysis
	}

	systemPrompt := fmt.Sprintf(`你是专业的%s内容创作专家。用户上传了一段%d秒的短视频，已经通过AI提取了：
- 关键帧截图 → 视觉分析：了解画面内容
- 音频对话 → Whisper转录：了解人物说了什么（已自动过滤背景音乐）

你需要综合这些信息，创作一条适合在%s发布的完整内容。`, rules.Style, int(meta.DurationSec), rules.Style)

	userPrompt := fmt.Sprintf(`## 视频信息
- 时长: %d秒
- 分辨率: %dx%d
- 目标平台: %s

## 视觉画面分析
%s

%s

## 创作要求

标题（%d字以内）：
- 抓眼球、有悬念或冲突感、让人想点开
- 知识类用"揭秘/方法/技巧"；vlog类用"日常/记录/第一次"；产品类用"测评/种草/好物"
- 避免标题党但要有吸引力

简介/文案（%s）：
- 语气：%s
- 将对话内容和画面信息融合成自然的文案
- 结尾加3-5个相关话题标签
- 如果视频有明确的"干货"内容，用简洁的要点提炼
- 如果视频是生活分享，用亲切的第一人称叙述

关键词标签：提取5-8个与视频内容强相关的标签词

请只输出JSON（不要markdown代码块）：
{
  "title": "平台适配的标题",
  "reply": "完整的发布文案（包含标题+正文+话题标签，适合直接复制发布）",
  "description": "视频简介（纯文案内容，不含话题标签）",
  "keywords": ["标签1", "标签2", "标签3", "标签4", "标签5"]
}`,
		int(meta.DurationSec), meta.Width, meta.Height, rules.Style,
		visualBlock, transcriptionBlock,
		rules.TitleMax, rules.DescStyle, rules.DescStyle)

	reqBody := map[string]interface{}{
		"model":       t.cfg.Model,
		"temperature": 0.7,
		"max_tokens":  t.cfg.MaxTokens,
		"messages": []map[string]interface{}{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}

	content, err := callLLMAPI(ctx, t.cfg, reqBody)
	if err != nil {
		zap.L().Warn("Copy generation LLM call failed", zap.Error(err))
		return "", "", "", nil, fmt.Errorf("大模型服务调用失败，请检查网络连接: %w", err)
	}

	var parsed struct {
		Title       string   `json:"title"`
		Reply       string   `json:"reply"`
		Description string   `json:"description"`
		Keywords    []string `json:"keywords"`
	}

	if err := jsonx.ExtractJSON(content, &parsed); err != nil {
		zap.L().Warn("Failed to parse copy generation JSON", zap.Error(err))
		reply = buildFallbackCopy(meta, transcription, visualAnalysis)
		return reply, "", reply, nil, nil
	}

	return parsed.Reply, parsed.Title, parsed.Description, parsed.Keywords, nil
}

func buildFallbackCopy(meta videoMeta, transcription, visualAnalysis string) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("视频时长 %.0f 秒，分辨率 %dx%d。", meta.DurationSec, meta.Width, meta.Height))
	if visualAnalysis != "" {
		parts = append(parts, "【画面分析】"+visualAnalysis)
	}
	if transcription != "" {
		preview := transcription
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}
		parts = append(parts, "【音频转录】"+preview)
	}
	return strings.Join(parts, "\n\n")
}

// --- Shared LLM API caller ---

func callLLMAPI(ctx context.Context, cfg config.OpenAIConfig, reqBody map[string]interface{}) (string, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/") + "/"
	endpoint := baseURL + "chat/completions"

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var responseMap map[string]interface{}
	if err := json.Unmarshal(respBody, &responseMap); err != nil {
		return "", fmt.Errorf("failed to parse LLM response: %w", err)
	}

	choices, ok := responseMap["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("no choices in LLM response")
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid choice format")
	}

	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid message format")
	}

	content, _ := message["content"].(string)
	if content == "" {
		return "", fmt.Errorf("empty response from LLM")
	}

	return content, nil
}
