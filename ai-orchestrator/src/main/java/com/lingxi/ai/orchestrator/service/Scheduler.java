package com.lingxi.ai.orchestrator.service;

import com.lingxi.ai.orchestrator.entity.Node;
import com.lingxi.ai.orchestrator.entity.NodeStatus;
import com.lingxi.ai.orchestrator.event.EventProducer;
import com.lingxi.ai.orchestrator.repository.NodeRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;

@Slf4j
@Service
@RequiredArgsConstructor
public class Scheduler {

    private final NodeRepository nodeRepository;
    private final EventProducer eventProducer;
    private final StateService stateService;
    private final TaskExecutionControl taskExecutionControl;

    @Scheduled(fixedRate = 1000)
    @Transactional
    public void schedule() {
        // Step 1: Recover CREATED nodes whose dependencies are met (system restart recovery)
        recoverCreatedNodes();

        // Step 2: Schedule READY nodes
        List<Node> readyNodes = nodeRepository.findReadyNodes();
        if (readyNodes.isEmpty()) {
            return;
        }

        Map<String, List<Node>> nodesByTask = readyNodes.stream()
                .collect(Collectors.groupingBy(Node::getTaskId));

        int scheduledCount = 0;
        for (Map.Entry<String, List<Node>> entry : nodesByTask.entrySet()) {
            String taskId = entry.getKey();
            List<Node> taskNodes = entry.getValue();

            if (!taskExecutionControl.canScheduleNodes(taskId)) {
                log.debug("Skipping scheduling for task {}: cannot schedule now", taskId);
                continue;
            }

            for (Node node : taskNodes) {
                if (tryLockAndRun(node)) {
                    log.info("Scheduled node: {} for task: {}", node.getId(), taskId);
                    eventProducer.publishNodeReady(node);
                    scheduledCount++;
                }
            }
        }

        if (scheduledCount > 0) {
            log.info("Scheduled {} nodes across {} tasks", scheduledCount, nodesByTask.size());
        }
    }

    private void recoverCreatedNodes() {
        List<Node> createdNodes = nodeRepository.findCreatedNodes();
        for (Node node : createdNodes) {
            if (stateService.checkDependenciesMet(node.getId())) {
                stateService.initializeNodeReady(node);
                log.info("Recovered node {} to READY state (dependencies met)", node.getId());
            }
        }
    }

    private boolean tryLockAndRun(Node node) {
        int updated = nodeRepository.updateStatusWithLock(
                node.getId(),
                NodeStatus.RUNNING,
                node.getVersion()
        );
        if (updated > 0) {
            node.setStatus(NodeStatus.RUNNING);
            node.setVersion(node.getVersion() + 1);
            stateService.transitionNode(node.getId(), NodeStatus.RUNNING, null, null);
            return true;
        }
        log.debug("Failed to lock node: {}", node.getId());
        return false;
    }
}
