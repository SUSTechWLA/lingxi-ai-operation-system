package com.lingxi.ai.worker.util;

import org.slf4j.MDC;

import java.util.Map;
import java.util.UUID;

/**
 * 全链路追踪上下文工具
 */
public class TraceContext {

    private static final String TRACE_ID_KEY = "traceId";
    private static final String TASK_ID_KEY = "taskId";
    private static final String NODE_ID_KEY = "nodeId";

    public static String generateTraceId() {
        return UUID.randomUUID().toString().replace("-", "");
    }

    public static void setTraceId(String traceId) {
        if (traceId != null) {
            MDC.put(TRACE_ID_KEY, traceId);
        }
    }

    public static String getTraceId() {
        return MDC.get(TRACE_ID_KEY);
    }

    public static void setTaskId(String taskId) {
        if (taskId != null) {
            MDC.put(TASK_ID_KEY, taskId);
        }
    }

    public static String getTaskId() {
        return MDC.get(TASK_ID_KEY);
    }

    public static void setNodeId(String nodeId) {
        if (nodeId != null) {
            MDC.put(NODE_ID_KEY, nodeId);
        }
    }

    public static String getNodeId() {
        return MDC.get(NODE_ID_KEY);
    }

    public static void setContext(String traceId, String taskId, String nodeId) {
        if (traceId != null) {
            MDC.put(TRACE_ID_KEY, traceId);
        }
        if (taskId != null) {
            MDC.put(TASK_ID_KEY, taskId);
        }
        if (nodeId != null) {
            MDC.put(NODE_ID_KEY, nodeId);
        }
    }

    public static void clear() {
        MDC.remove(TRACE_ID_KEY);
        MDC.remove(TASK_ID_KEY);
        MDC.remove(NODE_ID_KEY);
    }

    public static Map<String, String> getCopyOfContextMap() {
        return MDC.getCopyOfContextMap();
    }

    public static void setContextMap(Map<String, String> contextMap) {
        if (contextMap != null) {
            MDC.setContextMap(contextMap);
        }
    }
}
