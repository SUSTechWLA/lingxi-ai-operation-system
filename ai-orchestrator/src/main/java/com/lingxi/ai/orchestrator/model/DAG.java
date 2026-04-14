package com.lingxi.ai.orchestrator.model;

import java.util.List;
import java.util.Arrays;
import java.util.Collections;
import java.util.stream.Collectors;
import com.fasterxml.jackson.annotation.JsonIgnore;

public class DAG {
    private List<Node> nodes;

    // 生成示例DAG（2个节点，依赖关系1→2）
    public static DAG sample() {
        DAG dag = new DAG();
        Node node1 = new Node();
        node1.setNodeId("1");
        node1.setType("LLM");
        node1.setTask("write_article");
        node1.setDeps(Collections.emptyList());
        node1.setStatus(NodeStatus.PENDING);

        Node node2 = new Node();
        node2.setNodeId("2");
        node2.setType("LLM");
        node2.setTask("summarize");
        node2.setDeps(Arrays.asList("1"));
        node2.setStatus(NodeStatus.PENDING);

        dag.setNodes(Arrays.asList(node1, node2));
        return dag;
    }

    // 获取所有可执行节点（依赖全部完成）
    @JsonIgnore
    public List<Node> getReadyNodes() {
        return nodes.stream()
                .filter(node -> node.getStatus() == NodeStatus.PENDING)
                .filter(node -> node.getDeps().stream()
                        .allMatch(depId -> nodes.stream()
                                .anyMatch(n -> n.getNodeId().equals(depId) && n.getStatus() == NodeStatus.SUCCESS)))
                .collect(Collectors.toList());
    }

    // 检查DAG是否完成
    @JsonIgnore
    public boolean isCompleted() {
        return nodes.stream().allMatch(node -> node.getStatus() == NodeStatus.SUCCESS);
    }

    // 检查DAG是否失败
    @JsonIgnore
    public boolean isFailed() {
        return nodes.stream().anyMatch(node -> node.getStatus() == NodeStatus.FAILED);
    }

    public List<Node> getNodes() {
        return nodes;
    }

    public void setNodes(List<Node> nodes) {
        this.nodes = nodes;
    }
}