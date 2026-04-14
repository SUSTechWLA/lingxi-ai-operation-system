package com.lingxi.ai.orchestrator.controller;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;
import java.util.Map;
import com.lingxi.ai.orchestrator.model.Task;
import com.lingxi.ai.orchestrator.service.OrchestratorService;

@RestController
@RequestMapping("/api/task")
public class TaskController {
    @Autowired
    private OrchestratorService orchestratorService;

    // 创建任务
    @PostMapping
    public ResponseEntity<Task> createTask(@RequestBody Map<String, String> request) {
        String prompt = request.get("prompt");
        Task task = orchestratorService.createTask(prompt);
        return ResponseEntity.ok(task);
    }

    // 查询任务状态
    @GetMapping("/{taskId}")
    public ResponseEntity<Task> getTask(@PathVariable String taskId) {
        Task task = orchestratorService.getTask(taskId);
        return ResponseEntity.ok(task);
    }
}