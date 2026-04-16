package com.lingxi.ai.orchestrator.event;

import com.lingxi.ai.orchestrator.model.NodeResultEvent;
import com.lingxi.ai.orchestrator.model.NodeTaskEvent;
import com.lingxi.ai.orchestrator.model.NodeStatus;
import com.lingxi.ai.orchestrator.service.StateMachine;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Service;

@Service
public class EventConsumer {

    private static final Logger logger = LoggerFactory.getLogger(EventConsumer.class);

    @Autowired
    private StateMachine stateMachine;

    @Autowired
    private WorkerService workerService;

    @KafkaListener(topics = "ai.node.ready", groupId = "worker-group")
    public void handleNodeReady(NodeTaskEvent event) {
        logger.info("Received node ready event: {}", event.getNodeId());
        workerService.executeTask(event);
    }

    @KafkaListener(topics = "ai.node.result", groupId = "orchestrator-group")
    public void handleNodeResult(NodeResultEvent event) {
        logger.info("Received node result event: {} - {}", event.getNodeId(), event.getStatus());

        if (event.getStatus() == NodeStatus.SUCCESS) {
            stateMachine.onSuccess(event.getNodeId(), event.getOutput());
        } else if (event.getStatus() == NodeStatus.FAILED) {
            stateMachine.onFailure(event.getNodeId(), event.getErrorMessage());
        }
    }
}
