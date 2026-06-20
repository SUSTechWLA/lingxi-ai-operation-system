# AIOS 通用智能体编排平台

> **一句话介绍**：Go 单体应用，端口 8080，模块化架构支持 DAG 工作流编排、多工具协作、人工审核节点和外部工具注册。
>
> 🆕 **v2.0** — 新增视频创作子系统：AIGC 镜头式视频 + 文字口播可视化视频，支持 Skill Package 热加载、Model Gateway 统一治理、WorkflowRun 阶段审核与局部重跑。

---

## 🚀 快速开始

### 一键启动

```bash
cd aios-core && cp .env.example .env
# 编辑 .env — 填入 OPENAI_API_KEY

./aios-core/scripts/startup.sh    # infra → build backend → run backend
```

打开浏览器访问 `http://localhost:3000`

### 启用视频创作功能

```bash
# .env 中设置
VIDEO_CREATION_ENABLED=true
MODEL_PROVIDER_MODE=fake    # 测试用 fake，生产用 real
```

---

## ✨ 核心功能

### 内容发布（v1 已有）
- ✅ 素材上传与库管理（MinIO + 标签筛选）
- ✅ AI 内容生成/润色/合规检查
- ✅ 跨平台适配（抖音、小红书、B站等 7 平台）
- ✅ AI 对话助手（多轮对话 + DAG 规划）
- ✅ 短视频智能创作（元数据提取 → 关键帧分析 → 音频转录 → 多模态文案生成）
- ✅ 沙箱隔离执行（Rust gRPC，资源限制）
- ✅ Electron 桌面应用（.dmg/.exe）

### 视频创作（🆕 v2.0）
- ✅ **AIGC 镜头式视频**：剧本生成 → 角色/场景设计 → Shot 拆解 → 分镜 → 关键帧 Prompt → 视频生成 → 逐 Shot 审核
- ✅ **文字口播可视化视频**：观点输入 → 口播稿 → VisualBeat → 组件 DSL → HyperGenKeyframe Bundle → 本地渲染
- ✅ **Skill Package 系统**：`skills/{name}/{version}/` 目录热加载，版本锁定，健康检查
- ✅ **Model Gateway**：统一模型调用网关，Fake Provider 测试，fingerprint 幂等，指数退避重试
- ✅ **Workflow Run**：阶段状态追踪、人工审核（CONTROL 节点）、局部重跑、版本化 Artifact
- ✅ **Skill→Workflow 自动转换**：写好 `skill.yaml`，一键生成 DAG workflow template

---

## 🏗️ 架构

```
Electron / React Browser
        │ HTTPS + SSE
        ▼
Go AIOS Core (:8080)
├── internal/core/                 # 通用引擎（不绑定业务）
│   ├── orchestrator/              #   DAG 调度：状态机、依赖检查、重试
│   ├── worker/                    #   工具执行：内置14个工具 + 外部注册
│   │   └── executor/sandboxpb/    #     Rust gRPC 沙箱执行器
│   ├── workflow/                  #   工作流模板 + WorkflowRun + 🆕 Skill Compiler
│   ├── modelgateway/              #   🆕 统一模型网关（Provider/Fake/OpenAI）
│   ├── skillruntime/              #   🆕 Skill Package 加载/校验/注册
│   ├── artifact/                  #   🆕 版本化产物管理
│   ├── localrunner/               #   🆕 Electron 本地任务协议
│   ├── translator/                #   NL → DAG 翻译
│   ├── context/                   #   审计追踪
│   ├── outbox/                    #   事件可靠性保障
│   └── ...
├── internal/agents/               # 业务 Agent（视频、标书、发布、对话）
│   ├── video/                     #   🆕 视频创作领域（Project/Shot/Voice/Review）
│   ├── bid/                       #   标书生成（招标解析→章节生成→审核→导出）
│   ├── chat/                      #   AI 对话助手
│   └── publish/                   #   内容发布
├── skills/                        # 🆕 Skill Package 目录（6 个）
│   ├── create-opinion-videos/1.0.0/   # 口播/知识视频（8 stage，默认路由）
│   ├── aigc-shot-video/1.0.0/         # 镜头式 AIGC 短片（9 stage）
│   ├── video-creator/1.0.0/           # 导演级视频流水线（12 stage）
│   ├── film-shot-reconstruction/1.0.0/# 经典镜头拉片学习（10 stage）
│   ├── voice-post-production/1.0.0/   # 音频后期（4 stage）
│   └── voice-visual-video/1.0.0/      # 旧版口播可视化（8 stage，hidden）
├── cmd/tangying-ai-os/            # 主入口
├── cmd/skill2workflow/            # 🆕 CLI 转换工具
├── deploy/                        # 🆕 Docker 云端部署
└── sandbox/                       # Rust 沙箱服务
    ↓
PostgreSQL · Redis · Redpanda · MinIO · Qdrant
```

---

## 🛠️ 快速开发

### 环境要求
| 工具 | 版本 | 安装 |
|------|------|------|
| Go | 1.25+ | `brew install go` |
| Node.js | 18+ | `brew install node` |
| Docker | 20+ | `brew install --cask docker` |
| protoc（可选） | 3.x | `brew install protobuf` |

### 常用命令

```bash
cd aios-core

make build         # 编译后端
make run           # 编译+运行
make test          # 运行测试
go test -race ./...  # 全量测试 + 竞态检测

cd ../frontend
npm run dev        # 前端开发服务器 (:3000)
npm run build      # 前端生产构建

# 🆕 Skill → Workflow 转换
go run cmd/skill2workflow/main.go --skill skills/aigc-shot-video/1.0.0
go run cmd/skill2workflow/main.go --skill-root skills/ --output out/

# 🆕 Docker 云端部署
docker compose -f deploy/docker-compose.cloud.yml up -d
```

---

## 📡 API 概览

### 内容发布（v1）
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/publish` | 提交发布 |
| POST | `/api/ai/generate` | AI 生成内容 |
| POST | `/api/ai/polish` | AI 润色 |
| POST | `/api/media/upload` | 上传素材 |
| GET  | `/api/media/list` | 素材列表 |
| POST | `/api/chat/sessions/create` | 创建对话 |
| POST | `/api/chat/sessions/:id/chat` | 发送消息 |

### 任务调度
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/task/create` | 创建任务 |
| POST | `/api/task/:id/dag` | 提交 DAG |
| GET  | `/api/task/:id` | 查询任务 |
| GET  | `/api/trace/:taskId` | 审计追踪 |

### 工具注册
| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/api/tools` | 工具清单 |
| POST | `/api/tools/register` | 注册外部工具 |

### 工作流模板
| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/api/workflows` | 模板列表 |
| POST | `/api/workflows` | 创建模板 |
| POST | `/api/workflows/:id/instantiate` | 实例化 |

### 🆕 视频创作（v2.0）
| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/video-projects` | 创建视频项目 |
| GET  | `/api/video-projects` | 项目列表 |
| GET  | `/api/video-projects/:id` | 项目详情 |
| PATCH| `/api/video-projects/:id` | 更新项目 |
| DELETE| `/api/video-projects/:id` | 归档（软删除） |
| POST | `/api/video-projects/:pid/workflow-runs` | 启动工作流 |
| GET  | `/api/video-projects/:pid/workflow-runs/:rid` | 查询 Run |
| POST | `/api/video-projects/:pid/workflow-runs/:rid/pause` | 暂停 |
| POST | `/api/video-projects/:pid/workflow-runs/:rid/cancel` | 取消 |

### 🆕 Skill 管理
| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/api/skills` | Skill 列表+健康状态 |
| GET  | `/api/skills/:name/:version` | Skill 详情 |
| POST | `/api/skills/:name/:version/compile` | 编译为 DAG |

---

## 🆕 Skill → Workflow 快速转换

写一个 `skill.yaml`，三种方式转为可执行 workflow：

```bash
# 方式1: CLI 工具
go run cmd/skill2workflow/main.go --skill skills/my-skill/1.0.0

# 方式2: API
curl -X POST http://localhost:8080/api/skills/my-skill/1.0.0/compile

# 方式3: 启动时自动注册（设置 VIDEO_CREATION_ENABLED=true 即可，零操作）
```

转换规则：
- `stage.name` → `node.id`
- `approval_required: true` → `TOOL` + `CONTROL` 双节点链
- `optional: true` → 自动生成 skip 分支

---

## 📁 项目结构

```
├── aios-core/
│   ├── cmd/
│   │   ├── tangying-ai-os/main.go      # ★ 主入口
│   │   └── skill2workflow/main.go      # 🆕 CLI 转换工具
│   ├── internal/
│   │   ├── core/                       # 通用核心引擎
│   │   │   ├── orchestrator/           #   DAG 调度
│   │   │   ├── worker/                 #   工具执行引擎
│   │   │   ├── workflow/               #   工作流模板 + 🆕 Run/Compiler
│   │   │   ├── modelgateway/           #   🆕 模型网关
│   │   │   ├── skillruntime/           #   🆕 Skill 运行时
│   │   │   ├── artifact/               #   🆕 产物管理
│   │   │   ├── localrunner/            #   🆕 本地 Runner
│   │   │   └── ...
│   │   └── agents/                     # 业务 Agent
│   │       ├── video/                  #   🆕 视频创作
│   │       ├── bid/                    #   标书生成
│   │       ├── chat/                   #   AI 对话
│   │       └── publish/                #   内容发布
│   ├── skills/                         # 🆕 Skill Package 目录
│   ├── deploy/                         # 🆕 Docker 部署配置
│   ├── sandbox/                        # Rust 沙箱
│   ├── scripts/                        # 启动脚本
│   └── docker-compose.yml
├── frontend/                           # React 前端
├── electron/                           # Electron 桌面端
└── docs/                               # 文档
```

---

## 🔧 环境变量

### 必填
| 变量 | 说明 |
|------|------|
| `OPENAI_API_KEY` | AI 服务 API 密钥 |

### 🆕 视频创作 Feature Flags
| 变量 | 默认值 | 说明 |
|------|--------|------|
| `VIDEO_CREATION_ENABLED` | `false` | 启用视频创作功能 |
| `LOCAL_RUNNER_ENABLED` | `false` | 启用 Electron 本地 Runner |
| `MODEL_PROVIDER_MODE` | `fake` | 模型调用模式（fake/real） |
| `SKILL_ROOT` | `skills` | Skill 包根目录 |

### 基础设施
| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SERVER_PORT` | `8080` | 后端端口 |
| `POSTGRES_HOST` | `localhost` | 数据库地址 |
| `REDIS_HOST` | `localhost` | Redis 地址 |
| `KAFKA_BOOTSTRAP_SERVERS` | `localhost:9092` | Kafka/Redpanda 地址 |
| `MINIO_ENDPOINT` | `localhost:9000` | MinIO 地址 |

---

## 📚 文档

| 文档 | 说明 |
|------|------|
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | ★ **完整产品架构设计说明**（新成员必读，含全部模块/接口/部署/优化建议） |
| [CLAUDE.md](CLAUDE.md) | 项目开发指南（Coding Agent 用） |
| [docs/upgrade/video-creation-v1/BASELINE_TEST_REPORT.md](docs/upgrade/video-creation-v1/BASELINE_TEST_REPORT.md) | 🆕 升级基线报告 |
| [docs/upgrade/video-creation-v1/TASK_STATUS.md](docs/upgrade/video-creation-v1/TASK_STATUS.md) | 🆕 任务完成状态 |
| [docs/upgrade/video-creation-v1/KNOWN_LIMITATIONS.md](docs/upgrade/video-creation-v1/KNOWN_LIMITATIONS.md) | 🆕 已知限制 |

---

## 📄 License

Apache License 2.0
