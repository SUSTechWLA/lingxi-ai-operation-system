package com.lingxi.ai.context.config;

import org.springframework.boot.CommandLineRunner;
import org.springframework.core.annotation.Order;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Component;

@Component
@Order(1)
public class DatabaseInitializer implements CommandLineRunner {

    private final JdbcTemplate jdbcTemplate;

    public DatabaseInitializer(JdbcTemplate jdbcTemplate) {
        this.jdbcTemplate = jdbcTemplate;
    }

    @Override
    public void run(String... args) {
        String createTableSql = """
            CREATE TABLE IF NOT EXISTS ai_context (
                id BIGSERIAL PRIMARY KEY,
                context_type VARCHAR(50),
                task_id VARCHAR(64),
                node_id VARCHAR(64),
                metadata JSONB,
                message VARCHAR(1000),
                snapshot_data JSONB,
                created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
            )
            """;

        String createTaskIndexSql = "CREATE INDEX IF NOT EXISTS idx_context_task ON ai_context(task_id)";
        String createNodeIndexSql = "CREATE INDEX IF NOT EXISTS idx_context_node ON ai_context(node_id)";

        try {
            jdbcTemplate.execute(createTableSql);
            jdbcTemplate.execute(createTaskIndexSql);
            jdbcTemplate.execute(createNodeIndexSql);
            System.out.println("Database initialized successfully");
        } catch (Exception e) {
            System.err.println("Database initialization warning: " + e.getMessage());
        }
    }
}
