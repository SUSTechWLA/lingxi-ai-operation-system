package com.lingxi.ai.worker.client;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;
import org.springframework.web.reactive.function.client.WebClient;
import reactor.core.publisher.Mono;

import java.util.HashMap;
import java.util.Map;
import java.util.Optional;

/**
 * AI-Context服务客户端
 * 用于调用上下文服务的API
 */
@Component
public class ContextClient {

    private final WebClient webClient;

    public ContextClient(
            WebClient.Builder webClientBuilder,
            @Value("${context.service.url:http://localhost:8082}") String contextServiceUrl) {
        this.webClient = webClientBuilder.baseUrl(contextServiceUrl).build();
    }

    /**
     * 保存节点快照
     */
    public Mono<Void> saveNodeSnapshot(String taskId, String nodeId,
                                       String nodeType, String nodeName,
                                       String status, Map<String, Object> input,
                                       Map<String, Object> output, int retryCount,
                                       int maxRetry, int priority, String workerGroup,
                                       Integer version, String errorMessage) {
        Map<String, Object> request = new HashMap<>();
        request.put("taskId", taskId);
        request.put("nodeId", nodeId);
        request.put("type", nodeType);
        request.put("name", nodeName);
        request.put("status", status);
        request.put("input", input != null ? input : Map.of());
        request.put("output", output != null ? output : Map.of());
        request.put("retryCount", retryCount);
        request.put("maxRetry", maxRetry);
        request.put("priority", priority);
        request.put("workerGroup", workerGroup);
        request.put("version", version != null ? version : 0);
        request.put("errorMessage", errorMessage != null ? errorMessage : "");

        return webClient.post()
                .uri("/api/context/node-snapshot")
                .bodyValue(request)
                .retrieve()
                .bodyToMono(Void.class);
    }

    /**
     * 获取节点最新快照
     */
    public Mono<Optional<Map<String, Object>>> getLatestSnapshot(String nodeId) {
        return webClient.get()
                .uri("/api/node/{nodeId}/snapshot/latest", nodeId)
                .retrieve()
                .bodyToMono(Map.class)
                .map(response -> Optional.ofNullable((Map<String, Object>) response.get("snapshot")))
                .onErrorResume(e -> Mono.just(Optional.empty()));
    }

    /**
     * 记录节点开始运行
     */
    public Mono<Void> recordNodeScheduled(String taskId, String nodeId, String type, String name) {
        Map<String, Object> request = Map.of(
                "taskId", taskId,
                "nodeId", nodeId,
                "type", type,
                "name", name
        );

        return webClient.post()
                .uri("/api/context/node-scheduled")
                .bodyValue(request)
                .retrieve()
                .bodyToMono(Void.class);
    }

    /**
     * 记录节点成功
     */
    public Mono<Void> recordNodeSuccess(String taskId, String nodeId) {
        Map<String, String> request = Map.of(
                "taskId", taskId,
                "nodeId", nodeId
        );

        return webClient.post()
                .uri("/api/context/node-success")
                .bodyValue(request)
                .retrieve()
                .bodyToMono(Void.class);
    }

    /**
     * 记录节点失败
     */
    public Mono<Void> recordNodeFailed(String taskId, String nodeId, String errorMessage) {
        Map<String, String> request = Map.of(
                "taskId", taskId,
                "nodeId", nodeId,
                "errorMessage", errorMessage
        );

        return webClient.post()
                .uri("/api/context/node-failed")
                .bodyValue(request)
                .retrieve()
                .bodyToMono(Void.class);
    }
}
