package com.lingxi.ai.worker.event;

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

    @KafkaListener(topics = "ai.node.ready", groupId = "ai-worker-group")
    public void consumeNodeReady(NodeTaskEvent event) {
        log.info("Received node ready event: taskId={}, nodeId={}", event.getTaskId(), event.getNodeId());
        try {
            nodeExecutor.executeNode(event);
        } catch (Exception e) {
            log.error("Error processing node ready event: taskId={}, nodeId={}",
                    event.getTaskId(), event.getNodeId(), e);
        }
    }
}
