`react-go-fullstack-check`

## Skill 描述
适用于前后端分离项目：前端为 React（CRA/Vite），后端为 Go（标准库/gin/echo 等）。自动启动项目，识别所有后端主流程 API，逐一冒烟测试，修复常见问题，并确保前端能正确调用后端接口。完成项目启动诊断。

---

## 执行流程（严格按序执行）

### 第 1 步：项目结构分析与环境识别
1. 列出顶层目录内容，识别前端与后端目录。常见模式：
   - 单仓：`server/`（或 `.`） + `client/`（或 `frontend/`）
   - 分仓：当前目录即为后端，前端可能在另一个目录或需要用户指定。先假设后端为当前目录，同时寻找包含 `package.json` 且含 `react` 依赖的前端子目录。
2. 确认后端技术：
   - 检查根目录是否存在 `go.mod`，读取模块名。
   - 扫描 `*.go` 文件，识别使用的 Web 框架（如 `gin`、`echo`、`gorilla/mux`、`net/http`）。
3. 确认前端技术：
   - 进入前端目录，检查 `package.json` 中的 `dependencies`，判断是 CRA（`react-scripts`）还是 Vite（`vite`）。
   - 查看 `scripts` 中的启动命令（如 `start`、`dev`）。
4. 收集关键配置：
   - **后端端口**：在 `main.go` 或路由初始化代码中搜索 `:端口号` 或 `PORT` 环境变量；若未显式指定，Go 默认端口需从日志或代码推断。
   - **前端端口**：CRA 默认为 3000，Vite 默认为 5173；查找 `package.json` 中是否自定义。
   - **API 基地址**：前端项目中搜索 `baseURL`、`VITE_API_URL`、`REACT_APP_API_URL` 等。
   -** 数据库/外部依赖**：`.env` 文件中 `DATABASE_URL`、`DB_HOST` 等，以及 `docker-compose.yml`。

---

### 第 2 步：环境准备
1. **验证工具链**：
   - 执行 `go version`，确保 Go 已安装且版本 ≥ 1.18。
   - 执行 `node --version` 和 `npm --version`，确保 Node.js 可用。
   - 如未安装，提示用户并终止流程。
2. **处理环境变量**：
   - 在项目根目录及后端目录查找 `.env.example` 或 `.env.template`，若不存在 `.env`，则复制一份并填入示例值（提醒用户需自行修改敏感信息）。
   - 若存在 `.env`，读取并检查是否有必填变量缺失（如数据库连接），缺失则警告。
3. **安装依赖**：
   - 后端：在包含 `go.mod` 的目录执行 `go mod tidy && go mod download`。
   - 前端：在前端目录执行 `npm install`（若 `node_modules` 不存在）。
4. **启动外部服务**（如有）：
   - 若项目含 `docker-compose.yml`，询问用户是否执行 `docker compose up -d` 启动依赖，获得允许则执行。

---

### 第 3 步：启动后端 & 发现主流程 API

#### 3.1 启动后端服务
1. 确定启动命令：
   - 查找 `Makefile` 中的 `run` 目标，或直接找入口文件 `main.go`（通常在根目录或 `cmd/` 下）。
   - 常见启动方式：`go run main.go` 或 `go run ./cmd/server`。
2. 后台启动并记录 PID：`go run main.go & echo $! > /tmp/go_server.pid`。
3. 等待后端就绪：循环请求 `http://localhost:{port}/health` 或根路径 `/`，超时 30 秒。若框架无健康检查端点，尝试请求已知的 API 路径（如 `/api/v1/`）。

#### 3.2 提取主流程 API 清单
1. 搜索所有 `.go` 文件中的路由注册，使用正则匹配框架特征：
   - gin: `router\.(GET|POST|PUT|PATCH|DELETE)\(`
   - echo: `e\.(GET|POST|PUT|PATCH|DELETE)\(`
   - net/http: `http\.HandleFunc\(` 或 `mux\.HandleFunc\(`
2. 从匹配行提取 `"路径"` 和 HTTP 方法，排除明显非业务路由（如 `/health`, `/metrics`, `/swagger`）。
3. 整理为“主流程 API 清单”，并按模块分组（如 `/auth/*`, `/users/*`, `/products/*`）。

#### 3.3 对每个 API 进行冒烟测试
1. **构造请求规则**：
   - GET 请求不带 body。
   - POST/PUT 若需要数据，尝试用最小合法 payload（从代码中的结构体猜测，或使用空对象 `{}`）。
   - 需要认证的端点，先调用登录接口获取 token，并在后续请求头中携带 `Authorization: Bearer <token>`。
2. 使用 `curl` 执行测试，检查：
   - HTTP 状态码 200-299 为正常（创建资源 201 也算正常）。
   - 响应体是否为有效 JSON（如果预期是 JSON），且不包含 `"error": true` 或非空 `"error"` 字段。
3. **失败处理**（按优先级自动修复）：
   - **CORS 错误**（前端验证时才会出现，此处暂不处理，留到第 4 步）。
   - **端口冲突**：更换端口或杀死占用进程。
   - **环境变量缺失**：如 `dial tcp: lookup db on ...`，提示并尝试补充 `.env`。
   - **数据库未迁移/表不存在**：查找项目中的迁移工具（如 `golang-migrate`、`goose`、自行编写的 `migrate` 命令），执行迁移。若找不到，提示用户手动执行。
   - **编译错误**：查看启动日志，修复语法或导入错误后重启服务。
4. 重复修复 - 重启 - 测试直到所有主流程 API 均返回正常响应。

---

### 第 4 步：对接前端功能

#### 4.1 配置前端 API 地址
1. 在前端目录搜索：
   - CRA: `REACT_APP_API_URL` 在 `.env` 文件中
   - Vite: `VITE_API_URL` 在 `.env` 或 `.env.development` 中
   - 可能硬编码在 `src/api.js`、`src/config.js` 或 `axios` 实例中。
2. 将其值修改为当前后端实际地址（`http://localhost:{后端端口}`），若无此配置则创建环境变量并提醒用户。

#### 4.2 启动前端开发服务器
1. 执行 `npm start`（CRA）或 `npm run dev`（Vite），后台运行并记录 PID。
2. 等待前端就绪，可通过 `curl http://localhost:3000` 验证。

#### 4.3 验证核心交互
1. 扫描前端 `src/` 目录，找到调用后端 API 的服务函数（通过搜索 `axios.get`, `fetch(` 等）。
2. 挑选至少 3 个关键页面：
   - 登录/注册页 → 验证认证流程
   - 主数据列表页 → 验证数据获取
   - 表单提交页 → 验证数据写入
3. 使用无头浏览器工具（如 `puppeteer`）或直接构造 `curl` 模拟前端请求（带前端可能使用的 headers），确认：
   - 请求到达后端且无 CORS 错误（Access-Control-Allow-Origin 头缺失等）。
   - 响应数据与前端期望的结构匹配（通过查看前端代码中的 `.then(res => res.data)` 判断字段）。
4. **修复 CORS**：
   - 若后端缺少 CORS 中间件，自动为 Go 添加。例如对于 gin：在其 `main.go` 添加上下文 `router.Use(cors.Default())`，并导入 `github.com/gin-contrib/cors`。若框架不支持，则用 `net/http` 中间件包裹。提示用户安装缺失的包。
5. **修复代理错误**：
   - 如果使用 CRA 的 `proxy` 字段，检查 `package.json` 中 `"proxy"` 是否正确指向后端。
   - Vite 的 `server.proxy` 配置检查 `vite.config.js`。

---

### 第 5 步：生成诊断报告
汇总所有操作与修复，输出结构化报告：

```
✅ React + Go 项目启动诊断报告

【后端】Go 服务
- 框架：gin v1.9.0
- 地址：http://localhost:8080
- 主流程 API：12 个
  - ✅ 通过：10 个
  - 🛠️ 修复后通过：2 个（/api/users 缺少 db 环境变量，已补全；/api/products 返回 500，经查是空指针，已添加零值检查）
- 数据库：PostgreSQL 连接正常，迁移已执行

【前端】React (Vite)
- 地址：http://localhost:5173
- API 基地址已配置为 http://localhost:8080
- 核心页面验证：
  - ✅ 登录/注册：成功获取 token，页面跳转正常
  - ✅ 产品列表：数据加载成功，无 CORS 错误
  - 🛠️ 订单提交：原 422 错误，因后端缺少字段验证，已添加 `binding:"required"` 标签

【依然存在的问题】
- 文件上传接口 `/api/upload` 需云存储凭证，请手动配置 `.env` 中的 `S3_BUCKET`

项目现已连通，可正常开发。