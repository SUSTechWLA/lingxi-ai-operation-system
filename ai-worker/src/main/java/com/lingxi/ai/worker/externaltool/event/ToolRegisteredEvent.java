package com.lingxi.ai.worker.externaltool.event;

import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.List;
import java.util.Map;
import java.util.UUID;

/**
 * 工具注册事件 (ai.tool.registered)
 * 与现有事件格式完全兼容
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ToolRegisteredEvent {

    @JsonProperty("event_id")
    private String eventId;

    @JsonProperty("event_type")
    private String eventType;

    @JsonProperty("task_id")
    private String taskId;

    @JsonProperty("node_id")
    private String nodeId;

    @JsonProperty("timestamp")
    private Long timestamp;

    @JsonProperty("source")
    private String source;

    @JsonProperty("data")
    private ToolRegisteredData data;

    @JsonProperty("version")
    private String version;

    public static ToolRegisteredEvent create(ToolRegisteredData data) {
        return ToolRegisteredEvent.builder()
                .eventId(UUID.randomUUID().toString())
                .eventType("ai.tool.registered")
                .taskId("")
                .nodeId("")
                .timestamp(System.currentTimeMillis())
                .source("ai-worker")
                .data(data)
                .version("1.0")
                .build();
    }

    @Data
    @Builder
    @NoArgsConstructor
    @AllArgsConstructor
    public static class ToolRegisteredData {
        // 完整的工具元数据（与/tool/info接口返回的data完全一致）
        @JsonProperty("toolName")
        private String toolName;

        @JsonProperty("toolVersion")
        private String toolVersion;

        @JsonProperty("description")
        private String description;

        @JsonProperty("author")
        private String author;

        @JsonProperty("tags")
        private List<String> tags;

        @JsonProperty("inputSchema")
        private Map<String, Object> inputSchema;

        @JsonProperty("outputSchema")
        private Map<String, Object> outputSchema;

        @JsonProperty("examples")
        private List<ToolExample> examples;

        @JsonProperty("timeout")
        private Integer timeout;

        @JsonProperty("maxRetry")
        private Integer maxRetry;

        // 额外注册信息
        @JsonProperty("workerId")
        private String workerId;

        @JsonProperty("workerGroup")
        private String workerGroup;

        @JsonProperty("toolEndpoint")
        private String toolEndpoint;
    }

    @Data
    @Builder
    @NoArgsConstructor
    @AllArgsConstructor
    public static class ToolExample {
        @JsonProperty("input")
        private Map<String, Object> input;

        @JsonProperty("output")
        private Map<String, Object> output;
    }
}
