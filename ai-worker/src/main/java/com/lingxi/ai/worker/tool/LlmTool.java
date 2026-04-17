package com.lingxi.ai.worker.tool;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.theokanning.openai.completion.chat.ChatCompletionRequest;
import com.theokanning.openai.completion.chat.ChatMessage;
import com.theokanning.openai.service.OpenAiService;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * LLM工具 - 对接OpenAI API
 */
@Slf4j
@Component
public class LlmTool implements Tool {

    private final OpenAiService openAiService;
    private final ObjectMapper objectMapper;

    @Value("${openai.model:gpt-4}")
    private String model;

    @Value("${openai.max-tokens:2000}")
    private int maxTokens;

    @Value("${openai.temperature:0.7}")
    private double temperature;

    public LlmTool(OpenAiService openAiService, ObjectMapper objectMapper) {
        this.openAiService = openAiService;
        this.objectMapper = objectMapper;
    }

    @Override
    public String getName() {
        return "llm";
    }

    @Override
    public String getDescription() {
        return "大语言模型工具，支持文本生成、对话、摘要等功能";
    }

    @Override
    public ToolType getType() {
        return ToolType.LLM;
    }

    @Override
    public ToolResult execute(Map<String, Object> parameters, ToolContext context) {
        Instant startTime = Instant.now();
        log.info("Executing LLM tool for task: {}, node: {}", context.getTaskId(), context.getNodeId());

        try {
            String prompt = getParameter(parameters, "prompt", String.class);
            String systemPrompt = getParameter(parameters, "system_prompt", String.class, "你是一个有帮助的AI助手。");
            String modelOverride = getParameter(parameters, "model", String.class, model);
            Integer maxTokensOverride = getParameter(parameters, "max_tokens", Integer.class, maxTokens);
            Double temperatureOverride = getParameter(parameters, "temperature", Double.class, temperature);

            List<ChatMessage> messages = new ArrayList<>();
            messages.add(new ChatMessage("system", systemPrompt));
            messages.add(new ChatMessage("user", prompt));

            ChatCompletionRequest request = ChatCompletionRequest.builder()
                    .model(modelOverride)
                    .messages(messages)
                    .maxTokens(maxTokensOverride)
                    .temperature(temperatureOverride)
                    .build();

            var response = openAiService.createChatCompletion(request);
            String resultText = response.getChoices().get(0).getMessage().getContent();

            Map<String, Object> resultData = new HashMap<>();
            resultData.put("text", resultText);
            resultData.put("model", modelOverride);
            resultData.put("usage", response.getUsage());

            log.info("LLM tool executed successfully for task: {}", context.getTaskId());
            return ToolResult.success(resultData, startTime, Instant.now());

        } catch (Exception e) {
            log.error("LLM tool execution failed for task: {}", context.getTaskId(), e);
            return ToolResult.failure(e.getMessage(), startTime, Instant.now());
        }
    }

    @Override
    public boolean validateParameters(Map<String, Object> parameters) {
        if (parameters == null || !parameters.containsKey("prompt")) {
            return false;
        }
        Object prompt = parameters.get("prompt");
        return prompt instanceof String && !((String) prompt).isBlank();
    }

    @SuppressWarnings("unchecked")
    private <T> T getParameter(Map<String, Object> parameters, String key, Class<T> type) {
        Object value = parameters.get(key);
        if (type.isInstance(value)) {
            return (T) value;
        }
        return null;
    }

    @SuppressWarnings("unchecked")
    private <T> T getParameter(Map<String, Object> parameters, String key, Class<T> type, T defaultValue) {
        Object value = parameters.get(key);
        if (type.isInstance(value)) {
            return (T) value;
        }
        return defaultValue;
    }
}
