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

@Service
public class Scheduler {

    private static final Logger logger = LoggerFactory.getLogger(Scheduler.class);

    @Autowired
    private NodeRepository nodeRepository;

    @Autowired
    private EventProducer eventProducer;

    @Autowired
    private ContextService contextService;

    @Scheduled(fixedRate = 1000)
    @Transactional
    public void schedule() {
        List<Node> readyNodes = nodeRepository.findReadyNodes();
        if (readyNodes.isEmpty()) {
            return;
        }

        logger.info("Found {} ready nodes to schedule", readyNodes.size());

        for (Node node : readyNodes) {
            if (tryLockAndRun(node)) {
                logger.info("Scheduled node: {}", node.getId());
                // 保存执行前快照
                contextService.recordNodeSnapshot(node, "Before execution snapshot");
                contextService.recordNodeScheduled(node);
                eventProducer.publishNodeReady(node);
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
            return true;
        }
        logger.debug("Failed to lock node: {}", node.getId());
        return false;
    }
}
