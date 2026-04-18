package com.lingxi.ai.worker.externaltool.service;

import com.lingxi.ai.worker.externaltool.client.ExternalToolClient;
import com.lingxi.ai.worker.externaltool.model.RegisteredTool;
import com.lingxi.ai.worker.externaltool.registry.ExternalToolRegistry;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Service;

import java.time.Duration;
import java.time.Instant;

/**
 * 工具健康检查服务
 */
@Slf4j
@Service
@RequiredArgsConstructor
public class ToolHealthCheckService {

    private final ExternalToolRegistry toolRegistry;
    private final ExternalToolClient toolClient;

    /**
     * 每30秒执行一次健康检查
     */
    @Scheduled(fixedRate = 30000, initialDelay = 10000)
    public void checkAllToolsHealth() {
        var tools = toolRegistry.getAllTools();
        if (tools.isEmpty()) {
            return;
        }

        log.debug("Starting health check for {} tools", tools.size());

        for (RegisteredTool tool : tools) {
            checkToolHealth(tool);
        }
    }

    private void checkToolHealth(RegisteredTool tool) {
        String endpoint = tool.getToolEndpoint();
        String toolName = tool.getToolInfo().getToolName();

        toolClient.checkHealth(endpoint)
                .subscribe(
                        health -> {
                            boolean healthy = health.isUp();
                            toolRegistry.updateHeartbeat(endpoint, healthy);

                            if (!healthy) {
                                checkExpired(tool);
                            }
                        },
                        error -> {
                            log.warn("Health check failed for tool {}: {}", toolName, error.getMessage());
                            toolRegistry.updateHeartbeat(endpoint, false);
                            checkExpired(tool);
                        }
                );
    }

    private void checkExpired(RegisteredTool tool) {
        Instant lastHeartbeat = tool.getLastHeartbeatAt();
        if (lastHeartbeat != null) {
            Duration elapsed = Duration.between(lastHeartbeat, Instant.now());
            if (elapsed.getSeconds() > 90) {
                toolRegistry.markUnavailable(tool.getToolEndpoint());
            }
        }
    }
}
