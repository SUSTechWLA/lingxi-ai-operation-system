package com.lingxi.ai.orchestrator.service;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;
import java.util.List;
import java.util.UUID;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import com.lingxi.ai.orchestrator.model.Task;
import com.lingxi.ai.orchestrator.model.Node;
import com.lingxi.ai.orchestrator.model.DAG;
import com.lingxi.ai.orchestrator.model.NodeTaskEvent;
import com.lingxi.ai.orchestrator.model.TaskStatus;
import com.lingxi.ai.orchestrator.event.EventProducer;
import com.lingxi.ai.orchestrator.repository.RedisTaskRepository;

@Service
public class OrchestratorService {
    private static final Logger logger = LoggerFactory.getLogger(OrchestratorService.class);

    @Autowired
    private EventProducer eventProducer;
    @Autowired
    private RedisTaskRepository taskRepository;

    // 创建任务（直接接收 DAG）
    public Task createTask(DAG dag) {
        // 1. 生成唯一ID
        String taskId = UUID.randomUUID().toString();
        String traceId = UUID.randomUUID().toString();

        // 2. 初始化所有节点状态为 PENDING
        for (Node node : dag.getNodes()) {
            if (node.getStatus() == null) {
                node.setStatus(com.lingxi.ai.orchestrator.model.NodeStatus.PENDING);
            }
        }

        // 3. 创建任务
        Task task = new Task();
        task.setTaskId(taskId);
        task.setPrompt("");  // prompt 由 nl-translator 处理，这里留空
        task.setStatus(TaskStatus.CREATED);
        task.setDag(dag);
        task.setTraceId(traceId);
        task.setCreateTime(System.currentTimeMillis());

        // 4. 保存到Redis
        taskRepository.saveTask(task);

        // 5. 发布任务创建事件
        eventProducer.sendTaskCreatedEvent(task);

        // 6. 启动任务执行
        startTask(task);

        return task;
    }

    // 启动任务
    public void startTask(Task task) {
        // 更新任务状态为RUNNING
        task.setStatus(TaskStatus.RUNNING);
        task.setStartTime(System.currentTimeMillis());
        taskRepository.saveTask(task);

        // 调度所有就绪节点
        scheduleReadyNodes(task);
    }

    // 调度就绪节点
    public void scheduleReadyNodes(Task task) {
        List<Node> readyNodes = task.getDag().getReadyNodes();
        for (Node node : readyNodes) {
            // 更新节点状态为RUNNING
            node.setStatus(com.lingxi.ai.orchestrator.model.NodeStatus.RUNNING);
            taskRepository.saveNode(task.getTaskId(), node);

            // 构建 payload（包含 task 信息）
            java.util.Map<String, Object> payload = new java.util.HashMap<>();
            if (node.getInput() != null) {
                payload.putAll(node.getInput());
            }
            payload.put("task", node.getTask());

            // 发布节点就绪事件
            NodeTaskEvent event = new NodeTaskEvent(
                    task.getTaskId(),
                    node.getNodeId(),
                    node.getType(),
                    payload,
                    task.getTraceId()
            );
            eventProducer.sendNodeReadyEvent(event);
        }
    }

    // 查询任务
    public Task getTask(String taskId) {
        return taskRepository.getTask(taskId);
    }
}