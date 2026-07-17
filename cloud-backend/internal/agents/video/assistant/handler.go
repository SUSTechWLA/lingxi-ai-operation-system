// Package assistant provides the video-scoped AI assistant.
// Replaces the former general-purpose chat assistant with
// video-creation-specific conversational capabilities.
package assistant

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tangying-ai/aios-core/internal/core/common/httpx"
)

// Handler exposes a video-project-scoped assistant. It deliberately does not
// register any global /api/chat route and does not write artifacts directly.
type Handler struct {
	middleware []gin.HandlerFunc
}

func NewHandler(middleware ...gin.HandlerFunc) *Handler {
	return &Handler{middleware: middleware}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/video-projects", h.middleware...)
	{
		api.POST("/:id/assistant/message", h.Message)
		api.POST("/:id/assistant/revise", h.Revise)
		api.POST("/:id/assistant/explain-stage", h.ExplainStage)
	}
}

type MessageRequest struct {
	Message     string   `json:"message"`
	Stage       string   `json:"stage,omitempty"`
	RunID       string   `json:"runId,omitempty"`
	ArtifactIDs []string `json:"artifactIds,omitempty"`
}

type ReviseRequest struct {
	ArtifactID string `json:"artifactId" binding:"required"`
	Message    string `json:"message" binding:"required"`
	RunID      string `json:"runId,omitempty"`
	ReviewID   string `json:"reviewId,omitempty"`
}

type ExplainStageRequest struct {
	Stage string `json:"stage" binding:"required"`
}

type AssistantAction struct {
	Type   string         `json:"type"`
	Label  string         `json:"label"`
	Method string         `json:"method,omitempty"`
	Path   string         `json:"path,omitempty"`
	Body   map[string]any `json:"body,omitempty"`
}

type MessageResponse struct {
	ProjectID             string            `json:"projectId"`
	Scope                 string            `json:"scope"`
	Answer                string            `json:"answer"`
	Stage                 string            `json:"stage,omitempty"`
	RunID                 string            `json:"runId,omitempty"`
	SuggestedActions      []AssistantAction `json:"suggestedActions"`
	ForbiddenCapabilities []string          `json:"forbiddenCapabilities"`
	ReferencedArtifactIDs []string          `json:"referencedArtifactIds,omitempty"`
}

type ReviseResponse struct {
	ProjectID        string          `json:"projectId"`
	ArtifactID       string          `json:"artifactId"`
	RunID            string          `json:"runId,omitempty"`
	ReviewID         string          `json:"reviewId,omitempty"`
	Answer           string          `json:"answer"`
	BypassesArtifact bool            `json:"bypassesArtifact"`
	ArtifactAction   AssistantAction `json:"artifactAction"`
}

type ExplainStageResponse struct {
	ProjectID       string   `json:"projectId"`
	Stage           string   `json:"stage"`
	DisplayName     string   `json:"displayName"`
	Explanation     string   `json:"explanation"`
	RequiredInputs  []string `json:"requiredInputs"`
	RequiredOutputs []string `json:"requiredOutputs"`
	ReviewFocus     []string `json:"reviewFocus"`
	NextUserActions []string `json:"nextUserActions"`
	BetaLimitations []string `json:"betaLimitations"`
}

func (h *Handler) Message(c *gin.Context) {
	var req MessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	projectID := c.Param("id")
	stage := normalizeStage(req.Stage)
	answer := buildScopedAnswer(strings.TrimSpace(req.Message), stage)

	httpx.OK(c, MessageResponse{
		ProjectID:             projectID,
		Scope:                 "video_project",
		Answer:                answer,
		Stage:                 stage,
		RunID:                 strings.TrimSpace(req.RunID),
		SuggestedActions:      messageActions(projectID, stage),
		ForbiddenCapabilities: betaForbiddenCapabilities(),
		ReferencedArtifactIDs: cleanStrings(req.ArtifactIDs),
	})
}

func (h *Handler) Revise(c *gin.Context) {
	var req ReviseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	artifactID := strings.TrimSpace(req.ArtifactID)
	message := strings.TrimSpace(req.Message)
	action := AssistantAction{
		Type:   "artifact_revision",
		Label:  "提交到产物返工",
		Method: http.MethodPost,
		Path:   "/api/artifacts/" + artifactID + "/revise",
		Body: map[string]any{
			"message": message,
		},
	}
	httpx.OK(c, ReviseResponse{
		ProjectID:        c.Param("id"),
		ArtifactID:       artifactID,
		RunID:            strings.TrimSpace(req.RunID),
		ReviewID:         strings.TrimSpace(req.ReviewID),
		Answer:           "已整理为产物返工指令。Beta-0.2 中助手不会直接改写产物，请通过 artifactAction 调用现有 Artifact 修订接口并继续走审核链路。",
		BypassesArtifact: false,
		ArtifactAction:   action,
	})
}

func (h *Handler) ExplainStage(c *gin.Context) {
	var req ExplainStageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	stage := normalizeStage(req.Stage)
	info := stageInfoFor(stage)
	httpx.OK(c, ExplainStageResponse{
		ProjectID:       c.Param("id"),
		Stage:           stage,
		DisplayName:     info.displayName,
		Explanation:     info.explanation,
		RequiredInputs:  info.requiredInputs,
		RequiredOutputs: info.requiredOutputs,
		ReviewFocus:     info.reviewFocus,
		NextUserActions: info.nextActions,
		BetaLimitations: []string{
			"不会自动发布",
			"不会替代人工审核",
			"不会绕过 Artifact 版本记录",
		},
	})
}

type stageInfo struct {
	displayName     string
	explanation     string
	requiredInputs  []string
	requiredOutputs []string
	reviewFocus     []string
	nextActions     []string
}

func stageInfoFor(stage string) stageInfo {
	infos := map[string]stageInfo{
		"proposal": {
			displayName:     "创意方案",
			explanation:     "确认主题、视频类型、平台、时长和交付物边界，是后续脚本与分镜的方向输入。",
			requiredOutputs: []string{"VIDEO_PROPOSAL", "PROJECT_BRIEF"},
			reviewFocus:     []string{"主题是否准确", "视频类型是否匹配", "时长是否合理"},
			nextActions:     []string{"确认方向", "补充目标受众", "驳回并说明想调整的角度"},
		},
		"script": {
			displayName:     "脚本",
			explanation:     "把创意方案落成可拍摄或可生成的中文短视频脚本。",
			requiredInputs:  []string{"VIDEO_PROPOSAL"},
			requiredOutputs: []string{"VIDEO_SCRIPT"},
			reviewFocus:     []string{"开头是否有钩子", "表达是否自然", "长度是否接近目标时长"},
			nextActions:     []string{"复制脚本", "返工脚本", "通过后进入分镜"},
		},
		"storyboard": {
			displayName:     "分镜",
			explanation:     "把脚本拆成镜头、画面卡片、字幕节奏或关键帧提示词。",
			requiredInputs:  []string{"VIDEO_SCRIPT"},
			requiredOutputs: []string{"CARD_PLAN", "SHOT_LIST", "KEYFRAME_PROMPTS"},
			reviewFocus:     []string{"镜头顺序是否清楚", "画面是否能表达脚本", "提示词是否可交付"},
			nextActions:     []string{"检查镜头完整性", "补充手动素材", "通过后生成结构"},
		},
		"composition": {
			displayName:     "视频结构",
			explanation:     "生成时间轴、版式、安全区和组件结构，为 HyperFrames 预览做准备。",
			requiredInputs:  []string{"CARD_PLAN", "VIDEO_SCRIPT"},
			requiredOutputs: []string{"VIDEO_COMPOSITION_SPEC"},
			reviewFocus:     []string{"结构是否可渲染", "文字是否适合屏幕阅读", "节奏是否顺畅"},
			nextActions:     []string{"通过结构", "返工卡片节奏", "进入预览"},
		},
		"preview": {
			displayName:     "预览",
			explanation:     "预览阶段在 render 前生成本地快照，用来检查可读性、版式和基本视觉错误。Beta-0.2 要求先确认预览再进入最终渲染。",
			requiredInputs:  []string{"VIDEO_COMPOSITION_SPEC", "HYPERFRAMES_PROJECT"},
			requiredOutputs: []string{"PREVIEW_SNAPSHOTS", "PREVIEW_REPORT"},
			reviewFocus:     []string{"文字是否溢出", "画面是否可读", "是否允许进入最终渲染"},
			nextActions:     []string{"查看快照", "通过预览", "驳回并重做结构或素材"},
		},
		"render": {
			displayName:     "渲染",
			explanation:     "渲染阶段把已确认的预览项目交给本地执行器生成视频文件。",
			requiredInputs:  []string{"PREVIEW_SNAPSHOTS"},
			requiredOutputs: []string{"VIDEO", "RENDER_REPORT"},
			reviewFocus:     []string{"预览是否已确认", "本地执行器是否在线", "最终文件是否生成"},
			nextActions:     []string{"检查本地服务", "等待渲染", "失败后重试当前阶段"},
		},
		"package": {
			displayName:     "导出",
			explanation:     "整理脚本、分镜、Prompt、发布文案和交付包，便于手动发布。",
			requiredInputs:  []string{"VIDEO_SCRIPT", "CARD_PLAN", "FINAL_REVIEW"},
			requiredOutputs: []string{"PROJECT_PACKAGE", "PUBLISH_COPY"},
			reviewFocus:     []string{"发布素材是否完整", "是否能复制 Markdown/JSON", "是否保留人工发布边界"},
			nextActions:     []string{"复制发布文案", "导出 Markdown", "导出 JSON"},
		},
	}
	if info, ok := infos[stage]; ok {
		return info
	}
	return stageInfo{
		displayName:     stage,
		explanation:     "这是视频项目内的一个执行阶段。请结合 Trace、Artifact 和 Review 状态判断下一步。",
		requiredInputs:  []string{},
		requiredOutputs: []string{},
		reviewFocus:     []string{"产物是否符合目标", "是否可以进入下游阶段"},
		nextActions:     []string{"查看 Trace", "查看 Artifact", "根据审核意见返工"},
	}
}

func buildScopedAnswer(message string, stage string) string {
	if message == "" {
		message = "当前项目需要下一步建议"
	}
	info := stageInfoFor(stage)
	return "这是视频项目内助手，只根据当前 VideoProject、Run、Review 和 Artifact 状态给建议。当前关注阶段：" +
		info.displayName + "。" + info.explanation + " 你的问题是：" + message
}

func messageActions(projectID string, stage string) []AssistantAction {
	return []AssistantAction{
		{Type: "view_trace", Label: "查看执行追踪", Method: http.MethodGet, Path: "/api/agent/runs/:runId/trace"},
		{Type: "view_artifacts", Label: "查看项目产物", Method: http.MethodGet, Path: "/api/video-projects/" + projectID + "/artifacts"},
		{Type: "explain_stage", Label: "解释当前阶段", Method: http.MethodPost, Path: "/api/video-projects/" + projectID + "/assistant/explain-stage", Body: map[string]any{"stage": stage}},
	}
}

func normalizeStage(stage string) string {
	stage = strings.TrimSpace(strings.ToLower(stage))
	if stage == "" {
		return "proposal"
	}
	return stage
}

func betaForbiddenCapabilities() []string {
	return []string{
		"auto_publish",
		"global_chat",
		"bid_generation",
		"video_question_answering",
		"long_video_understanding",
	}
}

func cleanStrings(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}
