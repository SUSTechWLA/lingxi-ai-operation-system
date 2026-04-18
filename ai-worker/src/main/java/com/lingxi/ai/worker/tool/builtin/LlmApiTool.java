package com.lingxi.ai.worker.tool.builtin;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.lingxi.ai.worker.tool.Tool;
import com.lingxi.ai.worker.tool.ToolContext;
import com.lingxi.ai.worker.tool.ToolResult;
import com.lingxi.ai.worker.tool.ToolType;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;
import org.springframework.web.reactive.function.client.WebClient;

import java.time.Instant;
import java.util.List;
import java.util.Map;

/**
 * 内置LLM API调用工具
 * 直接调用OpenAI兼容的API，无网络开销（除了API本身的调用）
 */
@Slf4j
@Component
@RequiredArgsConstructor
public class LlmApiTool implements Tool {

    @Value("${openai.api-key:}")
    private String apiKey;

    @Value("${openai.base-url:https://api.openai.com/v1}")
    private String baseUrl;

    @Value("${openai.model:gpt-4}")
    private String defaultModel;

    @Value("${openai.max-tokens:2000}")
    private int defaultMaxTokens;

    @Value("${openai.temperature:0.7}")
    private double defaultTemperature;

    private final WebClient.Builder webClientBuilder;
    private final ObjectMapper objectMapper;

    @Override
    public String getName() {
        return "llm_api";
    }

    @Override
    public String getDescription() {
        return "调用LLM API进行对话、补全、嵌入等操作";
    }

    @Override
    public ToolType getType() {
        return ToolType.LLM;
    }

    @Override
    public ToolResult execute(Map<String, Object> parameters, ToolContext context) {
        Instant startTime = Instant.now();

        try {
            String prompt = (String) parameters.get("prompt");
            String model = parameters.get("model") != null
                    ? parameters.get("model").toString()
                    : defaultModel;

            if (prompt == null || prompt.trim().isEmpty()) {
                return ToolResult.failure("Prompt is required", startTime, Instant.now());
            }

            if (apiKey == null || apiKey.trim().isEmpty()) {
                return ToolResult.failure("OpenAI API key not configured", startTime, Instant.now());
            }

            log.info("Calling LLM API: taskId={}, model={}", context.getTaskId(), model);

            // 构建请求
            Map<String, Object> requestBody = Map.of(
                    "model", model,
                    "messages", List.of(
                            Map.of("role", "user", "content", prompt)
                    ),
                    "max_tokens", parameters.get("max_tokens") != null
                            ? parameters.get("max_tokens")
                            : defaultMaxTokens,
                    "temperature", parameters.get("temperature") != null
                            ? parameters.get("temperature")
                            : defaultTemperature
            );

            // 同步调用（实际应该在NodeExecutor中使用异步）
            String response = webClientBuilder.build()
                    .post()
                    .uri(baseUrl + "/chat/completions")
                    .header("Authorization", "Bearer " + apiKey)
                    .header("Content-Type", "application/json")
                    .bodyValue(requestBody)
                    .retrieve()
                    .bodyToMono(String.class)
                    .block();

            // 解析响应
            Map<String, Object> responseMap = objectMapper.readValue(response, Map.class);
            String content = extractContent(responseMap);

            log.info("LLM API call completed: taskId={}", context.getTaskId());

            return ToolResult.success(
                    Map.of(
                            "content", content,
                            "model", model,
                            "rawResponse", responseMap
                    ),
                    startTime,
                    Instant.now()
            );

        } catch (Exception e) {
            log.error("LLM API call failed", e);
            return ToolResult.failure("LLM API call failed: " + e.getMessage(), startTime, Instant.now());
        }
    }

    @Override
    public boolean validateParameters(Map<String, Object> parameters) {
        return parameters != null && parameters.containsKey("prompt");
    }

    @SuppressWarnings("unchecked")
    private String extractContent(Map<String, Object> response) {
        try {
            List<Map<String, Object>> choices = (List<Map<String, Object>>) response.get("choices");
            if (choices != null && !choices.isEmpty()) {
                Map<String, Object> message = (Map<String, Object>) choices.get(0).get("message");
                if (message != null) {
                    return (String) message.get("content");
                }
            }
        } catch (Exception e) {
            log.warn("Failed to extract content from LLM response", e);
        }
        return null;
    }
}
