package com.lingxi.ai.translator.service;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.lingxi.ai.translator.config.OpenAiConfig;
import com.theokanning.openai.completion.chat.ChatCompletionRequest;
import com.theokanning.openai.completion.chat.ChatMessage;
import com.theokanning.openai.service.OpenAiService;
import okhttp3.OkHttpClient;
import retrofit2.Retrofit;
import retrofit2.adapter.rxjava2.RxJava2CallAdapterFactory;
import retrofit2.converter.jackson.JacksonConverterFactory;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

import java.time.Duration;
import java.util.Arrays;

@Service
public class OpenAiClientService {
    private static final Logger logger = LoggerFactory.getLogger(OpenAiClientService.class);

    @Autowired
    private OpenAiConfig openAiConfig;

    @Autowired
    private ObjectMapper objectMapper;

    private static final String SYSTEM_PROMPT = """
        你是一个任务分解专家。请将用户的自然语言任务分解为多个执行节点（Node），并构建一个有向无环图（DAG）。

        每个 Node 的格式：
        - nodeId: 节点唯一标识（字符串）
        - type: 节点类型（LLM 或 TOOL）
        - task: 任务描述（如 write_article, summarize 等）
        - deps: 依赖的节点ID列表（空列表表示无依赖）
        - input: 输入数据（Map格式，可为空）

        要求：
        1. 节点之间可以有线性依赖、并行关系
        2. 返回纯 JSON 格式，不要有其他文字
        3. JSON 结构应该是：{"nodes": [node1, node2, ...]}

        示例输出：
        {
          "nodes": [
            {
              "nodeId": "1",
              "type": "LLM",
              "task": "write_article",
              "deps": [],
              "input": {}
            },
            {
              "nodeId": "2",
              "type": "LLM",
              "task": "summarize",
              "deps": ["1"],
              "input": {}
            }
          ]
        }
        """;

    public String callOpenAi(String userPrompt) {
        logger.info("Calling OpenAI with prompt: {}", userPrompt);

        String apiKey = openAiConfig.getApiKey();
        if (apiKey == null || apiKey.isBlank() || apiKey.equals("your-api-key-here")) {
            throw new RuntimeException("OpenAI API key is not configured. Please set the OPENAI_API_KEY environment variable or update the openai.api-key in application.yml");
        }

        String baseUrl = openAiConfig.getBaseUrl();
        if (baseUrl == null || baseUrl.isBlank()) {
            baseUrl = "https://api.openai.com/v1";
        }
        if (!baseUrl.endsWith("/")) {
            baseUrl = baseUrl + "/";
        }

        OkHttpClient client = OpenAiService.defaultClient(apiKey, Duration.ofMillis(openAiConfig.getTimeout()));
        Retrofit retrofit = new Retrofit.Builder()
                .baseUrl(baseUrl)
                .client(client)
                .addConverterFactory(JacksonConverterFactory.create(objectMapper))
                .addCallAdapterFactory(RxJava2CallAdapterFactory.create())
                .build();
        com.theokanning.openai.client.OpenAiApi api = retrofit.create(com.theokanning.openai.client.OpenAiApi.class);
        OpenAiService service = new OpenAiService(api);

        ChatMessage systemMessage = new ChatMessage("system", SYSTEM_PROMPT);
        ChatMessage userMessage = new ChatMessage("user", userPrompt);

        ChatCompletionRequest request = ChatCompletionRequest.builder()
                .model(openAiConfig.getModel())
                .messages(Arrays.asList(systemMessage, userMessage))
                .temperature(openAiConfig.getTemperature())
                .maxTokens(openAiConfig.getMaxTokens())
                .build();

        try {
            String response = service.createChatCompletion(request)
                    .getChoices()
                    .get(0)
                    .getMessage()
                    .getContent();

            logger.info("OpenAI response received: {}", response);
            return response;
        } catch (Exception e) {
            logger.error("Error calling OpenAI", e);
            throw new RuntimeException("Failed to call OpenAI: " + e.getMessage(), e);
        }
    }
}
