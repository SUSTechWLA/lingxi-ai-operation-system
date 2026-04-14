package com.lingxi.ai.orchestrator.service;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;
import java.util.List;
import com.lingxi.ai.orchestrator.model.Task;
import com.lingxi.ai.orchestrator.model.Node;
import com.lingxi.ai.orchestrator.model.NodeTaskEvent;
import com.lingxi.ai.orchestrator.model.NodeResultEvent;
import com.lingxi.ai.orchestrator.model.TaskStatus;
import com.lingxi.ai.orchestrator.event.EventProducer;
import com.lingxi.ai.orchestrator.repository.RedisTaskRepository;

@Service
public class StateMachineService {
    @Autowired
    private EventProducer eventProducer;
    @Autowired
    private RedisTaskRepository taskRepository;

    // 处理节点执行结果
    public void handleNodeResult(NodeResultEvent event) {
        String taskId = event.getTaskId();
        String nodeId = event.getNodeId();

        // 1. 获取任务和节点
        Task task = taskRepository.getTask(taskId);
        Node node = task.getDag().getNodes().stream()
                .filter(n -> n.getNodeId().equals(nodeId))
                .findFirst()
                .orElseThrow(() -> new RuntimeException("Node not found: " + nodeId));

        // 2. 更新节点状态
        node.setStatus(event.getStatus());
        node.setOutput(event.getOutput());
        node.setErrorMessage(event.getErrorMessage());
        taskRepository.saveNode(taskId, node);

        // 3. 检查任务状态
        if (task.getDag().isFailed()) {
            // 任务失败
            task.setStatus(TaskStatus.FAILED);
            task.setEndTime(System.currentTimeMillis());
            taskRepository.saveTask(task);
            eventProducer.sendTaskFailedEvent(task);
            return;
        }

        if (task.getDag().isCompleted()) {
            // 任务成功
            task.setStatus(TaskStatus.SUCCESS);
            task.setEndTime(System.currentTimeMillis());
            taskRepository.saveTask(task);
            eventProducer.sendTaskCompletedEvent(task);
            return;
        }

        // 4. 调度下一批就绪节点
        scheduleReadyNodes(task);
    }

    // 调度就绪节点
    public void scheduleReadyNodes(Task task) {
        List<Node> readyNodes = task.getDag().getReadyNodes();
        for (Node node : readyNodes) {
            // 更新节点状态为RUNNING
            node.setStatus(com.lingxi.ai.orchestrator.model.NodeStatus.RUNNING);
            taskRepository.saveNode(task.getTaskId(), node);

            // 发布节点就绪事件
            NodeTaskEvent event = new NodeTaskEvent(
                    task.getTaskId(),
                    node.getNodeId(),
                    node.getType(),
                    node.getInput(),
                    task.getTraceId()
            );
            eventProducer.sendNodeReadyEvent(event);
        }
    }
}