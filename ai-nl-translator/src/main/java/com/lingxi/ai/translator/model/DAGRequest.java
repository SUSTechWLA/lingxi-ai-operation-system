package com.lingxi.ai.translator.model;

import lombok.Data;
import java.util.List;
import java.util.Map;

@Data
public class DAGRequest {

    private List<NodeRequest> nodes;
    private List<Edge> edges;

    @Data
    public static class NodeRequest {
        private String id;
        private String type;
        private String name;
        private Map<String, Object> input;
        private Integer maxRetry;
        private Integer priority;
        private String workerGroup;
    }

    @Data
    public static class Edge {
        private String from;
        private String to;
    }
}
