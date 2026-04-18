package com.lingxi.ai.worker.externaltool.model;

import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.List;
import java.util.Map;

/**
 * 外部工具完整元数据模型
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class ToolInfo {

    /**
     * 工具唯一标识（全局唯一）
     */
    @JsonProperty("toolId")
    private String toolId;

    /**
     * 工具名称（语义化）
     */
    @JsonProperty("toolName")
    private String toolName;

    /**
     * 语义化版本
     */
    @JsonProperty("toolVersion")
    private String toolVersion;

    /**
     * 详细功能描述（用于LLM理解和向量生成）
     */
    @JsonProperty("description")
    private String description;

    /**
     * 工具类型：LLM/API/SCRIPT/DL/CORE
     */
    @JsonProperty("type")
    private String type;

    /**
     * 作者
     */
    @JsonProperty("author")
    private String author;

    /**
     * 标签（用于分类和检索）
     */
    @JsonProperty("tags")
    private List<String> tags;

    /**
     * 输入参数JSON Schema
     */
    @JsonProperty("inputSchema")
    private Map<String, Object> inputSchema;

    /**
     * 输出结果JSON Schema
     */
    @JsonProperty("outputSchema")
    private Map<String, Object> outputSchema;

    /**
     * 输入输出示例（用于LLM学习和向量生成）
     */
    @JsonProperty("examples")
    private List<ToolExample> examples;

    /**
     * 执行超时时间（毫秒）
     */
    @JsonProperty("timeout")
    private Integer timeout;

    /**
     * 最大重试次数
     */
    @JsonProperty("maxRetry")
    private Integer maxRetry;
}
