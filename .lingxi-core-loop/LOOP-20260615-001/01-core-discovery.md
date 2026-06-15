# 01-Core-Discovery: 当前 AIOS Core 发现

## 1. 入口层 (cmd/tangying-ai-os/main.go)

### HTTP 路由 (全部注册在 `:8080`)
- 编排路由: `/api/task/*`, `/api/node/*`, `/api/health`
- 翻译路由: `/api/translate`, `/api/translate/submit`
- 发布路由: `/api/publish`, `/api/ai/*`
- 媒体路由: `/api/media/*` (MinIO 可用时)
- 技能路由: `/api/skill/dialog/session/*`
- 工具路由: `/api/tools/*`
- 追踪路由: `/api/trace/*`
- 上下文路由: `/api/context/*`

### 中间件
- Gin Logger + Recovery (内置)
- 自定义 CORS (全开放)

### 模块接线
```
Infrastructure (DB/Redis/Kafka) → Repositories → Services → Handlers
                                                    ↓
                                           Worker (ToolRegistry + Executor)
```

## 2. 编排层

### 任务模型
- Task: ID, UserID, Status, Input/Output (JSONB), PauseReason
- Node: ID, TaskID, Type, Name, Status, Input/Output, ErrorMessage, Condition, RetryCount, MaxRetry(默认3), Priority(默认5), WorkerGroup
- NodeDependency: ParentNodeID → ChildNodeID (DAG 边)

### 节点类型
- TOOL: 工具执行节点
- LLM: LLM 调用节点
- LOG: 日志节点
- CONTROL: 控制流节点 (已定义但未实现特殊逻辑)

### 状态机
- Task: CREATED → RUNNING → SUCCESS | FAILED | PAUSED
- Node: CREATED → READY → RUNNING → SUCCESS | FAILED (可重试到 RETRYING→CREATED)

### 已有关键能力
- ✅ DAG 图提交、验证 (循环检测)、拓扑执行
- ✅ 节点级指数退避重试 (InitialDelay: 1s, MaxDelay: 60s, Multiplier: 2.0)
- ✅ 条件分支 (condition: "nodeID.status == success")
- ✅ 节点间引用解析 ({{node_id.output.field}})
- ✅ 暂停/恢复/强制失败控制
- ✅ 30秒心跳调度器 (恢复卡住的 CREATED 节点)
- ✅ 快照/恢复 (节点级检查点)
- ✅ 乐观并发 (version 字段)
- ❌ 长任务模式 (当前是 HTTP 同步轮询)
- ❌ 人工审批节点
- ❌ 流式进度推送 (无 WebSocket/SSE)

## 3. 工具层

### 14 个内置工具
| 工具 | 类型 | 执行方式 |
|------|------|----------|
| bash | BuildableTool | Direct/Sandbox |
| python | BuildableTool | Direct/Sandbox |
| llm_api | ExecutableTool | 内联 HTTP |
| chat_generate | ExecutableTool | 内联 |
| chat_revise | ExecutableTool | 内联 |
| polisher | ExecutableTool | 内联 |
| content_generator | ExecutableTool | 内联 |
| content_checker | ExecutableTool | 内联 |
| media_analyzer | ExecutableTool | 内联 |
| platform_adapter | ExecutableTool | 内联 |
| video_metadata | ExecutableTool | 内联 |
| video_analyzer | ExecutableTool | 内联 |
| video_copy_generator | ExecutableTool | 内联 |
| external | ExternalToolProvider | HTTP 桥接 |

### 外部工具支持
- ✅ 运行时注册/注销 (POST/DELETE /api/tools/register)
- ✅ DB 持久化 + Redis 缓存
- ✅ ExternalTool 桥接 (HTTP POST 到注册的 endpoint)
- ✅ ToolManifest 知识库 (用于 LLM DAG 规划)

## 4. 事件系统

### 7 个 Kafka Topic
- ai.node.ready: Node 就绪 → Worker 消费执行
- ai.node.result: Worker 返回结果 → Orchestrator 消费
- ai.node.executed: Node 执行完成 (审计)
- ai.node.failed: Node 永久失败 (审计)
- ai.task.created/completed/failed: Task 生命周期

### 发件箱模式
- 事件写入 outbox 表 → Relay (100ms ticker) 读取 → Kafka 发布 → 删除
- 超过 5MB 的消息静默丢弃 (大 image_urls 已通过 DB 通路绕过)

## 5. 中间件
- PostgreSQL 16 (pgx): 6 张业务表 + 1 张 outbox 表
- Redis 7: Session 状态 + Tool Manifest 缓存
- Redpanda (Kafka 兼容): 事件总线
- MinIO: 媒体文件存储 + 预签名 URL
- Qdrant: 向量数据库 (已部署但功能未确认)

## 6. 与标书生成相关的已有能力

| 能力 | 状态 | 复用度 |
|------|------|--------|
| DAG 任务编排 | ✅ 成熟 | 高 — 标书生成是典型 DAG |
| LLM 调用 | ✅ 成熟 (llm_api/chat_generate) | 高 — 章节生成 |
| 外部工具注册 | ✅ 成熟 | 高 — 解析/检索/导出 |
| 条件分支 | ✅ 有 | 中 — 评分项命中 |
| 暂停/恢复 | ✅ 有 (软暂停) | 中 — 人工审核节点 |
| 快照/恢复 | ✅ 有 | 中 — 长任务断点 |
| 文件上传 | ✅ 成熟 (MinIO) | 高 — 招标文件上传 |
| 会话管理 | ✅ 成熟 (Redis) | 中 — 交互式标书 Q&A |
| 工具知识库 | ✅ 成熟 | 中 — LLM 规划标书 DAG |
| Trace/审计 | ✅ 有 | 高 — 标书生成过程追溯 |

## 7. 已知缺口 (待 Phase 3 评估)

1. 无 Word/PDF 文档生成工具
2. 无招标文件解析工具 (OCR/NLP)
3. 无企业资料检索系统 (知识库)
4. 无人工审批节点集成
5. 长任务依赖 HTTP 同步轮询 (不适用于 10min+ 的标书生成)
6. 无 DAG 模板/复用机制
7. API 响应格式不统一

## 8. 测试与质量

- go build: ✅
- go vet: ✅
- go test: 部分模块有测试 (orchestrator, publish, worker/tool 等)
- 无 CI/CD 配置
- 无性能/压力测试

---

*发现日期: 2026-06-15*
*Commit: 3271747*
