package com.lingxi.ai.orchestrator.event;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Service;
import com.lingxi.ai.orchestrator.model.NodeTaskEvent;
import com.lingxi.ai.orchestrator.model.NodeResultEvent;
import com.lingxi.ai.orchestrator.service.StateMachineService;

@Service
public class EventConsumer {
    @Autowired
    private StateMachineService stateMachineService;
    @Autowired
    private WorkerService workerService; // 内置Mock Worker

    // 消费节点就绪事件（内置Worker执行）
    @KafkaListener(topics = "ai.node.ready", groupId = "worker-group")
    public void handleNodeReady(NodeTaskEvent event) {
        workerService.executeTask(event);
    }

    // 消费节点结果事件
    @KafkaListener(topics = "ai.node.result", groupId = "orchestrator-group")
    public void handleNodeResult(NodeResultEvent event) {
        stateMachineService.handleNodeResult(event);
    }
}