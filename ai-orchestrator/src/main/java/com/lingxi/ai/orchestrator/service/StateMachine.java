package com.lingxi.ai.orchestrator.service;

import com.lingxi.ai.orchestrator.entity.Node;
import com.lingxi.ai.orchestrator.entity.NodeStatus;
import com.lingxi.ai.orchestrator.entity.Task;
import com.lingxi.ai.orchestrator.entity.TaskStatus;
import com.lingxi.ai.orchestrator.event.EventProducer;
import com.lingxi.ai.orchestrator.repository.NodeRepository;
import com.lingxi.ai.orchestrator.repository.TaskRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.Map;

@Slf4j
@Service
@RequiredArgsConstructor
public class StateMachine {

    private final StateService stateService;
    private final NodeRepository nodeRepository;
    private final TaskRepository taskRepository;
    private final EventProducer eventProducer;
    private final RetryPolicy retryPolicy;

    @Transactional
    public void onSuccess(String nodeId, Map<String, Object> output) {
        Node node = stateService.transitionNode(nodeId, NodeStatus.SUCCESS, output, null);
        log.info("Node {} succeeded", nodeId);

        // Publish NodeExecuted event for dependency-driven scheduling
        eventProducer.publishNodeExecuted(node);

        // Check if task was paused and can resume
        Task task = taskRepository.findById(node.getTaskId()).orElse(null);
        if (task != null && task.getStatus() == TaskStatus.PAUSED) {
            if (!stateService.hasRetryableNodes(node.getTaskId())) {
                stateService.transitionTask(node.getTaskId(), TaskStatus.RUNNING);
            }
        }

        // Check task completion
        if (stateService.checkTaskCompleted(node.getTaskId())) {
            stateService.transitionTask(node.getTaskId(), TaskStatus.SUCCESS);
        }
    }

    @Transactional
    public void onFailure(String nodeId, String errorMessage) {
        Node node = nodeRepository.findById(nodeId)
                .orElseThrow(() -> new IllegalArgumentException("Node not found: " + nodeId));

        log.info("Node {} failed: {}", nodeId, errorMessage);

        if (retryPolicy.shouldRetry(node.getRetryCount(), node.getMaxRetry())) {
            retryNode(node, errorMessage);
        } else {
            failNode(node, errorMessage);
        }
    }

    private void retryNode(Node node, String errorMessage) {
        log.info("Retrying node {} (attempt {}/{})",
                node.getId(), node.getRetryCount() + 1, node.getMaxRetry());

        node.setErrorMessage(errorMessage);
        node.setStatus(NodeStatus.RETRYING);
        node.setRetryCount(node.getRetryCount() + 1);
        nodeRepository.save(node);

        stateService.transitionNode(node.getId(), NodeStatus.CREATED, null, null);
    }

    private void failNode(Node node, String errorMessage) {
        log.info("Node {} failed after {} retries", node.getId(), node.getMaxRetry());

        stateService.transitionNode(node.getId(), NodeStatus.FAILED, null, errorMessage);

        // Publish NodeFailed event
        eventProducer.publishNodeFailed(node);

        stateService.transitionTask(node.getTaskId(), TaskStatus.PAUSED);

        if (!stateService.hasRetryableNodes(node.getTaskId())) {
            stateService.transitionTask(node.getTaskId(), TaskStatus.FAILED);
        }
    }
}
