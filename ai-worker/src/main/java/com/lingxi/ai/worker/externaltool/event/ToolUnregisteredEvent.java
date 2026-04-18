package com.lingxi.ai.worker.externaltool.event;

import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.UUID;

/**
 * 工具注销事件 (ai.tool.unregistered)
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ToolUnregisteredEvent {

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
    private ToolUnregisteredData data;

    @JsonProperty("version")
    private String version;

    public static ToolUnregisteredEvent create(String toolName, String toolEndpoint) {
        return ToolUnregisteredEvent.builder()
                .eventId(UUID.randomUUID().toString())
                .eventType("ai.tool.unregistered")
                .taskId("")
                .nodeId("")
                .timestamp(System.currentTimeMillis())
                .source("ai-worker")
                .data(ToolUnregisteredData.builder()
                        .toolName(toolName)
                        .toolEndpoint(toolEndpoint)
                        .build())
                .version("1.0")
                .build();
    }

    @Data
    @Builder
    @NoArgsConstructor
    @AllArgsConstructor
    public static class ToolUnregisteredData {
        @JsonProperty("toolName")
        private String toolName;

        @JsonProperty("toolEndpoint")
        private String toolEndpoint;
    }
}
