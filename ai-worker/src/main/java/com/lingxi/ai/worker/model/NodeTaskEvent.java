package com.lingxi.ai.worker.model;

import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;
import java.util.Map;

@Data
@NoArgsConstructor
@AllArgsConstructor
public class NodeTaskEvent {
    private String taskId;
    private String nodeId;
    private String type;
    private Map<String, Object> payload;
    private String traceId;
    private String idempotencyKey;
}
