package com.lingxi.ai.worker.config;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import java.util.concurrent.ExecutorService;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.ThreadFactory;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

@Configuration
public class WorkerConfig {

    @Value("${worker.thread-pool.core-size:10}")
    private int corePoolSize;

    @Value("${worker.thread-pool.max-size:50}")
    private int maxPoolSize;

    @Value("${worker.thread-pool.queue-capacity:100}")
    private int queueCapacity;

    @Value("${worker.tool.timeout-seconds:120}")
    private int toolTimeoutSeconds;

    @Bean(name = "toolExecutorService")
    public ExecutorService toolExecutorService() {
        ThreadFactory threadFactory = new ThreadFactory() {
            private final AtomicInteger threadNumber = new AtomicInteger(1);

            @Override
            public Thread newThread(Runnable r) {
                Thread thread = new Thread(r, "tool-executor-" + threadNumber.getAndIncrement());
                thread.setDaemon(false);
                return thread;
            }
        };

        return new ThreadPoolExecutor(
                corePoolSize,
                maxPoolSize,
                60L,
                TimeUnit.SECONDS,
                new LinkedBlockingQueue<>(queueCapacity),
                threadFactory,
                new ThreadPoolExecutor.CallerRunsPolicy()
        );
    }

    public int getToolTimeoutSeconds() {
        return toolTimeoutSeconds;
    }
}
