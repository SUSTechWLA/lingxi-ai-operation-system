package com.lingxi.ai.worker.controller;

import com.lingxi.ai.worker.tool.ToolRegistry;
import lombok.RequiredArgsConstructor;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.Map;
import java.util.stream.Collectors;

@RestController
@RequestMapping("/api")
@RequiredArgsConstructor
public class WorkerController {

    private final ToolRegistry toolRegistry;

    @GetMapping("/health")
    public ResponseEntity<Map<String, String>> health() {
        return ResponseEntity.ok(Map.of(
                "status", "UP",
                "service", "ai-worker"
        ));
    }

    @GetMapping("/tools")
    public ResponseEntity<Map<String, Object>> listTools() {
        var tools = toolRegistry.getAllTools().values().stream()
                .map(tool -> Map.of(
                        "name", tool.getName(),
                        "description", tool.getDescription(),
                        "type", tool.getType().name()
                ))
                .collect(Collectors.toList());

        return ResponseEntity.ok(Map.of(
                "count", tools.size(),
                "tools", tools
        ));
    }
}
