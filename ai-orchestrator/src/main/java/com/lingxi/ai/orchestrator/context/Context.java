package com.lingxi.ai.orchestrator.context;

import jakarta.persistence.*;
import lombok.Data;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;

import java.time.LocalDateTime;
import java.util.Map;

@Data
@Entity
@Table(name = "ai_context")
public class Context {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Enumerated(EnumType.STRING)
    @Column(name = "context_type", length = 50)
    private ContextType contextType;

    @Column(name = "task_id", length = 64)
    private String taskId;

    @Column(name = "node_id", length = 64)
    private String nodeId;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(columnDefinition = "jsonb")
    private Map<String, Object> metadata;

    @Column(length = 1000)
    private String message;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "snapshot_data", columnDefinition = "jsonb")
    private Map<String, Object> snapshotData;

    @Column(name = "created_at")
    private LocalDateTime createdAt;

    @PrePersist
    protected void onCreate() {
        createdAt = LocalDateTime.now();
    }

    public static Context create(ContextType type, String taskId, String message) {
        Context ctx = new Context();
        ctx.setContextType(type);
        ctx.setTaskId(taskId);
        ctx.setMessage(message);
        return ctx;
    }

    public static Context create(ContextType type, String taskId, String nodeId, String message) {
        Context ctx = create(type, taskId, message);
        ctx.setNodeId(nodeId);
        return ctx;
    }

    public static Context createSnapshot(String taskId, String nodeId, Map<String, Object> snapshotData, String message) {
        Context ctx = create(ContextType.NODE_SNAPSHOT, taskId, nodeId, message);
        ctx.setSnapshotData(snapshotData);
        return ctx;
    }
}
