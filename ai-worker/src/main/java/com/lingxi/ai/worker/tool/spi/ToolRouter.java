package com.lingxi.ai.worker.tool.spi;

import com.lingxi.ai.worker.tool.Tool;
import com.lingxi.ai.worker.tool.ToolContext;
import com.lingxi.ai.worker.tool.ToolResult;
import jakarta.annotation.PostConstruct;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.*;
import java.util.concurrent.ConcurrentHashMap;
import java.util.stream.Collectors;

/**
 * 统一工具路由器
 * 根据工具名称自动选择最优的ToolProvider
 */
@Slf4j
@Component
@RequiredArgsConstructor
public class ToolRouter {

    private final List<ToolProvider> toolProviders;

    // 按优先级排序的Provider列表
    private List<ToolProvider> sortedProviders;

    // 工具名到Provider的缓存
    private final Map<String, ToolProvider> toolProviderCache = new ConcurrentHashMap<>();

    @PostConstruct
    public void init() {
        // 按优先级排序（数字越小优先级越高）
        sortedProviders = toolProviders.stream()
                .sorted(Comparator.comparingInt(ToolProvider::getPriority))
                .toList();

        log.info("ToolRouter initialized with {} providers: {}",
                sortedProviders.size(),
                sortedProviders.stream()
                        .map(p -> p.getName() + "(" + p.getType() + ")")
                        .collect(Collectors.joining(", ")));
    }

    /**
     * 获取工具
     */
    public Optional<Tool> getTool(String toolName) {
        ToolProvider provider = findProvider(toolName);
        if (provider != null) {
            return Optional.ofNullable(provider.getTool(toolName));
        }
        return Optional.empty();
    }

    /**
     * 执行工具（通过最优Provider）
     */
    public ToolResult execute(String toolName, Map<String, Object> parameters, ToolContext context) {
        ToolProvider provider = findProvider(toolName);
        if (provider == null) {
            return ToolResult.failure(
                    "Tool not found: " + toolName,
                    Instant.now(),
                    Instant.now()
            );
        }

        log.debug("Executing tool '{}' via provider '{}' ({})",
                toolName, provider.getName(), provider.getType());

        return provider.execute(toolName, parameters, context);
    }

    /**
     * 检查工具是否存在
     */
    public boolean hasTool(String toolName) {
        return findProvider(toolName) != null;
    }

    /**
     * 获取所有可用工具
     */
    public Map<String, Tool> getAllTools() {
        Map<String, Tool> allTools = new HashMap<>();
        // 按优先级遍历，高优先级的工具会覆盖低优先级的同名工具
        for (ToolProvider provider : sortedProviders) {
            allTools.putAll(provider.listTools());
        }
        return allTools;
    }

    /**
     * 获取工具的Provider类型
     */
    public Optional<ToolProvider.ProviderType> getToolProviderType(String toolName) {
        ToolProvider provider = findProvider(toolName);
        return provider != null ? Optional.of(provider.getType()) : Optional.empty();
    }

    /**
     * 清除缓存（当工具注册/注销时调用）
     */
    public void invalidateCache(String toolName) {
        toolProviderCache.remove(toolName);
    }

    /**
     * 清除所有缓存
     */
    public void invalidateAllCache() {
        toolProviderCache.clear();
    }

    /**
     * 查找处理指定工具的Provider
     */
    private ToolProvider findProvider(String toolName) {
        // 先查缓存
        ToolProvider cached = toolProviderCache.get(toolName);
        if (cached != null && cached.supports(toolName)) {
            return cached;
        }

        // 按优先级遍历查找
        for (ToolProvider provider : sortedProviders) {
            if (provider.supports(toolName)) {
                toolProviderCache.put(toolName, provider);
                log.debug("Tool '{}' mapped to provider '{}' ({})",
                        toolName, provider.getName(), provider.getType());
                return provider;
            }
        }

        return null;
    }

    /**
     * 获取所有Provider信息（用于监控/调试）
     */
    public List<Map<String, Object>> getProviderInfo() {
        return sortedProviders.stream()
                .map(p -> Map.<String, Object>of(
                        "name", p.getName(),
                        "type", p.getType().name(),
                        "priority", p.getPriority(),
                        "toolCount", p.listTools().size()
                ))
                .toList();
    }
}
