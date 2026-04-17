package com.lingxi.ai.context.event;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.Map;

/**
 * 统一上下文事件格式
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ContextEvent {
    private String eventId;
    private String eventType;
    private String taskId;
    private String nodeId;
    private Long timestamp;
    private String source;
    private Map<String, Object> data;
    private String version;
}
