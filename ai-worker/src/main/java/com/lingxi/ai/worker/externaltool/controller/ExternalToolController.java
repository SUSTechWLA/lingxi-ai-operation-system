package com.lingxi.ai.worker.externaltool.controller;

import com.lingxi.ai.worker.externaltool.model.RegisteredTool;
import com.lingxi.ai.worker.externaltool.model.ToolRegisterRequest;
import com.lingxi.ai.worker.externaltool.registry.ExternalToolRegistry;
import com.lingxi.ai.worker.externaltool.service.ToolRegistrationService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;
import reactor.core.publisher.Mono;

import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;

/**
 * 外部工具管理控制器
 */
@Slf4j
@RestController
@RequestMapping("/worker")
@RequiredArgsConstructor
public class ExternalToolController {

    private final ToolRegistrationService registrationService;
    private final ExternalToolRegistry toolRegistry;

    /**
     * 注册外部工具
     */
    @PostMapping("/register")
    public Mono<ResponseEntity<Map<String, Object>>> registerTool(
            @RequestBody ToolRegisterRequest request) {

        log.info("Received tool registration request: endpoint={}", request.getEndpoint());

        return registrationService.registerTool(request.getEndpoint(), request.getWorkerGroup())
                .map(registeredTool -> ResponseEntity.ok(Map.of(
                        "code", 200,
                        "message", "success",
                        "data", Map.of(
                                "toolName", registeredTool.getToolInfo().getToolName(),
                                "toolVersion", registeredTool.getToolInfo().getToolVersion(),
                                "status", "REGISTERED"
                        )
                )))
                .onErrorResume(e -> {
                    log.error("Tool registration failed", e);
                    return Mono.just(ResponseEntity.badRequest().body(Map.of(
                            "code", 400,
                            "message", e.getMessage() != null ? e.getMessage() : "Registration failed"
                    )));
                });
    }

    /**
     * 注销外部工具
     */
    @PostMapping("/unregister")
    public Mono<ResponseEntity<Map<String, Object>>> unregisterTool(
            @RequestBody ToolRegisterRequest request) {

        log.info("Received tool unregistration request: endpoint={}", request.getEndpoint());

        return registrationService.unregisterTool(request.getEndpoint())
                .then(Mono.just(ResponseEntity.ok(Map.of(
                        "code", 200,
                        "message", "success"
                ))));
    }

    /**
     * 获取所有已注册的外部工具
     */
    @GetMapping("/tools")
    public ResponseEntity<Map<String, Object>> listTools() {
        List<Map<String, Object>> tools = toolRegistry.getAllTools().stream()
                .map(this::toToolSummary)
                .collect(Collectors.toList());

        return ResponseEntity.ok(Map.of(
                "code", 200,
                "message", "success",
                "data", Map.of(
                        "count", tools.size(),
                        "tools", tools
                )
        ));
    }

    /**
     * 获取指定工具的详细信息
     */
    @GetMapping("/tools/{toolName}")
    public ResponseEntity<Map<String, Object>> getTool(@PathVariable String toolName) {
        return toolRegistry.getToolByName(toolName)
                .map(tool -> ResponseEntity.ok(Map.<String, Object>of(
                        "code", 200,
                        "message", "success",
                        "data", tool
                )))
                .orElse(ResponseEntity.notFound().build());
    }

    private Map<String, Object> toToolSummary(RegisteredTool tool) {
        return Map.of(
                "toolName", tool.getToolInfo().getToolName(),
                "toolVersion", tool.getToolInfo().getToolVersion(),
                "description", tool.getToolInfo().getDescription(),
                "endpoint", tool.getToolEndpoint(),
                "status", tool.getStatus().name(),
                "registeredAt", tool.getRegisteredAt().toString()
        );
    }
}
