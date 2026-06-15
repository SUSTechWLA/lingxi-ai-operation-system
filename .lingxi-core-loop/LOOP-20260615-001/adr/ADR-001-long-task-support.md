# ADR-001: 长任务支持架构

## 状态
PROPOSED

## 背景
AIOS Core 编排层当前只能处理秒级到分钟级的短任务。标书生成、视频分析等场景需要小时级甚至天级的任务执行能力，需要进度追踪、心跳检测和 checkpoint 恢复机制。

## 决策

### 1. 进度上报使用 Kafka 事件驱动

**选择**: 进度通过 Kafka `ai.node.progress` topic 异步分发

**备选**:
- WebSocket 直连: 需要在 Worker 和 API 之间增加连接管理，复杂度高
- 仅数据库轮询: 实时性差，增加 DB 负载

**理由**: 复用已有 Kafka 基础设施；解耦进度生产者和消费者；支持多消费者扩展

### 2. 心跳由 Worker 端主动上报

**选择**: Worker executor 在执行长任务时启动 heartbeat goroutine，定期发布进度事件

**备选**:
- 编排层主动健康检查: 需要 RPC 调用 Worker，增加网络依赖
- 外部健康检查: 需要额外基础设施

**理由**: Worker 最清楚自身是否存活；与进度事件合并减少消息量；实现简单

### 3. Checkpoint 使用已有 Context 表

**选择**: checkpoint 数据存储在 `ai_context.snapshot_data`

**备选**:
- Node 表新增 checkpoint 列: 增加 schema 变更
- Redis 存储: 不可靠持久化

**理由**: Context 表已有 snapshot_data 字段设计；与审计日志统一管理；持久化到 PostgreSQL

### 4. 不修改已有 Tool 接口签名

**选择**: 新增可选接口 `ProgressReporter`，已有 Tool 不需要任何修改

**备选**:
- 修改 Execute 方法签名: 破坏所有已有 Tool
- 使用 context.Context 传值: 隐式契约，不易发现

**理由**: 显式接口，Go 惯用法；向后兼容；可选实现

### 5. 线程安全使用 version 乐观锁

**选择**: Node 已有 version 字段，心跳更新时使用乐观锁

**理由**: 避免行锁竞争；与已有 Save 语义一致

## 后果

### 正面
- 所有长任务场景获得统一进度/心跳/恢复能力
- 不破坏任何现有功能
- 复用已有基础设施

### 负面
- Kafka 消息量增加（监控需关注）
- 心跳 goroutine 增加内存开销（长任务节点数量级可控）

### 风险
- 心跳消息可能延迟但不会丢失（Kafka at-least-once）
- 心跳超时检测有最多 30s 延迟（调度器扫描间隔）
