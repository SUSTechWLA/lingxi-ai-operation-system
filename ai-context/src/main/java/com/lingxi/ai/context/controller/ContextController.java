package com.lingxi.ai.context.controller;

import com.lingxi.ai.context.entity.Context;
import com.lingxi.ai.context.service.ContextService;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.util.List;
import java.util.Map;
import java.util.Optional;

@RestController
@RequestMapping("/api")
public class ContextController {

    @Autowired
    private ContextService contextService;

    @GetMapping("/task/{taskId}/context")
    public ResponseEntity<List<Context>> getTaskContext(@PathVariable String taskId) {
        List<Context> context = contextService.getContextForTask(taskId);
        return ResponseEntity.ok(context);
    }

    @GetMapping("/node/{nodeId}/snapshot/latest")
    public ResponseEntity<Map<String, Object>> getLatestSnapshot(@PathVariable String nodeId) {
        return contextService.getLatestSnapshotForNode(nodeId)
                .map(snapshot -> ResponseEntity.ok(Map.of(
                        "nodeId", nodeId,
                        "snapshot", snapshot
                )))
                .orElse(ResponseEntity.notFound().build());
    }

    @PostMapping("/context/task-created")
    public ResponseEntity<Context> recordTaskCreated(@RequestBody Map<String, String> request) {
        String taskId = request.get("taskId");
        return ResponseEntity.ok(contextService.recordTaskCreated(taskId));
    }

    @PostMapping("/context/task-success")
    public ResponseEntity<Context> recordTaskSuccess(@RequestBody Map<String, String> request) {
        String taskId = request.get("taskId");
        return ResponseEntity.ok(contextService.recordTaskSuccess(taskId));
    }

    @PostMapping("/context/task-failed")
    public ResponseEntity<Context> recordTaskFailed(@RequestBody Map<String, String> request) {
        String taskId = request.get("taskId");
        return ResponseEntity.ok(contextService.recordTaskFailed(taskId));
    }

    @PostMapping("/context/dag-submitted")
    public ResponseEntity<Context> recordDagSubmitted(@RequestBody Map<String, String> request) {
        String taskId = request.get("taskId");
        return ResponseEntity.ok(contextService.recordDagSubmitted(taskId));
    }

    @PostMapping("/context/dag-validated")
    public ResponseEntity<Context> recordDagValidated(@RequestBody Map<String, String> request) {
        String taskId = request.get("taskId");
        return ResponseEntity.ok(contextService.recordDagValidated(taskId));
    }

    @PostMapping("/context/node-scheduled")
    public ResponseEntity<Context> recordNodeScheduled(@RequestBody Map<String, Object> request) {
        String taskId = (String) request.get("taskId");
        String nodeId = (String) request.get("nodeId");
        String nodeType = (String) request.get("type");
        String nodeName = (String) request.get("name");
        return ResponseEntity.ok(contextService.recordNodeScheduled(taskId, nodeId, nodeType, nodeName));
    }

    @PostMapping("/context/node-ready")
    public ResponseEntity<Context> recordNodeReady(@RequestBody Map<String, String> request) {
        String taskId = request.get("taskId");
        String nodeId = request.get("nodeId");
        return ResponseEntity.ok(contextService.recordNodeReady(taskId, nodeId));
    }

    @PostMapping("/context/node-success")
    public ResponseEntity<Context> recordNodeSuccess(@RequestBody Map<String, String> request) {
        String taskId = request.get("taskId");
        String nodeId = request.get("nodeId");
        return ResponseEntity.ok(contextService.recordNodeSuccess(taskId, nodeId));
    }

    @PostMapping("/context/node-failed")
    public ResponseEntity<Context> recordNodeFailed(@RequestBody Map<String, String> request) {
        String taskId = request.get("taskId");
        String nodeId = request.get("nodeId");
        String errorMessage = request.get("errorMessage");
        return ResponseEntity.ok(contextService.recordNodeFailed(taskId, nodeId, errorMessage));
    }

    @PostMapping("/context/node-retry")
    public ResponseEntity<Context> recordNodeRetry(@RequestBody Map<String, Object> request) {
        String taskId = (String) request.get("taskId");
        String nodeId = (String) request.get("nodeId");
        int retryCount = (Integer) request.get("retryCount");
        int maxRetry = (Integer) request.get("maxRetry");
        return ResponseEntity.ok(contextService.recordNodeRetry(taskId, nodeId, retryCount, maxRetry));
    }

    @PostMapping("/context/node-snapshot")
    public ResponseEntity<Context> recordNodeSnapshot(@RequestBody Map<String, Object> request) {
        String taskId = (String) request.get("taskId");
        String nodeId = (String) request.get("nodeId");
        String nodeType = (String) request.get("type");
        String nodeName = (String) request.get("name");
        String status = (String) request.get("status");
        @SuppressWarnings("unchecked")
        Map<String, Object> input = (Map<String, Object>) request.get("input");
        @SuppressWarnings("unchecked")
        Map<String, Object> output = (Map<String, Object>) request.get("output");
        int retryCount = (Integer) request.get("retryCount");
        int maxRetry = (Integer) request.get("maxRetry");
        int priority = (Integer) request.get("priority");
        String workerGroup = (String) request.get("workerGroup");
        Integer version = (Integer) request.get("version");
        String errorMessage = (String) request.get("errorMessage");

        return ResponseEntity.ok(contextService.recordNodeSnapshot(
                taskId, nodeId, nodeType, nodeName, status, input, output,
                retryCount, maxRetry, priority, workerGroup, version, errorMessage));
    }

    @GetMapping("/health")
    public ResponseEntity<Map<String, String>> health() {
        return ResponseEntity.ok(Map.of("status", "UP", "service", "ai-context"));
    }
}
