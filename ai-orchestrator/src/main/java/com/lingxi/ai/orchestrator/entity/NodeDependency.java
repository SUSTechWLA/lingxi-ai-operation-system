package com.lingxi.ai.orchestrator.entity;

import jakarta.persistence.*;
import lombok.Data;

import java.io.Serializable;

@Data
@Entity
@Table(name = "ai_node_dependency")
@IdClass(NodeDependency.NodeDependencyId.class)
public class NodeDependency {

    @Id
    @Column(name = "parent_node_id", length = 64)
    private String parentNodeId;

    @Id
    @Column(name = "child_node_id", length = 64)
    private String childNodeId;

    @Data
    public static class NodeDependencyId implements Serializable {
        private String parentNodeId;
        private String childNodeId;
    }
}
