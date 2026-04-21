package com.lingxi.ai.orchestrator.model;

import lombok.Data;
import java.util.List;
import java.util.Arrays;
import java.util.Collections;
import java.util.stream.Collectors;
import com.fasterxml.jackson.annotation.JsonIgnore;

@Data
public class DAG {
    private List<Node> nodes;

    public static DAG sample() {
        DAG dag = new DAG();
        Node node1 = new Node();
        node1.setNodeId("1");
        node1.setType("LLM");
        node1.setName("write_article");
        node1.setDeps(Collections.emptyList());
        node1.setStatus(NodeStatus.CREATED);

        Node node2 = new Node();
        node2.setNodeId("2");
        node2.setType("LLM");
        node2.setName("summarize");
        node2.setDeps(Arrays.asList("1"));
        node2.setStatus(NodeStatus.CREATED);

        dag.setNodes(Arrays.asList(node1, node2));
        return dag;
    }

    @JsonIgnore
    public List<Node> getReadyNodes() {
        return nodes.stream()
                .filter(node -> node.getStatus() == NodeStatus.CREATED || node.getStatus() == NodeStatus.READY)
                .filter(node -> node.getDeps().stream()
                        .allMatch(depId -> nodes.stream()
                                .anyMatch(n -> n.getNodeId().equals(depId) && n.getStatus() == NodeStatus.SUCCESS)))
                .collect(Collectors.toList());
    }

    @JsonIgnore
    public boolean isCompleted() {
        return nodes.stream().allMatch(node -> node.getStatus() == NodeStatus.SUCCESS);
    }

    @JsonIgnore
    public boolean isFailed() {
        return nodes.stream().anyMatch(node -> node.getStatus() == NodeStatus.FAILED);
    }
}
