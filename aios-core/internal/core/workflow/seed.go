package workflow

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// EnsureSchema creates the workflow_templates table if it doesn't exist.
func EnsureSchema(ctx context.Context, pool *pgxpool.Pool) {
	_, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS workflow_templates (
	    id VARCHAR(64) PRIMARY KEY,
	    version VARCHAR(32) DEFAULT '1.0.0',
	    name VARCHAR(255) NOT NULL,
	    description TEXT,
	    category VARCHAR(128),
	    dag JSONB NOT NULL,
	    created_at TIMESTAMPTZ DEFAULT NOW(),
	    updated_at TIMESTAMPTZ DEFAULT NOW()
	)`)
	if err != nil {
		zap.L().Error("Failed to create workflow_templates table (non-fatal)", zap.Error(err))
		return
	}
	_, _ = pool.Exec(ctx, `ALTER TABLE workflow_templates ADD COLUMN IF NOT EXISTS version VARCHAR(32) DEFAULT '1.0.0'`)
	SeedBuiltinTemplates(ctx, pool)
}

// SeedBuiltinTemplates inserts the standard workflow templates if they don't exist.
func SeedBuiltinTemplates(ctx context.Context, pool *pgxpool.Pool) {
	templates := []struct {
		ID, Name, Desc, Cat, DAG string
	}{
		{
			ID:   "wf-bid-standard",
			Name: "标准标书生成",
			Desc: "解析招标文件 → 章节规划 → 审核 → 并发生成 → 合规检查 → 导出Word",
			Cat:  "bid",
			DAG: `{"nodes":[
				{"id":"parse_tender","type":"TOOL","name":"doc_parser","input":{"file_path":"{{tender_file}}"}},
				{"id":"plan_structure","type":"LLM","name":"llm_api","input":{"prompt":"根据招标分析 {{parse_tender.output}} 规划标书目录结构"}},
				{"id":"hr_approve_plan","type":"CONTROL","name":"审核-章节规划"},
				{"id":"gen_ch1","type":"TOOL","name":"chapter_generator","input":{"chapter":{"index":1,"title":"技术方案"}}},
				{"id":"gen_ch2","type":"TOOL","name":"chapter_generator","input":{"chapter":{"index":2,"title":"项目实施"}}},
				{"id":"gen_ch3","type":"TOOL","name":"chapter_generator","input":{"chapter":{"index":3,"title":"售后服务"}}},
				{"id":"hr_review_1","type":"CONTROL","name":"审核-技术方案"},
				{"id":"hr_review_2","type":"CONTROL","name":"审核-项目实施"},
				{"id":"hr_review_3","type":"CONTROL","name":"审核-售后服务"},
				{"id":"compliance","type":"TOOL","name":"compliance_checker"},
				{"id":"hr_final","type":"CONTROL","name":"终稿审核"},
				{"id":"export","type":"TOOL","name":"doc_exporter","input":{"format":"docx"}}
			],"edges":[
				{"from":"parse_tender","to":"plan_structure"},
				{"from":"plan_structure","to":"hr_approve_plan"},
				{"from":"hr_approve_plan","to":"gen_ch1"},{"from":"hr_approve_plan","to":"gen_ch2"},{"from":"hr_approve_plan","to":"gen_ch3"},
				{"from":"gen_ch1","to":"hr_review_1"},{"from":"gen_ch2","to":"hr_review_2"},{"from":"gen_ch3","to":"hr_review_3"},
				{"from":"hr_review_1","to":"compliance"},{"from":"hr_review_2","to":"compliance"},{"from":"hr_review_3","to":"compliance"},
				{"from":"compliance","to":"hr_final"},
				{"from":"hr_final","to":"export"}
			]}`,
		},
		{
			ID:   "wf-content-publish",
			Name: "内容发布流水线",
			Desc: "分析媒体素材 → 生成内容 → 合规检查 → 平台适配 → 发布",
			Cat:  "publish",
			DAG: `{"nodes":[
				{"id":"analyze","type":"TOOL","name":"media_analyzer","input":{"media_url":"{{media_url}}"}},
				{"id":"generate","type":"TOOL","name":"content_generator","input":{"analysis":"{{analyze.output}}"}},
				{"id":"check","type":"TOOL","name":"content_checker","input":{"content":"{{generate.output}}"}},
				{"id":"adapt","type":"TOOL","name":"platform_adapter","input":{"content":"{{generate.output}}","platform":"douyin"}},
				{"id":"publish","type":"TOOL","name":"bash","input":{"command":"echo Published"}}
			],"edges":[
				{"from":"analyze","to":"generate"},
				{"from":"generate","to":"check"},
				{"from":"check","to":"adapt"},
				{"from":"adapt","to":"publish"}
			]}`,
		},
		{
			ID:   "wf-code-review",
			Name: "代码审查流水线",
			Desc: "代码格式检查 → 静态分析 → 单元测试 → 构建",
			Cat:  "devops",
			DAG: `{"nodes":[
				{"id":"format","type":"TOOL","name":"bash","input":{"command":"gofmt -l ."}},
				{"id":"vet","type":"TOOL","name":"bash","input":{"command":"go vet ./..."}},
				{"id":"test","type":"TOOL","name":"bash","input":{"command":"go test ./... -count=1"}},
				{"id":"build","type":"TOOL","name":"bash","input":{"command":"go build -o /dev/null ./..."}}
			],"edges":[
				{"from":"format","to":"vet"},
				{"from":"vet","to":"test"},
				{"from":"test","to":"build"}
			]}`,
		},
	}

	for _, t := range templates {
		// Validate JSON before inserting
		if !json.Valid([]byte(t.DAG)) {
			zap.L().Warn("Invalid DAG JSON in seed template", zap.String("id", t.ID))
			continue
		}
		_, err := pool.Exec(ctx,
			`INSERT INTO workflow_templates (id, name, description, category, dag)
			 VALUES ($1,$2,$3,$4,$5) ON CONFLICT (id) DO NOTHING`,
			t.ID, t.Name, t.Desc, t.Cat, []byte(t.DAG))
		if err != nil {
			zap.L().Warn("Failed to seed template", zap.String("id", t.ID), zap.Error(err))
		}
	}
	zap.L().Info("Workflow templates seeded", zap.Int("count", len(templates)))
}
