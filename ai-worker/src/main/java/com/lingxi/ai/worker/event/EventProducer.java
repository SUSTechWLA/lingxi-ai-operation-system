package com.lingxi.ai.worker.event;

import com.lingxi.ai.worker.model.NodeResultEvent;
import lombok.extern.slf4j.Slf4j;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.stereotype.Service;

@Slf4j
@Service
public class EventProducer {

    private final KafkaTemplate<String, Object> kafkaTemplate;

    public EventProducer(KafkaTemplate<String, Object> kafkaTemplate) {
        this.kafkaTemplate = kafkaTemplate;
    }

    public void publishNodeResult(NodeResultEvent event) {
        String key = event.getTaskId() + "-" + event.getNodeId();
        log.info("Publishing node result event: taskId={}, nodeId={}, status={}",
                event.getTaskId(), event.getNodeId(), event.getStatus());
        kafkaTemplate.send("ai.node.result", key, event);
    }

    public void publishNodeRunning(String taskId, String nodeId, String traceId) {
        log.info("Node started running: taskId={}, nodeId={}", taskId, nodeId);
    }
}
