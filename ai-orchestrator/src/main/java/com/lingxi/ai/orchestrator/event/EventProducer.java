package com.lingxi.ai.orchestrator.event;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.stereotype.Service;
import com.lingxi.ai.orchestrator.model.Task;
import com.lingxi.ai.orchestrator.model.NodeTaskEvent;

@Service
public class EventProducer {
    @Autowired
    private KafkaTemplate<String, Object> kafkaTemplate;

    public void sendTaskCreatedEvent(Task task) {
        kafkaTemplate.send("ai.task.created", task.getTaskId(), task);
    }

    public void sendNodeReadyEvent(NodeTaskEvent event) {
        kafkaTemplate.send("ai.node.ready", event.getTaskId() + "-" + event.getNodeId(), event);
    }

    public void sendTaskCompletedEvent(Task task) {
        kafkaTemplate.send("ai.task.completed", task.getTaskId(), task);
    }

    public void sendTaskFailedEvent(Task task) {
        kafkaTemplate.send("ai.task.failed", task.getTaskId(), task);
    }
}