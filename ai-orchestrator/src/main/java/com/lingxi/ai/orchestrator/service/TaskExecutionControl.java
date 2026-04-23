package com.lingxi.ai.orchestrator.service;

import com.lingxi.ai.orchestrator.entity.Node;
import com.lingxi.ai.orchestrator.entity.NodeStatus;
import com.lingxi.ai.orchestrator.entity.Task;
import com.lingxi.ai.orchestrator.entity.TaskStatus;
import com.lingxi.ai.orchestrator.repository.NodeRepository;
import com.lingxi.ai.orchestrator.repository.TaskRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;
import java.util.Optional;

@Slf4j
@Service
@RequiredArgsConstructor
public class TaskExecutionControl {

    private final TaskRepository taskRepository;
    private final NodeRepository nodeRepository;
    private final StateService stateService;
    private final RetryPolicy retryPolicy;

    public boolean canScheduleNodes(String taskId) {
        Optional<Task> taskOpt = taskRepository.findById(taskId);
        if (taskOpt.isEmpty()) {
            return false;
        }

        Task task = taskOpt.get();

        if (task.getStatus() == TaskStatus.PAUSED) {
            log.debug("Task {} is PAUSED, skipping scheduling", taskId);
            return false;
        }

        if (task.getStatus() == TaskStatus.SUCCESS || task.getStatus() == TaskStatus.FAILED) {
            return false;
        }

        List<Node> failedNodes = nodeRepository.findByTaskIdAndStatusIn(
                taskId,
                List.of(NodeStatus.FAILED)
        );

        if (!failedNodes.isEmpty()) {
            log.debug("Task {} has {} failed nodes, pausing scheduling",
                    taskId, failedNodes.size());
            return false;
        }

        return true;
    }

    @Transactional
    public void pauseTask(String taskId, String reason) {
        stateService.transitionTask(taskId, TaskStatus.PAUSED);
        log.info("Task {} paused, reason: {}", taskId, reason);
    }

    @Transactional
    public void resumeTask(String taskId) {
        Task task = taskRepository.findById(taskId)
                .orElseThrow(() -> new IllegalArgumentException("Task not found: " + taskId));

        if (task.getStatus() == TaskStatus.PAUSED) {
            stateService.transitionTask(taskId, TaskStatus.RUNNING);
            log.info("Task {} resumed", taskId);
        }
    }

    public boolean canRetryNode(String nodeId) {
        Node node = nodeRepository.findById(nodeId).orElse(null);
        if (node == null) {
            return false;
        }
        return node.getStatus() == NodeStatus.FAILED
                && retryPolicy.shouldRetry(node.getRetryCount(), node.getMaxRetry());
    }

    @Transactional
    public void retryNode(String nodeId) {
        Node node = nodeRepository.findById(nodeId)
                .orElseThrow(() -> new IllegalArgumentException("Node not found: " + nodeId));

        if (node.getStatus() != NodeStatus.FAILED) {
            throw new IllegalStateException("Node is not in FAILED status: " + nodeId);
        }

        if (!retryPolicy.shouldRetry(node.getRetryCount(), node.getMaxRetry())) {
            throw new IllegalStateException("Node has reached max retry count: " + nodeId);
        }

        node.setStatus(NodeStatus.RETRYING);
        node.setRetryCount(node.getRetryCount() + 1);
        node.setErrorMessage(null);
        nodeRepository.save(node);

        stateService.transitionNode(nodeId, NodeStatus.CREATED, null, null);

        log.info("Node {} marked for retry (attempt {}/{})",
                nodeId, node.getRetryCount(), node.getMaxRetry());

        if (node.getTaskId() != null) {
            Task task = taskRepository.findById(node.getTaskId()).orElse(null);
            if (task != null && task.getStatus() == TaskStatus.PAUSED) {
                resumeTask(node.getTaskId());
            }
        }
    }

    public String getTaskPauseReason(String taskId) {
        List<Node> failedNodes = nodeRepository.findByTaskIdAndStatus(taskId, NodeStatus.FAILED);
        if (!failedNodes.isEmpty()) {
            Node failedNode = failedNodes.get(0);
            return String.format("Node %s failed: %s (retry %d/%d)",
                    failedNode.getId(),
                    failedNode.getErrorMessage(),
                    failedNode.getRetryCount(),
                    failedNode.getMaxRetry());
        }
        return "Unknown reason";
    }
}
