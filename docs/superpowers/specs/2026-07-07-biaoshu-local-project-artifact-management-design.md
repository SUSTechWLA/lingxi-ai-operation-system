# 标书本地项目与产物管理重构设计

日期：2026-07-07

## 背景

当前标书项目系统同时存在两套本地记录：

- 旧索引：`/api/local/biaoshu-projects/:runId`，以 `runId` 记录历史项目。
- 新 manifest：`/api/local/biaoshu/projects/:projectId`，以 `project.json` 记录托管项目。

同时，`/api/local/biaoshu-artifacts/read` 仍以任意 `filePath` 读取本地文件，只校验路径是否在可信根目录内。这个模型会导致几个问题：

- 同名项目可以多次创建，历史列表和托管项目列表会出现重复。
- 项目 manifest 与 `biaoshu-tools/output/<项目名>` 下的真实文件没有强绑定。
- 前端会根据文件名和目录名自行推导产物路径，容易读到错误目录。
- 删除项目后，旧历史快照或 legacy migrate 可能把项目重新恢复出来。
- 用户无法通过一个专用接口完整删除项目及其本地产物。

这次重构的目标是让项目生命周期以本地产物目录为事实来源：项目存在必须对应本地输出目录；同名项目只能有一个；删除项目必须删除全部本地产物，并允许后续再次创建同名项目。

## 目标

1. 同名项目只能存在一个。
2. 项目必须对应一个本地输出目录，并且目录中必须能找到项目 manifest。
3. 产物读取必须绑定项目和 artifact，不再依赖前端传任意绝对路径作为主路径。
4. 提供删除整个标书项目的接口，物理删除项目输出目录和本地索引。
5. 删除后，后续创建同名项目必须成功。
6. 保持 `local-backend` 轻量，不引入数据库、Docker、Redis、Kafka、MinIO 或 LLM API key。

## 非目标

- 不迁移云端 artifact 数据模型。
- 不引入数据库式本地索引。
- 不删除用户原始招标文件 `bidFilePath`。
- 不一次性移除旧 `/api/local/biaoshu-projects` 兼容接口。

## 推荐架构

采用“本地输出目录驱动”的项目管理方式：

```text
biaoshu-tools/output/<项目名>/
  project.manifest.json
  00_招标文件原文解析.md
  00_招标文件解析报告.md
  01_项目信息确认表.md
  02_评分标准拆解表.md
  ...

<local-data>/projects/biaoshu/<projectId>/
  project.json
```

`project.json` 是 local agent 的内部索引，`project.manifest.json` 是输出目录内的镜像。两者内容一致。后端列表、读取、删除都以内部 manifest 为入口，但项目是否可打开还要校验输出目录和镜像 manifest 是否存在。

## 项目命名与输出目录

项目名规范化规则：

- 去掉首尾空白。
- 折叠连续空白为一个空格。
- 拒绝 Windows 文件名非法字符：`< > : " / \ | ? *`。
- 拒绝空项目名。
- Windows 下同名判断大小写不敏感。

默认输出目录：

```text
<workspace>/biaoshu-tools/output/<项目名>
```

如果后续支持 `BIAOSHU_OUTPUT_DIR`，则可信输出根目录优先使用该环境变量，否则使用仓库内 `biaoshu-tools/output`。无论使用哪个根目录，项目目录名都必须由后端生成并返回，前端不再自行拼接。

## 创建项目

接口：

```text
POST /api/local/biaoshu/projects
```

请求：

```json
{
  "projectName": "养护",
  "bidFilePath": "E:/yhbs/招标文件/招标文件.docx"
}
```

后端流程：

1. 校验并规范化 `projectName`。
2. 校验 `bidFilePath` 非空。
3. 读取当前 managed manifests。
4. 如果已有同名项目，返回 `409 Conflict`。
5. 检查 `<output-root>/<项目名>` 是否存在。
6. 如果输出目录已存在，返回 `409 Conflict`，提示先删除已有项目或换名。
7. 创建 `projectId`。
8. 创建输出目录。
9. 写入内部 `project.json` 和输出目录 `project.manifest.json`。
10. 返回项目对象。

响应新增字段：

```json
{
  "project": {
    "projectId": "bp_xxx",
    "projectName": "养护",
    "outputDir": "E:/lingxi/tangying-ai-operation-system/biaoshu-tools/output/养护",
    "outputDirName": "养护"
  }
}
```

后续启动 agent run 时，前端必须使用返回的 `outputDirName` 填入 `output_dir`，避免 `parse_bid_files` 按招标文件名创建错误目录。

## 登记产物

接口保持：

```text
POST /api/local/biaoshu/projects/:projectId/artifacts
```

登记规则加强：

- `storageRef` 可以是绝对路径，也可以是相对项目输出目录的路径。
- 后端规范化后必须确认文件位于该项目 `outputDir` 内。
- 如果文件尚未生成，可以登记为 `pending` 或 `running`；如果状态为 `valid`，文件必须存在。
- 同一项目内同一 `kind` 仍按现有逻辑替换最新版本。

## 读取产物

新增推荐接口：

```text
GET /api/local/biaoshu/projects/:projectId/artifacts/:artifactId/content
```

读取流程：

1. 读取项目 manifest。
2. 校验项目输出目录存在。
3. 根据 `artifactId` 找到 artifact。
4. 解析 artifact 的 `storageRef`。
5. 校验真实文件位于项目 `outputDir` 内。
6. 校验文件存在、不是目录、大小不超过预览上限。
7. 返回 `filePath`、`format`、`content`、`size`。

保留兼容接口：

```text
POST /api/local/biaoshu-artifacts/read
```

兼容接口支持两种请求：

```json
{
  "projectId": "bp_xxx",
  "artifactId": "artifact_bid_analysis"
}
```

或旧格式：

```json
{
  "filePath": "E:/lingxi/tangying-ai-operation-system/biaoshu-tools/output/养护/00_招标文件解析报告.md"
}
```

旧 `filePath` 格式只作为兼容路径。后端必须确认该文件属于某个当前 managed 项目的 `outputDir`，否则拒绝读取。这样可以阻止前端读取到 `招标文件_converted` 等未登记项目目录里的文件。

## 写入产物

现有接口：

```text
POST /api/local/biaoshu-artifacts/write
```

同样增加项目绑定请求：

```json
{
  "projectId": "bp_xxx",
  "artifactId": "artifact_bid_project_context",
  "content": "...",
  "expectedPreviousContent": "..."
}
```

旧 `filePath` 写入保留兼容，但必须满足：

- 文件位于某个 managed 项目的 `outputDir` 内。
- 后缀为 `.md`、`.txt`、`.json`。
- 如果传入 `expectedPreviousContent`，必须匹配当前文件内容。

## 删除项目

新增接口：

```text
DELETE /api/local/biaoshu/projects/:projectId
```

删除是物理删除，范围包括：

1. 项目输出目录：`biaoshu-tools/output/<项目名>/`
2. 内部 manifest 目录：`<local-data>/projects/biaoshu/<projectId>/`
3. 本地 artifact 目录：`<local-data>/artifacts/<projectId>/`
4. 本地 cache 目录：`<local-data>/cache/<projectId>/`
5. 与该项目 run 关联的本地 conversation 文件。
6. 旧 `/api/local/biaoshu-projects` 索引中与该项目 `runId` 相关的记录。
7. 写入最新历史快照，确保历史接口不会从旧快照恢复已删除项目。

安全规则：

- 只允许删除可信输出根目录内的项目目录。
- 不删除 `bidFilePath` 指向的原始招标文件。
- 如果 `outputDir` 不在可信根目录内，返回错误并拒绝删除。
- 删除操作必须校验最终绝对路径位于预期根目录内。

响应：

```json
{
  "status": "deleted",
  "projectId": "bp_xxx",
  "projectName": "养护",
  "deletedPaths": [
    "E:/lingxi/tangying-ai-operation-system/biaoshu-tools/output/养护",
    "E:/Users/.../TangyingAIOS/projects/biaoshu/bp_xxx",
    "E:/Users/.../TangyingAIOS/artifacts/bp_xxx",
    "E:/Users/.../TangyingAIOS/cache/bp_xxx"
  ]
}
```

删除后，同名判断不再命中 manifest 或输出目录，因此再次创建相同 `projectName` 必须成功。

## 历史与迁移

调整历史逻辑，避免删除后复活：

- `listBiaoshuProjectManifests()` 只返回输出目录存在且有 `project.manifest.json` 的项目。
- `importManagedBiaoshuProjectsFromRoots()` 跳过输出目录不存在的 legacy manifest。
- `migrateLegacyBiaoshuProjects()` 不再自动创建没有本地输出目录的 managed 项目。
- `/api/local/biaoshu/history` 可以展示纯 legacy run 记录，但必须标记为不可打开，且不能参与同名项目占用。
- 删除项目时同步清理旧 run 索引和写入最新快照。

## 前端改动

前端需要从“路径推导”改为“项目绑定”：

- 创建项目时先调用 `POST /api/local/biaoshu/projects`。
- 如果返回 `409`，提示用户项目名已存在，需要删除后再创建或换名。
- 启动 agent run 时使用后端返回的 `outputDirName` 作为 `output_dir`。
- 打开产物优先调用项目绑定读取接口。
- 恢复扫描只作为兜底，并且扫描结果必须注册到当前项目后才能展示。
- 历史列表增加删除入口，调用 `DELETE /api/local/biaoshu/projects/:projectId`。
- 删除成功后刷新 managed projects、history、active project 和 artifact 列表。

## OpenAPI 与文档

修改接口后必须同步：

- `local-backend/internal/localagent/openapi.go`
- `local-backend/docs/API_REFERENCE.md`

运行：

```bash
cd local-backend
go run ./cmd/gen-local-apidocs
```

如果云端 API 没有变化，不需要修改 `cloud-backend/internal/core/apispec/cloud_spec.go`。

## 测试计划

后端测试：

- 创建项目会创建输出目录和 `project.manifest.json`。
- 创建同名项目返回 `409 Conflict`。
- 输出目录已存在时创建同名项目返回 `409 Conflict`。
- 删除项目会删除输出目录、内部 manifest、local artifacts 和 cache。
- 删除后可以再次创建同名项目。
- 读取项目产物时，文件必须位于该项目 `outputDir` 内。
- 旧 `filePath` 读取不能读取未登记项目目录下的文件。
- legacy migrate 不再创建没有输出目录的 managed 项目。
- history 不再恢复已删除项目。

前端测试：

- 同名创建错误提示正确。
- 删除项目后历史列表移除该项目。
- 删除后再次创建同名项目成功。
- 产物打开走项目绑定读取接口。
- agent run 使用后端返回的 `outputDirName`。

验证命令：

```bash
cd local-backend && go test ./...
cd frontend && npm run build
```

## 验收标准

- 用户不能创建两个同名标书项目。
- 每个可打开项目都能在本地输出目录中找到对应目录和 manifest。
- `/api/local/biaoshu-artifacts/read` 不再能读取未登记项目目录中的文件。
- 删除项目会物理删除本地输出目录。
- 删除后，同名项目可以正常重新创建。
- 本地后端仍不依赖数据库、Docker、Redis、Kafka、MinIO 或 LLM API key。
