package com.lingxi.ai.orchestrator.model;

import lombok.Data;

@Data
public class Task {
    private String taskId;
    private String prompt;
    private TaskStatus status;
    private DAG dag;
    private String traceId;
    private long createTime;
    private long startTime;
    private long endTime;
}
