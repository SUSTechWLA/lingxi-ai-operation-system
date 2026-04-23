package com.lingxi.ai.orchestrator.service;

import com.lingxi.ai.orchestrator.context.ContextService;
import com.lingxi.ai.orchestrator.entity.Node;
import com.lingxi.ai.orchestrator.entity.NodeDependency;
import com.lingxi.ai.orchestrator.entity.NodeStatus;
import com.lingxi.ai.orchestrator.entity.NodeType;
import com.lingxi.ai.orchestrator.entity.Task;
import com.lingxi.ai.orchestrator.entity.TaskStatus;
import com.lingxi.ai.orchestrator.model.DAGRequest;
import com.lingxi.ai.orchestrator.repository.NodeDependencyRepository;
import com.lingxi.ai.orchestrator.repository.NodeRepository;
import com.lingxi.ai.orchestrator.repository.TaskRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;

@Slf4j
@Service
@RequiredArgsConstructor
public class OrchestratorService {

    private final TaskRepository taskRepository;
    private final NodeRepository nodeRepository;
    private final NodeDependencyRepository nodeDependencyRepository;
    private final DAGValidator dagValidator;
    private final ContextService contextService;
    private final StateService stateService;

    @Transactional
    public Task createTask(Map<String, Object> input) {
        String taskId = UUID.randomUUID().toString();

        Task task = new Task();
        task.setId(taskId);
        task.setInput(input);
        task.setStatus(TaskStatus.CREATED);
        task = taskRepository.save(task);

        contextService.recordTaskCreated(task);
        log.info("Created task: {}", taskId);

        return task;
    }

    @Transactional
    public void submitDAG(String taskId, DAGRequest dagRequest) {
        Task task = taskRepository.findById(taskId)
                .orElseThrow(() -> new IllegalArgumentException("Task not found: " + taskId));

        log.info("Submitting DAG for task: {}", taskId);

        dagValidator.validate(dagRequest);
        contextService.recordDagValidated(taskId);

        for (DAGRequest.NodeRequest nodeReq : dagRequest.getNodes()) {
            Node node = new Node();
            node.setId(nodeReq.getId());
            node.setTaskId(taskId);
            node.setType(NodeType.valueOf(nodeReq.getType()));
            node.setName(nodeReq.getName());
            node.setStatus(NodeStatus.CREATED);
            node.setInput(nodeReq.getInput());
            node.setIdempotencyKey(taskId + "-" + nodeReq.getId());
            if (nodeReq.getMaxRetry() != null) {
                node.setMaxRetry(nodeReq.getMaxRetry());
            }
            if (nodeReq.getPriority() != null) {
                node.setPriority(nodeReq.getPriority());
            }
            if (nodeReq.getWorkerGroup() != null) {
                node.setWorkerGroup(nodeReq.getWorkerGroup());
            }
            nodeRepository.save(node);
        }

        if (dagRequest.getEdges() != null) {
            for (DAGRequest.Edge edge : dagRequest.getEdges()) {
                NodeDependency dep = new NodeDependency();
                dep.setParentNodeId(edge.getFrom());
                dep.setChildNodeId(edge.getTo());
                nodeDependencyRepository.save(dep);
            }
        }

        stateService.transitionTask(taskId, TaskStatus.RUNNING);
        contextService.recordDagSubmitted(taskId);

        // Use StateService to initialize ready nodes (dependency-driven)
        initializeReadyNodes(taskId, dagRequest);

        log.info("DAG submitted for task: {}", taskId);
    }

    public Task getTask(String taskId) {
        return taskRepository.findById(taskId).orElse(null);
    }

    public List<Node> getNodesForTask(String taskId) {
        return nodeRepository.findByTaskId(taskId);
    }

    public Map<String, Object> getTaskWithDetails(String taskId) {
        Task task = getTask(taskId);
        if (task == null) {
            return null;
        }

        List<Node> nodes = getNodesForTask(taskId);

        Map<String, Object> result = new HashMap<>();
        result.put("taskId", task.getId());
        result.put("status", task.getStatus());
        result.put("input", task.getInput());
        result.put("output", task.getOutput());
        result.put("createdAt", task.getCreatedAt());
        result.put("nodes", nodes);

        return result;
    }

    private void initializeReadyNodes(String taskId, DAGRequest dagRequest) {
        List<String> allNodeIds = dagRequest.getNodes().stream()
                .map(DAGRequest.NodeRequest::getId)
                .toList();

        java.util.Set<String> nodesWithDependencies = new java.util.HashSet<>();
        if (dagRequest.getEdges() != null) {
            for (DAGRequest.Edge edge : dagRequest.getEdges()) {
                nodesWithDependencies.add(edge.getTo());
            }
        }

        for (String nodeId : allNodeIds) {
            if (!nodesWithDependencies.contains(nodeId)) {
                Node node = nodeRepository.findById(nodeId).orElse(null);
                if (node != null && node.getStatus() == NodeStatus.CREATED) {
                    stateService.initializeNodeReady(node);
                }
            }
        }
    }
}
