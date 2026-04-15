package com.lingxi.ai.translator.service;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.lingxi.ai.translator.model.DAG;
import com.lingxi.ai.translator.model.Node;
import com.lingxi.ai.translator.model.NodeStatus;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.HttpStatusCode;
import org.springframework.stereotype.Service;
import org.springframework.web.reactive.function.client.WebClient;
import reactor.core.publisher.Mono;

import java.util.List;
import java.util.Map;

@Service
public class NlToDagService {
    private static final Logger logger = LoggerFactory.getLogger(NlToDagService.class);

    @Autowired
    private OpenAiClientService openAiClientService;

    @Autowired
    private ObjectMapper objectMapper;

    @Autowired
    private WebClient.Builder webClientBuilder;

    private WebClient webClient;

    public DAG translateToDag(String prompt) {
        logger.info("Translating natural language to DAG: {}", prompt);

        // 调用 OpenAI 获取 JSON 响应
        String openAiResponse = openAiClientService.callOpenAi(prompt);

        // 解析 JSON 为 DAG 对象
        try {
            DAG dag = objectMapper.readValue(openAiResponse, DAG.class);
            // 初始化所有节点状态为 PENDING
            for (Node node : dag.getNodes()) {
                node.setStatus(NodeStatus.PENDING);
            }
            logger.info("Successfully translated to DAG with {} nodes", dag.getNodes().size());
            return dag;
        } catch (Exception e) {
            logger.error("Failed to parse OpenAI response to DAG", e);
            throw new RuntimeException("Failed to parse DAG: " + e.getMessage(), e);
        }
    }

    public Map<String, Object> translateAndSubmit(String prompt, String orchestratorUrl) {
        // 1. 翻译为 DAG
        DAG dag = translateToDag(prompt);

        // 2. 提交给 orchestrator
        if (webClient == null) {
            webClient = webClientBuilder.baseUrl(orchestratorUrl).build();
        }

        try {
            Map<String, Object> result = webClient.post()
                    .uri("/api/node")
                    .bodyValue(dag)
                    .retrieve()
                    .onStatus(HttpStatusCode::isError, clientResponse ->
                            clientResponse.bodyToMono(String.class)
                                    .flatMap(errorBody -> Mono.error(
                                            new RuntimeException("Orchestrator error: " + errorBody)
                                    ))
                    )
                    .bodyToMono(Map.class)
                    .block();

            logger.info("Successfully submitted task to orchestrator: {}", result);
            return result;
        } catch (Exception e) {
            logger.error("Failed to submit task to orchestrator", e);
            throw new RuntimeException("Failed to submit to orchestrator: " + e.getMessage(), e);
        }
    }
}
