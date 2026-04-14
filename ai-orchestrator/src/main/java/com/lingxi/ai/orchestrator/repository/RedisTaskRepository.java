package com.lingxi.ai.orchestrator.repository;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.stereotype.Repository;
import com.lingxi.ai.orchestrator.model.Task;
import com.lingxi.ai.orchestrator.model.Node;

@Repository
public class RedisTaskRepository {
    @Autowired
    private RedisTemplate<String, Object> redisTemplate;

    private static final String TASK_KEY_PREFIX = "task:";
    private static final String NODE_KEY_PREFIX = "node:";

    // 保存任务
    public void saveTask(Task task) {
        redisTemplate.opsForValue().set(TASK_KEY_PREFIX + task.getTaskId(), task);
    }

    // 获取任务
    public Task getTask(String taskId) {
        return (Task) redisTemplate.opsForValue().get(TASK_KEY_PREFIX + taskId);
    }

    // 保存节点
    public void saveNode(String taskId, Node node) {
        redisTemplate.opsForValue().set(NODE_KEY_PREFIX + taskId + ":" + node.getNodeId(), node);
        // 同时更新任务中的节点
        Task task = getTask(taskId);
        task.getDag().getNodes().stream()
                .filter(n -> n.getNodeId().equals(node.getNodeId()))
                .findFirst()
                .ifPresent(n -> {
                    n.setStatus(node.getStatus());
                    n.setOutput(node.getOutput());
                    n.setErrorMessage(node.getErrorMessage());
                });
        saveTask(task);
    }
}