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

            // Mock输出
            Map<String, Object> output = new HashMap<>();
            String task = getTask(event);
            if (task.equals("write_article")) {
                output.put("content", "这是一篇AI生成的文章...");
            } else if (task.equals("summarize")) {
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

    // 获取任务类型
    private String getTask(NodeTaskEvent event) {
        // 这里简化处理，直接返回nodeId对应的任务类型
        // 实际场景中应该从event.payload中获取
        if (event.getNodeId().equals("1")) {
            return "write_article";
        } else if (event.getNodeId().equals("2")) {
            return "summarize";
        }
        return "unknown";
    }
}