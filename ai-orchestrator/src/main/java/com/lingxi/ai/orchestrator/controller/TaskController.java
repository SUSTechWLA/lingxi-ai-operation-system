package com.lingxi.ai.orchestrator.controller;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;
import com.lingxi.ai.orchestrator.model.Task;
import com.lingxi.ai.orchestrator.model.DAG;
import com.lingxi.ai.orchestrator.service.OrchestratorService;

@RestController
@RequestMapping("/api")
public class TaskController {
    @Autowired
    private OrchestratorService orchestratorService;

    // 创建任务（接收 DAG）
    @PostMapping("/node")
    public ResponseEntity<Task> createTask(@RequestBody DAG dag) {
        Task task = orchestratorService.createTask(dag);
        return ResponseEntity.ok(task);
    }

    // 查询任务状态
    @GetMapping("/task/{taskId}")
    public ResponseEntity<Task> getTask(@PathVariable String taskId) {
        Task task = orchestratorService.getTask(taskId);
        return ResponseEntity.ok(task);
    }
}