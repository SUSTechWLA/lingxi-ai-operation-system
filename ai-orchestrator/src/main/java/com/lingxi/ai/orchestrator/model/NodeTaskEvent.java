package com.lingxi.ai.orchestrator.model;

import java.util.Map;

public class NodeTaskEvent {
    private String taskId;
    private String nodeId;
    private String type; // LLM / TOOL
    private Map<String, Object> payload;
    private String traceId;

    public NodeTaskEvent() {
    }

    public NodeTaskEvent(String taskId, String nodeId, String type, Map<String, Object> payload, String traceId) {
        this.taskId = taskId;
        this.nodeId = nodeId;
        this.type = type;
        this.payload = payload;
        this.traceId = traceId;
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

    public String getType() {
        return type;
    }

    public void setType(String type) {
        this.type = type;
    }

    public Map<String, Object> getPayload() {
        return payload;
    }

    public void setPayload(Map<String, Object> payload) {
        this.payload = payload;
    }

    public String getTraceId() {
        return traceId;
    }

    public void setTraceId(String traceId) {
        this.traceId = traceId;
    }
}