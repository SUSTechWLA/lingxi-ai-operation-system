package com.lingxi.ai.worker.externaltool.model;

import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * 工具标准响应格式
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ToolResponse<T> {

    /**
     * 状态码
     */
    @JsonProperty("code")
    private Integer code;

    /**
     * 响应消息
     */
    @JsonProperty("message")
    private String message;

    /**
     * 响应数据
     */
    @JsonProperty("data")
    private T data;

    /**
     * 错误信息
     */
    @JsonProperty("error")
    private String error;

    /**
     * 判断是否成功
     */
    public boolean isSuccess() {
        return code != null && code == 200;
    }
}
