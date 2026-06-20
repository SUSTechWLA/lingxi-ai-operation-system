# 00-Intake: 长任务支持

## 1. 任务来源

**类型**: Core 平台能力新增 (`core-update`)

**原始需求**:
当前 AIOS Core 的编排层和 Worker 层都是为短程任务设计的（通常秒级到分钟级完成）。对于用户来说，无法直观感受到长任务的进展，并且长任务需要考虑：
1. 任务失败如何回溯
2. 任务状态检测
3. 任务如何记录

## 2. 问题分析

### 2.1 当前短任务模型

```
CREATED → READY → RUNNING → SUCCESS/FAILED
```

- 节点从 RUNNING 到 SUCCESS/FAILED 是一次性同步转换
- 没有中间状态来表示"仍在运行但还没完成"
- 没有进度百分比或阶段性输出
- 失败后只能从 CREATED 重新开始（调度器 30s fallback）
- 没有部分成功的概念
- 没有心跳/存活检测

### 2.2 长任务需要的额外能力

| 能力 | 当前状态 | 缺口 |
|------|---------|------|
| 进度追踪 | 无 | 需要进度上报与存储 |
| 心跳/存活检测 | 无 | 需要心跳机制检测僵死任务 |
| 失败回溯 | 只能重试整个节点 | 需要从 checkpoint 恢复 |
| 任务记录 | 只有状态变更日志 | 需要结构化进度日志 |
| 状态检测 | 只有最终状态 | 需要查询实时进度 |
| 超时处理 | 仅 Worker 执行超时 | 需要节点级总体超时+Tick 超时 |

## 3. 初步影响范围

- `internal/orchestrator/` — 状态机、状态服务、调度器
- `internal/worker/` — 节点执行器、执行接口
- `internal/context/` — 审计日志扩展
- `internal/model/` — 数据模型和数据库迁移
- `internal/eventbus/` — 新事件类型（心跳、进度）

## 4. 待确认问题

1. 长任务的"进度"粒度是什么？（百分比？阶段描述？还是两者都有？）
2. 是否需要支持长任务的"暂停/恢复"？
3. 心跳超时后是自动标记失败还是通知用户？
4. checkpoint 恢复需要 Tool 侧配合实现吗？
5. 进度信息是否需要 API 实时推送（SSE/WebSocket）还是轮询？

## 5. 风险

- 不能破坏现有短任务的行为（向后兼容）
- 数据库 schema 变更需要安全迁移
- 长任务可能消耗更多内存和数据库连接

## 6. 模式与运行参数

```yaml
mode: core-update
core_write_enabled: false
execution_mode: EXTERNAL_MANUAL
max_fix_iterations: 5
```
