package com.lingxi.ai.worker.model;

import java.util.Map;

public class NodeResultEvent {
    private String taskId;
    private String nodeId;
    private NodeStatus status;
    private Map<String, Object> output;
    private String traceId;
    private String errorMessage;

    public NodeResultEvent() {
    }

    public NodeResultEvent(String taskId, String nodeId, NodeStatus status, Map<String, Object> output, String traceId, String errorMessage) {
        this.taskId = taskId;
        this.nodeId = nodeId;
        this.status = status;
        this.output = output;
        this.traceId = traceId;
        this.errorMessage = errorMessage;
    }

    public String getTaskId() {
        return taskId;
    }

    public void setTaskId(String taskId) {
        this.taskId = taskId;
    }

    public String getNodeId() {
        return nodeId;
    }

    public void setNodeId(String nodeId) {
        this.nodeId = nodeId;
    }

    public NodeStatus getStatus() {
        return status;
    }

    public void setStatus(NodeStatus status) {
        this.status = status;
    }

    public Map<String, Object> getOutput() {
        return output;
    }

    public void setOutput(Map<String, Object> output) {
        this.output = output;
    }

    public String getTraceId() {
        return traceId;
    }

    public void setTraceId(String traceId) {
        this.traceId = traceId;
    }

    public String getErrorMessage() {
        return errorMessage;
    }

    public void setErrorMessage(String errorMessage) {
        this.errorMessage = errorMessage;
    }
}
