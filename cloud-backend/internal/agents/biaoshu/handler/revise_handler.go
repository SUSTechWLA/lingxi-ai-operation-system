package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tangying-ai/aios-core/internal/core/modelgateway"
	"go.uber.org/zap"
)

// ReviseHandler handles AI-powered artifact revision requests.
type ReviseHandler struct {
	gw *modelgateway.Gateway
}

// NewReviseHandler creates a ReviseHandler with the given model gateway.
func NewReviseHandler(gw *modelgateway.Gateway) *ReviseHandler {
	return &ReviseHandler{gw: gw}
}

// ReviseRequest is the JSON body for POST /api/biaoshu/artifacts/revise.
type ReviseRequest struct {
	RunID           string                 `json:"runId"`
	ArtifactKind    string                 `json:"artifactKind"`
	ArtifactName    string                 `json:"artifactName"`
	ArtifactContent string                 `json:"artifactContent"`
	UserInstruction string                 `json:"userInstruction"`
	ContextMessages []ReviseContextMessage `json:"contextMessages"`
}

// ReviseContextMessage is a single chat message from the conversation history.
type ReviseContextMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ReviseResponse is the JSON response for a successful revision.
type ReviseResponse struct {
	RevisedContent string `json:"revisedContent"`
	Summary        string `json:"summary"`
	Model          string `json:"model"`
}

const maxContextMessages = 10
const maxContentLength = 200 * 1024 // 200KB

// RegisterRoutes registers the biaoshu revise endpoint.
func (h *ReviseHandler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/biaoshu/artifacts")
	api.POST("/revise", h.Revise)
}

// Revise handles POST /api/biaoshu/artifacts/revise.
func (h *ReviseHandler) Revise(c *gin.Context) {
	var req ReviseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body", "data": nil})
		return
	}

	if strings.TrimSpace(req.ArtifactContent) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "artifactContent is required", "data": nil})
		return
	}
	if len(req.ArtifactContent) > maxContentLength {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "artifactContent exceeds 200KB limit", "data": nil})
		return
	}
	if strings.TrimSpace(req.UserInstruction) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "userInstruction is required", "data": nil})
		return
	}

	// Build system prompt and user prompt.
	systemPrompt := buildReviseSystemPrompt(req.ArtifactKind, req.ArtifactName)
	userPrompt := buildReviseUserPrompt(req.ArtifactContent, req.UserInstruction, req.ContextMessages)

	result, err := h.gw.Execute(c.Request.Context(), &modelgateway.ModelRequest{
		Capability: modelgateway.CapTextToText,
		Messages: []modelgateway.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	})
	if err != nil {
		zap.L().Error("biaoshu revise LLM call failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "AI revision failed: " + err.Error(), "data": nil})
		return
	}

	parsed := parseReviseModelResponse(result.Content, result.Usage.Model)

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": parsed})
}

func parseReviseModelResponse(content, model string) ReviseResponse {
	normalized := stripMarkdownCodeFence(strings.TrimSpace(content))
	var parsed ReviseResponse
	if err := json.Unmarshal([]byte(normalized), &parsed); err == nil && strings.TrimSpace(parsed.RevisedContent) != "" {
		return finalizeReviseResponse(parsed, model)
	}

	if loose, ok := parseLooseReviseJSON(normalized); ok {
		return finalizeReviseResponse(loose, model)
	}

	return finalizeReviseResponse(ReviseResponse{
		RevisedContent: content,
		Summary:        "AI revision completed",
	}, model)
}

func finalizeReviseResponse(resp ReviseResponse, model string) ReviseResponse {
	if resp.Summary == "" {
		resp.Summary = "AI revision completed"
	}
	resp.Model = model
	return resp
}

func stripMarkdownCodeFence(content string) string {
	if !strings.HasPrefix(content, "```") {
		return content
	}
	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		return content
	}
	if strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
	}
	return content
}

func parseLooseReviseJSON(content string) (ReviseResponse, bool) {
	revised, ok := extractLooseJSONStringField(content, "revisedContent")
	if !ok || strings.TrimSpace(revised) == "" {
		return ReviseResponse{}, false
	}
	summary, _ := extractLooseJSONStringField(content, "summary")
	return ReviseResponse{
		RevisedContent: revised,
		Summary:        summary,
	}, true
}

func extractLooseJSONStringField(content, field string) (string, bool) {
	key := `"` + field + `"`
	keyIndex := strings.Index(content, key)
	if keyIndex < 0 {
		return "", false
	}
	afterKey := content[keyIndex+len(key):]
	colonIndex := strings.Index(afterKey, ":")
	if colonIndex < 0 {
		return "", false
	}
	afterColon := strings.TrimLeft(afterKey[colonIndex+1:], " \t\r\n")
	if !strings.HasPrefix(afterColon, `"`) {
		return "", false
	}
	return scanLooseJSONString(afterColon[1:])
}

func scanLooseJSONString(input string) (string, bool) {
	var sb strings.Builder
	for i := 0; i < len(input); i++ {
		ch := input[i]
		switch ch {
		case '\\':
			if i+1 >= len(input) {
				sb.WriteByte(ch)
				continue
			}
			next := input[i+1]
			switch next {
			case '"', '\\', '/':
				sb.WriteByte(next)
				i++
			case 'b':
				sb.WriteByte('\b')
				i++
			case 'f':
				sb.WriteByte('\f')
				i++
			case 'n':
				sb.WriteByte('\n')
				i++
			case 'r':
				sb.WriteByte('\r')
				i++
			case 't':
				sb.WriteByte('\t')
				i++
			case 'u':
				if i+5 < len(input) {
					if value, err := strconv.ParseInt(input[i+2:i+6], 16, 32); err == nil {
						sb.WriteRune(rune(value))
						i += 5
						continue
					}
				}
				sb.WriteByte(ch)
			default:
				sb.WriteByte(ch)
				sb.WriteByte(next)
				i++
			}
		case '"':
			if isLooseJSONStringTerminator(input[i+1:]) {
				return sb.String(), true
			}
			sb.WriteByte(ch)
		default:
			sb.WriteByte(ch)
		}
	}
	return "", false
}

func isLooseJSONStringTerminator(rest string) bool {
	trimmed := strings.TrimLeft(rest, " \t\r\n")
	return strings.HasPrefix(trimmed, ",") || strings.HasPrefix(trimmed, "}")
}

// buildReviseSystemPrompt constructs the system prompt based on artifact kind.
func buildReviseSystemPrompt(artifactKind, artifactName string) string {
	name := artifactName
	if name == "" {
		name = "标书产物"
	}
	kindDesc := artifactKindDescription(artifactKind)

	return fmt.Sprintf(`你是一个专业的技术标文档编辑助手，正在帮助用户修订《%s》。

当前产物的类型：%s

请严格按照用户的修订指令，对下方提供的产物内容进行修改。要求：
1. 保持原有的 Markdown 格式和章节结构。
2. 只修改用户指定的部分，其余内容保持不变。
3. 使用正式、规范的技术标语言风格。
4. 输出必须是合法的 JSON 对象，包含两个字段：
   - "revisedContent": 修改后的完整内容（Markdown格式）
   - "summary": 简短的中文摘要，说明做了哪些修改（1-2句话）`, name, kindDesc)
}

// buildReviseUserPrompt assembles the user prompt with artifact content, instruction, and context.
func buildReviseUserPrompt(artifactContent, userInstruction string, contextMessages []ReviseContextMessage) string {
	var sb strings.Builder
	sb.WriteString("## 当前产物内容\n\n")
	sb.WriteString(artifactContent)
	sb.WriteString("\n\n## 用户修改指令\n\n")
	sb.WriteString(userInstruction)

	if len(contextMessages) > 0 {
		// Truncate to recent window.
		recent := contextMessages
		if len(recent) > maxContextMessages {
			recent = recent[len(recent)-maxContextMessages:]
		}
		sb.WriteString("\n\n## 最近的对话上下文\n\n")
		for _, msg := range recent {
			roleLabel := "用户"
			if msg.Role == "assistant" {
				roleLabel = "助手"
			} else if msg.Role == "system" {
				roleLabel = "系统"
			}
			sb.WriteString(fmt.Sprintf("**%s**: %s\n\n", roleLabel, msg.Content))
		}
	}

	sb.WriteString("\n请根据以上信息，输出修改后的完整产物内容和修改摘要。")

	return sb.String()
}

// artifactKindDescription returns a human-readable description for common artifact kinds.
func artifactKindDescription(kind string) string {
	switch kind {
	case "BID_ANALYSIS":
		return "招标文件解析报告"
	case "BID_OUTLINE":
		return "技术标大纲"
	case "BID_CHAPTERS":
		return "章节初稿"
	case "WORD_COUNT_REPORT":
		return "字数检查报告"
	case "MERGED_DRAFT":
		return "整合成稿"
	case "TECHNICAL_BID_DOCX":
		return "技术标 Word 文档"
	default:
		return kind
	}
}
