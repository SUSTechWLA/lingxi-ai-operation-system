package com.lingxi.ai.worker.externaltool.model;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.time.Instant;

/**
 * 已注册的外部工具信息
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class RegisteredTool {

    /**
     * 工具元数据
     */
    private ToolInfo toolInfo;

    /**
     * 工具服务地址
     */
    private String toolEndpoint;

    /**
     * Worker ID
     */
    private String workerId;

    /**
     * Worker Group
     */
    private String workerGroup;

    /**
     * 注册时间
     */
    private Instant registeredAt;

    /**
     * 最后心跳时间
     */
    private Instant lastHeartbeatAt;

    /**
     * 工具状态
     */
    private ToolStatus status;

    public enum ToolStatus {
        AVAILABLE,
        UNAVAILABLE,
        OFFLINE
    }
}
