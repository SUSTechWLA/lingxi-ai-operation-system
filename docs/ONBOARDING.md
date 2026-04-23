# 灵犀AI OS - 新开发者上手指南

> 本文档面向**完全没有 Go 语言经验**的开发者，帮助你从零理解项目并快速上手开发。

---

## 目录

1. [Go 语言速成](#1-go-语言速成)
2. [项目总览](#2-项目总览)
3. [项目结构详解](#3-项目结构详解)
4. [核心概念](#4-核心概念)
5. [从零启动项目](#5-从零启动项目)
6. [代码阅读路线](#6-代码阅读路线)
7. [如何开发新功能](#7-如何开发新功能)
8. [常见问题](#8-常见问题)

---

## 1. Go 语言速成

### 1.1 安装 Go

```bash
# macOS
brew install go

# 验证安装
go version
# 应输出: go version go1.23.x darwin/arm64
```

### 1.2 Go vs Java 关键对照

| 概念 | Java | Go |
|------|------|-----|
| 包管理 | Maven (pom.xml) | Go Modules (go.mod) |
| 入口函数 | `public static void main(String[] args)` | `func main()` |
| 类 | `class Foo { ... }` | `type Foo struct { ... }` |
| 方法 | `class Foo { void bar() {} }` | `func (f *Foo) bar() {}` |
| 接口 | `interface IFoo { void bar(); }` | `type IFoo interface { bar() }` (隐式实现) |
| 异常 | `try/catch/finally` | `func foo() (result, error)` 返回 error |
| 空值 | `null` | `nil` |
| 继承 | `class Child extends Parent` | 组合嵌入 `type Child struct { Parent }` |
| 并发 | `Thread`, `ExecutorService` | `goroutine`: `go func(){}()` |
| 集合 | `List<T>`, `Map<K,V>` | 切片 `[]T`, 映射 `map[K]V` |
| 字符串 | `String` (引用类型) | `string` (值类型, UTF-8) |
| 指针 | 无(基本) | 有 `*T` 和 `&T`，但无指针运算 |
| 访问控制 | `public`, `private`, `protected` | 首字母大写 = 公开, 小写 = 私有 |
| 初始化 | 构造函数 | 工厂函数 `func NewFoo() *Foo` |
| 依赖注入 | Spring `@Autowired` | 构造函数手动注入 |
| JSON | Jackson `@JsonProperty` | struct tag `` `json:"name"` `` |
| HTTP | Spring `@RestController` | `gin.Engine` + handler 函数 |

### 1.3 Go 代码长什么样

```go
package main

import "fmt"

// 定义结构体 (类似 Java 的 class)
type User struct {
    Name string  // 大写 = public
    Age  int
}

// 方法 (类似 Java 的实例方法)
func (u *User) Greet() string {
    return fmt.Sprintf("Hello, I'm %s, age %d", u.Name, u.Age)
}

// 工厂函数 (类似 Java 的构造函数)
func NewUser(name string, age int) *User {
    return &User{Name: name, Age: age}
}

// 入口
func main() {
    user := NewUser("Alice", 30)       // := 自动推断类型
    fmt.Println(user.Greet())
}
```

### 1.4 Go 习惯用法速查

```go
// 错误处理：Go 没有异常，用返回值
result, err := someFunc()
if err != nil {
    return fmt.Errorf("failed to do something: %w", err)
}

// 指针 vs 值：struct 通常传指针避免拷贝
func (s *Service) DoWork() { }  // 指针接收者，可修改 s
func (s Service) ReadOnly() { } // 值接收者，只读

// 接口隐式实现：只要实现了接口的全部方法就算实现了
type Tool interface {
    Name() string
    Execute(ctx context.Context, params map[string]interface{}) ToolResult
}

// BashTool 自动满足 Tool 接口，不需要 `implements Tool`
type BashTool struct{}
func (b *BashTool) Name() string { return "bash" }
func (b *BashTool) Execute(ctx context.Context, params map[string]interface{}) ToolResult { ... }

// goroutine 并发
go func() {
    // 这段代码在另一个 goroutine 中并发执行
    doSomething()
}()

// channel 通信
ch := make(chan string)
go func() { ch <- "hello" }()
msg := <-ch  // 阻塞等待

// context 取消 (类似 Java 的中断)
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
// 在其他地方调用 cancel() 可以通知所有使用 ctx 的代码停止
```

---

## 2. 项目总览

### 2.1 灵犀AI OS 是什么

灵犀AI OS 是一个**自然语言驱动的任务操作系统**。用户输入自然语言（如"写一篇文章并总结"），系统会：

1. **翻译**：将自然语言分解为可执行的任务图 (DAG)
2. **调度**：按照依赖关系自动编排任务执行顺序
3. **执行**：调用各种工具（LLM、Shell命令等）完成实际操作
4. **记录**：全程审计，支持快照和恢复

### 2.2 架构图

```
用户自然语言输入
       │
       ▼
┌──────────────┐    Kafka     ┌──────────────┐
│  NL-Translator│─────────────│  Orchestrator │
│  自然语言→DAG  │             │  任务调度引擎  │
└──────────────┘             └──────┬───────┘
                                    │ ai.node.ready
                              ┌─────▼───────┐
                              │    Worker     │
                              │  工具执行引擎  │
                              └──────┬───────┘
                                     │ ai.node.result
                              ┌──────▼───────┐
                              │   Context     │
                              │  上下文审计    │
                              └──────────────┘

所有模块运行在同一个 Go 进程中 (端口 8080)
通过 Kafka 事件总线异步通信
数据持久化到 PostgreSQL
```

### 2.3 技术栈

| 功能 | 选型 | 说明 |
|------|------|------|
| Web 框架 | [Gin](https://gin-gonic.com/) | 最流行的 Go HTTP 框架，类似 Spring MVC |
| 数据库 | [pgx](https://github.com/jackc/pgx) | 原生 PostgreSQL 驱动，性能最好 |
| 缓存 | [go-redis](https://github.com/redis/go-redis) | Redis 官方推荐 Go 客户端 |
| 消息队列 | [Sarama](https://github.com/Shopify/sarama) | Kafka/Redpanda Go 客户端 |
| 配置 | [Viper](https://github.com/spf13/viper) | 支持 .env、JSON、YAML 等 |
| 日志 | [Zap](https://github.com/uber-go/zap) | Uber 开源的高性能日志库 |

---

## 3. 项目结构详解

```
lingxi-ai-operation-system/
│
├── cmd/                          # 程序入口
│   └── lingxi-ai-os/
│       └── main.go               # ★ 启动文件，所有组件在此组装
│
├── internal/                     # 内部代码（不可被外部导入）
│   ├── config/                   # 配置加载（从 .env 读取）
│   │   └── config.go             # Config 结构体 + Load() 函数
│   │
│   ├── logger/                   # 日志初始化
│   │   └── logger.go             # Zap 日志配置
│   │
│   ├── database/                 # 数据库连接
│   │   └── database.go           # pgx 连接池 + 建表迁移
│   │
│   ├── redis/                    # Redis 连接
│   │   └── redis.go              # go-redis 客户端
│   │
│   ├── eventbus/                 # Kafka 事件总线
│   │   └── eventbus.go           # Producer(发送) + Consumer(消费)
│   │
│   ├── model/                    # 数据模型（纯数据，无业务逻辑）
│   │   ├── model.go              # Task, Node, DAGRequest 等结构体
│   │   └── repository/
│   │       └── repository.go     # 数据库 CRUD 操作
│   │
│   ├── orchestrator/             # ★ 核心调度模块
│   │   ├── service/
│   │   │   ├── orchestrator.go   #   任务创建、DAG提交
│   │   │   ├── state.go          #   状态转换服务
│   │   │   ├── statemachine.go   #   节点成功/失败状态机
│   │   │   ├── dependency_checker.go  # 依赖检查器
│   │   │   ├── scheduler.go      #   定时恢复调度器
│   │   │   ├── dag_validator.go  #   DAG 环检测+验证
│   │   │   ├── retry_policy.go   #   指数退避重试策略
│   │   │   └── task_execution_control.go  # 暂停/恢复/重试
│   │   └── handler/
│   │       └── handler.go        #   Gin HTTP 路由处理
│   │
│   ├── worker/                   # ★ 工具执行模块
│   │   ├── service/
│   │   │   └── executor.go       #   节点执行器（调度工具执行）
│   │   └── tool/
│   │       ├── tool.go           #   Tool 接口 + ToolRegistry
│   │       └── builtin/
│   │           └── builtin.go    #   BashTool, LlmApiTool
│   │
│   ├── translator/               # ★ 自然语言翻译模块
│   │   ├── service/
│   │   │   └── translator.go     #   调用 LLM 将自然语言转为 DAG
│   │   └── handler/
│   │       └── handler.go        #   Gin HTTP 路由处理
│   │
│   └── context/                  # ★ 上下文审计模块
│       ├── service/
│       │   └── context.go        #   记录事件、快照/恢复
│       └── handler/
│           └── handler.go        #   Gin HTTP 路由处理
│
├── docs/                         # 文档
├── scripts/                      # 构建/运行/测试脚本
├── docker-compose.yml            # Docker 基础设施
├── go.mod                        # ★ Go 依赖管理（类似 pom.xml）
├── Makefile                      # 常用命令快捷方式
└── .env.example                  # 环境变量模板
```

### 文件命名规则

- `model.go` - 数据结构定义
- `repository.go` - 数据库操作
- `service.go` 或以功能命名如 `state.go` - 业务逻辑
- `handler.go` - HTTP 接口处理
- `*_test.go` - 测试文件（Go 自动识别）

---

## 4. 核心概念

### 4.1 DAG 任务图

任务是**有向无环图 (DAG)**，由节点和边组成：

```
  [节点A: 写文章] ──→ [节点B: 总结] ──→ [节点C: 发布]
       │                                       ↑
       └───────────────────────────────────────┘
                    (C 同时依赖 A)
```

- **节点 (Node)**：一个执行单元，类型包括 LLM 调用、工具执行等
- **边 (Edge)**：依赖关系，`from → to` 表示 `to` 依赖 `from` 先完成

### 4.2 节点状态流转

```
CREATED ──→ READY ──→ RUNNING ──→ SUCCESS
  │           │           │
  │           │           └──→ FAILED ──→ RETRYING ──→ CREATED (重试)
  │           │                         └──→ FAILED (永久失败)
  └───────────┘
   (依赖满足时由 Scheduler 恢复到 READY)
```

| 状态 | 含义 |
|------|------|
| CREATED | 已创建，等待依赖满足 |
| READY | 依赖已满足，可执行 |
| RUNNING | 正在执行中 |
| SUCCESS | 执行成功 |
| FAILED | 执行失败 |
| RETRYING | 重试中（指数退避：1s → 2s → 4s → ... → 60s） |

### 4.3 事件驱动流程

```
1. 用户提交 DAG
       │
2. Orchestrator 保存节点，检测无依赖的节点 → 转为 READY
       │
3. 发布 ai.node.ready 事件到 Kafka
       │
4. Worker 消费事件，执行对应工具
       │
5. Worker 发布 ai.node.result 事件 (SUCCESS/FAILED)
       │
6. Orchestrator 消费结果事件
   ├── SUCCESS → DependencyChecker 检查下游节点是否可执行
   │            → 如果全部节点完成 → 任务 SUCCESS
   └── FAILED  → StateMachine 判断是否重试
                → 超过最大重试次数 → 任务 PAUSED/FAILED
       │
7. Context 服务消费所有事件，记录审计日志
```

### 4.4 幂等性保证

每个节点有 `idempotencyKey = taskId + "-" + nodeId`，作为 Kafka 消息的 key，确保同一节点不会被重复执行。

### 4.5 工具系统

Worker 的工具采用**插件式架构**：

```go
// 1. 定义 Tool 接口
type Tool interface {
    Name() string
    Description() string
    Type() ToolType
    Execute(ctx context.Context, params map[string]interface{}, toolCtx ToolContext) ToolResult
    ValidateParameters(params map[string]interface{}) bool
}

// 2. 实现具体工具
type BashTool struct { timeoutSeconds int }
func (t *BashTool) Name() string { return "bash" }
func (t *BashTool) Execute(...) ToolResult { /* 执行 shell 命令 */ }

// 3. 注册到 Registry
toolRegistry.Register(builtin.NewBashTool(cfg.BashTool))
toolRegistry.Register(builtin.NewLlmApiTool(cfg.OpenAI))

// 4. 按名称查找并执行
tool, ok := toolRegistry.Get("bash")
result := tool.Execute(ctx, params, toolCtx)
```

已实现的内置工具：
- **bash** - 执行 Shell 命令
- **llm_api** - 调用 OpenAI 兼容的 LLM API

---

## 5. 从零启动项目

### 5.1 前置条件

| 工具 | 最低版本 | 安装 |
|------|---------|------|
| Go | 1.23+ | `brew install go` |
| Docker | 20+ | `brew install --cask docker` |
| curl | 任意 | 系统自带 |

### 5.2 一键启动

```bash
# 1. 克隆项目
git clone <repo-url>
cd lingxi-ai-operation-system

# 2. 安装环境 + 构建 + 启动基础设施
./scripts/install_lingxi_env.sh

# 3. 配置 API Key（必须）
# 编辑 .env 文件，设置 OPENAI_API_KEY
vim .env

# 4. 启动服务
./scripts/startup.sh
```

### 5.3 手动启动

```bash
# 1. 安装依赖
go mod tidy

# 2. 启动 PostgreSQL、Redis、Redpanda
docker compose up -d

# 3. 配置环境
cp .env.example .env
# 编辑 .env 设置 OPENAI_API_KEY

# 4. 构建
make build

# 5. 运行
make run

# 6. 验证
curl http://localhost:8080/api/health
```

### 5.4 环境变量说明

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `OPENAI_API_KEY` | **是** | - | LLM API 密钥 |
| `OPENAI_BASE_URL` | 否 | `https://api.openai.com/v1` | LLM API 地址 |
| `OPENAI_MODEL` | 否 | `gpt-4` | 使用的模型 |
| `SERVER_PORT` | 否 | `8080` | 服务监听端口 |
| `POSTGRES_HOST` | 否 | `localhost` | PostgreSQL 地址 |
| `POSTGRES_DB` | 否 | `lingxi_db` | 数据库名 |
| `POSTGRES_USER` | 否 | `wanglian` | 数据库用户 |
| `POSTGRES_PASSWORD` | 否 | `123` | 数据库密码 |
| `KAFKA_BOOTSTRAP_SERVERS` | 否 | `localhost:9092` | Kafka 地址 |
| `REDIS_HOST` | 否 | `localhost` | Redis 地址 |

---

## 6. 代码阅读路线

按照以下顺序阅读源码，可以最快理解系统全貌：

```
第 1 站：数据模型
  internal/model/model.go
  → 理解 Task, Node, DAGRequest 等核心数据结构

第 2 站：程序入口
  cmd/lingxi-ai-os/main.go
  → 看清所有组件如何组装在一起

第 3 站：配置系统
  internal/config/config.go
  → 理解项目如何读取配置

第 4 站：Orchestrator 服务（核心）
  internal/orchestrator/service/orchestrator.go    → 任务创建、DAG 提交
  internal/orchestrator/service/state.go            → 状态转换
  internal/orchestrator/service/statemachine.go     → 成功/失败处理
  internal/orchestrator/service/dependency_checker.go → 依赖驱动调度

第 5 站：Worker 工具执行
  internal/worker/tool/tool.go                      → Tool 接口
  internal/worker/tool/builtin/builtin.go           → 具体工具实现
  internal/worker/service/executor.go               → 节点执行器

第 6 站：事件总线
  internal/eventbus/eventbus.go                     → Kafka 生产者/消费者

第 7 站：HTTP 接口
  internal/orchestrator/handler/handler.go          → API 路由处理

第 8 站：翻译器
  internal/translator/service/translator.go         → NL → DAG

第 9 站：上下文审计
  internal/context/service/context.go               → 事件记录
```

---

## 7. 如何开发新功能

### 7.1 添加新的内置工具

**示例：添加一个 HTTP 请求工具**

第 1 步：创建工具实现文件 `internal/worker/tool/builtin/http_tool.go`

```go
package builtin

import (
    "context"
    "net/http"
    "io"
    "github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

type HttpTool struct{}

func NewHttpTool() *HttpTool {
    return &HttpTool{}
}

func (t *HttpTool) Name() string        { return "http" }
func (t *HttpTool) Description() string  { return "Send HTTP requests" }
func (t *HttpTool) Type() tool.ToolType  { return tool.ToolTypeCustom }

func (t *HttpTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
    url, _ := params["url"].(string)
    if url == "" {
        return tool.FailureResult("url is required")
    }

    req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return tool.FailureResult(err.Error())
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)
    return tool.SuccessResult(map[string]interface{}{
        "statusCode": resp.StatusCode,
        "body":       string(body),
    })
}

func (t *HttpTool) ValidateParameters(params map[string]interface{}) bool {
    _, ok := params["url"].(string)
    return ok
}
```

第 2 步：在 `cmd/lingxi-ai-os/main.go` 中注册

```go
toolRegistry := tool.NewToolRegistry()
toolRegistry.Register(builtin.NewBashTool(cfg.BashTool))
toolRegistry.Register(builtin.NewLlmApiTool(cfg.OpenAI))
toolRegistry.Register(builtin.NewHttpTool())  // ← 新增
```

第 3 步：使用时在 DAG 的 input 中指定

```json
{
  "id": "http-1",
  "type": "TOOL",
  "name": "fetch_api",
  "input": {
    "tool": "http",
    "parameters": {
      "url": "https://api.example.com/data"
    }
  }
}
```

### 7.2 添加新的 API 端点

第 1 步：在对应 handler 中添加方法

```go
// internal/orchestrator/handler/handler.go

func (h *OrchestratorHandler) MyNewEndpoint(c *gin.Context) {
    // 解析请求
    var request struct {
        Name string `json:"name" binding:"required"`
    }
    if err := c.ShouldBindJSON(&request); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // 调用 service
    result, err := h.orchestratorService.DoSomething(c.Request.Context(), request.Name)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    // 返回响应
    c.JSON(200, result)
}
```

第 2 步：在 `RegisterRoutes` 中注册路由

```go
func (h *OrchestratorHandler) RegisterRoutes(r *gin.Engine) {
    api := r.Group("/api")
    {
        // ... 已有路由 ...
        api.GET("/my-endpoint", h.MyNewEndpoint)  // ← 新增
    }
}
```

### 7.3 添加数据库查询

在 `internal/model/repository/repository.go` 中添加：

```go
func (r *TaskRepository) FindByStatus(ctx context.Context, status model.TaskStatus) ([]*model.Task, error) {
    rows, err := r.pool.Query(ctx,
        `SELECT id, user_id, status, input, output, created_at
         FROM ai_task WHERE status=$1`, string(status),
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var tasks []*model.Task
    for rows.Next() {
        var task model.Task
        var input, output []byte
        var userID *string

        if err := rows.Scan(&task.ID, &userID, &task.Status, &input, &output, &task.CreatedAt); err != nil {
            return nil, err
        }

        if userID != nil {
            task.UserID = *userID
        }
        if len(input) > 0 {
            _ = json.Unmarshal(input, &task.Input)
        }

        tasks = append(tasks, &task)
    }
    return tasks, nil
}
```

### 7.4 运行测试

```bash
# 运行全部测试
go test ./... -v

# 运行某个包的测试
go test ./internal/orchestrator/service/... -v

# 查看覆盖率
go test ./... -cover

# 运行特定的测试
go test -run TestDAGValidator ./internal/orchestrator/service/... -v
```

### 7.5 常用开发命令

```bash
# 格式化代码（类似 Java 的格式化，Go 强制要求）
go fmt ./...

# 检查代码问题
go vet ./...

# 下载新依赖
go get github.com/some/package@latest
go mod tidy

# 清理构建缓存
go clean -cache
```

---

## 8. 常见问题

### Q: Go 编译报错 `cannot find package`

```bash
go mod tidy    # 重新整理依赖
```

### Q: 连接数据库失败 `connection refused`

```bash
docker compose up -d    # 确保 PostgreSQL 已启动
docker ps               # 检查容器状态
```

### Q: Kafka 连接失败

```bash
docker compose up -d    # 确保 Redpanda 已启动
docker logs lingxi-redpanda  # 查看日志
```

### Q: 如何查看运行日志？

服务启动后日志直接输出到终端。如需文件日志：

```bash
make run 2>&1 | tee /tmp/lingxi.log
```

### Q: 如何添加新的环境变量？

1. 在 `.env.example` 和 `.env` 中添加
2. 在 `internal/config/config.go` 中添加对应结构体字段和 `mapstructure` tag
3. 在 `setDefaults()` 中添加默认值

### Q: 和 Java 版本有什么区别？

| 方面 | Java 版 | Go 版 |
|------|---------|--------|
| 进程数 | 4 个独立进程 | 1 个进程 |
| 端口 | 8080, 8081, 8082, 8083 | 仅 8080 |
| 启动时间 | ~30s | ~1s |
| 内存占用 | ~1.5GB (4个JVM) | ~50MB |
| 部署 | 4 个 JAR | 1 个二进制 |
| 通信 | HTTP + Kafka | 内部调用 + Kafka |

### Q: Go 项目的依赖管理是什么？

`go.mod` 文件类似 Java 的 `pom.xml`，记录所有依赖。`go.sum` 类似 Maven 的 lock 文件，记录依赖哈希。

```bash
# 添加依赖
go get github.com/gin-gonic/gin@latest

# 清理未使用的依赖
go mod tidy

# 下载所有依赖到本地
go mod download
```

---

> 有问题？查看 [ARCHITECTURE.md](./ARCHITECTURE.md) 了解系统设计，或 [API_REFERENCE.md](./API_REFERENCE.md) 查看 API 文档。
