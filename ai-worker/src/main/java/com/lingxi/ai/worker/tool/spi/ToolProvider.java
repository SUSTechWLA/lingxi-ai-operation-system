package com.lingxi.ai.worker.tool.spi;

import com.lingxi.ai.worker.tool.Tool;
import com.lingxi.ai.worker.tool.ToolContext;
import com.lingxi.ai.worker.tool.ToolResult;

import java.util.Map;

/**
 * 工具提供器SPI接口
 * 支持多种工具实现方式：内置、进程内、外部HTTP等
 */
public interface ToolProvider {

    /**
     * 获取提供器名称
     */
    String getName();

    /**
     * 获取提供器类型
     */
    ProviderType getType();

    /**
     * 获取优先级（数字越小优先级越高）
     */
    default int getPriority() {
        return 100;
    }

    /**
     * 检查是否支持指定工具
     */
    boolean supports(String toolName);

    /**
     * 获取工具实例
     */
    Tool getTool(String toolName);

    /**
     * 执行工具（可选的优化路径，直接执行不经过Tool接口）
     */
    default ToolResult execute(String toolName, Map<String, Object> parameters, ToolContext context) {
        Tool tool = getTool(toolName);
        return tool.execute(parameters, context);
    }

    /**
     * 列出所有可用工具
     */
    Map<String, Tool> listTools();

    /**
     * 提供器类型
     */
    enum ProviderType {
        /**
         * 内置工具（同JVM，直接调用）
         */
        BUILTIN,
        /**
         * 进程内动态加载（如Groovy脚本、JS脚本）
         */
        IN_PROCESS,
        /**
         * 本地进程（通过UNIX Socket/共享内存通信）
         */
        LOCAL_PROCESS,
        /**
         * 外部HTTP服务
         */
        HTTP,
        /**
         * gRPC服务
         */
        GRPC
    }
}
