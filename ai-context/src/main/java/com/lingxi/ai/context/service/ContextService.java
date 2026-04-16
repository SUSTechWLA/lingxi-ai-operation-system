package com.lingxi.ai.context.service;

import com.lingxi.ai.context.entity.Context;
import com.lingxi.ai.context.entity.ContextType;
import com.lingxi.ai.context.repository.ContextRepository;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Optional;

@Service
public class ContextService {

    private static final Logger logger = LoggerFactory.getLogger(ContextService.class);

    @Autowired
    private ContextRepository contextRepository;

    public Context recordTaskCreated(String taskId) {
        Context ctx = Context.create(ContextType.TASK_CREATED, taskId, "Task created");
        contextRepository.save(ctx);
        logger.info("Context recorded: TASK_CREATED for task {}", taskId);
        return ctx;
    }

    public Context recordTaskSuccess(String taskId) {
        Context ctx = Context.create(ContextType.TASK_SUCCESS, taskId, "Task completed successfully");
        contextRepository.save(ctx);
        logger.info("Context recorded: TASK_SUCCESS for task {}", taskId);
        return ctx;
    }

    public Context recordTaskFailed(String taskId) {
        Context ctx = Context.create(ContextType.TASK_FAILED, taskId, "Task failed");
        contextRepository.save(ctx);
        logger.info("Context recorded: TASK_FAILED for task {}", taskId);
        return ctx;
    }

    public Context recordDagSubmitted(String taskId) {
        Context ctx = Context.create(ContextType.DAG_SUBMITTED, taskId, "DAG submitted");
        contextRepository.save(ctx);
        logger.info("Context recorded: DAG_SUBMITTED for task {}", taskId);
        return ctx;
    }

    public Context recordDagValidated(String taskId) {
        Context ctx = Context.create(ContextType.DAG_VALIDATED, taskId, "DAG validated");
        contextRepository.save(ctx);
        logger.info("Context recorded: DAG_VALIDATED for task {}", taskId);
        return ctx;
    }

    public Context recordNodeScheduled(String taskId, String nodeId, String nodeType, String nodeName) {
        Context ctx = Context.create(ContextType.NODE_SCHEDULED, taskId, nodeId, "Node scheduled");
        Map<String, Object> metadata = new HashMap<>();
        metadata.put("type", nodeType);
        metadata.put("name", nodeName);
        ctx.setMetadata(metadata);
        contextRepository.save(ctx);
        logger.info("Context recorded: NODE_SCHEDULED for node {}", nodeId);
        return ctx;
    }

    public Context recordNodeReady(String taskId, String nodeId) {
        Context ctx = Context.create(ContextType.NODE_READY, taskId, nodeId, "Node ready");
        contextRepository.save(ctx);
        logger.info("Context recorded: NODE_READY for node {}", nodeId);
        return ctx;
    }

    public Context recordNodeSuccess(String taskId, String nodeId) {
        Context ctx = Context.create(ContextType.NODE_SUCCESS, taskId, nodeId, "Node succeeded");
        contextRepository.save(ctx);
        logger.info("Context recorded: NODE_SUCCESS for node {}", nodeId);
        return ctx;
    }

    public Context recordNodeFailed(String taskId, String nodeId, String errorMessage) {
        Context ctx = Context.create(ContextType.NODE_FAILED, taskId, nodeId, "Node failed");
        if (errorMessage != null) {
            ctx.setMessage("Node failed: " + errorMessage);
        }
        contextRepository.save(ctx);
        logger.info("Context recorded: NODE_FAILED for node {}", nodeId);
        return ctx;
    }

    public Context recordNodeRetry(String taskId, String nodeId, int retryCount, int maxRetry) {
        Context ctx = Context.create(ContextType.NODE_RETRY, taskId, nodeId,
                "Node retry " + retryCount + "/" + maxRetry);
        contextRepository.save(ctx);
        logger.info("Context recorded: NODE_RETRY for node {}", nodeId);
        return ctx;
    }

    public List<Context> getContextForTask(String taskId) {
        return contextRepository.findByTaskIdOrderByCreatedAtDesc(taskId);
    }

    public Context recordNodeSnapshot(String taskId, String nodeId, String nodeType, String nodeName,
                                      String status, Map<String, Object> input, Map<String, Object> output,
                                      int retryCount, int maxRetry, int priority, String workerGroup,
                                      Integer version, String errorMessage) {
        Map<String, Object> snapshot = new HashMap<>();
        snapshot.put("nodeId", nodeId);
        snapshot.put("taskId", taskId);
        snapshot.put("type", nodeType);
        snapshot.put("name", nodeName);
        snapshot.put("status", status);
        snapshot.put("input", input);
        snapshot.put("output", output);
        snapshot.put("retryCount", retryCount);
        snapshot.put("maxRetry", maxRetry);
        snapshot.put("priority", priority);
        snapshot.put("workerGroup", workerGroup);
        snapshot.put("version", version);
        snapshot.put("errorMessage", errorMessage);
        snapshot.put("snapshotTime", System.currentTimeMillis());

        Context ctx = Context.createSnapshot(taskId, nodeId, snapshot, "Node snapshot");
        contextRepository.save(ctx);
        logger.info("Snapshot recorded for node {} at {}", nodeId, snapshot.get("snapshotTime"));
        return ctx;
    }

    public Optional<Map<String, Object>> getLatestSnapshotForNode(String nodeId) {
        List<Context> snapshots = contextRepository.findByNodeIdAndContextTypeOrderByCreatedAtDesc(
                nodeId, ContextType.NODE_SNAPSHOT);
        if (snapshots.isEmpty()) {
            return Optional.empty();
        }
        return Optional.ofNullable(snapshots.get(0).getSnapshotData());
    }

    public Optional<Map<String, Object>> restoreNodeFromSnapshot(String nodeId) {
        Optional<Map<String, Object>> snapshotOpt = getLatestSnapshotForNode(nodeId);
        if (snapshotOpt.isEmpty()) {
            logger.warn("No snapshot found for node {}", nodeId);
            return Optional.empty();
        }

        Map<String, Object> snapshot = snapshotOpt.get();
        logger.info("Restoring node {} from snapshot at {}", nodeId, snapshot.get("snapshotTime"));

        return snapshotOpt;
    }
}
