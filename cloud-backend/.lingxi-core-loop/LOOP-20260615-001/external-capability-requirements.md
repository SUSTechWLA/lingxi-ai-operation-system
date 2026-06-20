# External-Capability-Requirements: 标书生成 — 外部工具能力需求

> 不生成工具代码。每项能力仅定义: capability_id, 职责, 输入输出, 质量指标, 性能, 安全, 错误结构, Core 调用约束, 黑盒验收。

---

## CAP-001: doc_parser — 招标文件解析

### 职责
- 接收 PDF/Word/图片格式的招标文件
- 提取文本内容、表格数据、图片
- 识别评分项、技术要求、资质要求、格式要求

### 非职责
- 不存储文件 (MinIO 负责)
- 不进行语义理解 (LLM 负责)
- 不处理手写体 OCR

### 输入
```json
{
  "file_path": "minio://bucket/path/to/tender.pdf",
  "file_type": "pdf",
  "options": {
    "extract_tables": true,
    "extract_images": true,
    "max_pages": 500
  }
}
```

### 输出
```json
{
  "success": true,
  "data": {
    "project_info": {
      "name": "XX项目招标",
      "bid_number": "2026-001",
      "budget": "500万元",
      "deadline": "2026-07-15",
      "purchaser": "XX公司"
    },
    "sections": [{"title": "...", "content": "..."}],
    "tables": [{"caption": "...", "rows": [["...","..."]], "page": 3}],
    "score_items": [
      {"id": "S1", "name": "技术方案", "max_score": 30, "criteria": "..."}
    ],
    "tech_requirements": [{"id": "T1", "description": "..."}],
    "qualifications": [{"type": "certificate", "name": "ISO9001", "required": true}],
    "format_requirements": {"page_limit": 100, "font": "宋体", "font_size": "小四"}
  }
}
```

### 质量指标
- 文本提取准确率 > 95%
- 表格识别准确率 > 90%
- 评分项提取准确率 > 85%

### 性能
- 100 页 PDF 解析 < 30 秒
- 支持最大 500 页

### 错误结构
```json
{
  "success": false,
  "error": {
    "code": "UNSUPPORTED_FORMAT | FILE_TOO_LARGE | PARSE_FAILED | CORRUPTED_FILE",
    "message": "..."
  }
}
```

### Core 调用约束
- 通过 External Tool 注册: `POST /api/tools/register`
- Type: `http`, Endpoint: `http://doc-parser:8001/parse`
- Timeout: 120s
- Tool name: `doc_parser`

### 黑盒验收
1. 上传 10 份不同类型的招标文件，验证解析输出格式正确
2. 上传损坏文件，验证返回 CORRUPTED_FILE 错误
3. 上传 600 页文件，验证返回 FILE_TOO_LARGE
4. 验证评分项提取结果与人工标注对比 > 85%

---

## CAP-002: knowledge_base — 企业资料检索

### 职责
- 管理企业资质、案例、人员、财务等结构化/非结构化资料
- 根据检索查询返回最匹配的企业资料

### 非职责
- 不负责资料录入的审批流程
- 不处理资料的版本管理

### 输入
```json
{
  "query": {
    "type": "certificate | project_case | personnel | financial | technical_template",
    "keywords": ["ISO9001", "信息系统集成"],
    "filters": {"industry": "IT", "year_range": [2023, 2026]}
  }
}
```

### 输出
```json
{
  "success": true,
  "data": {
    "matches": [
      {
        "id": "M001",
        "type": "certificate",
        "name": "ISO9001质量管理体系认证",
        "description": "...",
        "file_path": "minio://...",
        "relevance": 0.95,
        "metadata": {"issued_date": "2025-01", "expiry_date": "2028-01"}
      }
    ]
  }
}
```

### 质量指标
- Top-5 检索准确率 > 90%
- 召回率 > 85%

### 性能
- 检索响应 < 5 秒

### 错误结构
```json
{"success": false, "error": {"code": "NO_MATCH | INDEX_ERROR | TIMEOUT", "message": "..."}}
```

### Core 调用约束
- Type: `http`, Endpoint: `http://knowledge-base:8002/search`
- Timeout: 30s
- Tool name: `knowledge_base`

### 黑盒验收
1. 用标书中提取的资质要求查询，验证返回匹配的企业资质
2. 用不存在的资质要求查询，验证返回空列表
3. 验证返回结果包含文件路径和相关性分数

---

## CAP-003: chapter_generator — 章节内容生成

### 职责
- 基于招标要求 + 企业资料 + 写作规范，生成标书章节内容
- 支持 Markdown 格式输出
- 支持插入表格和列表

### 非职责
- 不负责格式排版 (Word 导出工具负责)
- 不负责合规检查

### 输入
```json
{
  "chapter": {
    "id": "ch-3",
    "title": "技术方案",
    "sub_chapters": ["3.1 总体架构", "3.2 技术路线"],
    "score_items": [{"id": "S1", "name": "技术方案", "max_score": 30}]
  },
  "tender_requirements": {"tech_requirements": [...], "format_requirements": {...}},
  "materials": [{"name": "...", "content": "..."}],
  "style": {"tone": "formal", "length": "comprehensive", "keywords": ["国产化", "信创"]}
}
```

### 输出
```json
{
  "success": true,
  "data": {
    "content": "# 3. 技术方案\n\n## 3.1 总体架构\n...",
    "references": ["使用了 ISO9001 认证", "参考了 XX 案例"],
    "tables": [{"caption": "...", "rows": [...]}],
    "token_usage": {"prompt": 3500, "completion": 2000}
  }
}
```

### 质量指标
- 内容与招标要求相关性 > 85%
- 不包含虚假信息 (对企业资料的内容不篡改)

### 性能
- 单章节生成 < 120 秒

### Core 调用约束
- Type: `http`, Endpoint: `http://chapter-generator:8003/generate`
- Timeout: 180s
- Tool name: `chapter_generator`

### 黑盒验收
1. 给定招标要求和企业资料，验证生成的章节包含关键评分点
2. 验证生成内容不篡改企业资料中的事实数据
3. 验证 Markdown 格式正确

---

## CAP-004: compliance_checker — 合规检查

### 职责
- 检查标书章节内容是否覆盖所有评分项
- 检查格式合规（字数、页数估算）
- 检查内容合规（敏感词、竞争对手名等）

### 输入
```json
{
  "chapters": [{"id": "ch-1", "title": "...", "content": "..."}],
  "score_items": [{"id": "S1", "name": "技术方案", "max_score": 30}],
  "format_rules": {"max_pages": 100, "min_font_size": "小四"},
  "sensitive_patterns": ["竞争对手A", "竞争对手B"]
}
```

### 输出
```json
{
  "success": true,
  "data": {
    "coverage": [
      {"score_item_id": "S1", "covered": true, "chapter_id": "ch-3", "confidence": 0.9},
      {"score_item_id": "S2", "covered": false, "suggestion": "未找到关于售后服务的章节"}
    ],
    "format_issues": [{"type": "page_exceed", "message": "预估页数 120 页，超过限制 100 页"}],
    "content_issues": [{"type": "sensitive_word", "location": "ch-1:para-3", "match": "竞争对手A"}],
    "overall_score": 85
  }
}
```

### 性能
- 检查时间 < 30 秒

### Core 调用约束
- Type: `http`, Endpoint: `http://compliance-checker:8004/check`
- Timeout: 60s
- Tool name: `compliance_checker`

### 黑盒验收
1. 给一份故意遗漏评分项的标书，验证覆盖率报告正确指出遗漏
2. 给一份包含敏感词的标书，验证检测结果

---

## CAP-005: doc_exporter — 标书导出

### 职责
- 将 Markdown 格式的标书内容转换为 Word (.docx) 或 PDF
- 保留标题层级、表格、列表格式
- 生成封面和目录

### 输入
```json
{
  "chapters": [{"title": "...", "content": "..."}],
  "project_info": {"name": "...", "bid_number": "..."},
  "cover_info": {"company_name": "XX科技", "date": "2026-06-15", "logo_url": "minio://..."},
  "format": "docx",
  "options": {"header": "...", "footer": "...", "page_numbers": true}
}
```

### 输出
```json
{
  "success": true,
  "data": {
    "file_path": "minio://bucket/exports/bid-xxx.docx",
    "file_size": 2048576,
    "download_url": "http://minio:9000/bucket/exports/bid-xxx.docx?presigned=..."
  }
}
```

### 质量
- 格式还原度 > 95% (标题、表格、列表)
- 目录页码正确

### 性能
- 500 页标书导出 < 60 秒

### 错误结构
```json
{"success": false, "error": {"code": "GENERATION_FAILED | TEMPLATE_ERROR", "message": "..."}}
```

### Core 调用约束
- Type: `http`, Endpoint: `http://doc-exporter:8005/export`
- Timeout: 120s
- Tool name: `doc_exporter`

### 黑盒验收
1. 导出一份多章节标书，验证 Word 格式正确
2. 验证封面和目录自动生成
3. 验证 presigned URL 可下载
