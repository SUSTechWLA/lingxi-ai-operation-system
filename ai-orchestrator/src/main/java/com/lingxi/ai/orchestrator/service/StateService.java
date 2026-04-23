package com.lingxi.ai.orchestrator.service;

import com.lingxi.ai.orchestrator.context.ContextService;
import com.lingxi.ai.orchestrator.entity.Node;
import com.lingxi.ai.orchestrator.entity.NodeDependency;
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

import java.util.List;
import java.util.Map;

@Slf4j
@Service
@RequiredArgsConstructor
public class StateService {

    private final NodeRepository nodeRepository;
    private final TaskRepository taskRepository;
    private final ContextService contextService;
    private final EventProducer eventProducer;
    private final RetryPolicy retryPolicy;

    @Transactional
    public Node transitionNode(String nodeId, NodeStatus newStatus, Map<String, Object> output, String errorMessage) {
        Node node = nodeRepository.findById(nodeId)
                .orElseThrow(() -> new IllegalArgumentException("Node not found: " + nodeId));

        NodeStatus oldStatus = node.getStatus();
        log.info("Node {} transitioning: {} -> {}", nodeId, oldStatus, newStatus);

        node.setStatus(newStatus);
        if (output != null) {
            node.setOutput(output);
        }
        if (errorMessage != null) {
            node.setErrorMessage(errorMessage);
        }
        node = nodeRepository.save(node);

        recordContextForTransition(node, oldStatus, newStatus);

        return node;
    }

    @Transactional
    public Task transitionTask(String taskId, TaskStatus newStatus) {
        Task task = taskRepository.findById(taskId)
                .orElseThrow(() -> new IllegalArgumentException("Task not found: " + taskId));

        TaskStatus oldStatus = task.getStatus();
        log.info("Task {} transitioning: {} -> {}", taskId, oldStatus, newStatus);

        task.setStatus(newStatus);
        task = taskRepository.save(task);

        switch (newStatus) {
            case SUCCESS -> {
                contextService.recordTaskSuccess(task);
                eventProducer.publishTaskCompleted(task);
            }
            case FAILED -> {
                contextService.recordTaskFailed(task);
                eventProducer.publishTaskFailed(task);
            }
            case PAUSED -> log.info("Task {} paused", taskId);
            case RUNNING -> {
                if (oldStatus == TaskStatus.PAUSED) {
                    log.info("Task {} resumed", taskId);
                }
            }
            default -> log.debug("Task {} transition to {} (no special action)", taskId, newStatus);
        }

        return task;
    }

    public boolean checkDependenciesMet(String nodeId) {
        List<NodeDependency> dependencies = nodeRepository.findByChildNodeId(nodeId);
        for (NodeDependency dep : dependencies) {
            Node parent = nodeRepository.findById(dep.getParentNodeId()).orElse(null);
            if (parent == null || parent.getStatus() != NodeStatus.SUCCESS) {
                return false;
            }
        }
        return true;
    }

    public boolean checkTaskCompleted(String taskId) {
        List<Node> nodes = nodeRepository.findByTaskId(taskId);
        return nodes.stream().allMatch(n -> n.getStatus() == NodeStatus.SUCCESS);
    }

    public boolean checkTaskFailed(String taskId) {
        List<Node> nodes = nodeRepository.findByTaskId(taskId);
        return nodes.stream().anyMatch(n ->
                n.getStatus() == NodeStatus.FAILED && n.getRetryCount() >= n.getMaxRetry());
    }

    public boolean hasRetryableNodes(String taskId) {
        List<Node> nodes = nodeRepository.findByTaskId(taskId);
        return nodes.stream()
                .anyMatch(n -> n.getStatus() == NodeStatus.FAILED
                        && retryPolicy.shouldRetry(n.getRetryCount(), n.getMaxRetry()));
    }

    @Transactional
    public Node initializeNodeReady(Node node) {
        if (node.getStatus() == NodeStatus.CREATED && checkDependenciesMet(node.getId())) {
            node.setStatus(NodeStatus.READY);
            if (node.getIdempotencyKey() == null) {
                node.setIdempotencyKey(node.getTaskId() + "-" + node.getId());
            }
            node = nodeRepository.save(node);
            contextService.recordNodeReady(node);
            log.info("Node {} is READY (dependencies met)", node.getId());
        }
        return node;
    }

    private void recordContextForTransition(Node node, NodeStatus oldStatus, NodeStatus newStatus) {
        switch (newStatus) {
            case READY -> contextService.recordNodeReady(node);
            case RUNNING -> contextService.recordNodeScheduled(node);
            case SUCCESS -> contextService.recordNodeSuccess(node);
            case FAILED -> contextService.recordNodeFailed(node);
            case RETRYING -> contextService.recordNodeRetry(node);
            default -> log.debug("No context recording for transition {} -> {}", oldStatus, newStatus);
        }
    }
}
