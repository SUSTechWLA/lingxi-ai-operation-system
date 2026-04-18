-- ============================================
-- 灵犀AI OS Worker层升级 - 数据库迁移脚本
-- 扩展 ai_tool 表以支持多语言工具元数据
-- ============================================

-- 首先检查 ai_tool 表是否存在，如果不存在则创建
CREATE TABLE IF NOT EXISTS ai_tool (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    version VARCHAR(20) DEFAULT '1.0.0',
    status VARCHAR(20) DEFAULT 'ACTIVE',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 添加元数据字段（仅在字段不存在时添加）
-- 注意：PostgreSQL 不支持简单的 "ADD COLUMN IF NOT EXISTS"，
-- 但我们可以使用 DO 语句来安全地添加列

DO $$
BEGIN
    -- 添加 description 字段
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'ai_tool' AND column_name = 'description') THEN
        ALTER TABLE ai_tool ADD COLUMN description TEXT;
    END IF;

    -- 添加 tags 字段
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'ai_tool' AND column_name = 'tags') THEN
        ALTER TABLE ai_tool ADD COLUMN tags JSONB;
    END IF;

    -- 添加 input_schema 字段
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'ai_tool' AND column_name = 'input_schema') THEN
        ALTER TABLE ai_tool ADD COLUMN input_schema JSONB;
    END IF;

    -- 添加 output_schema 字段
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'ai_tool' AND column_name = 'output_schema') THEN
        ALTER TABLE ai_tool ADD COLUMN output_schema JSONB;
    END IF;

    -- 添加 examples 字段
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'ai_tool' AND column_name = 'examples') THEN
        ALTER TABLE ai_tool ADD COLUMN examples JSONB;
    END IF;

    -- 添加 timeout 字段
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'ai_tool' AND column_name = 'timeout') THEN
        ALTER TABLE ai_tool ADD COLUMN timeout INT DEFAULT 5000;
    END IF;

    -- 添加 max_retry 字段
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'ai_tool' AND column_name = 'max_retry') THEN
        ALTER TABLE ai_tool ADD COLUMN max_retry INT DEFAULT 3;
    END IF;

    -- 添加 tool_endpoint 字段
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'ai_tool' AND column_name = 'tool_endpoint') THEN
        ALTER TABLE ai_tool ADD COLUMN tool_endpoint VARCHAR(255);
    END IF;

    -- 添加 worker_id 字段
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'ai_tool' AND column_name = 'worker_id') THEN
        ALTER TABLE ai_tool ADD COLUMN worker_id VARCHAR(64);
    END IF;

    -- 添加 worker_group 字段
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'ai_tool' AND column_name = 'worker_group') THEN
        ALTER TABLE ai_tool ADD COLUMN worker_group VARCHAR(50) DEFAULT 'default';
    END IF;

    -- 添加 last_heartbeat 字段
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'ai_tool' AND column_name = 'last_heartbeat') THEN
        ALTER TABLE ai_tool ADD COLUMN last_heartbeat TIMESTAMP;
    END IF;

    -- 添加 author 字段
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'ai_tool' AND column_name = 'author') THEN
        ALTER TABLE ai_tool ADD COLUMN author VARCHAR(100);
    END IF;

    -- 添加 embedding 字段（预留向量字段，用于后续语义检索）
    -- 注意：需要先安装 pgvector 扩展
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'ai_tool' AND column_name = 'embedding') THEN
        BEGIN
            ALTER TABLE ai_tool ADD COLUMN embedding vector(1536);
        EXCEPTION
            WHEN undefined_object THEN
                RAISE NOTICE 'pgvector extension not installed, skipping embedding column';
        END;
    END IF;
END $$;

-- 创建索引（如果不存在）
CREATE INDEX IF NOT EXISTS idx_ai_tool_status ON ai_tool(status);
CREATE INDEX IF NOT EXISTS idx_ai_tool_worker ON ai_tool(worker_id);
CREATE INDEX IF NOT EXISTS idx_ai_tool_endpoint ON ai_tool(tool_endpoint);

-- 为 tags 字段创建 GIN 索引（用于 JSONB 搜索）
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_ai_tool_tags') THEN
        CREATE INDEX idx_ai_tool_tags ON ai_tool USING GIN(tags);
    END IF;
END $$;
