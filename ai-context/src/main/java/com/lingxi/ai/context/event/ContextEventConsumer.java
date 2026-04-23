package com.lingxi.ai.context.event;

import com.lingxi.ai.context.service.ContextService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Service;

import java.util.Map;

@Slf4j
@Service
@RequiredArgsConstructor
public class ContextEventConsumer {

    private final ContextService contextService;

    @KafkaListener(topics = {
            "ai.task.created",
            "ai.task.validated",
            "ai.task.running",
            "ai.task.success",
            "ai.task.failed",
            "ai.node.created",
            "ai.node.scheduled",
            "ai.node.running",
            "ai.node.success",
            "ai.node.failed",
            "ai.node.executed",
            "ai.context.events"
    }, groupId = "ai-context-group")
    public void consumeContextEvent(Map<String, Object> event) {
        try {
            String eventType = (String) event.get("event_type");
            if (eventType == null) {
                eventType = inferEventType(event);
            }

            String taskId = (String) event.get("taskId");
            if (taskId == null) {
                taskId = (String) event.get("task_id");
            }

            if (taskId == null) {
                log.warn("Event missing taskId: {}", event);
                return;
            }

            log.info("Consuming context event: type={}, taskId={}", eventType, taskId);
            processEvent(eventType, taskId, event);

        } catch (Exception e) {
            log.error("Error processing context event", e);
        }
    }

    private String inferEventType(Map<String, Object> event) {
        if (event.containsKey("status")) {
            Object status = event.get("status");
            if ("SUCCESS".equals(status) || "success".equals(status)) {
                return "ai.node.success";
            } else if ("FAILED".equals(status) || "failed".equals(status)) {
                return "ai.node.failed";
            } else if ("RUNNING".equals(status) || "running".equals(status)) {
                return "ai.node.running";
            }
        }
        return "unknown";
    }

    private void processEvent(String eventType, String taskId, Map<String, Object> event) {
        String nodeId = (String) event.get("nodeId");
        if (nodeId == null) {
            nodeId = (String) event.get("node_id");
        }

        switch (eventType) {
            case "ai.task.created" -> contextService.recordTaskCreated(taskId);
            case "ai.task.success" -> contextService.recordTaskSuccess(taskId);
            case "ai.task.failed" -> contextService.recordTaskFailed(taskId);
            case "ai.node.success" -> {
                if (nodeId != null) {
                    contextService.recordNodeSuccess(taskId, nodeId);
                }
            }
            case "ai.node.executed" -> {
                if (nodeId != null) {
                    contextService.recordNodeSuccess(taskId, nodeId);
                }
            }
            case "ai.node.failed" -> {
                if (nodeId != null) {
                    String errorMessage = (String) event.get("errorMessage");
                    if (errorMessage == null) {
                        errorMessage = (String) event.get("error_message");
                    }
                    contextService.recordNodeFailed(taskId, nodeId, errorMessage);
                }
            }
            default -> log.debug("Ignoring event type: {}", eventType);
        }
    }
}
