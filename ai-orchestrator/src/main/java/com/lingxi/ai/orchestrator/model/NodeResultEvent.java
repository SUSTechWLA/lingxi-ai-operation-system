package com.lingxi.ai.orchestrator.model;

import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;
import java.util.Map;

@Data
@NoArgsConstructor
@AllArgsConstructor
public class NodeResultEvent {
    private String taskId;
    private String nodeId;
    private NodeStatus status;
    private Map<String, Object> output;
    private String traceId;
    private String errorMessage;
}
