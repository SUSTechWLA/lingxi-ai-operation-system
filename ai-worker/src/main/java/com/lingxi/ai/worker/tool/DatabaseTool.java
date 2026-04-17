package com.lingxi.ai.worker.tool;

import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Component;

import javax.sql.DataSource;
import java.time.Instant;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * 数据库工具 - 支持SQL查询执行
 */
@Slf4j
@Component
public class DatabaseTool implements Tool {

    private final JdbcTemplate jdbcTemplate;

    @Autowired
    public DatabaseTool(DataSource dataSource) {
        this.jdbcTemplate = new JdbcTemplate(dataSource);
    }

    @Override
    public String getName() {
        return "database";
    }

    @Override
    public String getDescription() {
        return "数据库查询工具，支持执行SELECT查询";
    }

    @Override
    public ToolType getType() {
        return ToolType.DATABASE;
    }

    @Override
    public ToolResult execute(Map<String, Object> parameters, ToolContext context) {
        Instant startTime = Instant.now();
        log.info("Executing Database tool for task: {}, node: {}", context.getTaskId(), context.getNodeId());

        try {
            String sql = getParameter(parameters, "sql", String.class);
            if (sql == null || sql.isBlank()) {
                throw new IllegalArgumentException("SQL query is required");
            }

            String sqlUpper = sql.trim().toUpperCase();
            if (!sqlUpper.startsWith("SELECT")) {
                throw new IllegalArgumentException("Only SELECT queries are allowed for safety");
            }

            List<Map<String, Object>> results = jdbcTemplate.queryForList(sql);

            Map<String, Object> resultData = new HashMap<>();
            resultData.put("query", sql);
            resultData.put("rowCount", results.size());
            resultData.put("rows", results);

            log.info("Database tool executed successfully, returned {} rows", results.size());
            return ToolResult.success(resultData, startTime, Instant.now());

        } catch (Exception e) {
            log.error("Database tool execution failed for task: {}", context.getTaskId(), e);
            return ToolResult.failure(e.getMessage(), startTime, Instant.now());
        }
    }

    @Override
    public boolean validateParameters(Map<String, Object> parameters) {
        if (parameters == null || !parameters.containsKey("sql")) {
            return false;
        }
        Object sql = parameters.get("sql");
        return sql instanceof String && !((String) sql).isBlank();
    }

    @SuppressWarnings("unchecked")
    private <T> T getParameter(Map<String, Object> parameters, String key, Class<T> type) {
        Object value = parameters.get(key);
        if (type.isInstance(value)) {
            return (T) value;
        }
        return null;
    }
}
