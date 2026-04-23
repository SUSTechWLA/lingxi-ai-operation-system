package com.lingxi.ai.orchestrator.event;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.lingxi.ai.orchestrator.entity.Node;
import com.lingxi.ai.orchestrator.entity.Task;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.stereotype.Service;

import java.util.HashMap;
import java.util.Map;

@Slf4j
@Service
@RequiredArgsConstructor
public class EventProducer {

    private final KafkaTemplate<String, String> kafkaTemplate;
    private final ObjectMapper objectMapper;

    public void publishNodeReady(Node node) {
        try {
            Map<String, Object> event = new HashMap<>();
            event.put("taskId", node.getTaskId());
            event.put("nodeId", node.getId());
            event.put("type", node.getType().name());
            event.put("payload", node.getInput());
            event.put("traceId", node.getTaskId() + "-" + node.getId());
            event.put("idempotencyKey", node.getIdempotencyKey());

            String message = objectMapper.writeValueAsString(event);
            String key = node.getIdempotencyKey() != null ? node.getIdempotencyKey() : node.getTaskId() + "-" + node.getId();
            log.info("Publishing node ready event: taskId={}, nodeId={}", node.getTaskId(), node.getId());
            kafkaTemplate.send("ai.node.ready", key, message);
        } catch (Exception e) {
            log.error("Failed to publish node ready event", e);
        }
    }

    public void publishNodeExecuted(Node node) {
        try {
            Map<String, Object> event = new HashMap<>();
            event.put("taskId", node.getTaskId());
            event.put("nodeId", node.getId());
            event.put("status", "SUCCESS");
            event.put("output", node.getOutput());

            String message = objectMapper.writeValueAsString(event);
            log.info("Publishing node executed event: taskId={}, nodeId={}", node.getTaskId(), node.getId());
            kafkaTemplate.send("ai.node.executed", node.getTaskId() + "-" + node.getId(), message);
        } catch (Exception e) {
            log.error("Failed to publish node executed event", e);
        }
    }

    public void publishNodeFailed(Node node) {
        try {
            Map<String, Object> event = new HashMap<>();
            event.put("taskId", node.getTaskId());
            event.put("nodeId", node.getId());
            event.put("status", "FAILED");
            event.put("errorMessage", node.getErrorMessage());

            String message = objectMapper.writeValueAsString(event);
            log.info("Publishing node failed event: taskId={}, nodeId={}", node.getTaskId(), node.getId());
            kafkaTemplate.send("ai.node.failed", node.getTaskId() + "-" + node.getId(), message);
        } catch (Exception e) {
            log.error("Failed to publish node failed event", e);
        }
    }

    public void publishTaskCompleted(Task task) {
        try {
            Map<String, Object> event = new HashMap<>();
            event.put("taskId", task.getId());
            event.put("status", "SUCCESS");

            String message = objectMapper.writeValueAsString(event);
            log.info("Publishing task completed event: taskId={}", task.getId());
            kafkaTemplate.send("ai.task.completed", task.getId(), message);
        } catch (Exception e) {
            log.error("Failed to publish task completed event", e);
        }
    }

    public void publishTaskFailed(Task task) {
        try {
            Map<String, Object> event = new HashMap<>();
            event.put("taskId", task.getId());
            event.put("status", "FAILED");

            String message = objectMapper.writeValueAsString(event);
            log.info("Publishing task failed event: taskId={}", task.getId());
            kafkaTemplate.send("ai.task.failed", task.getId(), message);
        } catch (Exception e) {
            log.error("Failed to publish task failed event", e);
        }
    }
}
