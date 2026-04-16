package com.lingxi.ai.orchestrator.event;

import com.lingxi.ai.orchestrator.model.NodeResultEvent;
import com.lingxi.ai.orchestrator.model.NodeStatus;
import com.lingxi.ai.orchestrator.model.NodeTaskEvent;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.stereotype.Service;

import java.util.HashMap;
import java.util.Map;

@Service
public class WorkerService {

    private static final Logger logger = LoggerFactory.getLogger(WorkerService.class);

    @Autowired
    private KafkaTemplate<String, Object> kafkaTemplate;

    public void executeTask(NodeTaskEvent event) {
        try {
            logger.info("Worker executing task for node: {}", event.getNodeId());

            Thread.sleep(1000);

            NodeResultEvent result = new NodeResultEvent();
            result.setTaskId(event.getTaskId());
            result.setNodeId(event.getNodeId());
            result.setStatus(NodeStatus.SUCCESS);
            result.setTraceId(event.getTraceId());

            Map<String, Object> output = new HashMap<>();
            Map<String, Object> payload = event.getPayload();
            if (payload != null && payload.get("task") != null) {
                String taskName = payload.get("task").toString();
                if ("write_article".equals(taskName) || "write".equals(taskName)) {
                    output.put("content", "这是一篇AI生成的文章...");
                } else if ("summarize".equals(taskName) || "summary".equals(taskName)) {
                    output.put("summary", "这是文章的摘要...");
                }
            }
            if (output.isEmpty()) {
                output.put("result", "Node executed successfully");
            }
            result.setOutput(output);

            logger.info("Worker completed task for node: {}", event.getNodeId());
            kafkaTemplate.send("ai.node.result", event.getTaskId() + "-" + event.getNodeId(), result);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();

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
