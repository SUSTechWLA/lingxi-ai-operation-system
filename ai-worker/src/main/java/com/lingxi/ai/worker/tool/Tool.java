package com.lingxi.ai.worker.tool;

import java.util.Map;

/**
 * 统一工具接口
 * 所有AI-Worker工具必须实现此接口
 */
public interface Tool {

    /**
     * 获取工具名称
     * @return 工具唯一标识名称
     */
    String getName();

    /**
     * 获取工具描述
     * @return 工具描述信息
     */
    String getDescription();

    /**
     * 获取工具类型
     * @return 工具类型
     */
    ToolType getType();

    /**
     * 执行工具
     * @param parameters 工具参数
     * @param context 执行上下文
     * @return 执行结果
     */
    ToolResult execute(Map<String, Object> parameters, ToolContext context);

    /**
     * 验证参数
     * @param parameters 待验证的参数
     * @return 是否验证通过
     */
    boolean validateParameters(Map<String, Object> parameters);
}
