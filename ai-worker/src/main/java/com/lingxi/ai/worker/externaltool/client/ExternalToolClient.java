package com.lingxi.ai.worker.externaltool.client;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.lingxi.ai.worker.externaltool.model.*;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.http.HttpStatusCode;
import org.springframework.stereotype.Component;
import org.springframework.web.reactive.function.client.WebClient;
import reactor.core.publisher.Mono;
import reactor.util.retry.Retry;

import java.time.Duration;
import java.util.List;
import java.util.Map;

/**
 * 外部工具HTTP客户端
 */
@Slf4j
@Component
@RequiredArgsConstructor
public class ExternalToolClient {

    private final WebClient.Builder webClientBuilder;
    private final ObjectMapper objectMapper;

    /**
     * 获取工具元数据
     */
    public Mono<ToolInfo> getToolInfo(String endpoint) {
        String url = endpoint + "/info";
        log.debug("Fetching tool info from: {}", url);

        return webClientBuilder.build()
                .get()
                .uri(url)
                .retrieve()
                .onStatus(HttpStatusCode::isError, response ->
                        response.bodyToMono(String.class)
                                .flatMap(body -> Mono.error(
                                        new ToolClientException("Failed to get tool info: " + body, response.statusCode().value()))))
                .bodyToMono(String.class)
                .map(this::parseToolInfoResponse)
                .timeout(Duration.ofSeconds(10))
                .retryWhen(Retry.backoff(2, Duration.ofMillis(100))
                        .filter(this::isRetryable))
                .doOnError(e -> log.debug("Failed to get tool info from {}: {}", url, e.getMessage()));
    }

    /**
     * 执行工具
     */
    public Mono<Map<String, Object>> executeTool(String endpoint, ToolExecuteRequest request) {
        String url = endpoint + "/run";
        log.debug("Executing tool at: {}, nodeId={}, taskId={}", url, request.getNodeId(), request.getTaskId());

        return webClientBuilder.build()
                .post()
                .uri(url)
                .bodyValue(request)
                .retrieve()
                .onStatus(HttpStatusCode::isError, response ->
                        response.bodyToMono(String.class)
                                .flatMap(body -> Mono.error(
                                        new ToolClientException("Tool execution failed: " + body, response.statusCode().value()))))
                .bodyToMono(String.class)
                .map(this::parseExecuteResponse)
                .timeout(Duration.ofMillis(request.getInput() != null && request.getInput().get("timeout") != null
                        ? Long.parseLong(request.getInput().get("timeout").toString())
                        : 30000))
                .doOnError(e -> log.debug("Tool execution failed at {}: {}", url, e.getMessage()));
    }

    /**
     * 健康检查
     */
    public Mono<ToolHealth> checkHealth(String endpoint) {
        String url = endpoint + "/health";
        log.debug("Checking tool health at: {}", url);

        return webClientBuilder.build()
                .get()
                .uri(url)
                .retrieve()
                .onStatus(HttpStatusCode::isError, response ->
                        Mono.error(new ToolClientException("Health check failed", response.statusCode().value())))
                .bodyToMono(String.class)
                .map(this::parseHealthResponse)
                .timeout(Duration.ofSeconds(5))
                .onErrorResume(e -> {
                    log.debug("Health check failed for {}: {}", url, e.getMessage());
                    return Mono.just(ToolHealth.builder()
                            .status("DOWN")
                            .timestamp(System.currentTimeMillis())
                            .build());
                });
    }

    /**
     * 解析工具元数据响应
     */
    private ToolInfo parseToolInfoResponse(String responseBody) {
        try {
            ToolResponse<ToolInfo> response = objectMapper.readValue(
                    responseBody,
                    new TypeReference<ToolResponse<ToolInfo>>() {}
            );

            if (response.isSuccess()) {
                return response.getData() != null ? response.getData() : convertToStandardToolInfo(responseBody);
            } else {
                throw new ToolClientException("Tool info response failed: " + response.getMessage(), response.getCode());
            }
        } catch (ToolClientException e) {
            throw e;
        } catch (Exception e) {
            log.warn("Failed to parse tool info with standard methods, trying raw parse: {}", e.getMessage());
            return convertToStandardToolInfo(responseBody);
        }
    }

    /**
     * 从原始JSON转换为ToolInfo
     */
    @SuppressWarnings("unchecked")
    private ToolInfo convertToStandardToolInfo(String rawJson) {
        try {
            Map<String, Object> map = objectMapper.readValue(rawJson, Map.class);

            if (map.containsKey("data") && map.get("data") instanceof Map) {
                map = (Map<String, Object>) map.get("data");
            }

            ToolInfo.ToolInfoBuilder builder = ToolInfo.builder();

            if (map.containsKey("toolId")) {
                builder.toolId(map.get("toolId").toString());
            }

            if (map.containsKey("toolName")) {
                builder.toolName(map.get("toolName").toString());
            }

            if (map.containsKey("toolVersion")) {
                builder.toolVersion(map.get("toolVersion").toString());
            }

            if (map.containsKey("description")) {
                builder.description(map.get("description").toString());
            }

            if (map.containsKey("author")) {
                builder.author(map.get("author").toString());
            }

            if (map.containsKey("type")) {
                builder.type(map.get("type").toString());
            }

            if (map.containsKey("tags")) {
                builder.tags((List<String>) map.get("tags"));
            }

            if (map.containsKey("inputSchema")) {
                builder.inputSchema((Map<String, Object>) map.get("inputSchema"));
            }

            if (map.containsKey("outputSchema")) {
                builder.outputSchema((Map<String, Object>) map.get("outputSchema"));
            }

            if (map.containsKey("timeout")) {
                builder.timeout((Integer) map.get("timeout"));
            }

            if (map.containsKey("maxRetry")) {
                builder.maxRetry((Integer) map.get("maxRetry"));
            }

            return builder.build();
        } catch (Exception e) {
            throw new ToolClientException("Failed to parse tool info: " + e.getMessage(), 500);
        }
    }

    /**
     * 解析工具执行响应
     */
    @SuppressWarnings("unchecked")
    private Map<String, Object> parseExecuteResponse(String responseBody) {
        try {
            ToolResponse<Map<String, Object>> response = objectMapper.readValue(
                    responseBody,
                    new TypeReference<ToolResponse<Map<String, Object>>>() {}
            );

            if (!response.isSuccess()) {
                String errorMsg = response.getError() != null ? response.getError() : response.getMessage();
                throw new ToolExecutionException(errorMsg, response.getCode(), response.getData());
            }
            return response.getData();
        } catch (ToolExecutionException e) {
            throw e;
        } catch (Exception e) {
            throw new ToolClientException("Failed to parse execute response: " + e.getMessage(), 500);
        }
    }

    /**
     * 解析健康检查响应
     */
    private ToolHealth parseHealthResponse(String responseBody) {
        try {
            ToolResponse<ToolHealth> response = objectMapper.readValue(
                    responseBody,
                    new TypeReference<ToolResponse<ToolHealth>>() {}
            );
            return response.isSuccess() && response.getData() != null
                    ? response.getData()
                    : ToolHealth.builder().status("DOWN").timestamp(System.currentTimeMillis()).build();
        } catch (Exception e) {
            log.debug("Failed to parse health response, returning DOWN: {}", e.getMessage());
            return ToolHealth.builder()
                    .status("DOWN")
                    .timestamp(System.currentTimeMillis())
                    .build();
        }
    }

    private boolean isRetryable(Throwable throwable) {
        if (throwable instanceof ToolClientException) {
            int code = ((ToolClientException) throwable).getStatusCode();
            return code >= 500 || code == 408 || code == 429;
        }
        return !(throwable instanceof ToolExecutionException);
    }

    /**
     * 工具客户端异常
     */
    public static class ToolClientException extends RuntimeException {
        private final int statusCode;

        public ToolClientException(String message, int statusCode) {
            super(message);
            this.statusCode = statusCode;
        }

        public int getStatusCode() {
            return statusCode;
        }
    }

    /**
     * 工具执行异常（业务异常）
     */
    public static class ToolExecutionException extends RuntimeException {
        private final int errorCode;
        private final Map<String, Object> errorData;

        public ToolExecutionException(String message, int errorCode, Map<String, Object> errorData) {
            super(message);
            this.errorCode = errorCode;
            this.errorData = errorData;
        }

        public int getErrorCode() {
            return errorCode;
        }

        public Map<String, Object> getErrorData() {
            return errorData;
        }
    }
}
