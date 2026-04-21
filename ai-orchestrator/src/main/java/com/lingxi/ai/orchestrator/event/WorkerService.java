package com.lingxi.ai.orchestrator.event;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.lingxi.ai.orchestrator.model.NodeResultEvent;
import com.lingxi.ai.orchestrator.model.NodeStatus;
import com.lingxi.ai.orchestrator.model.NodeTaskEvent;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.stereotype.Service;

import java.util.HashMap;
import java.util.Map;

@Slf4j
@Service
@RequiredArgsConstructor
public class WorkerService {

    private final KafkaTemplate<String, String> kafkaTemplate;
    private final ObjectMapper objectMapper;

    public void executeTask(NodeTaskEvent event) {
        try {
            log.info("Worker executing task for node: {}", event.getNodeId());

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

            log.info("Worker completed task for node: {}", event.getNodeId());
            String message = objectMapper.writeValueAsString(result);
            kafkaTemplate.send("ai.node.result", event.getTaskId() + "-" + event.getNodeId(), message);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            try {
                NodeResultEvent result = new NodeResultEvent();
                result.setTaskId(event.getTaskId());
                result.setNodeId(event.getNodeId());
                result.setStatus(NodeStatus.FAILED);
                result.setTraceId(event.getTraceId());
                result.setErrorMessage("执行中断");

                String message = objectMapper.writeValueAsString(result);
                kafkaTemplate.send("ai.node.result", event.getTaskId() + "-" + event.getNodeId(), message);
            } catch (Exception ex) {
                log.error("Failed to send failure message", ex);
            }
        } catch (Exception e) {
            log.error("Error in executeTask", e);
        }
    }
}
