package skillruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/tangying-ai/aios-core/internal/core/common/jsonx"
	"github.com/tangying-ai/aios-core/internal/core/config"
)

type RouteRequest struct {
	Brief string `json:"brief" binding:"required"`
}

type RouteResult struct {
	Skill             SkillSummary `json:"skill"`
	Route             string       `json:"route"`
	Deliverable       string       `json:"deliverable"`
	AspectRatio       string       `json:"aspectRatio"`
	TargetDurationSec int          `json:"targetDurationSec"`
	Reasoning         string       `json:"reasoning"`
	Confidence        float64      `json:"confidence"`
	Source            string       `json:"source"`
}

type routeDecision struct {
	SkillName         string  `json:"skillName"`
	SkillVersion      string  `json:"skillVersion"`
	Route             string  `json:"route"`
	Deliverable       string  `json:"deliverable"`
	AspectRatio       string  `json:"aspectRatio"`
	TargetDurationSec int     `json:"targetDurationSec"`
	Reasoning         string  `json:"reasoning"`
	Confidence        float64 `json:"confidence"`
}

type SkillRouter struct {
	reg        *Registry
	cfg        config.OpenAIConfig
	httpClient *http.Client
}

func NewSkillRouter(reg *Registry, cfg config.OpenAIConfig) *SkillRouter {
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &SkillRouter{
		reg:        reg,
		cfg:        cfg,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (r *SkillRouter) Route(ctx context.Context, req RouteRequest) (*RouteResult, error) {
	brief := strings.TrimSpace(req.Brief)
	if brief == "" {
		return nil, fmt.Errorf("brief is required")
	}

	catalog := filterCatalogByCategory(r.reg.Catalog(false), "video")
	if len(catalog) == 0 {
		return nil, fmt.Errorf("no visible video skills available")
	}

	if r.cfg.APIKey != "" {
		if decision, err := r.routeWithLLM(ctx, brief, catalog); err == nil {
			if result, ok := decisionToResult(decision, catalog, "llm"); ok {
				return result, nil
			}
		}
	}

	decision := fallbackDecision(brief, catalog)
	result, _ := decisionToResult(decision, catalog, "fallback")
	return result, nil
}

func (r *SkillRouter) routeWithLLM(ctx context.Context, brief string, catalog []SkillSummary) (*routeDecision, error) {
	allowed := make([]map[string]interface{}, 0, len(catalog))
	for _, skill := range catalog {
		allowed = append(allowed, map[string]interface{}{
			"name":         skill.Name,
			"version":      skill.Version,
			"displayName":  skill.DisplayName,
			"description":  skill.Description,
			"stageCount":   skill.StageCount,
			"requiresGate": skill.RequiresApproval,
		})
	}

	userPayload, _ := json.Marshal(map[string]interface{}{
		"brief":          brief,
		"allowed_skills": allowed,
	})

	messages := []map[string]interface{}{
		{
			"role":    "system",
			"content": "你是 AIOS 自媒体视频创作入口路由器。只能从 allowed_skills 里选择一个 skill。返回严格 JSON，不要 markdown。字段：skillName, skillVersion, route, deliverable, aspectRatio, targetDurationSec, reasoning, confidence。route 只能是 talking_head, cinematic_short, director_pipeline, shot_learning。deliverable 只能是 publish_pack, script_only, keyframes, video_prompt, shot_learning。优先根据用户自然语言判断，不要让用户手动选择视频类型。",
		},
		{
			"role":    "user",
			"content": string(userPayload),
		},
	}

	body, _ := json.Marshal(map[string]interface{}{
		"model":       r.cfg.Model,
		"temperature": 0.1,
		"max_tokens":  700,
		"messages":    messages,
	})

	baseURL := r.cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+r.cfg.APIKey)

	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("LLM route call returned %d: %.200s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Choices) == 0 || parsed.Choices[0].Message.Content == "" {
		return nil, fmt.Errorf("empty LLM route response")
	}

	var decision routeDecision
	if err := jsonx.ExtractJSON(parsed.Choices[0].Message.Content, &decision); err != nil {
		return nil, err
	}
	return &decision, nil
}

func filterCatalogByCategory(catalog []SkillSummary, category string) []SkillSummary {
	result := make([]SkillSummary, 0, len(catalog))
	for _, skill := range catalog {
		if skill.Category == category && skill.Health == HealthHealthy {
			result = append(result, skill)
		}
	}
	return result
}

func decisionToResult(decision *routeDecision, catalog []SkillSummary, source string) (*RouteResult, bool) {
	if decision == nil {
		return nil, false
	}
	for _, skill := range catalog {
		if skill.Name == decision.SkillName && skill.Version == decision.SkillVersion {
			normalizeDecision(decision)
			return &RouteResult{
				Skill:             skill,
				Route:             decision.Route,
				Deliverable:       decision.Deliverable,
				AspectRatio:       decision.AspectRatio,
				TargetDurationSec: decision.TargetDurationSec,
				Reasoning:         decision.Reasoning,
				Confidence:        decision.Confidence,
				Source:            source,
			}, true
		}
	}
	return nil, false
}

func normalizeDecision(decision *routeDecision) {
	if decision.Route == "" {
		decision.Route = "talking_head"
	}
	if decision.Deliverable == "" {
		decision.Deliverable = "publish_pack"
	}
	if decision.AspectRatio == "" {
		decision.AspectRatio = "9:16"
	}
	if decision.TargetDurationSec <= 0 {
		decision.TargetDurationSec = 60
	}
	if decision.Confidence <= 0 {
		decision.Confidence = 0.6
	}
}

func fallbackDecision(brief string, catalog []SkillSummary) *routeDecision {
	text := strings.ToLower(brief)
	route := "talking_head"
	deliverable := "publish_pack"
	skillName := "create-opinion-videos"

	if containsAny(text, "拉片", "参考片", "经典镜头", "学习镜头", "运镜学习") {
		route = "shot_learning"
		deliverable = "shot_learning"
		skillName = "film-shot-reconstruction"
	} else if containsAny(text, "导演级", "ip", "系列", "角色", "世界观", "长片", "完整故事", "导演剪辑") {
		route = "director_pipeline"
		skillName = "video-creator"
	} else if containsAny(text, "关键帧", "参考帧") {
		route = "cinematic_short"
		deliverable = "keyframes"
		skillName = "aigc-shot-video"
	} else if containsAny(text, "视频提示词", "prompt", "分镜", "镜头", "广告片", "剧情", "短片", "shot") {
		route = "cinematic_short"
		deliverable = "video_prompt"
		skillName = "aigc-shot-video"
	}

	skill := firstAvailable(catalog, skillName)
	if skill == nil {
		skill = &catalog[0]
	}

	return &routeDecision{
		SkillName:         skill.Name,
		SkillVersion:      skill.Version,
		Route:             route,
		Deliverable:       deliverable,
		AspectRatio:       inferAspectRatio(text),
		TargetDurationSec: inferDuration(text),
		Reasoning:         "根据自然语言关键词和可见 Skill 目录自动选择。",
		Confidence:        0.62,
	}
}

func firstAvailable(catalog []SkillSummary, name string) *SkillSummary {
	for _, skill := range catalog {
		if skill.Name == name {
			return &skill
		}
	}
	return nil
}

func containsAny(text string, keywords ...string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func inferAspectRatio(text string) string {
	switch {
	case strings.Contains(text, "16:9") || strings.Contains(text, "横屏"):
		return "16:9"
	case strings.Contains(text, "1:1") || strings.Contains(text, "方形"):
		return "1:1"
	default:
		return "9:16"
	}
}

func inferDuration(text string) int {
	re := regexp.MustCompile(`(\d{1,3})\s*秒`)
	match := re.FindStringSubmatch(text)
	if len(match) != 2 {
		return 60
	}
	var duration int
	if _, err := fmt.Sscanf(match[1], "%d", &duration); err != nil || duration <= 0 {
		return 60
	}
	return duration
}
