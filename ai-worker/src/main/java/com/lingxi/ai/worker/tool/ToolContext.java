package com.lingxi.ai.worker.tool;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * 工具执行上下文
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ToolContext {

    /**
     * 任务ID
     */
    private String taskId;

    /**
     * 节点ID
     */
    private String nodeId;

    /**
     * 尝试次数
     */
    private int retryCount;

    /**
     * 快照数据（从快照恢复时）
     */
    private String snapshotData;
}
