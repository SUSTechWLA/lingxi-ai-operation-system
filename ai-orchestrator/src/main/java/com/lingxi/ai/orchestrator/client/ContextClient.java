package com.lingxi.ai.orchestrator.client;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;
import org.springframework.web.client.RestTemplate;

import java.util.HashMap;
import java.util.List;
import java.util.Map;

@Service
public class ContextClient {

    @Value("${context.service.url:http://localhost:8082}")
    private String contextServiceUrl;

    private final RestTemplate restTemplate = new RestTemplate();

    public void recordTaskCreated(String taskId) {
        Map<String, String> request = Map.of("taskId", taskId);
        restTemplate.postForEntity(contextServiceUrl + "/api/context/task-created", request, Map.class);
    }

    public void recordTaskSuccess(String taskId) {
        Map<String, String> request = Map.of("taskId", taskId);
        restTemplate.postForEntity(contextServiceUrl + "/api/context/task-success", request, Map.class);
    }

    public void recordTaskFailed(String taskId) {
        Map<String, String> request = Map.of("taskId", taskId);
        restTemplate.postForEntity(contextServiceUrl + "/api/context/task-failed", request, Map.class);
    }

    public void recordDagSubmitted(String taskId) {
        Map<String, String> request = Map.of("taskId", taskId);
        restTemplate.postForEntity(contextServiceUrl + "/api/context/dag-submitted", request, Map.class);
    }

    public void recordDagValidated(String taskId) {
        Map<String, String> request = Map.of("taskId", taskId);
        restTemplate.postForEntity(contextServiceUrl + "/api/context/dag-validated", request, Map.class);
    }

    public void recordNodeScheduled(String taskId, String nodeId, String nodeType, String nodeName) {
        Map<String, Object> request = new HashMap<>();
        request.put("taskId", taskId);
        request.put("nodeId", nodeId);
        request.put("type", nodeType);
        request.put("name", nodeName);
        restTemplate.postForEntity(contextServiceUrl + "/api/context/node-scheduled", request, Map.class);
    }

    public void recordNodeReady(String taskId, String nodeId) {
        Map<String, String> request = Map.of("taskId", taskId, "nodeId", nodeId);
        restTemplate.postForEntity(contextServiceUrl + "/api/context/node-ready", request, Map.class);
    }

    public void recordNodeSuccess(String taskId, String nodeId) {
        Map<String, String> request = Map.of("taskId", taskId, "nodeId", nodeId);
        restTemplate.postForEntity(contextServiceUrl + "/api/context/node-success", request, Map.class);
    }

    public void recordNodeFailed(String taskId, String nodeId, String errorMessage) {
        Map<String, String> request = new HashMap<>();
        request.put("taskId", taskId);
        request.put("nodeId", nodeId);
        request.put("errorMessage", errorMessage != null ? errorMessage : "");
        restTemplate.postForEntity(contextServiceUrl + "/api/context/node-failed", request, Map.class);
    }

    public void recordNodeRetry(String taskId, String nodeId, int retryCount, int maxRetry) {
        Map<String, Object> request = new HashMap<>();
        request.put("taskId", taskId);
        request.put("nodeId", nodeId);
        request.put("retryCount", retryCount);
        request.put("maxRetry", maxRetry);
        restTemplate.postForEntity(contextServiceUrl + "/api/context/node-retry", request, Map.class);
    }

    public void recordNodeSnapshot(String taskId, String nodeId, String nodeType, String nodeName,
                                  String status, Map<String, Object> input, Map<String, Object> output,
                                  int retryCount, int maxRetry, int priority, String workerGroup,
                                  Integer version, String errorMessage) {
        Map<String, Object> request = new HashMap<>();
        request.put("taskId", taskId);
        request.put("nodeId", nodeId);
        request.put("type", nodeType);
        request.put("name", nodeName);
        request.put("status", status);
        request.put("input", input);
        request.put("output", output);
        request.put("retryCount", retryCount);
        request.put("maxRetry", maxRetry);
        request.put("priority", priority);
        request.put("workerGroup", workerGroup);
        request.put("version", version);
        request.put("errorMessage", errorMessage);
        restTemplate.postForEntity(contextServiceUrl + "/api/context/node-snapshot", request, Map.class);
    }

    @SuppressWarnings("unchecked")
    public List<Map<String, Object>> getContextForTask(String taskId) {
        String url = contextServiceUrl + "/api/task/" + taskId + "/context";
        try {
            return restTemplate.getForEntity(url, List.class).getBody();
        } catch (Exception e) {
            return List.of();
        }
    }

    @SuppressWarnings("unchecked")
    public Map<String, Object> getLatestSnapshotForNode(String nodeId) {
        String url = contextServiceUrl + "/api/node/" + nodeId + "/snapshot/latest";
        try {
            return restTemplate.getForEntity(url, Map.class).getBody();
        } catch (Exception e) {
            return null;
        }
    }
}
