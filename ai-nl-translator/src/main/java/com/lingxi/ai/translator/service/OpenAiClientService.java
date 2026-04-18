package com.lingxi.ai.translator.service;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.lingxi.ai.translator.config.OpenAiConfig;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;
import org.springframework.http.HttpEntity;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpMethod;
import org.springframework.http.MediaType;
import org.springframework.web.client.RestTemplate;

import java.util.Arrays;
import java.util.HashMap;
import java.util.Map;

@Service
public class OpenAiClientService {
    private static final Logger logger = LoggerFactory.getLogger(OpenAiClientService.class);

    @Autowired
    private OpenAiConfig openAiConfig;

    @Autowired
    private ObjectMapper objectMapper;

    @Autowired
    private RestTemplate restTemplate;

    private static final String SYSTEM_PROMPT = """
        你是一个任务分解专家。请将用户的自然语言任务分解为多个执行节点（Node），并构建一个有向无环图（DAG）。

        每个 Node 的格式：
        - id: 节点唯一标识（字符串）
        - type: 节点类型（LLM 或 TOOL）
        - name: 节点名称（如 write_article, summarize 等）
        - input: 输入数据（Map格式，可为空）
        - deps: 依赖的节点ID列表（空列表表示无依赖）

        要求：
        1. 节点之间可以有线性依赖、并行关系
        2. 返回纯 JSON 格式，不要有其他文字
        3. JSON 结构应该是：{"nodes": [node1, node2, ...], "edges": [edge1, edge2, ...]}
        4. edges 格式：{"from": "nodeId1", "to": "nodeId2"}

        示例输出：
        {
          "nodes": [
            {
              "id": "1",
              "type": "LLM",
              "name": "write_article",
              "input": {},
              "deps": []
            },
            {
              "id": "2",
              "type": "LLM",
              "name": "summarize",
              "input": {},
              "deps": ["1"]
            }
          ],
          "edges": [
            {"from": "1", "to": "2"}
          ]
        }
        """;

    public String callOpenAi(String userPrompt) {
        logger.info("Calling LLM API with prompt: {}", userPrompt);

        String apiKey = openAiConfig.getApiKey();
        if (apiKey == null || apiKey.isBlank() || apiKey.equals("your-api-key-here")) {
            throw new RuntimeException("API key is not configured. Please set the OPENAI_API_KEY environment variable");
        }

        String baseUrl = openAiConfig.getBaseUrl();
        if (baseUrl == null || baseUrl.isBlank()) {
            baseUrl = "https://api.openai.com/v1";
        }
        if (!baseUrl.endsWith("/")) {
            baseUrl = baseUrl + "/";
        }

        String endpoint = baseUrl + "chat/completions";
        logger.info("Calling endpoint: {}", endpoint);

        try {
            // 构建请求体
            Map<String, Object> requestBody = new HashMap<>();
            requestBody.put("model", openAiConfig.getModel());
            requestBody.put("temperature", openAiConfig.getTemperature());
            requestBody.put("max_tokens", openAiConfig.getMaxTokens());

            // 构建消息
            Map<String, String> systemMessage = new HashMap<>();
            systemMessage.put("role", "system");
            systemMessage.put("content", SYSTEM_PROMPT);

            Map<String, String> userMessage = new HashMap<>();
            userMessage.put("role", "user");
            userMessage.put("content", userPrompt);

            requestBody.put("messages", Arrays.asList(systemMessage, userMessage));

            // 构建请求头
            HttpHeaders headers = new HttpHeaders();
            headers.setContentType(MediaType.APPLICATION_JSON);
            headers.set("Authorization", "Bearer " + apiKey);

            // 构建请求实体
            HttpEntity<Map<String, Object>> entity = new HttpEntity<>(requestBody, headers);

            // 发送请求
            String response = restTemplate.postForObject(endpoint, entity, String.class);
            logger.info("LLM API response received: {}", response);

            // 解析响应
            Map<?, ?> responseMap = objectMapper.readValue(response, Map.class);
            Map<?, ?> choices = (Map<?, ?>) ((java.util.List<?>) responseMap.get("choices")).get(0);
            Map<?, ?> message = (Map<?, ?>) choices.get("message");
            String content = (String) message.get("content");

            logger.info("Extracted content: {}", content);
            return content;
        } catch (Exception e) {
            logger.error("Error calling LLM API", e);
            throw new RuntimeException("Failed to call LLM API: " + e.getMessage(), e);
        }
    }
}
