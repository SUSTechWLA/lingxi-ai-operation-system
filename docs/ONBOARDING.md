# 灵犀AI OS - 新开发者上手指南

> 本文档面向**新加入的开发者**，帮助你从零理解项目并快速上手开发。

---

## 目录

1. [Go 语言速成](#1-go-语言速成)
2. [React 前端速成](#2-react-前端速成)
3. [项目总览](#3-项目总览)
4. [项目结构详解](#4-项目结构详解)
5. [核心概念](#5-核心概念)
6. [从零启动项目](#6-从零启动项目)
7. [代码阅读路线](#7-代码阅读路线)
8. [如何开发新功能](#8-如何开发新功能)
9. [前端开发指南](#9-前端开发指南)
10. [完整 API 测试](#10-完整-api-测试)
11. [常见问题](#11-常见问题)

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
| 访问控制 | `public`, `private`, `protected` | 首字母大写 = 公开, 小写 = 私有 |
| 初始化 | 构造函数 | 工厂函数 `func NewFoo() *Foo` |
| JSON | Jackson `@JsonProperty` | struct tag `` `json:"name"` `` |
| HTTP | Spring `@RestController` + `@RequestMapping` | Gin `r.GET("/path", handler)` |

### 1.3 Go 代码示例

```go
package main

import "fmt"

// 定义结构体（类似 Java 的 class）
type User struct {
    Name string  // 大写开头 = 公开（public）
    Age  int
}

// 方法（类似 Java 的实例方法）
func (u *User) Greet() string {
    return fmt.Sprintf("Hello, I'm %s, age %d", u.Name, u.Age)
}

// 工厂函数
func NewUser(name string, age int) *User {
    return &User{Name: name, Age: age}
}

func main() {
    user := NewUser("Alice", 30)
    fmt.Println(user.Greet())
}
```

### 1.4 Go 习惯用法

```go
// 错误处理
result, err := someFunc()
if err != nil {
    return fmt.Errorf("wrapped: %w", err)
}

// goroutine 并发
go func() {
    doSomething()
}()

// context 取消
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// defer 延迟执行（类似 Java finally）
file, _ := os.Open("file.txt")
defer file.Close()
```

---

## 2. React 前端速成

### 2.1 前置知识

如果你熟悉以下概念，前端部分就容易上手：
- **JavaScript/TypeScript**：和 Java 类似，但更灵活
- **组件**：类似 HTML 标签，但可以包含逻辑和样式
- **Props**：组件的输入参数（类似 Java 的方法参数）
- **State**：组件的内部状态（类似 Java 的类字段）
- **Hooks**：在函数组件中使用状态和其他 React 特性的函数

### 2.2 基本概念

```tsx
// 组件 = 函数 + 返回值（JSX 语法，类似 HTML）
interface Props {
  name: string        // 组件的输入参数
}

// 函数组件
const Greeting: React.FC<Props> = ({ name }) => {
  const [count, setCount] = useState(0)  // State Hook

  return (
    <div>
      <h1>Hello, {name}!</h1>
      <p>Count: {count}</p>
      <button onClick={() => setCount(c => c + 1)}>+1</button>
    </div>
  )
}
```

### 2.3 项目中使用的核心技术

| 技术 | 用途 | 类似 Java 中的 |
|------|------|---------------|
| React 函数组件 | UI 构建 | JSP/Thymeleaf 模板 |
| TypeScript | 类型安全 | 编译期类型检查 |
| Zustand | 全局状态管理 | Spring Singleton Bean |
| Axios | HTTP 请求 | RestTemplate / WebClient |
| TailwindCSS | 样式 | CSS（无需额外框架） |
| Vite | 构建 + 开发服务器 | Maven + Tomcat |

### 2.4 TypeScript 速览

```typescript
// 类型定义（类似 Java 的类）
interface User {
  id: string
  name: string
  age?: number        // ? 表示可选
}

// 联合类型
type Status = 'success' | 'error' | 'loading'

// 泛型
interface ApiResponse<T> {
  code: number
  data: T
}
```

---

## 3. 项目总览

### 3.1 灵犀 AI OS 是什么

灵犀 AI 自媒体运营助手是一个**一站式自媒体内容管理平台**：

```
用户使用场景：
1. 用户上传图片/视频素材
2. 输入创作想法或使用 AI 生成内容
3. AI 润色标题和简介
4. 选择发布平台
5. 一键提交发布任务
```

### 3.2 系统架构

```
用户浏览器 (React SPA)
         │
    Vite Proxy (/api/* → localhost:8080)
         │
    ┌────▼────────────────────────────┐
    │        Go 后端 (端口 8080)        │
    │                                  │
    │  ┌──────────┐                    │
    │  │ Publish   │ ← 你主要开发的模块   │
    │  │ 发布服务   │                    │
    │  ├──────────┤                    │
    │  │ Orchestr │ ← 任务调度引擎       │
    │  │ 调度服务   │                    │
    │  ├──────────┤                    │
    │  │ Worker    │ ← 工具执行引擎      │
    │  │ 执行服务   │                    │
    │  ├──────────┤                    │
    │  │ Translator│ ← 自然语言翻译      │
    │  │ 翻译服务   │                    │
    │  └──────────┘                    │
    └──────────────────────────────────┘
```

### 3.3 技术栈

| 组件 | 技术 | 说明 |
|------|------|------|
| Web 框架 | [Gin](https://gin-gonic.com/) | Go 最流行的 HTTP 框架 |
| 数据库 | [pgx](https://github.com/jackc/pgx) | 原生 PostgreSQL 驱动 |
| 缓存 | [go-redis](https://github.com/redis/go-redis) | Redis 官方 Go 客户端 |
| 消息队列 | [IBM/sarama](https://github.com/IBM/sarama) | Kafka 兼容客户端 |
| 前端 | React 18 + TypeScript + Vite | 用户界面 |
| 样式 | TailwindCSS 3 | 无需写 CSS |
| 状态管理 | Zustand | 轻量级状态管理 |
| HTTP 客户端 | Axios | 前端 HTTP 请求 |
| 配置 | Viper | .env 文件管理 |
| 日志 | Zap | 高性能结构化日志 |

---

## 4. 项目结构详解

```
lingxi-ai-operation-system/
│
├── cmd/lingxi-ai-os/
│   └── main.go                    # ★ 启动入口，所有组件在此组装
│
├── internal/                      # 后端代码
│   ├── config/config.go           # 配置加载（从 .env 读取）
│   ├── logger/logger.go           # 日志初始化
│   ├── database/database.go       # 数据库连接 + 迁移
│   ├── redis/redis.go             # Redis 连接
│   ├── eventbus/eventbus.go       # Kafka 生产者/消费者
│   ├── outbox/relay.go            # Outbox 发件箱模式
│   ├── model/
│   │   ├── model.go               # Task, Node 等数据模型
│   │   └── repository/            # 数据库 CRUD 操作
│   │
│   ├── publish/                   # ★ 用户发布模块（主要开发模块）
│   │   ├── handler/
│   │   │   ├── handler.go         #   发布/AI生成/AI润色接口
│   │   │   └── trace_handler.go   #   任务追踪查询接口
│   │   ├── service/
│   │   │   └── service.go         #   发布/AI生成/AI润色业务逻辑
│   │   └── handler_test.go        #   单元测试
│   │
│   ├── orchestrator/              # 任务调度引擎
│   │   ├── service/
│   │   │   ├── orchestrator.go    #   任务创建、DAG 提交
│   │   │   ├── state.go           #   状态转换服务
│   │   │   ├── statemachine.go    #   节点状态机
│   │   │   ├── dependency_checker.go  # 依赖检查 + 条件分支
│   │   │   ├── scheduler.go       #   兜底恢复调度
│   │   │   ├── dag_validator.go   #   DAG 验证
│   │   │   └── retry_policy.go    #   重试策略
│   │   └── handler/handler.go     #   HTTP 接口
│   │
│   ├── worker/                    # 工具执行引擎
│   │   ├── service/executor.go    #   节点执行器
│   │   └── tool/
│   │       ├── tool.go            #   Tool 接口 + 注册表
│   │       └── builtin/
│   │           ├── bash_tool.go   #   Bash 沙箱工具
│   │           ├── polisher_tool.go #   文本润色工具
│   │           └── builtin.go     #   LLM API 工具
│   │
│   ├── translator/                # 自然语言翻译
│   └── context/                   # 上下文审计
│
├── frontend/                      # ★ 前端代码
│   ├── src/
│   │   ├── App.tsx                #   主入口
│   │   ├── components/            #   UI 组件
│   │   │   ├── UploadCard.tsx     #     上传素材（拖拽/点击）
│   │   │   ├── TitleInput.tsx     #     标题输入 + AI 润色
│   │   │   ├── DescriptionInput.tsx #   简介输入 + AI 润色
│   │   │   ├── KeywordInput.tsx   #     关键词标签输入
│   │   │   ├── AIHelperPanel.tsx  #     AI 助手面板
│   │   │   ├── PlatformSelector.tsx #  平台选择
│   │   │   ├── PublishButton.tsx  #     发布按钮
│   │   │   ├── Sidebar.tsx        #     侧边导航
│   │   │   ├── DesktopToolbar.tsx #     Electron 桌面工具栏
│   │   │   └── CommandPanel.tsx   #     命令面板
│   │   │   # （PublishPage.tsx 内嵌UI）
│   │   │   # - AI 加载遮罩：全屏进度条+spinner动画
│   │   │   # - 结果弹窗：成功/失败居中弹窗，2.5s自动消失
│   │   │   # - 调试追踪按钮：右下角浮动，点击查询最近任务链路
│   │   ├── pages/
│   │   │   └── PublishPage.tsx    #   创作发布主页面
│   │   ├── services/api.ts        #   Axios API 调用封装
│   │   ├── stores/appStore.ts     #   Zustand 状态管理
│   │   └── utils/
│   │       ├── types.ts           #   类型定义
│   │       └── electron.ts        #   Electron 工具函数
│   └── vite.config.ts             #   Vite 配置（代理等）
│
├── docs/                          # 文档
├── scripts/
│   ├── startup.sh                 # 一键启动脚本
│   ├── test-apis.sh               # API 测试脚本
│   └── install_lingxi_env.sh      # 环境安装脚本
├── docker-compose.yml             # 基础设施容器
├── Makefile                       # 常用命令
├── go.mod / go.sum                # Go 依赖
├── .env.example                   # 环境变量模板
└── CLAUDE.md                      # Claude Code 项目指引
```

### 后端文件命名规则

- `model.go` - 数据结构定义
- `repository.go` - 数据库操作
- `service.go` / 功能名如 `state.go` - 业务逻辑
- `handler.go` - HTTP 接口处理
- `*_test.go` - 测试文件

---

## 5. 核心概念

### 5.1 用户操作流程

用户在前端的使用流程如下：

```
1. 上传素材 ──→ 图片/视频拖拽到上传区域
      │
2. 生成内容 ──→ 点击"AI 生成标题和简介"
      │         ├── 有素材 → 调用 /api/ai/generate-from-media
      │         └── 无素材 → 调用 /api/ai/generate
      │
3. 润色内容 ──→ 点击标题或简介旁的"AI润色"按钮
      │         调用 /api/ai/polish
      │
4. 查天气 ────→ 在右侧天气面板输入城市名
      │         调用 /api/weather/query
      │         点击"生成天气内容"自动填充
      │
5. 选平台 ────→ 勾选要发布的平台
      │
6. 发布 ──────→ 点击"一键发布"
                 调用 /api/publish
                 后端创建 DAG 任务并执行
```

### 5.2 DAG 任务图

DAG（有向无环图）是本系统的核心抽象：

```
  [节点A: 写文章] ──→ [节点B: 总结] ──→ [节点C: 发布]
```

- **节点 (Node)**：一个执行单元
- **边 (Edge)**：依赖关系，`from → to` 表示 to 依赖 from
- **条件 (Condition)**：如 `"nodeA.status == success"`，不满足则 SKIPPED

### 5.3 前端的组件状态管理

使用 Zustand 管理全局状态：

```typescript
// Zustand Store ≈ 全局的 Java Service 类
const store = useAppStore()
store.title          // 读取标题
store.setTitle(x)    // 更新标题
store.images         // 读取已上传的图片
store.addImages(f)   // 添加图片
```

### 5.4 前端 API 调用

通过 Axios 封装的 API 函数调用后端：

```typescript
// api.ts 中的函数
const result = await aiGenerateContent("周末去哪儿玩")
// → POST /api/ai/generate { prompt: "周末去哪儿玩" }
// → 返回 { title: "...", description: "..." }
```

---

## 6. 从零启动项目

### 6.1 前置条件

| 工具 | 最低版本 | 安装 |
|------|---------|------|
| Go | 1.23+ | `brew install go` |
| Node.js | 18+ | `brew install node` |
| Docker | 20+ | `brew install --cask docker` |

### 6.2 一键启动

```bash
# 克隆项目
git clone <repo-url>
cd lingxi-ai-operation-system

# 配置 API Key（必须）
cp .env.example .env
# 编辑 .env，设置 OPENAI_API_KEY

# 一键启动
bash ./scripts/startup.sh
```

### 6.3 分步启动

```bash
# 终端 1：启动基础设施
docker compose up -d

# 终端 2：构建并运行后端
make run

# 终端 3：启动前端
cd frontend && npm install && npm run dev
```

### 6.4 环境变量说明

| 变量 | 必填 | 说明 |
|------|------|------|
| `OPENAI_API_KEY` | **是** | LLM API 密钥 |
| `OPENAI_BASE_URL` | 否 | LLM API 地址 |
| `OPENAI_MODEL` | 否 | 使用的模型（默认 doubao） |
| `SERVER_PORT` | 否 | 后端端口（默认 8080） |
| `POSTGRES_PASSWORD` | 否 | 数据库密码 |

---

## 7. 代码阅读路线

### 后端阅读顺序

```
第 1 站：Publish 模块（最常用）
  internal/publish/handler/handler.go
  internal/publish/service/service.go
  → 理解 AI 生成、润色、发布的业务流程

第 2 站：程序入口
  cmd/lingxi-ai-os/main.go
  → 看清所有组件如何组装

第 3 站：数据模型
  internal/model/model.go
  → Task, Node 等核心数据结构

第 4 站：Orchestrator 调度引擎
  internal/orchestrator/service/orchestrator.go
  internal/orchestrator/service/state.go
  internal/orchestrator/service/statemachine.go

第 5 站：Worker 工具执行
  internal/worker/tool/tool.go
  internal/worker/tool/builtin/polisher_tool.go

第 6 站：HTTP 路由注册
  internal/orchestrator/handler/handler.go
```

### 前端阅读顺序

```
第 1 站：主页面
  frontend/src/pages/PublishPage.tsx
  → 理解页面布局和组件组合

第 2 站：API 层
  frontend/src/services/api.ts
  → 所有后端 API 调用

第 3 站：状态管理
  frontend/src/stores/appStore.ts
  → 全局状态

第 4 站：各个组件
  frontend/src/components/
  → 逐个查看组件实现
```

---

## 8. 如何开发新功能

### 8.1 添加新的后端 API

**示例**：在 publish 模块下添加一个内容分类接口。

**第 1 步**：在 service 中添加方法

```go
// internal/publish/service/service.go
func (s *PublishService) CategorizeContent(ctx context.Context, content string) (string, error) {
    systemPrompt := "你是一个内容分类专家。请将以下内容分类为：科技、美食、旅行、娱乐、教育。只返回分类名称。"
    return s.callOpenAI(ctx, systemPrompt, content)
}
```

**第 2 步**：在 handler 中添加路由

```go
// internal/publish/handler/handler.go
func (h *PublishHandler) CategorizeContent(c *gin.Context) {
    var req struct {
        Content string `json:"content" binding:"required"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
        return
    }
    category, err := h.publishService.CategorizeContent(c.Request.Context(), req.Content)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": gin.H{"category": category}})
}
```

**第 3 步**：注册路由

```go
func (h *PublishHandler) RegisterRoutes(r *gin.Engine) {
    api := r.Group("/api")
    {
        api.POST("/publish", h.PublishContent)
        api.POST("/ai/categorize", h.CategorizeContent)  // ← 新增
        // ...
    }
}
```

### 8.2 添加新的内置工具

**示例**：添加一个 HTTP 请求工具（参考 `bash_tool.go` 作为最简模板）。

```go
// internal/worker/tool/builtin/http_tool.go
package builtin

import (
    "context"
    "github.com/lingxi-ai/lingxi-ai-operation-system/internal/worker/tool"
)

type HttpTool struct{}

func NewHttpTool() *HttpTool { return &HttpTool{} }

func (t *HttpTool) Name() string             { return "http" }
func (t *HttpTool) Description() string       { return "Send HTTP GET requests" }
func (t *HttpTool) Type() tool.ToolType       { return tool.ToolTypeCustom }
func (t *HttpTool) Execute(ctx context.Context, params map[string]interface{}, toolCtx tool.ToolContext) tool.ToolResult {
    url, _ := params["url"].(string)
    if url == "" {
        return tool.FailureResult("url is required")
    }
    // ... 执行 HTTP 请求 ...
    return tool.SuccessResult(map[string]interface{}{
        "statusCode": 200,
        "body":       "response body",
    })
}
func (t *HttpTool) ValidateParameters(params map[string]interface{}) bool {
    _, ok := params["url"].(string)
    return ok
}
```

然后在 `cmd/lingxi-ai-os/main.go` 注册：

```go
toolRegistry.Register(builtin.NewHttpTool())  // ← 新增
```

### 8.3 添加新的前端组件

参考已有的 `AIHelperPanel.tsx` 模式。详见下面的第 9 节。

### 8.4 运行测试

```bash
# 全部测试
go test ./... -v

# 指定包测试
go test ./internal/publish/... -v

# 覆盖率
go test ./... -cover
```

### 8.5 开发常用命令

```bash
go fmt ./...     # 格式化代码
go vet ./...     # 代码检查
go mod tidy      # 整理依赖
make build       # 构建
make run         # 构建并运行
make test        # 运行测试
```

---

## 9. 前端开发指南

### 9.1 前端目录结构

```
frontend/src/
├── App.tsx                  # 主入口：布局 + 服务状态检查
├── main.tsx                 # React 挂载点
├── index.css                # 全局样式 + TailwindCSS 导入
│
├── components/              # UI 组件
│   ├── Sidebar.tsx          #   侧边导航
│   ├── UploadCard.tsx       #   文件上传（拖拽/点击）
│   ├── TitleInput.tsx       #   标题输入框
│   ├── DescriptionInput.tsx #   简介文本域
│   ├── KeywordInput.tsx     #   关键词标签输入
│   ├── AIHelperPanel.tsx    #   AI 助手面板
│   ├── PlatformSelector.tsx #   平台选择器
│   ├── PublishButton.tsx    #   发布按钮
│   ├── DesktopToolbar.tsx   #   桌面工具栏
│   ├── CommandPanel.tsx     #   命令面板
│   └── index.ts             #   统一导出
│
├── pages/
│   └── PublishPage.tsx      # 创作发布页面
│
├── services/
│   └── api.ts               # Axios API 封装
│
├── stores/
│   └── appStore.ts          # Zustand 状态管理
│
└── utils/
    ├── types.ts             # TypeScript 类型定义
    └── electron.ts          # Electron 检测和 API
```

### 9.2 添加新前端组件的步骤

**第 1 步**：在 `frontend/src/components/` 创建组件文件

```tsx
// frontend/src/components/CategorySelector.tsx
import React, { useState } from 'react'

interface CategorySelectorProps {
  onSelect: (category: string) => void
}

const categories = ['科技', '美食', '旅行', '娱乐', '教育']

const CategorySelector: React.FC<CategorySelectorProps> = ({ onSelect }) => {
  const [selected, setSelected] = useState('')

  return (
    <div className="p-4 border rounded-xl">
      <h3 className="text-sm font-medium text-gray-700 mb-3">内容分类</h3>
      <div className="flex flex-wrap gap-2">
        {categories.map(cat => (
          <button
            key={cat}
            onClick={() => { setSelected(cat); onSelect(cat) }}
            className={`px-3 py-1.5 rounded-lg text-sm transition-colors ${
              selected === cat
                ? 'bg-primary text-white'
                : 'bg-gray-100 text-gray-600 hover:bg-gray-200'
            }`}
          >
            {cat}
          </button>
        ))}
      </div>
    </div>
  )
}

export default CategorySelector
```

**第 2 步**：在需要的页面中引入

```tsx
// PublishPage.tsx
import CategorySelector from '../components/CategorySelector'

// 在 JSX 中使用
<CategorySelector onSelect={(cat) => console.log(cat)} />
```

### 9.3 前端状态管理

使用 Zustand 管理全局状态：

```typescript
// 读取状态
const title = useAppStore((state) => state.title)
const images = useAppStore((state) => state.images)

// 更新状态
const setTitle = useAppStore((state) => state.setTitle)
setTitle('新的标题')

// 如果需要添加新的全局状态：
// 在 appStore.ts 的 AppState 接口中添加字段
// 在 create 调用中添加对应的值和 setter
```

### 9.4 前端调用 API

在 `api.ts` 中添加新函数，然后在组件中调用：

```typescript
// services/api.ts
export const categorizeContent = async (content: string): Promise<string> => {
  const response = await api.post<ApiResponse<{ category: string }>>('/ai/categorize', { content })
  return response.data.data.category
}

// 在组件中使用
const handleCategorize = async () => {
  const category = await categorizeContent("一些内容")
  showNotification(`分类结果: ${category}`)
}
```

### 9.5 样式说明

项目使用 TailwindCSS，无需写自定义 CSS：

```tsx
// TailwindCSS 类名说明
<div className="
  p-4          // padding: 16px
  bg-white     // 背景白色
  rounded-xl   // 圆角
  shadow-lg    // 大阴影
  border       // 边框
  border-gray-100 // 边框颜色
  hover:bg-gray-50 // 鼠标悬停效果
">
```

主题颜色在 `tailwind.config.js` 中定义：

```javascript
colors: {
  primary: {
    DEFAULT: '#7C5CFF',  // 主色（紫色）
    light: '#9B82FF',
    dark: '#6645E0',
  },
}
```

---

## 10. 完整 API 测试

### 10.1 基本测试

```bash
# 健康检查
curl http://localhost:8080/api/health

# AI 生成内容
curl -X POST http://localhost:8080/api/ai/generate \
  -H "Content-Type: application/json" \
  -d '{"prompt":"周末去哪儿玩"}'

# AI 润色
curl -X POST http://localhost:8080/api/ai/polish \
  -H "Content-Type: application/json" \
  -d '{"text":"今天天气很好","type":"description"}'

# 天气查询
curl "http://localhost:8080/api/weather/query?city=北京"

# 从媒体文件生成
curl -X POST http://localhost:8080/api/ai/generate-from-media \
  -F "prompt=风景" \
  -F "images=@photo.jpg"

# 发布内容
curl -X POST http://localhost:8080/api/publish \
  -F "title=测试发布" \
  -F "description=测试内容" \
  -F "keywords=测试" \
  -F 'platforms=["douyin"]'
```

### 10.2 DAG 任务创建和提交流程

```bash
# 1. 创建任务
TASK_ID=$(curl -s -X POST http://localhost:8080/api/task/create \
  -H "Content-Type: application/json" \
  -d '{"userId":"test"}' | python3 -c "import sys,json;print(json.load(sys.stdin).get('taskId',''))")

# 2. 提交 DAG
curl -X POST "http://localhost:8080/api/task/${TASK_ID}/dag" \
  -H "Content-Type: application/json" \
  -d '{
    "nodes": [
      {"id": "n1", "type": "TOOL", "name": "weather", "input": {"city": "北京"}},
      {"id": "n2", "type": "LLM", "name": "summary", "input": {"prompt": "根据天气生成出行建议"}}
    ],
    "edges": [
      {"from": "n1", "to": "n2"}
    ]
  }'

# 3. 查看任务状态
curl "http://localhost:8080/api/task/${TASK_ID}"
```

### 10.3 API 测试脚本

项目提供一键测试脚本：

```bash
bash ./scripts/test-apis.sh
```

---

## 11. 常见问题

### Q: Go 编译报错
```bash
go mod tidy   # 重新整理依赖
```

### Q: 数据库连接失败
```bash
docker compose up -d           # 启动 PostgreSQL
docker ps                      # 检查容器状态
```

### Q: 前端页面打不开
```bash
cd frontend && npm install && npm run dev  # 启动前端
```

### Q: AI 功能不工作
检查 `.env` 中的 `OPENAI_API_KEY` 是否配置正确。

### Q: 如何添加 API 测试
编辑 `scripts/test-apis.sh`，使用 `test_api` 函数添加新测试。

### Q: 如何调试前端
在 Chrome 中按 F12 打开开发者工具 → Console 查看日志 → Network 查看 API 请求。

---

> 更多信息查看 [ARCHITECTURE.md](./ARCHITECTURE.md) 和 [API_REFERENCE.md](./API_REFERENCE.md)。
