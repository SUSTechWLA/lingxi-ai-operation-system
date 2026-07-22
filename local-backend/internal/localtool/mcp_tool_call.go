package localtool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tangying-ai/tangying-ai-operation-system/local-backend/internal/localmcp"
)

type mcpToolCallExecutor struct {
	loadProviders MCPProviderLoader
	dataDir       string
}

var mcpTimedSegmentPattern = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*[-~—到至]\s*(\d+(?:\.\d+)?)\s*(秒|s)`)

func NewMCPToolCallExecutor(loader MCPProviderLoader) Executor {
	return &mcpToolCallExecutor{loadProviders: loader}
}

func NewMCPToolCallExecutorWithDataDir(loader MCPProviderLoader, dataDir string) Executor {
	return &mcpToolCallExecutor{loadProviders: loader, dataDir: dataDir}
}

func (e *mcpToolCallExecutor) Execute(ctx context.Context, job Job) (*Result, error) {
	started := time.Now()
	if e.loadProviders == nil {
		return nil, errors.New("mcp provider loader is not configured")
	}
	providerID := stringPayload(job.Payload, "providerId")
	if providerID == "" {
		providerID = stringPayload(job.Payload, "provider_id")
	}
	if providerID == "" {
		return nil, errors.New("providerId is required")
	}
	toolName := stringPayload(job.Payload, "toolName")
	if toolName == "" {
		toolName = stringPayload(job.Payload, "tool_name")
	}
	if toolName == "" {
		toolName = stringPayload(job.Payload, "mcpTool")
	}
	if toolName == "" {
		return nil, errors.New("toolName is required")
	}
	providers, err := e.loadProviders()
	if err != nil {
		return nil, err
	}
	provider, ok := findProvider(providers, providerID)
	if !ok {
		return nil, fmt.Errorf("mcp provider %q is not configured or enabled", providerID)
	}
	timeout := shortestPositiveMCPTimeout(job.TimeoutSec, provider.TimeoutSec)
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	deadline := started.Add(timeout)
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	operationCtx, cancelOperation := context.WithDeadline(ctx, deadline)
	defer cancelOperation()
	if err := operationCtx.Err(); err != nil {
		return nil, err
	}
	client := localmcp.NewClient(provider, nil)
	defer client.Close()
	if requests := slicePayload(job.Payload, "externalGenerationRequests"); len(requests) > 0 {
		return e.executeExternalGenerationBatch(operationCtx, client, provider.ID, toolName, job, requests)
	}
	args := mapPayload(job.Payload, "arguments")
	callResult, err := client.CallTool(operationCtx, toolName, args)
	if err != nil {
		return nil, err
	}
	return &Result{Output: map[string]interface{}{
		"providerId":        provider.ID,
		"toolName":          toolName,
		"content":           callResult.Content,
		"structuredContent": callResult.StructuredContent,
		"isError":           callResult.IsError,
		"meta":              callResult.Meta,
		"raw":               callResult.Raw,
		"error":             mcpErrorText(callResult),
	}}, nil
}

func shortestPositiveMCPTimeout(seconds ...int) time.Duration {
	var shortest time.Duration
	for _, value := range seconds {
		if value <= 0 {
			continue
		}
		candidate := time.Duration(value) * time.Second
		if shortest == 0 || candidate < shortest {
			shortest = candidate
		}
	}
	return shortest
}

func (e *mcpToolCallExecutor) executeExternalGenerationBatch(ctx context.Context, client *localmcp.Client, providerID string, toolName string, job Job, requests []interface{}) (*Result, error) {
	packages := make([]interface{}, 0, len(requests))
	results := make([]interface{}, 0, len(requests))
	remaining := make([]interface{}, 0)
	maxReady := mcpMaxReadyGenerations(job)
	readyCount := 0
	batchDeadline := time.Now().Add(mcpBatchTimeout(job))
	for idx, item := range requests {
		request := mcpMapFromInterface(item)
		requestToolName := mcpToolNameForExternalRequest(providerID, toolName, request)
		args := mcpArgumentsFromExternalRequest(request)
		generatedAt := time.Now().UTC().Format(time.RFC3339)
		result := map[string]interface{}{
			"schemaVersion":     1,
			"requestId":         request["requestId"],
			"shotId":            request["shotId"],
			"kind":              mcpExternalRequestType(request),
			"providerId":        providerID,
			"toolName":          requestToolName,
			"generatedAt":       generatedAt,
			"inputPromptHash":   mcpInputPromptHash(request),
			"sourceArtifactIds": mcpSourceArtifactIDs(request),
		}
		preflight := mcpPreflightForExternalRequest(request)
		result["preflightQa"] = preflight
		if !mcpBoolFromInterface(preflight["passed"]) {
			reason := mcpStringFromMap(preflight, "reason")
			if reason == "" {
				reason = "preflight_qa_failed"
			}
			message := mcpStringFromMap(preflight, "message")
			if message == "" {
				message = "AIGC preflight QA failed; provider call skipped"
			}
			result["status"] = "blocked"
			result["reason"] = reason
			result["error"] = message
			result["preflightQa"] = preflight
			blocked := copyMap(request)
			blocked["status"] = "blocked"
			blocked["reason"] = reason
			blocked["error"] = message
			blocked["preflightQa"] = preflight
			remaining = append(remaining, blocked)
			results = append(results, result)
			continue
		}
		requestTimeout := mcpRequestTimeout(job, request, batchDeadline)
		if requestTimeout <= 0 {
			result["status"] = "deferred"
			result["reason"] = "generation_batch_timeout"
			deferred := copyMap(request)
			deferred["status"] = "deferred"
			deferred["reason"] = "generation_batch_timeout"
			remaining = append(remaining, deferred)
			results = append(results, result)
			e.deferRemainingRequests(requests[idx+1:], &remaining, &results, providerID, toolName, "generation_batch_timeout")
			break
		}
		requestCtx, cancelRequest := context.WithTimeout(ctx, requestTimeout)
		callResult, err := client.CallTool(requestCtx, requestToolName, args)
		if err != nil {
			cancelRequest()
			if mcpIsTimeoutError(err) {
				result["status"] = "deferred"
				result["reason"] = "tool_call_timeout"
				result["error"] = err.Error()
				deferred := copyMap(request)
				deferred["status"] = "deferred"
				deferred["reason"] = "tool_call_timeout"
				deferred["error"] = err.Error()
				remaining = append(remaining, deferred)
				results = append(results, result)
				e.deferRemainingRequests(requests[idx+1:], &remaining, &results, providerID, toolName, "previous_generation_timeout")
				break
			}
			if mcpIsProviderBusyError(err.Error()) {
				result["status"] = "deferred"
				result["reason"] = "provider_busy"
				deferred := copyMap(request)
				deferred["status"] = "deferred"
				deferred["reason"] = "provider_busy"
				deferred["error"] = err.Error()
				remaining = append(remaining, deferred)
				results = append(results, result)
				e.deferRemainingRequests(requests[idx+1:], &remaining, &results, providerID, toolName, "provider_busy")
				break
			}
			result["status"] = "failed"
			result["error"] = err.Error()
			failed := copyMap(request)
			failed["status"] = "failed"
			failed["error"] = err.Error()
			remaining = append(remaining, failed)
			results = append(results, result)
			continue
		}
		result["status"] = "submitted"
		result["content"] = callResult.Content
		result["structuredContent"] = callResult.StructuredContent
		result["isError"] = callResult.IsError
		result["meta"] = callResult.Meta
		result["raw"] = callResult.Raw
		if submitID := mcpStringFromMap(callResult.StructuredContent, "submit_id", "submitId", "job_id", "jobId", "task_id", "taskId"); submitID != "" {
			result["submitId"] = submitID
			result["providerJobId"] = submitID
		}
		if callResult.IsError {
			errorText := mcpErrorText(callResult)
			if mcpIsProviderBusyError(errorText) {
				result["status"] = "deferred"
				result["reason"] = "provider_busy"
				result["error"] = errorText
				deferred := copyMap(request)
				deferred["status"] = "deferred"
				deferred["reason"] = "provider_busy"
				deferred["error"] = errorText
				deferred["mcpResult"] = map[string]interface{}{
					"content":           callResult.Content,
					"structuredContent": callResult.StructuredContent,
					"isError":           callResult.IsError,
					"meta":              callResult.Meta,
					"raw":               callResult.Raw,
					"error":             errorText,
				}
				remaining = append(remaining, deferred)
				results = append(results, result)
				e.deferRemainingRequests(requests[idx+1:], &remaining, &results, providerID, toolName, "provider_busy")
				cancelRequest()
				break
			}
			result["status"] = "failed"
			result["error"] = errorText
			failed := copyMap(request)
			failed["status"] = "failed"
			failed["error"] = errorText
			failed["mcpResult"] = map[string]interface{}{
				"content":           callResult.Content,
				"structuredContent": callResult.StructuredContent,
				"isError":           callResult.IsError,
				"meta":              callResult.Meta,
				"raw":               callResult.Raw,
				"error":             errorText,
			}
			remaining = append(remaining, failed)
		} else {
			structured := callResult.StructuredContent
			media := e.resolveGeneratedMedia(requestCtx, client, providerID, requestToolName, job, request, structured)
			cancelRequest()
			currentReady := false
			if len(media.StructuredContent) > 0 {
				structured = media.StructuredContent
				result["structuredContent"] = structured
			}
			if media.StorageRef != "" {
				result["status"] = "ready"
				result["storageRef"] = media.StorageRef
				currentReady = true
				if media.LocalPath != "" {
					result["localPath"] = media.LocalPath
				}
			} else if submitID := mcpStringFromMap(structured, "submit_id", "submitId"); submitID != "" {
				status := strings.ToLower(mcpStringFromMap(structured, "gen_status", "genStatus", "status"))
				if mcpIsFailedGenerationStatus(status) {
					errorText := mcpStringFromMap(structured, "error", "message", "fail_reason", "failReason")
					if errorText == "" {
						errorText = "mcp generation failed"
					}
					result["status"] = "failed"
					result["submitId"] = submitID
					result["genStatus"] = status
					result["error"] = errorText
					failed := copyMap(request)
					failed["status"] = "failed"
					failed["submitId"] = submitID
					failed["genStatus"] = status
					failed["error"] = errorText
					failed["mcpResult"] = map[string]interface{}{
						"content":           callResult.Content,
						"structuredContent": structured,
						"isError":           false,
						"meta":              callResult.Meta,
						"raw":               callResult.Raw,
						"error":             errorText,
					}
					remaining = append(remaining, failed)
					packages = append(packages, e.shotAssetPackageFromMCPResult(providerID, request, structured, media))
					results = append(results, result)
					continue
				}
				if status == "" {
					status = "pending"
				}
				result["status"] = "pending"
				result["submitId"] = submitID
				result["genStatus"] = status
				pending := copyMap(request)
				pending["status"] = "pending"
				pending["submitId"] = submitID
				pending["genStatus"] = status
				pending["mcpResult"] = map[string]interface{}{
					"content":           callResult.Content,
					"structuredContent": structured,
					"isError":           false,
					"meta":              callResult.Meta,
					"raw":               callResult.Raw,
				}
				remaining = append(remaining, pending)
				packages = append(packages, e.shotAssetPackageFromMCPResult(providerID, request, structured, media))
				results = append(results, result)
				e.deferRemainingRequests(requests[idx+1:], &remaining, &results, providerID, toolName, "previous_generation_pending")
				break
			}
			packages = append(packages, e.shotAssetPackageFromMCPResult(providerID, request, structured, media))
			if currentReady {
				readyCount++
			}
			results = append(results, result)
			if currentReady && maxReady > 0 && readyCount >= maxReady && idx+1 < len(requests) {
				e.deferRemainingRequests(requests[idx+1:], &remaining, &results, providerID, toolName, "generation_budget_reached")
				break
			}
			continue
		}
		cancelRequest()
		results = append(results, result)
	}
	sourceSummary := buildMCPSourceSummary(providerID, toolName, job, results)
	assetProvenance := buildMCPAssetProvenance(results)
	aRollPackages := make([]interface{}, 0, len(packages))
	for _, item := range packages {
		if pkg := mcpMapFromInterface(item); isIPArollKind(mcpStringFromMap(pkg, "kind", "sourceType")) {
			aRollPackages = append(aRollPackages, item)
		}
	}
	result := &Result{Output: map[string]interface{}{
		"providerId":                 providerID,
		"toolName":                   toolName,
		"shotAssetPackages":          packages,
		"aRollAssetPackages":         aRollPackages,
		"generationResults":          results,
		"externalGenerationResults":  results,
		"externalGenerationRequests": remaining,
		"assetProvenance":            assetProvenance,
		"sourceSummary":              sourceSummary,
		"requirementsSatisfied":      sourceSummary["externalVideoRequirementSatisfied"],
		"summary":                    mcpGenerationSummaryText(sourceSummary),
	}}
	if mcpBoolFromInterface(job.Payload["failOnUnmetRequirements"]) &&
		!mcpBoolFromInterface(sourceSummary["externalVideoRequirementSatisfied"]) {
		return nil, fmt.Errorf("required MCP video generation was not satisfied: %s", result.Output["summary"])
	}
	return result, nil
}

func mcpMaxReadyGenerations(job Job) int {
	for _, key := range []string{"maxReadyGenerations", "maxReadyExternalGenerations", "mcpMaxReadyGenerations", "mcp_max_ready_generations"} {
		if value := mcpIntFromInterface(mcpFirstPresent(job.Payload, key)); value > 0 {
			return value
		}
	}
	return 2
}

func mcpToolNameForExternalRequest(providerID, defaultToolName string, request map[string]interface{}) string {
	if explicit := strings.TrimSpace(mcpStringFromMap(request, "mcpTool", "toolName", "tool")); explicit != "" {
		return explicit
	}
	toolName := strings.TrimSpace(defaultToolName)
	kind := strings.ToLower(strings.TrimSpace(mcpStringFromMap(request, "kind", "generationKind", "assetKind")))
	if kind == "image" || kind == "reference_image" || kind == "keyframe" {
		if strings.Contains(toolName, "generate_video") {
			return strings.Replace(toolName, "generate_video", "generate_image", 1)
		}
		if strings.Contains(toolName, "generate_image") {
			return toolName
		}
		if strings.Contains(toolName, ".") {
			return toolName[:strings.LastIndex(toolName, ".")] + ".generate_image"
		}
		if strings.TrimSpace(providerID) != "" {
			return providerID + ".generate_image"
		}
		return "generate_image"
	}
	if kind == "video" || kind == "shot_video" || kind == "" {
		if strings.Contains(toolName, "generate_image") {
			return strings.Replace(toolName, "generate_image", "generate_video", 1)
		}
		if toolName != "" {
			return toolName
		}
		if strings.TrimSpace(providerID) != "" {
			return providerID + ".generate_video"
		}
		return "generate_video"
	}
	if toolName != "" {
		return toolName
	}
	if strings.TrimSpace(providerID) != "" {
		return providerID + ".generate_video"
	}
	return "generate_video"
}

func mcpBatchTimeout(job Job) time.Duration {
	if timeout := mcpDurationFromPayload(job.Payload, "mcpBatchTimeoutMs", "batchTimeoutMs", "mcp_batch_timeout_ms"); timeout > 0 {
		return timeout
	}
	if timeout := mcpSecondsFromPayload(job.Payload, "mcpBatchTimeoutSec", "batchTimeoutSec", "mcp_batch_timeout_sec"); timeout > 0 {
		return timeout
	}
	return 6 * time.Minute
}

func mcpRequestTimeout(job Job, request map[string]interface{}, batchDeadline time.Time) time.Duration {
	timeout := mcpToolCallTimeout(job, request)
	remaining := time.Until(batchDeadline)
	if remaining <= 0 {
		return 0
	}
	if timeout <= 0 || remaining < timeout {
		return remaining
	}
	return timeout
}

func mcpCallToolWithTimeout(ctx context.Context, client *localmcp.Client, toolName string, args map[string]interface{}, timeout time.Duration) (*localmcp.ToolCallResult, error) {
	if timeout <= 0 {
		return client.CallTool(ctx, toolName, args)
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return client.CallTool(callCtx, toolName, args)
}

func mcpToolCallTimeout(job Job, request map[string]interface{}) time.Duration {
	if timeout := mcpDurationFromPayload(job.Payload, "mcpToolCallTimeoutMs", "toolCallTimeoutMs", "mcp_call_timeout_ms"); timeout > 0 {
		return timeout
	}
	if timeout := mcpDurationFromPayload(request, "mcpToolCallTimeoutMs", "toolCallTimeoutMs", "mcp_call_timeout_ms"); timeout > 0 {
		return timeout
	}
	target := mcpMapFromInterface(request["target"])
	if timeout := mcpDurationFromPayload(target, "mcpToolCallTimeoutMs", "toolCallTimeoutMs", "mcp_call_timeout_ms"); timeout > 0 {
		return timeout
	}
	if timeout := mcpSecondsFromPayload(job.Payload, "mcpToolCallTimeoutSec", "toolCallTimeoutSec", "mcp_call_timeout_sec"); timeout > 0 {
		return timeout
	}
	if timeout := mcpSecondsFromPayload(request, "mcpToolCallTimeoutSec", "toolCallTimeoutSec", "mcp_call_timeout_sec"); timeout > 0 {
		return timeout
	}
	if timeout := mcpSecondsFromPayload(target, "mcpToolCallTimeoutSec", "toolCallTimeoutSec", "mcp_call_timeout_sec"); timeout > 0 {
		return timeout
	}
	return 5 * time.Minute
}

func mcpDurationFromPayload(payload map[string]interface{}, keys ...string) time.Duration {
	for _, key := range keys {
		if value := mcpIntFromInterface(mcpFirstPresent(payload, key)); value > 0 {
			return time.Duration(value) * time.Millisecond
		}
	}
	return 0
}

func mcpSecondsFromPayload(payload map[string]interface{}, keys ...string) time.Duration {
	for _, key := range keys {
		if value := mcpIntFromInterface(mcpFirstPresent(payload, key)); value > 0 {
			return time.Duration(value) * time.Second
		}
	}
	return 0
}

func mcpIsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "context deadline exceeded") ||
		strings.Contains(text, "client.timeout") ||
		strings.Contains(text, "timeout awaiting") ||
		strings.Contains(text, "i/o timeout")
}

func (e *mcpToolCallExecutor) deferRemainingRequests(items []interface{}, remaining *[]interface{}, results *[]interface{}, providerID, toolName, reason string) {
	for _, futureItem := range items {
		future := mcpMapFromInterface(futureItem)
		futureToolName := mcpToolNameForExternalRequest(providerID, toolName, future)
		deferred := copyMap(future)
		deferred["status"] = "deferred"
		deferred["reason"] = reason
		*remaining = append(*remaining, deferred)
		*results = append(*results, map[string]interface{}{
			"schemaVersion":     1,
			"requestId":         future["requestId"],
			"shotId":            future["shotId"],
			"kind":              mcpExternalRequestType(future),
			"providerId":        providerID,
			"toolName":          futureToolName,
			"status":            "deferred",
			"reason":            reason,
			"generatedAt":       time.Now().UTC().Format(time.RFC3339),
			"inputPromptHash":   mcpInputPromptHash(future),
			"sourceArtifactIds": mcpSourceArtifactIDs(future),
		})
	}
}

func mcpPreflightForExternalRequest(request map[string]interface{}) map[string]interface{} {
	if isIPArollExternalRequest(request) {
		return map[string]interface{}{
			"passed":          true,
			"score":           100,
			"promptPassed":    true,
			"referencePassed": true,
			"applicable":      false,
			"reason":          "deterministic_local_render",
			"issues":          []interface{}{},
		}
	}
	if mcpExternalRequestKind(request) != "video" {
		return map[string]interface{}{
			"passed":          true,
			"score":           100,
			"promptPassed":    true,
			"referencePassed": true,
			"reason":          "",
			"issues":          []interface{}{},
		}
	}
	promptReport := mcpVideoPromptQAReport(mcpPromptFromExternalRequest(request))
	referenceReport := mcpVideoReferenceQAReport(request)
	passed := mcpBoolFromInterface(promptReport["passed"]) && mcpBoolFromInterface(referenceReport["passed"])
	score := minInt(mcpIntFromInterface(promptReport["score"]), mcpIntFromInterface(referenceReport["score"]))
	reason := ""
	if !mcpBoolFromInterface(promptReport["passed"]) {
		reason = "prompt_qa_failed"
	} else if !mcpBoolFromInterface(referenceReport["passed"]) {
		reason = "reference_qa_failed"
	}
	message := ""
	if reason == "prompt_qa_failed" {
		message = "视频提示词不够清晰，已阻止调用 AIGC provider，避免浪费额度。"
	} else if reason == "reference_qa_failed" {
		message = "参考素材不可用或未落地，已阻止调用 AIGC provider，避免浪费额度。"
	}
	issues := []interface{}{}
	issues = append(issues, interfaceSlice(promptReport["issues"])...)
	issues = append(issues, interfaceSlice(referenceReport["issues"])...)
	return map[string]interface{}{
		"passed":               passed,
		"score":                score,
		"reason":               reason,
		"message":              message,
		"promptPassed":         promptReport["passed"],
		"promptScore":          promptReport["score"],
		"referencePassed":      referenceReport["passed"],
		"referenceScore":       referenceReport["score"],
		"timedSegmentCount":    promptReport["timedSegmentCount"],
		"visualAnchorCount":    promptReport["visualAnchorCount"],
		"actionVerbCount":      promptReport["actionVerbCount"],
		"emotionWordCount":     promptReport["emotionWordCount"],
		"usableReferenceCount": referenceReport["usableReferenceCount"],
		"issues":               issues,
	}
}

func mcpVideoPromptQAReport(prompt string) map[string]interface{} {
	prompt = strings.TrimSpace(prompt)
	issues := []interface{}{}
	score := 100
	if len([]rune(prompt)) < 120 {
		score -= 35
		issues = append(issues, map[string]interface{}{"code": "prompt_too_short", "message": "提示词太短，用户无法清晰脑补画面。"})
	}
	banned := mcpPromptBannedTerms(prompt)
	if len(banned) > 0 {
		score -= 45
		issues = append(issues, map[string]interface{}{"code": "production_jargon_leaked", "terms": banned, "message": "提示词包含内部生产说明或拼接术语。"})
	}
	timedSegments := len(mcpTimedSegmentPattern.FindAllString(prompt, -1))
	if timedSegments < 2 {
		score -= 25
		issues = append(issues, map[string]interface{}{"code": "missing_timed_story_beats", "message": "缺少按时间段展开的画面变化。"})
	}
	visualAnchors := mcpCountContains(prompt, []string{
		"桌", "房间", "屏幕", "窗口", "纸", "卡", "便利贴", "贴纸", "星", "机器人", "传送带", "工位", "放大镜", "胶囊", "看板", "灯", "门", "街", "城市", "海", "船", "树", "角色", "人物", "道具", "场景", "胶片", "咖啡", "按钮", "小旗",
	})
	if visualAnchors < 2 {
		score -= 20
		issues = append(issues, map[string]interface{}{"code": "visual_anchors_weak", "message": "缺少具体主体、道具或场景锚点。"})
	}
	actionVerbs := mcpCountContains(prompt, []string{
		"出现", "打开", "亮起", "弹出", "跳", "跑", "排队", "围住", "变成", "聚拢", "散开", "飘", "落下", "升起", "转", "挥手", "盖章", "收束", "流动", "递进", "合拢", "伸出",
	})
	if actionVerbs < 2 {
		score -= 20
		issues = append(issues, map[string]interface{}{"code": "motion_change_weak", "message": "缺少明确动作和状态变化。"})
	}
	emotionWords := mcpCountContains(prompt, []string{
		"轻松", "温暖", "治愈", "解压", "幽默", "荒诞", "积极", "明亮", "清爽", "热闹", "安静", "紧张", "松一口气", "会心一笑", "友好",
	})
	if emotionWords == 0 && !strings.Contains(prompt, "表达") {
		score -= 10
		issues = append(issues, map[string]interface{}{"code": "emotion_or_intent_missing", "message": "缺少情感氛围或表达思想。"})
	}
	if score < 0 {
		score = 0
	}
	return map[string]interface{}{
		"passed":            score >= 85,
		"score":             score,
		"timedSegmentCount": timedSegments,
		"visualAnchorCount": visualAnchors,
		"actionVerbCount":   actionVerbs,
		"emotionWordCount":  emotionWords,
		"issues":            issues,
	}
}

func mcpVideoReferenceQAReport(request map[string]interface{}) map[string]interface{} {
	referenceAssetIDs := stringSliceFromLocalMCPValue(mcpFirstPresent(request, "referenceAssetIds", "referenceIds"))
	references := append(interfaceSlice(request["references"]), interfaceSlice(request["referenceImages"])...)
	requiresReferences := len(referenceAssetIDs) > 0 || mcpBoolFromInterface(mcpFirstPresent(request, "requiresReferences", "requireReferences"))
	usable := 0
	for _, item := range references {
		ref := mcpMapFromInterface(item)
		if mcpReferenceUsable(ref) {
			usable++
		}
	}
	issues := []interface{}{}
	score := 100
	if (requiresReferences || len(references) > 0) && usable == 0 {
		score = 60
		issues = append(issues, map[string]interface{}{
			"code":    "reference_assets_unusable",
			"message": "请求声明了参考素材，但没有可用的本地/URL/storageRef 参考文件。",
		})
	}
	return map[string]interface{}{
		"passed":               score >= 85,
		"score":                score,
		"requiresReferences":   requiresReferences,
		"referenceCount":       len(references),
		"usableReferenceCount": usable,
		"issues":               issues,
	}
}

func mcpPromptBannedTerms(prompt string) []interface{} {
	lower := strings.ToLower(prompt)
	terms := []string{"ffmpeg", "shot_video_clip", "aigc_video", "b-roll", "broll", "artifact", "storageref", "simple_cut"}
	terms = append(terms, "素材意图", "镜头运动", "生成后可直接", "本 shot", "口播/字幕内容", "画面需包含", "转场只覆盖")
	found := []interface{}{}
	for _, term := range terms {
		if strings.Contains(lower, strings.ToLower(term)) {
			found = append(found, term)
		}
	}
	return found
}

func mcpCountContains(text string, needles []string) int {
	count := 0
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			count++
		}
	}
	return count
}

func mcpReferenceUsable(ref map[string]interface{}) bool {
	value := mcpStringFromMap(ref, "storageRef", "localPath", "path", "url", "imageUrl", "image_url")
	if value == "" {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(normalized, "manual://") || strings.Contains(normalized, "{{") || strings.Contains(normalized, "user_asset_redacted") {
		return false
	}
	return true
}

func stringSliceFromLocalMCPValue(value interface{}) []string {
	out := []string{}
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				out = append(out, strings.TrimSpace(item))
			}
		}
	case []interface{}:
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				out = append(out, text)
			}
		}
	case string:
		if strings.TrimSpace(typed) != "" {
			out = append(out, strings.TrimSpace(typed))
		}
	}
	return out
}

func buildMCPSourceSummary(providerID, toolName string, job Job, results []interface{}) map[string]interface{} {
	summary := map[string]interface{}{
		"providerId":          providerID,
		"toolName":            toolName,
		"totalRequestCount":   len(results),
		"videoRequestCount":   0,
		"imageRequestCount":   0,
		"readyCount":          0,
		"readyVideoCount":     0,
		"readyImageCount":     0,
		"pendingCount":        0,
		"deferredCount":       0,
		"failedCount":         0,
		"blockedCount":        0,
		"generatedMediaCount": 0,
	}
	for _, item := range results {
		result := mcpMapFromInterface(item)
		kind := mcpKindFromResult(result)
		status := strings.ToLower(strings.TrimSpace(mcpStringFromMap(result, "status")))
		if kind == "image" {
			summary["imageRequestCount"] = summary["imageRequestCount"].(int) + 1
		} else {
			summary["videoRequestCount"] = summary["videoRequestCount"].(int) + 1
		}
		switch status {
		case "ready":
			summary["readyCount"] = summary["readyCount"].(int) + 1
			summary["generatedMediaCount"] = summary["generatedMediaCount"].(int) + 1
			if kind == "image" {
				summary["readyImageCount"] = summary["readyImageCount"].(int) + 1
			} else {
				summary["readyVideoCount"] = summary["readyVideoCount"].(int) + 1
			}
		case "pending":
			summary["pendingCount"] = summary["pendingCount"].(int) + 1
		case "deferred":
			summary["deferredCount"] = summary["deferredCount"].(int) + 1
		case "failed":
			summary["failedCount"] = summary["failedCount"].(int) + 1
		case "blocked":
			summary["blockedCount"] = summary["blockedCount"].(int) + 1
		}
	}
	requiredReadyVideos := mcpRequiredReadyVideos(job)
	readyVideos := summary["readyVideoCount"].(int)
	videoRequests := summary["videoRequestCount"].(int)
	effectiveRequiredReadyVideos := requiredReadyVideos
	if videoRequests == 0 {
		effectiveRequiredReadyVideos = 0
	}
	videoSatisfied := effectiveRequiredReadyVideos <= 0 || readyVideos >= effectiveRequiredReadyVideos
	summary["requiredReadyVideoCount"] = effectiveRequiredReadyVideos
	summary["externalVideoRequirementSatisfied"] = videoSatisfied
	summary["unmetReadyVideoCount"] = maxInt(0, effectiveRequiredReadyVideos-readyVideos)
	summary["fallbackRequired"] = videoRequests > 0 && readyVideos == 0
	summary["needsAttention"] = !videoSatisfied || summary["failedCount"].(int) > 0 || summary["blockedCount"].(int) > 0
	return summary
}

func buildMCPAssetProvenance(results []interface{}) []interface{} {
	provenance := make([]interface{}, 0, len(results))
	for _, item := range results {
		result := mcpMapFromInterface(item)
		rawKind := strings.ToLower(strings.TrimSpace(mcpStringFromMap(result, "kind")))
		kind := mcpKindFromResult(result)
		if rawKind == "" {
			rawKind = kind
		}
		providerID := mcpStringFromMap(result, "providerId", "providerName")
		providerJobID := mcpStringFromMap(result, "providerJobId", "submitId", "jobId", "taskId")
		if providerJobID == "" {
			providerJobID = mcpStringFromMap(mcpMapFromInterface(result["structuredContent"]), "submit_id", "submitId", "job_id", "jobId", "task_id", "taskId")
		}
		entry := map[string]interface{}{
			"schemaVersion":     1,
			"requestId":         result["requestId"],
			"shotId":            result["shotId"],
			"kind":              rawKind,
			"sourceType":        mcpSourceTypeForKind(rawKind),
			"providerId":        providerID,
			"providerName":      providerID,
			"providerJobId":     providerJobID,
			"toolName":          result["toolName"],
			"status":            result["status"],
			"reason":            result["reason"],
			"error":             result["error"],
			"storageRef":        result["storageRef"],
			"localPath":         result["localPath"],
			"fallbackReason":    "",
			"isFallback":        false,
			"generatedAt":       result["generatedAt"],
			"inputPromptHash":   result["inputPromptHash"],
			"sourceArtifactIds": result["sourceArtifactIds"],
			"preflightQa":       result["preflightQa"],
		}
		provenance = append(provenance, entry)
	}
	return provenance
}

func mcpSourceTypeForKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "image", "reference_image", "keyframe":
		return "aigc_image"
	case "ip_aroll_video", "ip_a_roll_video":
		return "ip_aroll_video"
	case "video":
		return "aigc_video"
	default:
		return "unknown"
	}
}

func mcpInputPromptHash(request map[string]interface{}) string {
	prompt := mcpPromptFromExternalRequest(request)
	if strings.TrimSpace(prompt) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(prompt))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func mcpSourceArtifactIDs(request map[string]interface{}) []interface{} {
	for _, key := range []string{"sourceArtifactIds", "sourceArtifactIDs", "dependsOnArtifactIds", "referenceAssetIds"} {
		if values := stringSliceFromLocalMCPValue(request[key]); len(values) > 0 {
			out := make([]interface{}, 0, len(values))
			for _, value := range values {
				out = append(out, value)
			}
			return out
		}
	}
	return []interface{}{}
}

func mcpGenerationSummaryText(summary map[string]interface{}) string {
	readyVideos := mcpIntFromInterface(summary["readyVideoCount"])
	totalVideos := mcpIntFromInterface(summary["videoRequestCount"])
	readyImages := mcpIntFromInterface(summary["readyImageCount"])
	totalImages := mcpIntFromInterface(summary["imageRequestCount"])
	failed := mcpIntFromInterface(summary["failedCount"])
	blocked := mcpIntFromInterface(summary["blockedCount"])
	deferred := mcpIntFromInterface(summary["deferredCount"])
	pending := mcpIntFromInterface(summary["pendingCount"])
	text := fmt.Sprintf("MCP 生成结果：视频 ready %d/%d，图片 ready %d/%d，blocked %d，failed %d，deferred %d，pending %d。",
		readyVideos, totalVideos, readyImages, totalImages, blocked, failed, deferred, pending)
	requiredVideos := mcpIntFromInterface(summary["requiredReadyVideoCount"])
	if requiredVideos > 0 && !mcpBoolFromInterface(summary["externalVideoRequirementSatisfied"]) {
		text += fmt.Sprintf(" 未满足至少 %d 个 ready AIGC 视频素材要求，最终渲染只能使用 fallback 或等待重试。", requiredVideos)
	}
	return text
}

func mcpRequiredReadyVideos(job Job) int {
	for _, key := range []string{"minReadyVideoGenerations", "minimumReadyVideoGenerations", "requiredReadyVideoGenerations"} {
		if value := mcpIntFromInterface(mcpFirstPresent(job.Payload, key)); value > 0 {
			return value
		}
	}
	for _, key := range []string{"requireReadyVideoGenerations", "requireReadyVideos", "strictExternalVideoAssets"} {
		if mcpBoolFromInterface(mcpFirstPresent(job.Payload, key)) {
			return 1
		}
	}
	return 0
}

func mcpKindFromResult(result map[string]interface{}) string {
	if kind := strings.ToLower(strings.TrimSpace(mcpStringFromMap(result, "kind", "generationKind", "assetKind"))); kind != "" {
		if strings.Contains(kind, "image") || strings.Contains(kind, "keyframe") {
			return "image"
		}
		return "video"
	}
	toolName := strings.ToLower(strings.TrimSpace(mcpStringFromMap(result, "toolName")))
	if strings.Contains(toolName, "generate_image") {
		return "image"
	}
	return "video"
}

func mcpExternalRequestKind(request map[string]interface{}) string {
	kind := mcpExternalRequestType(request)
	if strings.Contains(kind, "image") || strings.Contains(kind, "keyframe") {
		return "image"
	}
	return "video"
}

func mcpExternalRequestType(request map[string]interface{}) string {
	kind := strings.ToLower(strings.TrimSpace(mcpStringFromMap(request, "kind", "generationKind", "assetKind")))
	if kind == "" {
		return "video"
	}
	return kind
}

func isIPArollExternalRequest(request map[string]interface{}) bool {
	return isIPArollKind(mcpExternalRequestType(request))
}

func isIPArollKind(kind string) bool {
	normalized := strings.ToLower(strings.TrimSpace(kind))
	return normalized == "ip_aroll_video" || normalized == "ip_a_roll_video"
}

type mcpGeneratedMedia struct {
	StorageRef        string
	LocalPath         string
	StructuredContent map[string]interface{}
}

func (e *mcpToolCallExecutor) resolveGeneratedMedia(ctx context.Context, client *localmcp.Client, providerID, toolName string, job Job, request map[string]interface{}, structured map[string]interface{}) mcpGeneratedMedia {
	media := e.importGeneratedMediaRef(job, request, firstGeneratedMediaRef(structured), structured)
	if media.StorageRef != "" {
		return media
	}
	submitID := mcpStringFromMap(structured, "submit_id", "submitId")
	if submitID == "" || strings.TrimSpace(e.dataDir) == "" || strings.TrimSpace(job.ProjectID) == "" {
		return mcpGeneratedMedia{}
	}
	queryTool := mcpQueryResultToolName(providerID, toolName)
	downloadDir := e.mcpDownloadDir(job.ProjectID, request)
	if downloadDir == "" {
		return mcpGeneratedMedia{}
	}
	pollTimeout := time.Duration(mcpIntFromInterface(mcpFirstPresent(mcpMapFromInterface(request["target"]), "pollTimeoutSec", "pollTimeout"))) * time.Second
	if pollTimeout <= 0 {
		pollTimeout = 4 * time.Minute
	}
	deadline := time.Now().Add(pollTimeout)
	for {
		callResult, err := mcpCallToolWithTimeout(ctx, client, queryTool, map[string]interface{}{
			"submit_id":    submitID,
			"download_dir": downloadDir,
		}, mcpQueryToolCallTimeout(request))
		if err != nil {
			if mcpIsTimeoutError(err) {
				return mcpGeneratedMedia{StructuredContent: map[string]interface{}{
					"submit_id":  submitID,
					"gen_status": "querying",
					"error":      err.Error(),
				}}
			}
			return mcpGeneratedMedia{StructuredContent: map[string]interface{}{
				"submit_id":  submitID,
				"gen_status": "failed",
				"error":      err.Error(),
			}}
		}
		if callResult != nil && callResult.IsError {
			return mcpGeneratedMedia{StructuredContent: map[string]interface{}{
				"submit_id":  submitID,
				"gen_status": "failed",
				"error":      mcpErrorText(callResult),
			}}
		}
		if callResult != nil {
			media = e.importGeneratedMediaRef(job, request, firstGeneratedMediaRef(callResult.StructuredContent), callResult.StructuredContent)
			if media.StorageRef != "" {
				return media
			}
			status := strings.ToLower(mcpStringFromMap(callResult.StructuredContent, "gen_status", "genStatus", "status"))
			if status == "failed" || status == "fail" || status == "error" {
				return mcpGeneratedMedia{StructuredContent: callResult.StructuredContent}
			}
		}
		if time.Now().After(deadline) {
			return mcpGeneratedMedia{}
		}
		select {
		case <-ctx.Done():
			return mcpGeneratedMedia{}
		case <-time.After(8 * time.Second):
		}
	}
}

func mcpQueryToolCallTimeout(request map[string]interface{}) time.Duration {
	if timeout := mcpDurationFromPayload(request, "mcpQueryToolCallTimeoutMs", "queryToolCallTimeoutMs", "mcp_query_timeout_ms"); timeout > 0 {
		return timeout
	}
	target := mcpMapFromInterface(request["target"])
	if timeout := mcpDurationFromPayload(target, "mcpQueryToolCallTimeoutMs", "queryToolCallTimeoutMs", "mcp_query_timeout_ms"); timeout > 0 {
		return timeout
	}
	if timeout := mcpSecondsFromPayload(request, "mcpQueryToolCallTimeoutSec", "queryToolCallTimeoutSec", "mcp_query_timeout_sec"); timeout > 0 {
		return timeout
	}
	if timeout := mcpSecondsFromPayload(target, "mcpQueryToolCallTimeoutSec", "queryToolCallTimeoutSec", "mcp_query_timeout_sec"); timeout > 0 {
		return timeout
	}
	return 45 * time.Second
}

func mcpQueryResultToolName(providerID, toolName string) string {
	name := strings.TrimSpace(toolName)
	if strings.Contains(name, "generate_video") {
		return strings.Replace(name, "generate_video", "query_result", 1)
	}
	if strings.Contains(name, "generate_image") {
		return strings.Replace(name, "generate_image", "query_result", 1)
	}
	if strings.Contains(name, ".") {
		prefix := name[:strings.LastIndex(name, ".")]
		return prefix + ".query_result"
	}
	if providerID != "" {
		return providerID + ".query_result"
	}
	return "query_result"
}

func (e *mcpToolCallExecutor) mcpDownloadDir(projectID string, request map[string]interface{}) string {
	projectID = safeImportSegment(projectID, "project")
	requestID := safeImportSegment(mcpStringFromMap(request, "requestId", "id"), "mcp-request")
	dir := filepath.Join(e.dataDir, "cache", "mcp", projectID, requestID)
	if err := ensureInside(e.dataDir, dir); err != nil {
		return ""
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	return dir
}

func firstGeneratedMediaRef(structured map[string]interface{}) string {
	if structured == nil {
		return ""
	}
	for _, key := range []string{"storageRef", "localPath", "path", "videoPath", "imagePath", "mediaPath", "video_url", "videoUrl", "image_url", "imageUrl", "url", "outputUrl"} {
		if value := mcpStringFromMap(structured, key); value != "" {
			return value
		}
	}
	resultJSON := mcpMapFromInterface(structured["result_json"])
	for _, key := range []string{"videos", "images", "files", "outputs"} {
		if ref := firstGeneratedMediaRefFromItems(interfaceSlice(resultJSON[key])); ref != "" {
			return ref
		}
	}
	return ""
}

func firstGeneratedMediaRefFromItems(items []interface{}) string {
	for _, item := range items {
		video := mcpMapFromInterface(item)
		for _, key := range []string{"path", "localPath", "storageRef", "videoUrl", "video_url", "imageUrl", "image_url", "url", "outputUrl"} {
			if value := mcpStringFromMap(video, key); value != "" {
				return value
			}
		}
	}
	return ""
}

func (e *mcpToolCallExecutor) importGeneratedMediaRef(job Job, request map[string]interface{}, mediaRef string, structured map[string]interface{}) mcpGeneratedMedia {
	mediaRef = strings.TrimSpace(mediaRef)
	if mediaRef == "" {
		return mcpGeneratedMedia{StructuredContent: structured}
	}
	if strings.HasPrefix(mediaRef, "http://") || strings.HasPrefix(mediaRef, "https://") || strings.HasPrefix(mediaRef, "local://") {
		return mcpGeneratedMedia{StorageRef: mediaRef, StructuredContent: structured}
	}
	if strings.TrimSpace(e.dataDir) == "" || strings.TrimSpace(job.ProjectID) == "" || !filepath.IsAbs(mediaRef) {
		return mcpGeneratedMedia{StructuredContent: structured}
	}
	source, err := os.Open(mediaRef)
	if err != nil {
		return mcpGeneratedMedia{StructuredContent: structured}
	}
	defer source.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, source); err != nil {
		return mcpGeneratedMedia{StructuredContent: structured}
	}
	hashHex := hex.EncodeToString(hasher.Sum(nil))
	if _, err := source.Seek(0, 0); err != nil {
		return mcpGeneratedMedia{StructuredContent: structured}
	}
	projectID := safeImportSegment(job.ProjectID, "project")
	artifactID := safeImportSegment(mcpStringFromMap(request, "requestId", "id"), "mcp-video")
	fallbackName := artifactID + ".mp4"
	if strings.Contains(strings.ToLower(mcpStringFromMap(request, "kind", "generationKind", "assetKind")), "image") {
		fallbackName = artifactID + ".png"
	}
	fileName := safeImportFileName(filepath.Base(mediaRef), fallbackName)
	contentPath := filepath.Join(e.dataDir, "artifacts", projectID, artifactID, "content")
	if err := ensureInside(e.dataDir, contentPath); err != nil {
		return mcpGeneratedMedia{StructuredContent: structured}
	}
	if err := os.MkdirAll(filepath.Dir(contentPath), 0o755); err != nil {
		return mcpGeneratedMedia{StructuredContent: structured}
	}
	target, err := os.Create(contentPath)
	if err != nil {
		return mcpGeneratedMedia{StructuredContent: structured}
	}
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()
		return mcpGeneratedMedia{StructuredContent: structured}
	}
	if err := target.Close(); err != nil {
		return mcpGeneratedMedia{StructuredContent: structured}
	}
	storageRef := fmt.Sprintf("local://projects/%s/artifacts/%s/sha256_%s/%s", projectID, artifactID, hashHex[:16], fileName)
	return mcpGeneratedMedia{StorageRef: storageRef, LocalPath: contentPath, StructuredContent: structured}
}

func mcpArgumentsFromExternalRequest(request map[string]interface{}) map[string]interface{} {
	target := mcpMapFromInterface(request["target"])
	kind := strings.ToLower(strings.TrimSpace(mcpStringFromMap(request, "kind", "generationKind", "assetKind")))
	args := copyMap(mcpMapFromInterface(request["arguments"]))
	if isIPArollExternalRequest(request) {
		if strings.TrimSpace(mcpStringFromMap(args, "script")) == "" {
			if script := mcpPromptFromExternalRequest(request); script != "" {
				args["script"] = script
			}
		}
		return args
	}
	if _, exists := args["prompt"]; !exists {
		args["prompt"] = mcpPromptFromExternalRequest(request)
	}
	if mode := mcpStringFromMap(request, "mode"); mode != "" && args["mode"] == nil {
		args["mode"] = mode
	}
	if duration := mcpIntFromInterface(mcpFirstPresent(target, "durationSec", "duration")); duration > 0 && !strings.Contains(kind, "image") && args["duration"] == nil {
		args["duration"] = duration
	}
	if ratio := mcpStringFromMap(target, "aspectRatio", "ratio"); ratio != "" && args["ratio"] == nil {
		args["ratio"] = ratio
	}
	if resolution := mcpStringFromMap(target, "videoResolution", "video_resolution", "resolution"); resolution != "" {
		if strings.Contains(kind, "image") {
			if args["resolution_type"] == nil {
				args["resolution_type"] = normalizeMCPImageResolution(resolution)
			}
		} else if args["video_resolution"] == nil {
			args["video_resolution"] = normalizeMCPVideoResolution(resolution)
		}
	}
	if generateNum := mcpIntFromInterface(mcpFirstPresent(target, "generateNum", "generate_num", "count")); generateNum > 0 && strings.Contains(kind, "image") && args["generate_num"] == nil {
		args["generate_num"] = generateNum
	}
	if model := mcpStringFromMap(target, "modelVersion", "model_version"); model != "" && args["model_version"] == nil {
		args["model_version"] = model
	}
	return args
}

func normalizeMCPImageResolution(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.ReplaceAll(normalized, "*", "x")
	switch normalized {
	case "1920x1080", "1080p", "fullhd", "fhd":
		return "2k"
	case "3840x2160", "2160p", "4k", "uhd":
		return "4k"
	default:
		return strings.TrimSpace(value)
	}
}

func normalizeMCPVideoResolution(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.ReplaceAll(normalized, "*", "x")
	switch normalized {
	case "3840x2160", "2160p", "4k", "uhd":
		return "4k"
	case "1920x1080", "1080p", "fullhd", "fhd":
		return "1080p"
	case "1280x720", "720p", "hd":
		return "720p"
	default:
		return strings.TrimSpace(value)
	}
}

func mcpErrorText(result *localmcp.ToolCallResult) string {
	if result == nil || !result.IsError {
		return ""
	}
	if text := strings.TrimSpace(mcpContentText(result.Content)); text != "" {
		return text
	}
	if message := mcpStringFromMap(result.StructuredContent, "error", "message", "fail_reason", "failReason"); message != "" {
		return message
	}
	return "mcp tool returned isError=true"
}

func mcpIsProviderBusyError(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	return strings.Contains(normalized, "exceedconcurrencylimit") ||
		strings.Contains(normalized, "concurrency limit") ||
		strings.Contains(normalized, "too many concurrent")
}

func mcpIsFailedGenerationStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "fail", "error":
		return true
	default:
		return false
	}
}

func mcpContentText(content []localmcp.ToolContent) string {
	var parts []string
	for _, item := range content {
		if text := strings.TrimSpace(item.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func (e *mcpToolCallExecutor) shotAssetPackageFromMCPResult(providerID string, request map[string]interface{}, structured map[string]interface{}, media mcpGeneratedMedia) map[string]interface{} {
	duration := mcpIntFromInterface(mcpFirstPresent(mcpMapFromInterface(request["target"]), "durationSec", "duration"))
	if actualDuration := mcpIntFromInterface(mcpFirstPresent(structured, "durationSec", "duration")); actualDuration > 0 {
		duration = actualDuration
	}
	if duration <= 0 {
		duration = 5
	}
	kind := strings.ToLower(strings.TrimSpace(mcpStringFromMap(request, "kind", "generationKind", "assetKind")))
	isImage := strings.Contains(kind, "image")
	isIPAroll := isIPArollKind(kind)
	outputKind := "SHOT_VIDEO_CLIP"
	mode := "aigc_video"
	baseKind := "video"
	baseRole := "base"
	if isImage {
		outputKind = mcpStringFromMap(request, "artifactKind", "outputArtifactKind")
		if outputKind == "" {
			outputKind = "REFERENCE_IMAGE"
		}
		mode = "aigc_image"
		baseKind = "image"
	} else if isIPAroll {
		outputKind = "IP_AROLL_VIDEO"
		mode = "ip_aroll_video"
		baseRole = "a_roll"
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	provenance := map[string]interface{}{
		"schemaVersion":     1,
		"sourceType":        mode,
		"providerName":      providerID,
		"providerJobId":     mcpStringFromMap(structured, "submit_id", "submitId", "job_id", "jobId", "task_id", "taskId"),
		"fallbackReason":    "",
		"isFallback":        false,
		"generatedAt":       generatedAt,
		"inputPromptHash":   mcpInputPromptHash(request),
		"sourceArtifactIds": mcpSourceArtifactIDs(request),
	}
	mediaPayload := map[string]interface{}{
		"requestId":  request["requestId"],
		"submitId":   mcpFirstPresent(structured, "submit_id", "submitId"),
		"genStatus":  mcpFirstPresent(structured, "gen_status", "genStatus", "status"),
		"provider":   providerID,
		"provenance": provenance,
		"raw":        structured,
	}
	packageItem := map[string]interface{}{
		"shotId":          request["shotId"],
		"durationSec":     duration,
		"requestId":       request["requestId"],
		"assetId":         mcpFirstPresent(request, "assetId", "referenceAssetId"),
		"kind":            kind,
		"schemaVersion":   1,
		"sourceType":      mode,
		"providerName":    providerID,
		"providerJobId":   provenance["providerJobId"],
		"fallbackReason":  "",
		"isFallback":      false,
		"generatedAt":     generatedAt,
		"provenance":      provenance,
		"referenceImages": request["references"],
		"prompts": map[string]interface{}{
			"videoPrompt":    mcpPromptFromExternalRequest(request),
			"negativePrompt": mcpStringFromMap(request, "negativePrompt"),
		},
	}
	if isIPAroll {
		packageItem["ipArollVideo"] = mediaPayload
	} else {
		packageItem["aigcVideo"] = mediaPayload
	}
	if media.StorageRef != "" {
		packageItem["generationPlan"] = map[string]interface{}{
			"mode":        mode,
			"primaryTool": "mcp_generation_runner",
			"fusionPlan": map[string]interface{}{
				"shotId": request["shotId"],
				"baseLayer": map[string]interface{}{
					"id":          request["requestId"],
					"kind":        baseKind,
					"role":        baseRole,
					"storageRef":  media.StorageRef,
					"durationSec": duration,
				},
				"overlayLayers":       []interface{}{},
				"assembler":           "hyperframes",
				"outputArtifactKind":  outputKind,
				"transitionCoveredIn": "shot_end",
			},
		}
		if isImage {
			packageItem["referenceImage"] = map[string]interface{}{
				"requestId":  request["requestId"],
				"storageRef": media.StorageRef,
				"provider":   providerID,
				"provenance": provenance,
				"raw":        structured,
			}
			if media.LocalPath != "" {
				packageItem["referenceImage"].(map[string]interface{})["localPath"] = media.LocalPath
			}
			return packageItem
		}
		if renderedVideo, ok := packageItem["ipArollVideo"].(map[string]interface{}); ok {
			renderedVideo["storageRef"] = media.StorageRef
			if media.LocalPath != "" {
				renderedVideo["localPath"] = media.LocalPath
			}
		} else if aigcVideo, ok := packageItem["aigcVideo"].(map[string]interface{}); ok {
			aigcVideo["storageRef"] = media.StorageRef
			if media.LocalPath != "" {
				aigcVideo["localPath"] = media.LocalPath
			}
		}
	}
	return packageItem
}

func findProvider(providers []localmcp.ProviderConfig, providerID string) (localmcp.ProviderConfig, bool) {
	for _, provider := range providers {
		if strings.EqualFold(provider.ID, providerID) && provider.Enabled {
			return provider, true
		}
	}
	return localmcp.ProviderConfig{}, false
}

func stringPayload(payload map[string]interface{}, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func mapPayload(payload map[string]interface{}, key string) map[string]interface{} {
	if payload == nil {
		return map[string]interface{}{}
	}
	if value, ok := payload[key].(map[string]interface{}); ok {
		return value
	}
	return map[string]interface{}{}
}

func slicePayload(payload map[string]interface{}, key string) []interface{} {
	if payload == nil {
		return nil
	}
	if value, ok := payload[key].([]interface{}); ok {
		return value
	}
	return nil
}

func mcpMapFromInterface(value interface{}) map[string]interface{} {
	if value, ok := value.(map[string]interface{}); ok {
		return value
	}
	return map[string]interface{}{}
}

func copyMap(input map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(input)+2)
	for key, value := range input {
		out[key] = value
	}
	return out
}

func mcpPromptFromExternalRequest(request map[string]interface{}) string {
	if prompt := mcpStringFromMap(request, "aigcPrompt", "aigcVideoPrompt", "positivePrompt"); prompt != "" {
		return prompt
	}
	if aigcPlan := mcpMapFromInterface(request["aigcPlan"]); len(aigcPlan) > 0 {
		if prompt := mcpStringFromMap(aigcPlan, "prompt", "videoPrompt", "positivePrompt"); prompt != "" {
			return prompt
		}
	}
	return mcpStringFromMap(request, "prompt", "promptText", "videoPrompt")
}

func mcpStringFromMap(input map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := input[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mcpFirstPresent(input map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, ok := input[key]; ok {
			return value
		}
	}
	return nil
}

func mcpIntFromInterface(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		return 0
	}
}

func mcpBoolFromInterface(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		normalized := strings.ToLower(strings.TrimSpace(v))
		return normalized == "true" || normalized == "1" || normalized == "yes" || normalized == "on"
	default:
		return false
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
