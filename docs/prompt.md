# aios 生产级）

目标：
实现一个基于数据库DAG的任务编排系统，根据 @docs/design_node 文档中的信息 和如下提示词优化当前项目代码和相关文档信息，保证 everything is node in this system。设计脚本放在 @scrpits 文件夹中。

技术要求：

- Spring Boot
- MySQL
- Redpanda（事件驱动）
- REST API

核心模型：

- Task
- Node
- NodeDependency

必须实现：

1️⃣ API

POST /task/create
POST /task/{taskId}/dag
GET  /task/{taskId}

---

2️⃣ DAG 存储

使用三张表：

- ai_task
- ai_node
- ai_node_dependency

禁止使用 JSON DAG

---

3️⃣ 调度器

- 定时扫描 CREATED Node
- 判断父节点是否全部 SUCCESS
- 使用乐观锁抢占
- 状态变为 RUNNING

---

4️⃣ 状态机

SUCCESS：

- 触发子节点 READY

FAILED：

- retry < max → 重试
- 否则 → FAILED

---

5️⃣ Worker 模拟

实现一个简单 Worker：

- 订阅 Node
- sleep 1s
- 标记 SUCCESS

---

6️⃣ DAG 校验

必须实现：

- 环检测
- 起点检测

---

测试要求：

1️⃣ 启动服务
2️⃣ 创建 Task
3️⃣ 提交 DAG
4️⃣ 自动执行
5️⃣ 查询状态为 SUCCESS

```
