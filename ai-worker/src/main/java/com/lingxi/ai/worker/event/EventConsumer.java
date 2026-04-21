package com.lingxi.ai.worker.event;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.lingxi.ai.worker.model.NodeTaskEvent;
import com.lingxi.ai.worker.service.NodeExecutor;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Service;

@Slf4j
@Service
@RequiredArgsConstructor
public class EventConsumer {

    private final NodeExecutor nodeExecutor;
    private final ObjectMapper objectMapper;

    @KafkaListener(topics = "ai.node.ready", groupId = "ai-worker-group")
    public void consumeNodeReady(String message) {
        log.info("Received node ready message: {}", message);
        try {
            NodeTaskEvent event = objectMapper.readValue(message, NodeTaskEvent.class);
            log.info("Parsed node ready event: taskId={}, nodeId={}", event.getTaskId(), event.getNodeId());
            nodeExecutor.executeNode(event);
        } catch (Exception e) {
            log.error("Error processing node ready message: {}", message, e);
        }
    }
}
