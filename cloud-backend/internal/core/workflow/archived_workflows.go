package workflow

// ArchivedWorkflows returns workflow templates that are preserved for future
// reference but are NOT registered by default. These include bid generation,
// content publishing, and code review workflows that are outside the OneClick
// Video v1 product scope.
func ArchivedWorkflows() []struct {
	ID, Name, Desc, Cat, DAG string
} {
	return []struct {
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
}
