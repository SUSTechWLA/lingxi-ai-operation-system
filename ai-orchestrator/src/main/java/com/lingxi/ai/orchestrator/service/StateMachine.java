package com.lingxi.ai.orchestrator.service;

import com.lingxi.ai.orchestrator.context.ContextService;
import com.lingxi.ai.orchestrator.entity.Node;
import com.lingxi.ai.orchestrator.entity.NodeStatus;
import com.lingxi.ai.orchestrator.entity.Task;
import com.lingxi.ai.orchestrator.entity.TaskStatus;
import com.lingxi.ai.orchestrator.repository.NodeRepository;
import com.lingxi.ai.orchestrator.repository.TaskRepository;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;
import java.util.Map;

@Service
public class StateMachine {

    private static final Logger logger = LoggerFactory.getLogger(StateMachine.class);

    @Autowired
    private NodeRepository nodeRepository;

    @Autowired
    private TaskRepository taskRepository;

    @Autowired
    private ContextService contextService;

    @Transactional
    public void onSuccess(String nodeId, Map<String, Object> output) {
        Node node = nodeRepository.findById(nodeId)
                .orElseThrow(() -> new IllegalArgumentException("Node not found: " + nodeId));

        logger.info("Node {} succeeded", nodeId);

        node.setStatus(NodeStatus.SUCCESS);
        node.setOutput(output);
        nodeRepository.save(node);

        contextService.recordNodeSuccess(node);

        triggerChildNodes(node);
        checkTaskCompletion(node.getTaskId());
    }

    @Transactional
    public void onFailure(String nodeId, String errorMessage) {
        Node node = nodeRepository.findById(nodeId)
                .orElseThrow(() -> new IllegalArgumentException("Node not found: " + nodeId));

        logger.info("Node {} failed: {}", nodeId, errorMessage);

        node.setErrorMessage(errorMessage);

        if (node.getRetryCount() < node.getMaxRetry()) {
            retryNode(node);
        } else {
            failNode(node);
        }
    }

    private void retryNode(Node node) {
        logger.info("Retrying node {} (attempt {}/{})",
                node.getId(), node.getRetryCount() + 1, node.getMaxRetry());

        node.setStatus(NodeStatus.CREATED);
        node.setRetryCount(node.getRetryCount() + 1);
        nodeRepository.save(node);

        contextService.recordNodeRetry(node);
    }

    private void failNode(Node node) {
        logger.info("Node {} failed after {} retries", node.getId(), node.getMaxRetry());

        node.setStatus(NodeStatus.FAILED);
        nodeRepository.save(node);

        contextService.recordNodeFailed(node);
        failTask(node.getTaskId());
    }

    private void triggerChildNodes(Node parentNode) {
        List<Node> childNodes = nodeRepository.findChildNodes(parentNode.getId());
        logger.info("Triggering {} child nodes for parent {}", childNodes.size(), parentNode.getId());

        for (Node child : childNodes) {
            if (child.getStatus() == NodeStatus.CREATED) {
                boolean allParentsSuccess = areAllParentsSuccessful(child);
                if (allParentsSuccess) {
                    child.setStatus(NodeStatus.READY);
                    nodeRepository.save(child);
                    logger.info("Child node {} is now READY", child.getId());
                    contextService.recordNodeReady(child);
                }
            }
        }
    }

    private boolean areAllParentsSuccessful(Node node) {
        List<com.lingxi.ai.orchestrator.entity.NodeDependency> dependencies =
                nodeRepository.findByChildNodeId(node.getId());

        for (com.lingxi.ai.orchestrator.entity.NodeDependency dep : dependencies) {
            Node parent = nodeRepository.findById(dep.getParentNodeId()).orElse(null);
            if (parent == null || parent.getStatus() != NodeStatus.SUCCESS) {
                return false;
            }
        }
        return true;
    }

    private void checkTaskCompletion(String taskId) {
        List<Node> nodes = nodeRepository.findByTaskId(taskId);

        boolean allSuccess = nodes.stream().allMatch(n -> n.getStatus() == NodeStatus.SUCCESS);
        if (allSuccess) {
            Task task = taskRepository.findById(taskId).orElse(null);
            if (task != null) {
                task.setStatus(TaskStatus.SUCCESS);
                taskRepository.save(task);
                logger.info("Task {} completed successfully", taskId);
                contextService.recordTaskSuccess(task);
            }
        }
    }

    private void failTask(String taskId) {
        Task task = taskRepository.findById(taskId).orElse(null);
        if (task != null) {
            task.setStatus(TaskStatus.FAILED);
            taskRepository.save(task);
            logger.info("Task {} failed", taskId);
            contextService.recordTaskFailed(task);
        }
    }
}
