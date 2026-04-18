package com.lingxi.ai.worker.tool.spi;

import com.lingxi.ai.worker.externaltool.model.RegisteredTool;
import com.lingxi.ai.worker.externaltool.registry.ExternalToolRegistry;
import com.lingxi.ai.worker.externaltool.service.ExternalToolExecutor;
import com.lingxi.ai.worker.tool.Tool;
import com.lingxi.ai.worker.tool.ToolContext;
import com.lingxi.ai.worker.tool.ToolResult;
import com.lingxi.ai.worker.tool.ToolType;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;
import reactor.core.publisher.Mono;

import java.time.Instant;
import java.util.Map;
import java.util.Optional;

/**
 * HTTP外部工具提供器
 * 用于调用独立部署的多语言工具服务
 */
@Slf4j
@Component
@RequiredArgsConstructor
public class HttpToolProvider implements ToolProvider {

    private final ExternalToolRegistry externalToolRegistry;
    private final ExternalToolExecutor externalToolExecutor;

    @Override
    public String getName() {
        return "http";
    }

    @Override
    public ProviderType getType() {
        return ProviderType.HTTP;
    }

    @Override
    public int getPriority() {
        return 100;
    }

    @Override
    public boolean supports(String toolName) {
        return externalToolRegistry.hasTool(toolName);
    }

    @Override
    public Tool getTool(String toolName) {
        Optional<RegisteredTool> toolOpt = externalToolRegistry.getToolByName(toolName);
        if (toolOpt.isEmpty()) {
            return null;
        }

        RegisteredTool registeredTool = toolOpt.get();
        return new HttpToolWrapper(registeredTool, externalToolExecutor);
    }

    @Override
    public ToolResult execute(String toolName, Map<String, Object> parameters, ToolContext context) {
        // 优化路径：直接使用ExternalToolExecutor，不经过Wrapper
        try {
            // 注意：这里是同步调用，实际应该在NodeExecutor中使用异步
            return externalToolExecutor.executeTool(toolName, parameters, context).block();
        } catch (Exception e) {
            log.error("HTTP tool execution failed: {}", toolName, e);
            return ToolResult.failure(
                    e.getMessage() != null ? e.getMessage() : "Execution failed",
                    Instant.now(),
                    Instant.now()
            );
        }
    }

    @Override
    public Map<String, Tool> listTools() {
        // 动态包装所有外部工具
        Map<String, Tool> tools = new java.util.HashMap<>();
        for (RegisteredTool registeredTool : externalToolRegistry.getAllTools()) {
            tools.put(registeredTool.getToolInfo().getToolName(),
                    new HttpToolWrapper(registeredTool, externalToolExecutor));
        }
        return tools;
    }

    /**
     * HTTP工具包装类
     */
    @RequiredArgsConstructor
    private static class HttpToolWrapper implements Tool {
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
        public ToolType getType() {
            return ToolType.EXTERNAL;
        }

        @Override
        public ToolResult execute(Map<String, Object> parameters, ToolContext context) {
            try {
                return executor.executeTool(getName(), parameters, context).block();
            } catch (Exception e) {
                return ToolResult.failure(e.getMessage(), Instant.now(), Instant.now());
            }
        }

        @Override
        public boolean validateParameters(Map<String, Object> parameters) {
            // 实际验证由外部工具自己完成
            return parameters != null;
        }
    }
}
