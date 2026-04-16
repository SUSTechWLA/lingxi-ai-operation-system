package com.lingxi.ai.orchestrator.entity;

import jakarta.persistence.*;
import lombok.Data;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;

import java.time.LocalDateTime;
import java.util.Map;

@Data
@Entity
@Table(name = "ai_task")
public class Task {

    @Id
    @Column(length = 64)
    private String id;

    @Column(name = "user_id", length = 64)
    private String userId;

    @Enumerated(EnumType.STRING)
    @Column(length = 20)
    private TaskStatus status;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(columnDefinition = "jsonb")
    private Map<String, Object> input;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(columnDefinition = "jsonb")
    private Map<String, Object> output;

    @Column(name = "created_at")
    private LocalDateTime createdAt;

    @PrePersist
    protected void onCreate() {
        createdAt = LocalDateTime.now();
        if (status == null) {
            status = TaskStatus.CREATED;
        }
    }
}
