package com.lingxi.ai.worker.tool.spi;

import com.lingxi.ai.worker.tool.Tool;
import com.lingxi.ai.worker.tool.ToolRegistry;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * 内置工具提供器
 * 同JVM内的工具，直接调用，无网络开销
 */
@Slf4j
@Component
@RequiredArgsConstructor
public class BuiltinToolProvider implements ToolProvider {

    private final ToolRegistry toolRegistry;

    @Override
    public String getName() {
        return "builtin";
    }

    @Override
    public ProviderType getType() {
        return ProviderType.BUILTIN;
    }

    @Override
    public int getPriority() {
        return 10;  // 最高优先级
    }

    @Override
    public boolean supports(String toolName) {
        return toolRegistry.hasTool(toolName);
    }

    @Override
    public Tool getTool(String toolName) {
        return toolRegistry.getTool(toolName).orElse(null);
    }

    @Override
    public Map<String, Tool> listTools() {
        return toolRegistry.getAllTools();
    }
}
