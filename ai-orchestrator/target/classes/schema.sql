-- ============================================
-- 灵犀AI OS Orchestrator 数据库初始化脚本 (PostgreSQL)
-- ============================================

-- Task 表
CREATE TABLE IF NOT EXISTS ai_task (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64),
    status VARCHAR(20),
    input JSONB,
    output JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Node 表
CREATE TABLE IF NOT EXISTS ai_node (
    id VARCHAR(64) PRIMARY KEY,
    task_id VARCHAR(64),
    type VARCHAR(20),
    name VARCHAR(100),
    status VARCHAR(20),
    input JSONB,
    output JSONB,
    error_message TEXT,
    retry_count INT DEFAULT 0,
    max_retry INT DEFAULT 3,
    priority INT DEFAULT 5,
    worker_group VARCHAR(50) DEFAULT 'default',
    version INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_task ON ai_node(task_id);
CREATE INDEX IF NOT EXISTS idx_status ON ai_node(status);

-- Node Dependency 表（DAG 边）
CREATE TABLE IF NOT EXISTS ai_node_dependency (
    parent_node_id VARCHAR(64),
    child_node_id VARCHAR(64),
    PRIMARY KEY (parent_node_id, child_node_id)
);

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

