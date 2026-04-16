package com.lingxi.ai.orchestrator.service;

import com.lingxi.ai.orchestrator.model.DAGRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Service;

import java.util.*;

@Service
public class DAGValidator {

    private static final Logger logger = LoggerFactory.getLogger(DAGValidator.class);

    public void validate(DAGRequest dag) {
        if (dag == null || dag.getNodes() == null || dag.getNodes().isEmpty()) {
            throw new IllegalArgumentException("DAG must contain at least one node");
        }

        Set<String> nodeIds = new HashSet<>();
        for (DAGRequest.NodeRequest node : dag.getNodes()) {
            if (node.getId() == null || node.getId().isEmpty()) {
                throw new IllegalArgumentException("Node id cannot be empty");
            }
            if (!nodeIds.add(node.getId())) {
                throw new IllegalArgumentException("Duplicate node id: " + node.getId());
            }
        }

        if (dag.getEdges() != null) {
            for (DAGRequest.Edge edge : dag.getEdges()) {
                if (!nodeIds.contains(edge.getFrom())) {
                    throw new IllegalArgumentException("Parent node not found: " + edge.getFrom());
                }
                if (!nodeIds.contains(edge.getTo())) {
                    throw new IllegalArgumentException("Child node not found: " + edge.getTo());
                }
            }

            detectCycle(dag, nodeIds);
            detectIsolatedNodes(dag, nodeIds);
        }

        logger.info("DAG validation passed");
    }

    private void detectCycle(DAGRequest dag, Set<String> nodeIds) {
        Map<String, List<String>> adjacencyList = new HashMap<>();
        for (String nodeId : nodeIds) {
            adjacencyList.put(nodeId, new ArrayList<>());
        }
        if (dag.getEdges() != null) {
            for (DAGRequest.Edge edge : dag.getEdges()) {
                adjacencyList.get(edge.getFrom()).add(edge.getTo());
            }
        }

        Set<String> visited = new HashSet<>();
        Set<String> recursionStack = new HashSet<>();

        for (String nodeId : nodeIds) {
            if (hasCycle(nodeId, adjacencyList, visited, recursionStack)) {
                throw new IllegalArgumentException("Cycle detected in DAG");
            }
        }
    }

    private boolean hasCycle(String nodeId, Map<String, List<String>> adjacencyList,
                              Set<String> visited, Set<String> recursionStack) {
        if (recursionStack.contains(nodeId)) {
            return true;
        }
        if (visited.contains(nodeId)) {
            return false;
        }

        visited.add(nodeId);
        recursionStack.add(nodeId);

        for (String child : adjacencyList.getOrDefault(nodeId, Collections.emptyList())) {
            if (hasCycle(child, adjacencyList, visited, recursionStack)) {
                return true;
            }
        }

        recursionStack.remove(nodeId);
        return false;
    }

    private void detectIsolatedNodes(DAGRequest dag, Set<String> nodeIds) {
        if (dag.getEdges() == null || dag.getEdges().isEmpty()) {
            if (nodeIds.size() > 1) {
                throw new IllegalArgumentException("DAG with multiple nodes must have at least one edge");
            }
            return;
        }

        Set<String> hasIncoming = new HashSet<>();
        Set<String> hasOutgoing = new HashSet<>();

        for (DAGRequest.Edge edge : dag.getEdges()) {
            hasOutgoing.add(edge.getFrom());
            hasIncoming.add(edge.getTo());
        }

        Set<String> startNodes = new HashSet<>();
        for (String nodeId : nodeIds) {
            if (!hasIncoming.contains(nodeId)) {
                startNodes.add(nodeId);
            }
        }

        if (startNodes.isEmpty()) {
            throw new IllegalArgumentException("DAG must have at least one start node (no incoming edges)");
        }

        logger.info("Found {} start nodes: {}", startNodes.size(), startNodes);
    }
}
