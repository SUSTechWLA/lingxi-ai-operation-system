package com.lingxi.ai.orchestrator.event;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.lingxi.ai.orchestrator.service.DependencyChecker;
import com.lingxi.ai.orchestrator.service.StateMachine;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Service;

import java.util.Map;

@Slf4j
@Service
@RequiredArgsConstructor
public class EventConsumer {

    private final StateMachine stateMachine;
    private final DependencyChecker dependencyChecker;
    private final ObjectMapper objectMapper;

    @KafkaListener(topics = "ai.node.result", groupId = "orchestrator-group")
    public void handleNodeResult(String message) {
        log.info("Received node result message: {}", message);
        try {
            @SuppressWarnings("unchecked")
            Map<String, Object> event = objectMapper.readValue(message, Map.class);
            String nodeId = (String) event.get("nodeId");
            String taskId = (String) event.get("taskId");
            String status = (String) event.get("status");

            log.info("Parsed node result event: nodeId={}, status={}", nodeId, status);

            if ("SUCCESS".equals(status)) {
                @SuppressWarnings("unchecked")
                Map<String, Object> output = (Map<String, Object>) event.get("output");
                stateMachine.onSuccess(nodeId, output);
            } else if ("FAILED".equals(status)) {
                String errorMessage = (String) event.get("errorMessage");
                stateMachine.onFailure(nodeId, errorMessage);
            }
        } catch (Exception e) {
            log.error("Error processing node result message: {}", message, e);
        }
    }

    @KafkaListener(topics = "ai.node.executed", groupId = "orchestrator-group")
    public void handleNodeExecuted(String message) {
        log.info("Received node executed message: {}", message);
        try {
            @SuppressWarnings("unchecked")
            Map<String, Object> event = objectMapper.readValue(message, Map.class);
            String nodeId = (String) event.get("nodeId");
            String taskId = (String) event.get("taskId");

            dependencyChecker.onNodeExecuted(nodeId, taskId);
        } catch (Exception e) {
            log.error("Error processing node executed message: {}", message, e);
        }
    }
}
