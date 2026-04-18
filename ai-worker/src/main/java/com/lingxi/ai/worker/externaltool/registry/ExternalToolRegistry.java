package com.lingxi.ai.worker.externaltool.registry;

import com.lingxi.ai.worker.externaltool.model.RegisteredTool;
import com.lingxi.ai.worker.externaltool.model.ToolInfo;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.*;
import java.util.concurrent.ConcurrentHashMap;

/**
 * 外部工具注册表（本地缓存）
 */
@Slf4j
@Component
public class ExternalToolRegistry {

    private final Map<String, RegisteredTool> toolsByEndpoint = new ConcurrentHashMap<>();
    private final Map<String, RegisteredTool> toolsByName = new ConcurrentHashMap<>();

    /**
     * 注册工具
     */
    public synchronized RegisteredTool registerTool(
            ToolInfo toolInfo,
            String endpoint,
            String workerId,
            String workerGroup) {

        // 获取统一的工具名称（兼容 toolId 和 toolName）
        String toolName = getUnifiedToolName(toolInfo);

        log.info("Registering tool: name={}, version={}, endpoint={}",
                toolName, toolInfo.getToolVersion(), endpoint);

        RegisteredTool existing = toolsByName.get(toolName);
        if (existing != null && !existing.getToolEndpoint().equals(endpoint)) {
            log.warn("Tool with same name already exists, unregistering old one: {}",
                    existing.getToolEndpoint());
            unregisterTool(existing.getToolEndpoint());
        }

        RegisteredTool registeredTool = RegisteredTool.builder()
                .toolInfo(toolInfo)
                .toolEndpoint(endpoint)
                .workerId(workerId)
                .workerGroup(workerGroup != null ? workerGroup : "default")
                .registeredAt(Instant.now())
                .lastHeartbeatAt(Instant.now())
                .status(RegisteredTool.ToolStatus.AVAILABLE)
                .build();

        toolsByEndpoint.put(endpoint, registeredTool);
        toolsByName.put(toolName, registeredTool);

        log.info("Tool registered successfully: {}", toolName);
        return registeredTool;
    }

    /**
     * 获取统一的工具名称（兼容 toolId 和 toolName）
     */
    private String getUnifiedToolName(ToolInfo toolInfo) {
        if (toolInfo.getToolName() != null && !toolInfo.getToolName().isEmpty()) {
            return toolInfo.getToolName();
        }
        if (toolInfo.getToolId() != null && !toolInfo.getToolId().isEmpty()) {
            return toolInfo.getToolId();
        }
        return "unknown_tool_" + System.currentTimeMillis();
    }

    /**
     * 注销工具
     */
    public synchronized Optional<RegisteredTool> unregisterTool(String endpoint) {
        RegisteredTool tool = toolsByEndpoint.remove(endpoint);
        if (tool != null) {
            String toolName = getUnifiedToolName(tool.getToolInfo());
            toolsByName.remove(toolName);
            tool.setStatus(RegisteredTool.ToolStatus.OFFLINE);
            log.info("Tool unregistered: name={}, endpoint={}",
                    toolName, endpoint);
            return Optional.of(tool);
        }
        return Optional.empty();
    }

    /**
     * 根据名称获取工具
     */
    public Optional<RegisteredTool> getToolByName(String toolName) {
        return Optional.ofNullable(toolsByName.get(toolName));
    }

    /**
     * 根据endpoint获取工具
     */
    public Optional<RegisteredTool> getToolByEndpoint(String endpoint) {
        return Optional.ofNullable(toolsByEndpoint.get(endpoint));
    }

    /**
     * 检查工具是否存在
     */
    public boolean hasTool(String toolName) {
        return toolsByName.containsKey(toolName);
    }

    /**
     * 获取所有已注册工具
     */
    public List<RegisteredTool> getAllTools() {
        return new ArrayList<>(toolsByName.values());
    }

    /**
     * 获取可用工具
     */
    public List<RegisteredTool> getAvailableTools() {
        return toolsByName.values().stream()
                .filter(t -> t.getStatus() == RegisteredTool.ToolStatus.AVAILABLE)
                .toList();
    }

    /**
     * 更新工具心跳
     */
    public void updateHeartbeat(String endpoint, boolean healthy) {
        RegisteredTool tool = toolsByEndpoint.get(endpoint);
        if (tool != null) {
            tool.setLastHeartbeatAt(Instant.now());
            if (healthy) {
                tool.setStatus(RegisteredTool.ToolStatus.AVAILABLE);
            } else {
                // 不要立即标记为不可用，可能是临时网络问题
                log.warn("Tool health check failed: {}", endpoint);
            }
        }
    }

    /**
     * 标记工具为不可用
     */
    public void markUnavailable(String endpoint) {
        RegisteredTool tool = toolsByEndpoint.get(endpoint);
        if (tool != null) {
            tool.setStatus(RegisteredTool.ToolStatus.UNAVAILABLE);
            String toolName = getUnifiedToolName(tool.getToolInfo());
            log.warn("Tool marked as unavailable: name={}, endpoint={}",
                    toolName, endpoint);
        }
    }

    /**
     * 获取工具数量
     */
    public int size() {
        return toolsByName.size();
    }

    /**
     * 清空注册表
     */
    public synchronized void clear() {
        toolsByEndpoint.clear();
        toolsByName.clear();
        log.warn("External tool registry cleared");
    }
}
