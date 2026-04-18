package com.lingxi.ai.worker.externaltool.model;

import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.Map;

/**
 * 工具输入输出示例
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ToolExample {

    @JsonProperty("input")
    private Map<String, Object> input;

    @JsonProperty("output")
    private Map<String, Object> output;
}
