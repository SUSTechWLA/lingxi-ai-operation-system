package com.lingxi.ai.orchestrator.service;

import org.springframework.stereotype.Service;
import com.lingxi.ai.orchestrator.model.DAG;

@Service
public class PlannerService {
    // Mock规划，返回固定2节点DAG
    public DAG plan(String prompt) {
        return DAG.sample();
    }
}