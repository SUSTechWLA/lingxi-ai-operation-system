package com.lingxi.ai.orchestrator.context;

import com.lingxi.ai.orchestrator.client.ContextClient;
import com.lingxi.ai.orchestrator.entity.Node;
import com.lingxi.ai.orchestrator.entity.Task;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Optional;

@Service
public class ContextService {

    private static final org.slf4j.Logger logger = org.slf4j.LoggerFactory.getLogger(ContextService.class);

    @Autowired
    private ContextClient contextClient;

    public void recordTaskCreated(Task task) {
        contextClient.recordTaskCreated(task.getId());
        logger.info("Context recorded: TASK_CREATED for task {}", task.getId());
    }

    public void recordTaskSuccess(Task task) {
        contextClient.recordTaskSuccess(task.getId());
        logger.info("Context recorded: TASK_SUCCESS for task {}", task.getId());
    }

    public void recordTaskFailed(Task task) {
        contextClient.recordTaskFailed(task.getId());
        logger.info("Context recorded: TASK_FAILED for task {}", task.getId());
    }

    public void recordDagSubmitted(String taskId) {
        contextClient.recordDagSubmitted(taskId);
        logger.info("Context recorded: DAG_SUBMITTED for task {}", taskId);
    }

    public void recordDagValidated(String taskId) {
        contextClient.recordDagValidated(taskId);
        logger.info("Context recorded: DAG_VALIDATED for task {}", taskId);
    }

    public void recordNodeScheduled(Node node) {
        contextClient.recordNodeScheduled(node.getTaskId(), node.getId(),
                node.getType().name(), node.getName());
        logger.info("Context recorded: NODE_SCHEDULED for node {}", node.getId());
    }

    public void recordNodeReady(Node node) {
        contextClient.recordNodeReady(node.getTaskId(), node.getId());
        logger.info("Context recorded: NODE_READY for node {}", node.getId());
    }

    public void recordNodeSuccess(Node node) {
        contextClient.recordNodeSuccess(node.getTaskId(), node.getId());
        logger.info("Context recorded: NODE_SUCCESS for node {}", node.getId());
    }

    public void recordNodeFailed(Node node) {
        contextClient.recordNodeFailed(node.getTaskId(), node.getId(), node.getErrorMessage());
        logger.info("Context recorded: NODE_FAILED for node {}", node.getId());
    }

    public void recordNodeRetry(Node node) {
        contextClient.recordNodeRetry(node.getTaskId(), node.getId(),
                node.getRetryCount(), node.getMaxRetry());
        logger.info("Context recorded: NODE_RETRY for node {}", node.getId());
    }

    public List<Map<String, Object>> getContextForTask(String taskId) {
        return contextClient.getContextForTask(taskId);
    }

    public void recordNodeSnapshot(Node node, String message) {
        contextClient.recordNodeSnapshot(
                node.getTaskId(), node.getId(),
                node.getType().name(), node.getName(),
                node.getStatus().name(),
                node.getInput() != null ? node.getInput() : new HashMap<>(),
                node.getOutput() != null ? node.getOutput() : new HashMap<>(),
                node.getRetryCount(),
                node.getMaxRetry(),
                node.getPriority(),
                node.getWorkerGroup(),
                node.getVersion(),
                node.getErrorMessage()
        );
        logger.info("Snapshot recorded for node {}", node.getId());
    }

    public Optional<Map<String, Object>> getLatestSnapshotForNode(String nodeId) {
        Map<String, Object> snapshot = contextClient.getLatestSnapshotForNode(nodeId);
        return Optional.ofNullable(snapshot);
    }

    public Optional<Node> restoreNodeFromSnapshot(String nodeId) {
        logger.warn("restoreNodeFromSnapshot is not supported when using remote context service");
        return Optional.empty();
    }
}
