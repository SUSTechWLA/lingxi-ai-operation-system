package com.lingxi.ai.translator.model;

import lombok.Data;
import java.util.List;
import java.util.Map;

@Data
public class Node {
    private String nodeId;
    private String type;
    private String name;
    private List<String> deps;
    private NodeStatus status;
    private Map<String, Object> input;
    private Map<String, Object> output;
    private String errorMessage;
    private Integer retryCount;
    private Integer maxRetry;
    private Integer priority;
    private String workerGroup;

    @Deprecated
    public String getTask() {
        return name;
    }

    @Deprecated
    public void setTask(String task) {
        this.name = task;
    }
}
