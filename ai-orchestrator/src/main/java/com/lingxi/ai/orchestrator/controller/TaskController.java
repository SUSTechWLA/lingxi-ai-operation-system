package com.lingxi.ai.orchestrator.controller;

import com.lingxi.ai.orchestrator.context.ContextService;
import com.lingxi.ai.orchestrator.entity.Task;
import com.lingxi.ai.orchestrator.model.DAGRequest;
import com.lingxi.ai.orchestrator.service.OrchestratorService;
import com.lingxi.ai.orchestrator.service.StateMachine;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Optional;

@RestController
@RequestMapping("/api")
public class TaskController {

    @Autowired
    private OrchestratorService orchestratorService;

    @Autowired
    private StateMachine stateMachine;

    @Autowired
    private ContextService contextService;

    @Autowired
    private com.lingxi.ai.orchestrator.service.TaskExecutionControl taskExecutionControl;

    @PostMapping("/task/create")
    public ResponseEntity<Map<String, Object>> createTask(@RequestBody Map<String, Object> request) {
        Task task = orchestratorService.createTask(request);

        Map<String, Object> result = Map.of(
                "taskId", task.getId(),
                "status", task.getStatus()
        );
        return ResponseEntity.ok(result);
    }

    @PostMapping("/task/{taskId}/dag")
    public ResponseEntity<Map<String, String>> submitDAG(
            @PathVariable String taskId,
            @RequestBody DAGRequest dagRequest) {
        orchestratorService.submitDAG(taskId, dagRequest);

        Map<String, String> result = Map.of(
                "taskId", taskId,
                "message", "DAG submitted successfully"
        );
        return ResponseEntity.ok(result);
    }

    @GetMapping("/task/{taskId}")
    public ResponseEntity<Map<String, Object>> getTask(@PathVariable String taskId) {
        Map<String, Object> task = orchestratorService.getTaskWithDetails(taskId);
        if (task == null) {
            return ResponseEntity.notFound().build();
        }
        return ResponseEntity.ok(task);
    }

    @GetMapping("/task/{taskId}/context")
    public ResponseEntity<List<Map<String, Object>>> getTaskContext(@PathVariable String taskId) {
        List<Map<String, Object>> context = contextService.getContextForTask(taskId);
        return ResponseEntity.ok(context);
    }

    @PostMapping("/node/{nodeId}/success")
    public ResponseEntity<Map<String, String>> onNodeSuccess(
            @PathVariable String nodeId,
            @RequestBody Map<String, Object> output) {
        stateMachine.onSuccess(nodeId, output);
        return ResponseEntity.ok(Map.of("message", "Node success recorded"));
    }

    @PostMapping("/node/{nodeId}/failure")
    public ResponseEntity<Map<String, String>> onNodeFailure(
            @PathVariable String nodeId,
            @RequestBody Map<String, String> request) {
        String errorMessage = request.getOrDefault("errorMessage", "Unknown error");
        stateMachine.onFailure(nodeId, errorMessage);
        return ResponseEntity.ok(Map.of("message", "Node failure recorded"));
    }

    @GetMapping("/node/{nodeId}/snapshot/latest")
    public ResponseEntity<Map<String, Object>> getLatestSnapshot(@PathVariable String nodeId) {
        Optional<Map<String, Object>> snapshot = contextService.getLatestSnapshotForNode(nodeId);
        if (snapshot.isPresent()) {
            return ResponseEntity.ok(Map.of(
                    "nodeId", nodeId,
                    "snapshot", snapshot.get()
            ));
        }
        return ResponseEntity.notFound().build();
    }

    @PostMapping("/node/{nodeId}/restore")
    public ResponseEntity<Map<String, Object>> restoreFromSnapshot(@PathVariable String nodeId) {
        return contextService.restoreNodeFromSnapshot(nodeId)
                .map(result -> ResponseEntity.ok(Map.of(
                        "nodeId", nodeId,
                        "message", "Node snapshot retrieved",
                        "snapshot", result
                )))
                .orElse(ResponseEntity.notFound().build());
    }

    @PostMapping("/task/{taskId}/pause")
    public ResponseEntity<Map<String, String>> pauseTask(
            @PathVariable String taskId,
            @RequestBody(required = false) Map<String, String> request) {
        String reason = request != null ? request.getOrDefault("reason", "Manual pause") : "Manual pause";
        taskExecutionControl.pauseTask(taskId, reason);
        return ResponseEntity.ok(Map.of(
                "taskId", taskId,
                "message", "Task paused successfully"
        ));
    }

    @PostMapping("/task/{taskId}/resume")
    public ResponseEntity<Map<String, String>> resumeTask(@PathVariable String taskId) {
        taskExecutionControl.resumeTask(taskId);
        return ResponseEntity.ok(Map.of(
                "taskId", taskId,
                "message", "Task resumed successfully"
        ));
    }

    @PostMapping("/node/{nodeId}/retry")
    public ResponseEntity<Map<String, String>> retryNode(@PathVariable String nodeId) {
        taskExecutionControl.retryNode(nodeId);
        return ResponseEntity.ok(Map.of(
                "nodeId", nodeId,
                "message", "Node retry initiated"
        ));
    }

    @GetMapping("/task/{taskId}/pause-reason")
    public ResponseEntity<Map<String, String>> getPauseReason(@PathVariable String taskId) {
        String reason = taskExecutionControl.getTaskPauseReason(taskId);
        return ResponseEntity.ok(Map.of(
                "taskId", taskId,
                "reason", reason
        ));
    }

    @GetMapping("/health")
    public ResponseEntity<Map<String, String>> health() {
        return ResponseEntity.ok(Map.of(
                "status", "UP",
                "service", "ai-orchestrator"
        ));
    }

    @PostMapping("/node")
    public ResponseEntity<Map<String, Object>> submitDAGFromNL(@RequestBody Map<String, Object> dagPayload) {
        try {
            Map<String, Object> input = new HashMap<>();
            input.put("source", "nl-translator");
            Task task = orchestratorService.createTask(input);

            DAGRequest dagRequest = convertToDAGRequest(dagPayload);
            orchestratorService.submitDAG(task.getId(), dagRequest);

            Map<String, Object> result = new HashMap<>();
            result.put("taskId", task.getId());
            result.put("status", task.getStatus());
            result.put("message", "DAG submitted successfully");
            return ResponseEntity.ok(result);
        } catch (Exception e) {
            Map<String, Object> error = new HashMap<>();
            error.put("error", e.getMessage());
            return ResponseEntity.badRequest().body(error);
        }
    }

    @SuppressWarnings("unchecked")
    private DAGRequest convertToDAGRequest(Map<String, Object> dagPayload) {
        DAGRequest dagRequest = new DAGRequest();

        List<Map<String, Object>> nodesData = (List<Map<String, Object>>) dagPayload.get("nodes");
        if (nodesData != null) {
            List<DAGRequest.NodeRequest> nodes = nodesData.stream().map(n -> {
                DAGRequest.NodeRequest nodeReq = new DAGRequest.NodeRequest();
                nodeReq.setId((String) n.get("nodeId"));
                if (nodeReq.getId() == null) {
                    nodeReq.setId((String) n.get("id"));
                }
                nodeReq.setType((String) n.get("type"));
                nodeReq.setName((String) n.get("name"));
                nodeReq.setInput((Map<String, Object>) n.get("input"));
                return nodeReq;
            }).toList();
            dagRequest.setNodes(nodes);
        }

        List<Map<String, String>> edgesData = (List<Map<String, String>>) dagPayload.get("edges");
        if (edgesData != null) {
            List<DAGRequest.Edge> edges = edgesData.stream().map(e -> {
                DAGRequest.Edge edge = new DAGRequest.Edge();
                edge.setFrom(e.get("from"));
                edge.setTo(e.get("to"));
                return edge;
            }).toList();
            dagRequest.setEdges(edges);
        }

        return dagRequest;
    }
}
