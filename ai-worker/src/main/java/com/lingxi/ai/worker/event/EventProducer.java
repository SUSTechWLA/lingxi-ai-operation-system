package com.lingxi.ai.worker.event;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.lingxi.ai.worker.model.NodeResultEvent;
import lombok.extern.slf4j.Slf4j;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.stereotype.Service;

@Slf4j
@Service
public class EventProducer {

    private final KafkaTemplate<String, String> kafkaTemplate;
    private final ObjectMapper objectMapper;

    public EventProducer(KafkaTemplate<String, String> kafkaTemplate, ObjectMapper objectMapper) {
        this.kafkaTemplate = kafkaTemplate;
        this.objectMapper = objectMapper;
    }

    public void publishNodeResult(NodeResultEvent event) {
        String key = event.getTaskId() + "-" + event.getNodeId();
        try {
            String message = objectMapper.writeValueAsString(event);
            log.info("Publishing node result event: taskId={}, nodeId={}, status={}",
                    event.getTaskId(), event.getNodeId(), event.getStatus());
            kafkaTemplate.send("ai.node.result", key, message);
        } catch (Exception e) {
            log.error("Failed to publish node result event", e);
        }
    }

    public void publishNodeRunning(String taskId, String nodeId, String traceId) {
        log.info("Node started running: taskId={}, nodeId={}", taskId, nodeId);
    }
}
