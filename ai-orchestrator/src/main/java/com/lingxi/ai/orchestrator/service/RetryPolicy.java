package com.lingxi.ai.orchestrator.service;

import org.springframework.stereotype.Service;

@Service
public class RetryPolicy {

    private static final long MAX_DELAY_MS = 60000;
    private static final long INITIAL_DELAY_MS = 1000;

    public long getRetryDelay(int retryCount) {
        return (long) Math.min(Math.pow(2, retryCount) * INITIAL_DELAY_MS, MAX_DELAY_MS);
    }

    public boolean shouldRetry(int retryCount, int maxRetry) {
        return retryCount < maxRetry;
    }
}
