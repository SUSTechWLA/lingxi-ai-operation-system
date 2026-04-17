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

/**
 * 任务执行控制服务
 * 负责任务的暂停、恢复、失败处理等控制逻辑
 */
@Slf4j
@Service
@RequiredArgsConstructor
public class TaskExecutionControl {

    private final TaskRepository taskRepository;
    private final NodeRepository nodeRepository;

    /**
     * 检查任务是否可以继续调度
     * 如果任务有失败节点正在重试，则暂停后续节点调度
     */
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

        List<Node> failedOrRetryingNodes = nodeRepository.findByTaskIdAndStatusIn(
                taskId,
                List.of(NodeStatus.FAILED)
        );

        if (!failedOrRetryingNodes.isEmpty()) {
            log.debug("Task {} has {} failed/retrying nodes, pausing scheduling",
                    taskId, failedOrRetryingNodes.size());
            return false;
        }

        return true;
    }

    /**
     * 暂停任务调度
     */
    @Transactional
    public void pauseTask(String taskId, String reason) {
        Task task = taskRepository.findById(taskId)
                .orElseThrow(() -> new IllegalArgumentException("Task not found: " + taskId));

        if (task.getStatus() == TaskStatus.RUNNING) {
            task.setStatus(TaskStatus.PAUSED);
            taskRepository.save(task);
            log.info("Task {} paused, reason: {}", taskId, reason);
        }
    }

    /**
     * 恢复任务调度
     */
    @Transactional
    public void resumeTask(String taskId) {
        Task task = taskRepository.findById(taskId)
                .orElseThrow(() -> new IllegalArgumentException("Task not found: " + taskId));

        if (task.getStatus() == TaskStatus.PAUSED) {
            task.setStatus(TaskStatus.RUNNING);
            taskRepository.save(task);
            log.info("Task {} resumed", taskId);

            triggerReadyNodes(taskId);
        }
    }

    /**
     * 检查节点是否可以重试
     */
    public boolean canRetryNode(String nodeId) {
        Node node = nodeRepository.findById(nodeId).orElse(null);
        if (node == null) {
            return false;
        }
        return node.getStatus() == NodeStatus.FAILED
                && node.getRetryCount() < node.getMaxRetry();
    }

    /**
     * 重试失败的节点
     */
    @Transactional
    public void retryNode(String nodeId) {
        Node node = nodeRepository.findById(nodeId)
                .orElseThrow(() -> new IllegalArgumentException("Node not found: " + nodeId));

        if (node.getStatus() != NodeStatus.FAILED) {
            throw new IllegalStateException("Node is not in FAILED status: " + nodeId);
        }

        if (node.getRetryCount() >= node.getMaxRetry()) {
            throw new IllegalStateException("Node has reached max retry count: " + nodeId);
        }

        node.setStatus(NodeStatus.CREATED);
        node.setRetryCount(node.getRetryCount() + 1);
        node.setErrorMessage(null);
        nodeRepository.save(node);

        log.info("Node {} marked for retry (attempt {}/{})",
                nodeId, node.getRetryCount(), node.getMaxRetry());

        if (node.getTaskId() != null) {
            Optional<Task> taskOpt = taskRepository.findById(node.getTaskId());
            if (taskOpt.isPresent() && taskOpt.get().getStatus() == TaskStatus.PAUSED) {
                resumeTask(node.getTaskId());
            }
        }
    }

    /**
     * 触发任务中所有READY状态的节点
     */
    private void triggerReadyNodes(String taskId) {
        List<Node> readyNodes = nodeRepository.findByTaskIdAndStatus(taskId, NodeStatus.READY);
        for (Node node : readyNodes) {
            log.info("Triggering ready node: {} for task: {}", node.getId(), taskId);
        }
    }

    /**
     * 获取任务的暂停原因
     */
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
