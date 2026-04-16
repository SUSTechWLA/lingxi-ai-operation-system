package com.lingxi.ai.orchestrator.entity;

import jakarta.persistence.*;
import lombok.Data;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;

import java.time.LocalDateTime;
import java.util.Map;

@Data
@Entity
@Table(name = "ai_node", indexes = {
    @Index(name = "idx_task", columnList = "task_id"),
    @Index(name = "idx_status", columnList = "status")
})
public class Node {

    @Id
    @Column(length = 64)
    private String id;

    @Column(name = "task_id", length = 64)
    private String taskId;

    @Enumerated(EnumType.STRING)
    @Column(length = 20)
    private NodeType type;

    @Column(length = 100)
    private String name;

    @Enumerated(EnumType.STRING)
    @Column(length = 20)
    private NodeStatus status;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(columnDefinition = "jsonb")
    private Map<String, Object> input;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(columnDefinition = "jsonb")
    private Map<String, Object> output;

    @Column(name = "retry_count")
    private Integer retryCount;

    @Column(name = "max_retry")
    private Integer maxRetry;

    private Integer priority;

    @Column(name = "worker_group", length = 50)
    private String workerGroup;

    @Version
    private Integer version;

    @Column(name = "error_message", columnDefinition = "TEXT")
    private String errorMessage;

    @Column(name = "created_at")
    private LocalDateTime createdAt;

    @PrePersist
    protected void onCreate() {
        createdAt = LocalDateTime.now();
        if (status == null) {
            status = NodeStatus.CREATED;
        }
        if (retryCount == null) {
            retryCount = 0;
        }
        if (maxRetry == null) {
            maxRetry = 3;
        }
        if (priority == null) {
            priority = 5;
        }
        if (workerGroup == null) {
            workerGroup = "default";
        }
        if (version == null) {
            version = 0;
        }
    }
}
