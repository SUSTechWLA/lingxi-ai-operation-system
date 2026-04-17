package com.lingxi.ai.orchestrator.service;

import com.lingxi.ai.orchestrator.context.ContextService;
import com.lingxi.ai.orchestrator.entity.Node;
import com.lingxi.ai.orchestrator.entity.NodeStatus;
import com.lingxi.ai.orchestrator.event.EventProducer;
import com.lingxi.ai.orchestrator.repository.NodeRepository;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;

@Service
public class Scheduler {

    private static final Logger logger = LoggerFactory.getLogger(Scheduler.class);

    @Autowired
    private NodeRepository nodeRepository;

    @Autowired
    private EventProducer eventProducer;

    @Autowired
    private ContextService contextService;

    @Autowired
    private TaskExecutionControl taskExecutionControl;

    @Scheduled(fixedRate = 1000)
    @Transactional
    public void schedule() {
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
                logger.debug("Skipping scheduling for task {}: cannot schedule now", taskId);
                continue;
            }

            for (Node node : taskNodes) {
                if (tryLockAndRun(node)) {
                    logger.info("Scheduled node: {} for task: {}", node.getId(), taskId);
                    contextService.recordNodeSnapshot(node, "Before execution snapshot");
                    contextService.recordNodeScheduled(node);
                    eventProducer.publishNodeReady(node);
                    scheduledCount++;
                }
            }
        }

        if (scheduledCount > 0) {
            logger.info("Scheduled {} nodes across {} tasks", scheduledCount, nodesByTask.size());
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
            return true;
        }
        logger.debug("Failed to lock node: {}", node.getId());
        return false;
    }
}
