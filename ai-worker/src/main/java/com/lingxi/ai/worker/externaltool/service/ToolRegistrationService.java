package com.lingxi.ai.worker.externaltool.service;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.lingxi.ai.worker.externaltool.client.ExternalToolClient;
import com.lingxi.ai.worker.externaltool.event.ToolRegisteredEvent;
import com.lingxi.ai.worker.externaltool.event.ToolUnregisteredEvent;
import com.lingxi.ai.worker.externaltool.model.RegisteredTool;
import com.lingxi.ai.worker.externaltool.model.ToolInfo;
import com.lingxi.ai.worker.externaltool.registry.ExternalToolRegistry;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.stereotype.Service;
import reactor.core.publisher.Mono;

import java.util.UUID;

/**
 * 工具注册服务
 */
@Slf4j
@Service
@RequiredArgsConstructor
public class ToolRegistrationService {

    private final ExternalToolClient toolClient;
    private final ExternalToolRegistry toolRegistry;
    private final KafkaTemplate<String, String> kafkaTemplate;
    private final ObjectMapper objectMapper;

    @Value("${spring.application.name:ai-worker}")
    private String applicationName;

    private final String workerId = UUID.randomUUID().toString();

    /**
     * 注册外部工具
     */
    public Mono<RegisteredTool> registerTool(String endpoint, String workerGroup) {
        log.info("Starting tool registration process for endpoint: {}", endpoint);

        return toolClient.getToolInfo(endpoint)
                .flatMap(toolInfo -> {
                    // 验证工具健康状态
                    return toolClient.checkHealth(endpoint)
                            .flatMap(health -> {
                                if (!health.isUp()) {
                                    return Mono.error(new RuntimeException("Tool health check failed, status: " + health.getStatus()));
                                }
                                return Mono.just(toolInfo);
                            });
                })
                .map(toolInfo -> {
                    // 注册到本地注册表
                    RegisteredTool registeredTool = toolRegistry.registerTool(
                            toolInfo,
                            endpoint,
                            workerId,
                            workerGroup
                    );

                    // 发送注册事件
                    publishToolRegisteredEvent(registeredTool);

                    return registeredTool;
                })
                .doOnSuccess(tool -> log.info("Tool registration completed: {}", tool.getToolInfo().getToolName()))
                .doOnError(e -> log.error("Tool registration failed for endpoint {}: {}", endpoint, e.getMessage()));
    }

    /**
     * 注销工具
     */
    public Mono<Void> unregisterTool(String endpoint) {
        log.info("Unregistering tool from endpoint: {}", endpoint);

        return Mono.fromCallable(() -> {
            var toolOpt = toolRegistry.unregisterTool(endpoint);
            toolOpt.ifPresent(tool -> {
                publishToolUnregisteredEvent(tool.getToolInfo().getToolName(), endpoint);
            });
            return null;
        });
    }

    /**
     * 发送工具注册事件
     */
    private void publishToolRegisteredEvent(RegisteredTool registeredTool) {
        try {
            ToolInfo toolInfo = registeredTool.getToolInfo();

            var eventData = ToolRegisteredEvent.ToolRegisteredData.builder()
                    .toolName(toolInfo.getToolName())
                    .toolVersion(toolInfo.getToolVersion())
                    .description(toolInfo.getDescription())
                    .author(toolInfo.getAuthor())
                    .tags(toolInfo.getTags())
                    .inputSchema(toolInfo.getInputSchema())
                    .outputSchema(toolInfo.getOutputSchema())
                    .examples(toolInfo.getExamples() != null
                            ? toolInfo.getExamples().stream()
                            .map(ex -> ToolRegisteredEvent.ToolExample.builder()
                                    .input(ex.getInput())
                                    .output(ex.getOutput())
                                    .build())
                            .toList()
                            : null)
                    .timeout(toolInfo.getTimeout())
                    .maxRetry(toolInfo.getMaxRetry())
                    .workerId(registeredTool.getWorkerId())
                    .workerGroup(registeredTool.getWorkerGroup())
                    .toolEndpoint(registeredTool.getToolEndpoint())
                    .build();

            ToolRegisteredEvent event = ToolRegisteredEvent.create(eventData);
            String eventJson = objectMapper.writeValueAsString(event);

            log.info("Publishing tool registered event: {}", toolInfo.getToolName());
            kafkaTemplate.send("ai.tool.registered", toolInfo.getToolName(), eventJson);
        } catch (Exception e) {
            log.error("Failed to publish tool registered event", e);
        }
    }

    /**
     * 发送工具注销事件
     */
    private void publishToolUnregisteredEvent(String toolName, String endpoint) {
        try {
            ToolUnregisteredEvent event = ToolUnregisteredEvent.create(toolName, endpoint);
            String eventJson = objectMapper.writeValueAsString(event);

            log.info("Publishing tool unregistered event: {}", toolName);
            kafkaTemplate.send("ai.tool.unregistered", toolName, eventJson);
        } catch (Exception e) {
            log.error("Failed to publish tool unregistered event", e);
        }
    }

    /**
     * 获取Worker ID
     */
    public String getWorkerId() {
        return workerId;
    }
}
