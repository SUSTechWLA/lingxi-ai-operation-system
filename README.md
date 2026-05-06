# 躺营AI自媒体运营助手 (Lingxi AI OS)

> **一句话介绍**：一个帮你管理自媒体内容创作和发布的智能助手。输入简单想法 → AI 帮你生成/润色内容 → 一键发布到多平台。

---

## 👋 这是什么？

**躺营AI自媒体运营助手** 是一个面向自媒体创作者的一站式内容管理平台。它帮你解决这些痛点：

| 场景 | 以前 | 现在 |
|------|------|------|
| **写标题** | 绞尽脑汁想标题 | 告诉 AI 你的想法，自动生成吸引人的标题 |
| **写简介** | 憋半天写不好 | AI 帮你润色，让文字更生动 |
| **配图视频** | 上传后要自己想文案 | 根据你上传的图片/视频，AI 分析后生成匹配的内容 |
| **素材管理** | 文件散落在文件夹里 | 上传到素材库统一管理、标签筛选、随时复用 |
| **合规检查** | 发完被平台警告 | AI 自动检测极限词/敏感词，提前规避风险 |
| **跨平台适配** | 每个平台手动调整格式 | 一键适配抖音、小红书、B站等平台的风格要求 |
| **多平台发布** | 每个平台手动复制粘贴 | 选好平台，一键提交发布任务 |

---

## 🚀 不需要懂技术也能使用

### 第一步：启动项目

如果你是**技术小白**，找技术人员帮你执行以下命令即可：

```bash
# 在终端执行（打开"终端"应用）
cd 项目目录
bash ./scripts/startup.sh
```

看到以下输出就说明启动成功了：
```
Backend:  http://localhost:8080
Frontend: http://localhost:3000
```

### 第二步：打开界面

1. 打开 Chrome 浏览器
2. 在地址栏输入：`http://localhost:3000`
3. 按回车键，即可看到主界面

### 第三步：开始使用

#### 功能 1：上传素材
- 点击"上传视频"或"上传图片"区域
- 选择你的视频或图片文件
- 支持 MP4、MOV、JPG、PNG 等常见格式

#### 功能 2：AI 生成内容
- 点击右侧"AI创作助手"区域中的**一键生成**按钮
- AI 会根据你的素材自动生成标题和简介
- 你也可以手动输入想法，再点击顶部的**AI 生成标题和简介**

#### 功能 3：AI 润色文字
- 在标题或简介输入框旁，点击 ✨**AI润色** 按钮
- AI 会让文字更生动、更吸引人

#### 功能 4：素材库管理
- 上传素材后，点击素材区右上角的 **素材库** 按钮
- 在侧边面板浏览已上传的图片和视频
- 支持按标签筛选，点击素材即可引用到当前编辑

#### 功能 5：内容生成工作台
- 上传素材后，展开右侧的**内容生成工作台**
- 选择目标平台（抖音、小红书等）和风格关键词
- 点击 **AI 智能生成**，一键生成适配平台风格的内容

#### 功能 6：AI 合规检查
- 提交发布时自动检测极限词和敏感词
- 提前提示违规内容，避免平台警告

#### 功能 7：选择发布平台
- 在右侧选择要发布的平台（抖音、小红书等）
- 已标注"开发中"的平台暂不可用

#### 功能 8：发布内容
- 确认标题和简介无误
- 点击**下一步：选择发布平台**或侧边栏的**一键发布**
- 任务创建成功后，内容即进入处理流程

#### 功能 9：AI 对话助手
- 在右侧面板切换到 **AI 对话助手** 标签
- 通过自然语言描述需求，AI 自动生成/修改内容
- 支持上传素材后直接对话：\"根据图片生成标题和简介\"
- 多轮对话自动记忆上下文，可持续优化内容

---

## ✨ 功能介绍（配图说明）

### 主界面布局

```
┌─────────────┬──────────────────────────────┬────────────────┐
│             │                              │                │
│  侧边导航   │     内容编辑区               │  AI助手面板    │
│             │   ┌──────────────────────┐   │  ┌──────────┐  │
│   · 创作发布│   │ 上传素材区域          │   │  │ AI对话    │  │
│   · 任务中心│   │ (图片/视频拖拽上传)    │   │  └──────────┘  │
│   · 设置    │   └──────────────────────┘   │  ┌──────────┐  │
│             │   ┌──────────────────────┐   │  │ AI创作    │  │
│             │   │ 标题 (可AI润色)       │   │  │ 助手      │  │
│             │   ├──────────────────────┤   │  │ · 一键生成 │  │
│             │   │ 简介 (可AI润色)       │   │  │ · 一键优化 │  │
│             │   ├──────────────────────┤   │  └──────────┘  │
│             │   │ 关键词输入            │   │  ┌──────────┐  │
│             │   └──────────────────────┘   │  │ 平台选择  │  │
│             │   ┌──────────────────────┐   │  │ 器        │  │
│             │   │ 操作按钮区            │   │  └──────────┘  │
│             │   └──────────────────────┘   │  ┌──────────┐  │
│             │                              │  │ 发布按钮  │  │
│             │                              │  └──────────┘  │
└─────────────┴──────────────────────────────┴────────────────┘
```

---

## 🎯 核心功能清单

### 已实现功能
- ✅ **素材上传与库管理**：拖拽上传图片/视频到素材库，支持标签筛选和快速引用
- ✅ **封面图上传**：支持上传内容封面图
- ✅ **AI 内容生成**：根据文字想法或上传的素材，自动生成标题和简介
- ✅ **AI 润色**：优化标题和简介的文字表达，支持标题和简介同时润色
- ✅ **AI 操作取消**：AI 调用时全屏遮罩+取消按钮，取消时自动终止后端任务并记录上下文
- ✅ **内容生成工作台**：选择目标平台和风格，AI 智能生成适配内容
- ✅ **AI 合规检查**：自动检测极限词、敏感词和平台违规风险
- ✅ **跨平台适配**：一键适配抖音、小红书、微博、B站等平台风格
- ✅ **多平台发布**：选择平台，提交发布任务
- ✅ **关键词管理**：标签式关键词输入
- ✅ **AI 对话助手**：多轮对话式内容创作，支持上传素材 → 自然语言描述 → AI 自动生成/修改标题、简介、关键词
- ✅ **内容清空**：一键清空所有输入内容
- ✅ **工具注册体系**：支持外部开发者通过 HTTP 注册自定义工具，AI 自动发现和调用
- ✅ **短视频智能创作**：视频上传 → 元数据提取 → 关键帧分析 → 音频转录 → 多模态大模型生成平台适配文案
- ✅ **沙箱隔离执行**：Bash/Python 工具通过 Rust gRPC 沙箱服务执行，资源隔离（内存/CPU/磁盘/PID 限制）

### 开发中功能
- 🔄 **电子桌面应用**：支持本地文件选择和系统托盘
- 🔄 **平台实际发布**：对接各平台 API 实现自动发布
- 🔄 **任务中心**：查看发布历史和状态

---

## 💻 技术架构（给开发者看）

### 系统结构

```
                   浏览器 (http://localhost:3000)
                           │
                    Vite 开发服务器 (代理 /api)
                           │
              ┌────────────▼───────────┐
              │    Go 后端 (端口 8080)   │
              │                         │
              │  ┌───────────────────┐  │
              │  │  Publish 发布模块   │  │ ← 用户直接使用的功能
              │  │  · 内容发布         │  │
              │  │  · AI 生成/润色     │  │
              │  │  · 内容生成工作台   │  │
              │  ├───────────────────┤  │
              │  │  Media 素材管理模块 │  │ ← 图片/视频管理
              │  │  · MinIO 对象存储  │  │
              │  │  · 素材库浏览筛选  │  │
              │  ├───────────────────┤  │
              │  │  Skill 对话助手模块 │  │ ← 多轮对话AI创作
              │  │  · 会话管理(Redis) │  │
              │  │  · 工具知识库      │  │
              │  │  · DAG 生成与执行  │  │
              │  └───────────────────┘  │
              │  ┌───────────────────┐  │
              │  │  Orchestrator 调度  │  │ ← 任务调度引擎
              │  │  Worker 工具执行    │  │ ← 14个内置工具+外部工具
              │  │  Context 审计记录   │  │ ← 记录操作历史
              │  └───────────────────┘  │
              │            │            │
              │     ┌──────▼──────┐     │
              │     │ Rust 沙箱   │     │ ← gRPC 隔离执行
              │     │ (端口 50051) │     │
              │     └─────────────┘     │
              └────────────┬────────────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
   PostgreSQL      Redis       Redpanda      MinIO       Qdrant
   (数据存储)      (缓存)     (事件消息)   (对象存储)   (向量数据库)
```

### 技术栈

| 组件 | 技术 | 做什么 |
|------|------|--------|
| 前端 | React + TypeScript + TailwindCSS | 用户操作界面 |
| 后端 | Go + Gin 框架 | 处理所有业务逻辑 |
| 沙箱 | Rust + gRPC (tonic) | 隔离执行 Bash/Python 工具 |
| 数据库 | PostgreSQL 16 | 存储用户数据和任务 |
| 缓存 | Redis 7 | 会话缓存 + 工具知识库 |
| 消息队列 | Redpanda (Kafka 兼容) | 模块间事件通信 |
| 对象存储 | MinIO | 素材文件存储 |
| 向量数据库 | Qdrant | 向量检索（预留） |
| AI 能力 | OpenAI 兼容 API | 内容生成和润色 |

### 目录结构

```
lingxi-ai-operation-system/
├── cmd/lingxi-ai-os/main.go      # ★ 后端启动入口
├── internal/                      # 后端代码
│   ├── publish/                   # ★ 发布模块（核心业务）
│   │   ├── handler/               #   HTTP 接口
│   │   └── service/               #   业务逻辑
│   ├── media/                     # ★ 素材管理模块
│   │   ├── handler.go             #   素材 CRUD 接口
│   │   ├── service.go             #   业务逻辑
│   │   └── storage.go             #   MinIO 对象存储
│   ├── skill/                      #   AI 对话助手模块
│   │   ├── handler/                 #     HTTP 接口（会话/对话/进度）
│   │   ├── service/                 #     业务逻辑（Plan/Result/Session）
│   │   └── prompts/                 #     LLM 系统提示词
│   ├── orchestrator/              #   任务调度引擎
│   ├── worker/                    #   工具执行引擎
│   │   └── tool/builtin/          #   内置工具（14个）
│   │       ├── builtin.go          #     LLM API 调用
│   │       ├── bash_tool.go        #     Shell 命令执行（沙箱）
│   │       ├── python_tool.go      #     Python 执行（沙箱）
│   │       ├── polisher_tool.go    #     文本润色
│   │       ├── media_analyzer.go   #     素材分析
│   │       ├── content_generator.go #    内容生成
│   │       ├── content_checker.go  #     合规检查
│   │       ├── platform_adapter.go #    平台适配
│   │       ├── chat_generate_tool.go #  对话式内容生成
│   │       ├── chat_revise_tool.go #    对话式内容修改
│   │       ├── external_tool.go    #     外部工具代理
│   │       ├── video_metadata.go   #     视频元数据提取
│   │       ├── video_analyzer.go   #     视频关键帧+音频转录
│   │       └── video_copy_generator.go # 视频文案生成
│   ├── translator/                #   自然语言翻译
│   ├── context/                   #   上下文审计
│   ├── config/                    #   配置管理
│   ├── database/                  #   数据库连接
│   ├── eventbus/                  #   消息队列
│   └── outbox/                    #   事件可靠性保障
├── sandbox/                       #   Rust 沙箱服务（gRPC 隔离执行）
│   ├── src/                       #     沙箱主逻辑
│   └── proto/                     #     Protobuf 定义
├── frontend/                      # ★ 前端代码
│   └── src/
│       ├── components/            #   界面组件
│       │   ├── UploadCard.tsx     #     上传素材卡片
│       │   ├── TitleInput.tsx     #     标题输入
│       │   ├── DescriptionInput.tsx#    简介输入
│       │   ├── KeywordInput.tsx   #     关键词标签输入
│       │   ├── BlockingOverlay.tsx   #     AI 操作全屏遮罩
│       │   ├── AIHelperPanel.tsx     #     AI 助手面板
│       │   ├── AIAssistantTab.tsx    #     AI 对话式创作面板
│       │   ├── ContentTypeSelector.tsx #   内容类型选择
│       │   ├── MediaLibraryPanel.tsx #     素材库浏览面板
│       │   ├── PlatformSelector.tsx  #   平台选择器
│       │   ├── PublishButton.tsx     #     发布按钮
│       │   ├── Sidebar.tsx           #     侧边导航
│       │   ├── DesktopToolbar.tsx    #     Electron 桌面工具栏
│       │   ├── CommandPanel.tsx      #     命令面板
│       │   └── index.ts              #     组件统一导出
│       ├── pages/
│       │   └── PublishPage.tsx    #   主页面
│       ├── services/api.ts        #   API 调用
│       ├── stores/appStore.ts     #   状态管理（含 cover、aiLoadingMessage、chatSessionId）
│       └── utils/
│           ├── types.ts           #   类型定义（含 PolishSubmitData、PolishQueryData、Chat 类型）
│           └── electron.ts        #   Electron 工具函数
├── docs/                          # 文档
├── scripts/                       # 启动脚本
└── docker-compose.yml             # 基础设施容器
```

---

## 🛠️ 快速开发指南

### 环境要求

| 工具 | 版本要求 | 安装方法 |
|------|---------|---------|
| Go | 1.23+ | `brew install go`（Mac）或访问 [go.dev](https://go.dev/dl/) |
| Node.js | 18+ | `brew install node` 或访问 [nodejs.org](https://nodejs.org/) |
| Docker | 20+ | `brew install --cask docker` 或访问 [docker.com](https://www.docker.com/) |

### 一键启动

```bash
# 1. 进入项目目录
cd lingxi-ai-operation-system

# 2. 配置环境变量
cp .env.example .env
# 编辑 .env 文件，填入你的 OPENAI_API_KEY

# 3. 一键启动（会启动所有服务）
bash ./scripts/startup.sh
```

### 分步启动

```bash
# 终端 1：启动基础设施（PostgreSQL、Redis、消息队列等）
docker compose up -d

# 终端 2：构建并启动后端（端口 8080）
make run

# 终端 3：启动前端开发服务器（端口 3000）
cd frontend && npm install && npm run dev
```

### 常用命令

```bash
make build    # 构建后端
make run      # 构建并运行
make test     # 运行测试
make tidy     # 整理依赖
make fmt      # 格式化代码
```

### API 测试

```bash
# 一键测试所有 API
bash ./scripts/test-apis.sh

# 或逐个测试
curl http://localhost:8080/api/health                    # 健康检查
curl -X POST http://localhost:8080/api/ai/generate \      # AI 生成
  -H "Content-Type: application/json" \
  -d '{"prompt":"周末去哪儿玩"}'
curl http://localhost:8080/api/tools                     # 查看所有工具
```

---

## 📚 文档索引

| 文档 | 适合谁 | 内容 |
|------|--------|------|
| **[ARCHITECTURE.md](docs/ARCHITECTURE.md)** | 开发者 | 系统架构、模块设计、数据流 |
| **[API_REFERENCE.md](docs/API_REFERENCE.md)** | 开发者/测试 | 所有 API 接口详细说明 |
| **[TOOL_DEVELOPMENT_GUIDE.md](docs/TOOL_DEVELOPMENT_GUIDE.md)** | 外部工具开发者 | 工具开发对接指南（注册、Manifest、执行契约） |
| **[ONBOARDING.md](docs/ONBOARDING.md)** | 新开发者 | 从零上手项目开发 |

### 其他文档
- `docs/AI_ASSISTANT_DESIGN.md` - AI 对话助手设计文档
- `docs/SANDBOX_INTEGRATION_GUIDE.md` - 沙箱集成指南

---

## ❓ 常见问题

### 我不会技术，能用这个项目吗？
项目的界面在浏览器中运行，操作方式类似普通网站。但**需要技术人员先帮你完成一次启动**。启动后你只需打开浏览器使用即可。

### AI 功能需要付费吗？
AI 功能依赖 OpenAI 兼容的 API 服务，你需要自行获取 API Key 并配置在 `.env` 文件中。API 服务通常需要付费，但很多平台提供免费额度。

### 支持哪些自媒体平台？
目前支持：**抖音**、**小红书**、**B站**、**微博**、**快手**、**微信视频号**、**YouTube**（通过 AI 平台适配功能）。实际自动发布 API 对接正在开发中。

### 上传的文件会保存到哪里？
文件上传到 MinIO 对象存储服务（Docker 容器内），元数据存储在 PostgreSQL 中。MinIO 提供可靠的持久化存储。

### 数据存储在哪里？
所有数据存储在本地 PostgreSQL 数据库中（Docker 容器内）。数据库文件在 Docker 数据卷中。

### 如何关闭服务？
按 `Ctrl+C` 可以停止所有服务。要完全清理，执行 `docker compose down`。

---

## 📝 环境变量说明

配置文件 `.env` 中的关键设置：

| 变量 | 必填 | 说明 |
|------|------|------|
| `OPENAI_API_KEY` | **是** | AI 服务的 API 密钥 |
| `OPENAI_BASE_URL` | 否 | AI 服务地址（默认 OpenAI） |
| `OPENAI_MODEL` | 否 | 使用的 AI 模型 |
| `SERVER_PORT` | 否 | 后端端口（默认 8080） |
| `POSTGRES_PASSWORD` | 否 | 数据库密码 |

---

## 📄 License

MIT License
