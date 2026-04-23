package com.lingxi.ai.worker.service;

import com.lingxi.ai.worker.client.ContextClient;
import com.lingxi.ai.worker.config.WorkerConfig;
import com.lingxi.ai.worker.event.EventProducer;
import com.lingxi.ai.worker.externaltool.service.ExternalToolExecutor;
import com.lingxi.ai.worker.model.NodeResultEvent;
import com.lingxi.ai.worker.model.NodeStatus;
import com.lingxi.ai.worker.model.NodeTaskEvent;
import com.lingxi.ai.worker.tool.Tool;
import com.lingxi.ai.worker.tool.ToolContext;
import com.lingxi.ai.worker.tool.ToolRegistry;
import com.lingxi.ai.worker.tool.ToolResult;
import com.lingxi.ai.worker.util.TraceContext;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.stereotype.Service;
import reactor.core.scheduler.Schedulers;

import java.time.Instant;
import java.util.Map;
import java.util.Optional;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;

@Slf4j
@Service
@RequiredArgsConstructor
public class NodeExecutor {

    private final ToolRegistry toolRegistry;
    private final EventProducer eventProducer;
    private final ContextClient contextClient;
    private final WorkerConfig workerConfig;
    private final ExternalToolExecutor externalToolExecutor;

    @Qualifier("toolExecutorService")
    private final ExecutorService toolExecutorService;

    public void executeNode(NodeTaskEvent event) {
        String taskId = event.getTaskId();
        String nodeId = event.getNodeId();
        String traceId = event.getTraceId() != null ? event.getTraceId() : TraceContext.generateTraceId();
        String nodeType = event.getType();
        String idempotencyKey = event.getIdempotencyKey() != null ? event.getIdempotencyKey() : taskId + "-" + nodeId;

        TraceContext.setContext(traceId, taskId, nodeId);

        log.info("Starting node execution: taskId={}, nodeId={}, type={}, traceId={}, idempotencyKey={}",
                taskId, nodeId, nodeType, traceId, idempotencyKey);

        Map<String, Object> payload = event.getPayload();
        Map<String, Object> input = payload != null ? payload : Map.of();

        try {
            savePreExecutionSnapshot(taskId, nodeId, nodeType, input);

            if (payload == null) {
                throw new IllegalArgumentException("Node payload is null");
            }

            String toolName = determineToolName(nodeType, payload);
            Map<String, Object> parameters = extractParameters(payload);
            ToolContext context = ToolContext.builder()
                    .taskId(taskId)
                    .nodeId(nodeId)
                    .retryCount(0)
                    .build();

            eventProducer.publishNodeRunning(taskId, nodeId, traceId);

            if (externalToolExecutor.isExternalTool(toolName)) {
                log.info("Executing as external tool: {}", toolName);
                executeExternalToolAsync(toolName, parameters, context, traceId, nodeType, input, idempotencyKey);
            } else {
                log.info("Executing as local tool: {}", toolName);
                executeLocalTool(toolName, parameters, context, traceId, nodeType, input, idempotencyKey);
            }

        } catch (Exception e) {
            log.error("Node execution failed: taskId={}, nodeId={}", taskId, nodeId, e);
            savePostExecutionSnapshot(taskId, nodeId, nodeType, input, null, e.getMessage());
            publishFailureResult(taskId, nodeId, traceId, e.getMessage(), idempotencyKey);
            recordNodeFailed(taskId, nodeId, e.getMessage());
            TraceContext.clear();
        }
    }

    private void executeExternalToolAsync(
            String toolName,
            Map<String, Object> parameters,
            ToolContext context,
            String traceId,
            String nodeType,
            Map<String, Object> input,
            String idempotencyKey) {

        String taskId = context.getTaskId();
        String nodeId = context.getNodeId();

        externalToolExecutor.executeTool(toolName, parameters, context)
                .subscribeOn(Schedulers.boundedElastic())
                .subscribe(
                        result -> {
                            try {
                                TraceContext.setContext(traceId, taskId, nodeId);
                                if (result.isSuccess()) {
                                    log.info("External tool execution succeeded: {}", toolName);
                                    savePostExecutionSnapshot(taskId, nodeId, nodeType, input, result.getData(), null);
                                    publishSuccessResult(taskId, nodeId, traceId, result.getData(), idempotencyKey);
                                    recordNodeSuccess(taskId, nodeId);
                                } else {
                                    log.warn("External tool execution failed: {}", toolName);
                                    savePostExecutionSnapshot(taskId, nodeId, nodeType, input, null, result.getErrorMessage());
                                    publishFailureResult(taskId, nodeId, traceId, result.getErrorMessage(), idempotencyKey);
                                    recordNodeFailed(taskId, nodeId, result.getErrorMessage());
                                }
                            } finally {
                                TraceContext.clear();
                            }
                        },
                        error -> {
                            try {
                                TraceContext.setContext(traceId, taskId, nodeId);
                                log.error("External tool execution error: {}", toolName, error);
                                String errorMessage = error.getMessage() != null ? error.getMessage() : "Unknown error";
                                savePostExecutionSnapshot(taskId, nodeId, nodeType, input, null, errorMessage);
                                publishFailureResult(taskId, nodeId, traceId, errorMessage, idempotencyKey);
                                recordNodeFailed(taskId, nodeId, errorMessage);
                            } finally {
                                TraceContext.clear();
                            }
                        }
                );
    }

    private void executeLocalTool(
            String toolName,
            Map<String, Object> parameters,
            ToolContext context,
            String traceId,
            String nodeType,
            Map<String, Object> input,
            String idempotencyKey) {

        String taskId = context.getTaskId();
        String nodeId = context.getNodeId();

        try {
            Optional<Tool> toolOpt = toolRegistry.getTool(toolName);
            if (toolOpt.isEmpty()) {
                throw new IllegalArgumentException("Tool not found: " + toolName);
            }

            Tool tool = toolOpt.get();

            if (!tool.validateParameters(parameters)) {
                throw new IllegalArgumentException("Invalid parameters for tool: " + toolName);
            }

            ToolResult result = executeWithTimeout(tool, parameters, context, traceId);

            if (result.isSuccess()) {
                savePostExecutionSnapshot(taskId, nodeId, nodeType, input, result.getData(), null);
                publishSuccessResult(taskId, nodeId, traceId, result.getData(), idempotencyKey);
                recordNodeSuccess(taskId, nodeId);
            } else {
                savePostExecutionSnapshot(taskId, nodeId, nodeType, input, null, result.getErrorMessage());
                publishFailureResult(taskId, nodeId, traceId, result.getErrorMessage(), idempotencyKey);
                recordNodeFailed(taskId, nodeId, result.getErrorMessage());
            }
        } catch (Exception e) {
            log.error("Local tool execution failed: taskId={}, nodeId={}", taskId, nodeId, e);
            savePostExecutionSnapshot(taskId, nodeId, nodeType, input, null, e.getMessage());
            publishFailureResult(taskId, nodeId, traceId, e.getMessage(), idempotencyKey);
            recordNodeFailed(taskId, nodeId, e.getMessage());
        } finally {
            TraceContext.clear();
        }
    }

    private ToolResult executeWithTimeout(Tool tool, Map<String, Object> parameters, ToolContext context, String traceId) {
        Instant startTime = Instant.now();
        Map<String, String> contextMap = TraceContext.getCopyOfContextMap();

        try {
            CompletableFuture<ToolResult> future = CompletableFuture.supplyAsync(
                    () -> {
                        TraceContext.setContextMap(contextMap);
                        try {
                            return tool.execute(parameters, context);
                        } finally {
                            TraceContext.clear();
                        }
                    },
                    toolExecutorService
            );

            int timeoutSeconds = workerConfig.getToolTimeoutSeconds();
            return future.get(timeoutSeconds, TimeUnit.SECONDS);

        } catch (TimeoutException e) {
            log.error("Tool execution timed out after {} seconds: taskId={}, nodeId={}",
                    workerConfig.getToolTimeoutSeconds(), context.getTaskId(), context.getNodeId());
            Instant endTime = Instant.now();
            return ToolResult.failure(
                    "Tool execution timed out after " + workerConfig.getToolTimeoutSeconds() + " seconds",
                    startTime,
                    endTime
            );
        } catch (Exception e) {
            log.error("Tool execution failed: taskId={}, nodeId={}",
                    context.getTaskId(), context.getNodeId(), e);
            Instant endTime = Instant.now();
            return ToolResult.failure(
                    e.getMessage() != null ? e.getMessage() : "Unknown error",
                    startTime,
                    endTime
            );
        }
    }

    private void savePreExecutionSnapshot(String taskId, String nodeId, String nodeType,
                                           Map<String, Object> input) {
        try {
            contextClient.saveNodeSnapshot(
                    taskId, nodeId, nodeType, nodeId,
                    "RUNNING", input, null, 0, 3, 5,
                    "default", 0, null
            ).subscribe();
        } catch (Exception e) {
            log.warn("Failed to save pre-execution snapshot for node: {}", nodeId, e);
        }
    }

    private void savePostExecutionSnapshot(String taskId, String nodeId, String nodeType,
                                            Map<String, Object> input, Map<String, Object> output,
                                            String errorMessage) {
        try {
            String status = errorMessage == null ? "SUCCESS" : "FAILED";
            contextClient.saveNodeSnapshot(
                    taskId, nodeId, nodeType, nodeId,
                    status, input, output, 0, 3, 5,
                    "default", 1, errorMessage
            ).subscribe();
        } catch (Exception e) {
            log.warn("Failed to save post-execution snapshot for node: {}", nodeId, e);
        }
    }

    private void recordNodeSuccess(String taskId, String nodeId) {
        try {
            contextClient.recordNodeSuccess(taskId, nodeId).subscribe();
        } catch (Exception e) {
            log.warn("Failed to record node success: {}", nodeId, e);
        }
    }

    private void recordNodeFailed(String taskId, String nodeId, String errorMessage) {
        try {
            contextClient.recordNodeFailed(taskId, nodeId, errorMessage).subscribe();
        } catch (Exception e) {
            log.warn("Failed to record node failed: {}", nodeId, e);
        }
    }

    private String determineToolName(String nodeType, Map<String, Object> payload) {
        if (payload.containsKey("tool")) {
            return payload.get("tool").toString();
        }
        if ("TOOL".equals(nodeType) && payload.containsKey("name")) {
            return payload.get("name").toString();
        }
        return "llm";
    }

    @SuppressWarnings("unchecked")
    private Map<String, Object> extractParameters(Map<String, Object> payload) {
        if (payload.containsKey("parameters")) {
            Object params = payload.get("parameters");
            if (params instanceof Map) {
                return (Map<String, Object>) params;
            }
        }
        if (payload.containsKey("input")) {
            Object input = payload.get("input");
            if (input instanceof Map) {
                return (Map<String, Object>) input;
            }
        }
        return payload;
    }

    private void publishSuccessResult(String taskId, String nodeId, String traceId, Map<String, Object> data, String idempotencyKey) {
        NodeResultEvent resultEvent = new NodeResultEvent(
                taskId,
                nodeId,
                NodeStatus.SUCCESS,
                data,
                traceId,
                null,
                idempotencyKey
        );
        eventProducer.publishNodeResult(resultEvent);
    }

    private void publishFailureResult(String taskId, String nodeId, String traceId, String errorMessage, String idempotencyKey) {
        NodeResultEvent resultEvent = new NodeResultEvent(
                taskId,
                nodeId,
                NodeStatus.FAILED,
                null,
                traceId,
                errorMessage,
                idempotencyKey
        );
        eventProducer.publishNodeResult(resultEvent);
    }
}
