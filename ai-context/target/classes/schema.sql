-- ============================================
-- 灵犀AI OS Context Service 数据库初始化脚本 (PostgreSQL)
-- ============================================

-- Context 表（上下文管理）
CREATE TABLE IF NOT EXISTS ai_context (
    id BIGSERIAL PRIMARY KEY,
    context_type VARCHAR(50),
    task_id VARCHAR(64),
    node_id VARCHAR(64),
    metadata JSONB,
    message VARCHAR(1000),
    snapshot_data JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_context_task ON ai_context(task_id);
CREATE INDEX IF NOT EXISTS idx_context_node ON ai_context(node_id);
