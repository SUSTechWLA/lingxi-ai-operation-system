package com.lingxi.ai.orchestrator.event;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.stereotype.Service;
import java.util.HashMap;
import java.util.Map;
import com.lingxi.ai.orchestrator.model.NodeTaskEvent;
import com.lingxi.ai.orchestrator.model.NodeResultEvent;
import com.lingxi.ai.orchestrator.model.NodeStatus;

@Service
public class WorkerService {
    @Autowired
    private KafkaTemplate<String, Object> kafkaTemplate;

    // 执行任务（Mock实现，模拟延迟）
    public void executeTask(NodeTaskEvent event) {
        try {
            // 模拟执行延迟
            Thread.sleep(1000);

            // 构造结果
            NodeResultEvent result = new NodeResultEvent();
            result.setTaskId(event.getTaskId());
            result.setNodeId(event.getNodeId());
            result.setStatus(NodeStatus.SUCCESS);
            result.setTraceId(event.getTraceId());

            // 从 payload 中获取 task 信息
            String task = "unknown";
            if (event.getPayload() != null && event.getPayload().get("task") != null) {
                task = event.getPayload().get("task").toString();
            }

            // Mock输出
            Map<String, Object> output = new HashMap<>();
            if ("write_article".equals(task)) {
                output.put("content", "这是一篇AI生成的文章...");
            } else if ("summarize".equals(task)) {
                output.put("summary", "这是文章的摘要...");
            }
            result.setOutput(output);

            // 发送结果
            kafkaTemplate.send("ai.node.result", event.getTaskId() + "-" + event.getNodeId(), result);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            // 发送失败结果
            NodeResultEvent result = new NodeResultEvent();
            result.setTaskId(event.getTaskId());
            result.setNodeId(event.getNodeId());
            result.setStatus(NodeStatus.FAILED);
            result.setTraceId(event.getTraceId());
            result.setErrorMessage("执行中断");
            kafkaTemplate.send("ai.node.result", event.getTaskId() + "-" + event.getNodeId(), result);
        }
    }
}