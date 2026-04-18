package com.lingxi.ai.worker.externaltool.model;

import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.Map;

/**
 * 工具执行请求
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ToolExecuteRequest {

    @JsonProperty("nodeId")
    private String nodeId;

    @JsonProperty("taskId")
    private String taskId;

    @JsonProperty("traceId")
    private String traceId;

    @JsonProperty("input")
    private Map<String, Object> input;
}
