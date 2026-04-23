package com.lingxi.ai.orchestrator.service;

import com.lingxi.ai.orchestrator.entity.Node;
import com.lingxi.ai.orchestrator.entity.NodeStatus;
import com.lingxi.ai.orchestrator.entity.TaskStatus;
import com.lingxi.ai.orchestrator.event.EventProducer;
import com.lingxi.ai.orchestrator.repository.NodeRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;

@Slf4j
@Service
@RequiredArgsConstructor
public class DependencyChecker {

    private final NodeRepository nodeRepository;
    private final StateService stateService;
    private final EventProducer eventProducer;

    @Transactional
    public void onNodeExecuted(String nodeId, String taskId) {
        log.info("DependencyChecker: processing node executed event for nodeId={}, taskId={}", nodeId, taskId);

        List<Node> childNodes = nodeRepository.findChildNodes(nodeId);
        log.info("Found {} child nodes for parent {}", childNodes.size(), nodeId);

        for (Node child : childNodes) {
            if (child.getStatus() == NodeStatus.CREATED || child.getStatus() == NodeStatus.RETRYING) {
                if (stateService.checkDependenciesMet(child.getId())) {
                    stateService.initializeNodeReady(child);
                    eventProducer.publishNodeReady(child);
                    log.info("Child node {} is now READY and published", child.getId());
                }
            }
        }

        // Check if the entire task is completed
        if (stateService.checkTaskCompleted(taskId)) {
            stateService.transitionTask(taskId, TaskStatus.SUCCESS);
            log.info("Task {} completed successfully", taskId);
        }
    }
}
