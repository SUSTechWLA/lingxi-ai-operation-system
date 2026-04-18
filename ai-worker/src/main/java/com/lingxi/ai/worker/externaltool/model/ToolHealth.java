package com.lingxi.ai.worker.externaltool.model;

import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * 工具健康状态
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ToolHealth {

    @JsonProperty("status")
    private String status;

    @JsonProperty("version")
    private String version;

    @JsonProperty("timestamp")
    private Long timestamp;

    public boolean isUp() {
        return "UP".equalsIgnoreCase(status);
    }
}
