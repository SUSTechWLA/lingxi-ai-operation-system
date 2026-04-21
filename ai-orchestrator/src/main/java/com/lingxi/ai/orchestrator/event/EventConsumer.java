package com.lingxi.ai.orchestrator.event;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.lingxi.ai.orchestrator.model.NodeResultEvent;
import com.lingxi.ai.orchestrator.model.NodeTaskEvent;
import com.lingxi.ai.orchestrator.model.NodeStatus;
import com.lingxi.ai.orchestrator.service.StateMachine;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Service;

@Slf4j
@Service
@RequiredArgsConstructor
public class EventConsumer {

    private final StateMachine stateMachine;
    private final WorkerService workerService;
    private final ObjectMapper objectMapper;

    @KafkaListener(topics = "ai.node.ready", groupId = "worker-group")
    public void handleNodeReady(String message) {
        log.info("Received node ready message: {}", message);
        try {
            NodeTaskEvent event = objectMapper.readValue(message, NodeTaskEvent.class);
            log.info("Parsed node ready event: {}", event.getNodeId());
            workerService.executeTask(event);
        } catch (Exception e) {
            log.error("Error processing node ready message: {}", message, e);
        }
    }

    @KafkaListener(topics = "ai.node.result", groupId = "orchestrator-group")
    public void handleNodeResult(String message) {
        log.info("Received node result message: {}", message);
        try {
            NodeResultEvent event = objectMapper.readValue(message, NodeResultEvent.class);
            log.info("Parsed node result event: {} - {}", event.getNodeId(), event.getStatus());

            if (event.getStatus() == NodeStatus.SUCCESS) {
                stateMachine.onSuccess(event.getNodeId(), event.getOutput());
            } else if (event.getStatus() == NodeStatus.FAILED) {
                stateMachine.onFailure(event.getNodeId(), event.getErrorMessage());
            }
        } catch (Exception e) {
            log.error("Error processing node result message: {}", message, e);
        }
    }
}
