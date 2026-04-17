package com.lingxi.ai.worker.tool;

import jakarta.annotation.PostConstruct;
import org.springframework.stereotype.Component;

import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Optional;

/**
 * 工具注册表
 * 负责工具的注册、发现和获取
 */
@Component
public class ToolRegistry {

    private final Map<String, Tool> tools = new HashMap<>();
    private final List<Tool> availableTools;

    public ToolRegistry(List<Tool> availableTools) {
        this.availableTools = availableTools;
    }

    @PostConstruct
    public void init() {
        for (Tool tool : availableTools) {
            registerTool(tool);
        }
    }

    /**
     * 注册工具
     */
    public void registerTool(Tool tool) {
        tools.put(tool.getName(), tool);
    }

    /**
     * 获取工具
     */
    public Optional<Tool> getTool(String name) {
        return Optional.ofNullable(tools.get(name));
    }

    /**
     * 根据类型获取工具列表
     */
    public List<Tool> getToolsByType(ToolType type) {
        return tools.values().stream()
                .filter(tool -> tool.getType() == type)
                .toList();
    }

    /**
     * 获取所有已注册工具
     */
    public Map<String, Tool> getAllTools() {
        return new HashMap<>(tools);
    }

    /**
     * 检查工具是否存在
     */
    public boolean hasTool(String name) {
        return tools.containsKey(name);
    }
}
