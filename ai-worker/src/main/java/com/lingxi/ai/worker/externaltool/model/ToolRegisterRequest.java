package com.lingxi.ai.worker.externaltool.model;

import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * 工具注册请求
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ToolRegisterRequest {

    /**
     * 工具服务地址，如 http://localhost:8090
     */
    @JsonProperty("endpoint")
    private String endpoint;

    /**
     * Worker Group（可选）
     */
    @JsonProperty("workerGroup")
    private String workerGroup;
}
