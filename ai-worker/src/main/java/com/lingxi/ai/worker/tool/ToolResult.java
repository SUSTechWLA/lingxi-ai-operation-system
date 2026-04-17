package com.lingxi.ai.worker.tool;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.time.Instant;
import java.util.Map;

/**
 * 工具执行结果
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ToolResult {

    /**
     * 是否执行成功
     */
    private boolean success;

    /**
     * 结果数据
     */
    private Map<String, Object> data;

    /**
     * 错误信息（失败时）
     */
    private String errorMessage;

    /**
     * 执行开始时间
     */
    private Instant startTime;

    /**
     * 执行结束时间
     */
    private Instant endTime;

    /**
     * 执行耗时（毫秒）
     */
    private long durationMs;

    /**
     * 创建成功结果
     */
    public static ToolResult success(Map<String, Object> data, Instant startTime, Instant endTime) {
        return ToolResult.builder()
                .success(true)
                .data(data)
                .startTime(startTime)
                .endTime(endTime)
                .durationMs(endTime.toEpochMilli() - startTime.toEpochMilli())
                .build();
    }

    /**
     * 创建失败结果
     */
    public static ToolResult failure(String errorMessage, Instant startTime, Instant endTime) {
        return ToolResult.builder()
                .success(false)
                .errorMessage(errorMessage)
                .startTime(startTime)
                .endTime(endTime)
                .durationMs(endTime.toEpochMilli() - startTime.toEpochMilli())
                .build();
    }
}
