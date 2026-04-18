package com.lingxi.ai.worker.externaltool.service;

import com.lingxi.ai.worker.externaltool.client.ExternalToolClient;
import com.lingxi.ai.worker.externaltool.model.RegisteredTool;
import com.lingxi.ai.worker.externaltool.model.ToolExecuteRequest;
import com.lingxi.ai.worker.externaltool.registry.ExternalToolRegistry;
import com.lingxi.ai.worker.tool.Tool;
import com.lingxi.ai.worker.tool.ToolContext;
import com.lingxi.ai.worker.tool.ToolResult;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;
import reactor.core.publisher.Mono;

import java.time.Instant;
import java.util.Map;
import java.util.Optional;

/**
 * 外部工具执行器
 * 实现 Tool 接口，让外部工具可以像本地工具一样被调用
 */
@Slf4j
@Component
@RequiredArgsConstructor
public class ExternalToolExecutor {

    private final ExternalToolRegistry toolRegistry;
    private final ExternalToolClient toolClient;

    /**
     * 执行外部工具
     */
    public Mono<ToolResult> executeTool(
            String toolName,
            Map<String, Object> parameters,
            ToolContext context) {

        log.info("Executing external tool: {}, taskId={}, nodeId={}",
                toolName, context.getTaskId(), context.getNodeId());

        Instant startTime = Instant.now();

        Optional<RegisteredTool> toolOpt = toolRegistry.getToolByName(toolName);
        if (toolOpt.isEmpty()) {
            return Mono.just(ToolResult.failure(
                    "External tool not found: " + toolName,
                    startTime,
                    Instant.now()
            ));
        }

        RegisteredTool registeredTool = toolOpt.get();
        if (registeredTool.getStatus() != RegisteredTool.ToolStatus.AVAILABLE) {
            return Mono.just(ToolResult.failure(
                    "Tool is not available: " + toolName + ", status: " + registeredTool.getStatus(),
                    startTime,
                    Instant.now()
            ));
        }

        ToolExecuteRequest request = ToolExecuteRequest.builder()
                .nodeId(context.getNodeId())
                .taskId(context.getTaskId())
                .traceId(context.getTaskId()) // 复用 taskId 作为 traceId
                .input(parameters)
                .build();

        return toolClient.executeTool(registeredTool.getToolEndpoint(), request)
                .map(result -> ToolResult.success(
                        result,
                        startTime,
                        Instant.now()
                ))
                .onErrorResume(e -> {
                    log.error("External tool execution failed: {}", toolName, e);
                    String errorMessage = e.getMessage();
                    if (e instanceof ExternalToolClient.ToolExecutionException) {
                        var execEx = (ExternalToolClient.ToolExecutionException) e;
                        errorMessage = execEx.getMessage();
                    }
                    return Mono.just(ToolResult.failure(
                            errorMessage != null ? errorMessage : "Tool execution failed",
                            startTime,
                            Instant.now()
                    ));
                });
    }

    /**
     * 检查是否为外部工具
     */
    public boolean isExternalTool(String toolName) {
        return toolRegistry.hasTool(toolName);
    }

    /**
     * 创建一个包装外部工具的 Tool 实现
     */
    public Tool wrapAsTool(String toolName) {
        Optional<RegisteredTool> toolOpt = toolRegistry.getToolByName(toolName);
        if (toolOpt.isEmpty()) {
            throw new IllegalArgumentException("External tool not found: " + toolName);
        }

        RegisteredTool registeredTool = toolOpt.get();
        return new ExternalToolWrapper(registeredTool, this);
    }

    /**
     * 外部工具包装类
     */
    @RequiredArgsConstructor
    private static class ExternalToolWrapper implements Tool {
        private final RegisteredTool registeredTool;
        private final ExternalToolExecutor executor;

        @Override
        public String getName() {
            return registeredTool.getToolInfo().getToolName();
        }

        @Override
        public String getDescription() {
            return registeredTool.getToolInfo().getDescription();
        }

        @Override
        public com.lingxi.ai.worker.tool.ToolType getType() {
            return com.lingxi.ai.worker.tool.ToolType.EXTERNAL;
        }

        @Override
        public ToolResult execute(Map<String, Object> parameters, ToolContext context) {
            // 注意：这里是同步调用，实际使用时应该在 NodeExecutor 中使用异步方式
            try {
                return executor.executeTool(getName(), parameters, context).block();
            } catch (Exception e) {
                return ToolResult.failure(e.getMessage(), Instant.now(), Instant.now());
            }
        }

        @Override
        public boolean validateParameters(Map<String, Object> parameters) {
            // 简单验证，实际验证由外部工具自己完成
            return parameters != null;
        }
    }
}
