package com.lingxi.ai.context.entity;

public enum ContextType {
    TASK_CREATED,
    TASK_SUCCESS,
    TASK_FAILED,
    NODE_SCHEDULED,
    NODE_READY,
    NODE_SUCCESS,
    NODE_FAILED,
    NODE_RETRY,
    NODE_SNAPSHOT,
    DAG_SUBMITTED,
    DAG_VALIDATED
}
