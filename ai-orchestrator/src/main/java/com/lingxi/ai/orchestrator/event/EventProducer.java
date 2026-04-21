package com.lingxi.ai.orchestrator.event;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.lingxi.ai.orchestrator.entity.Node;
import com.lingxi.ai.orchestrator.model.Task;
import com.lingxi.ai.orchestrator.model.NodeTaskEvent;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.stereotype.Service;

@Slf4j
@Service
public class EventProducer {

    @Autowired
    private KafkaTemplate<String, String> kafkaTemplate;

    @Autowired
    private ObjectMapper objectMapper;

    public void publishNodeReady(Node node) {
        NodeTaskEvent event = new NodeTaskEvent(
                node.getTaskId(),
                node.getId(),
                node.getType().name(),
                node.getInput(),
                node.getTaskId() + "-" + node.getId()
        );
        try {
            String message = objectMapper.writeValueAsString(event);
            log.info("Publishing node ready event: taskId={}, nodeId={}", node.getTaskId(), node.getId());
            kafkaTemplate.send("ai.node.ready", node.getTaskId() + "-" + node.getId(), message);
        } catch (Exception e) {
            log.error("Failed to publish node ready event", e);
        }
    }

    public void sendTaskCreatedEvent(Task task) {
        try {
            String message = objectMapper.writeValueAsString(task);
            kafkaTemplate.send("ai.task.created", task.getTaskId(), message);
        } catch (Exception e) {
            log.error("Failed to send task created event", e);
        }
    }

    public void sendNodeReadyEvent(NodeTaskEvent event) {
        try {
            String message = objectMapper.writeValueAsString(event);
            kafkaTemplate.send("ai.node.ready", event.getTaskId() + "-" + event.getNodeId(), message);
        } catch (Exception e) {
            log.error("Failed to send node ready event", e);
        }
    }

    public void sendTaskCompletedEvent(Task task) {
        try {
            String message = objectMapper.writeValueAsString(task);
            kafkaTemplate.send("ai.task.completed", task.getTaskId(), message);
        } catch (Exception e) {
            log.error("Failed to send task completed event", e);
        }
    }

    public void sendTaskFailedEvent(Task task) {
        try {
            String message = objectMapper.writeValueAsString(task);
            kafkaTemplate.send("ai.task.failed", task.getTaskId(), message);
        } catch (Exception e) {
            log.error("Failed to send task failed event", e);
        }
    }
}
