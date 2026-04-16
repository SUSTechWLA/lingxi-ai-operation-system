package com.lingxi.ai.orchestrator.controller;

import com.lingxi.ai.orchestrator.context.ContextService;
import com.lingxi.ai.orchestrator.entity.Task;
import com.lingxi.ai.orchestrator.model.DAGRequest;
import com.lingxi.ai.orchestrator.service.OrchestratorService;
import com.lingxi.ai.orchestrator.service.StateMachine;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

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
}
