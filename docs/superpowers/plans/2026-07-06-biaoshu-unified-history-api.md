# 标书项目统一历史接口修改计划

## 目标

新增一个本地后端历史接口：

```text
GET /api/local/biaoshu/history
```

由后端统一聚合“新项目清单、旧 Temp 项目清单、旧运行记录、项目输出目录产物线索”，前端历史项目页只调用这个新接口，不再依赖“先查 managed projects，失败再查 legacy projects”的临时兼容逻辑。

修复完成后，应满足：

- 历史项目中能同时看到 `养护`、`养护2`、`养护3` 等不同项目。
- 项目列表不因数据目录从 `%TEMP%\TangyingAIOS` 迁移到 `%LOCALAPPDATA%\TangyingAIOS` 而变空。
- 旧接口 `/api/local/biaoshu-projects` 中存在的旧记录仍可显示。
- 新接口返回来源统计和告警，避免“页面空白但不知道后端读了哪些目录”的问题再次出现。

## 当前问题判断

当前历史项目仍然不完整，本质原因不是前端表格渲染问题，而是历史数据来源被拆散：

1. `/api/local/biaoshu/projects` 只读取当前 managed project 根目录。
2. 运行中的本地后端数据目录是：

   ```text
   C:\Users\86183\AppData\Local\TangyingAIOS
   ```

3. 旧 managed 项目清单实际还在：

   ```text
   C:\Users\86183\AppData\Local\Temp\TangyingAIOS\projects\biaoshu
   ```

4. 旧接口 `/api/local/biaoshu-projects` 能返回旧记录，但这些记录主要是旧的 `养护` 运行，不覆盖 `养护2`、`养护3` 这类 managed 项目。
5. 前端现在需要同时理解 managed project 和 legacy project 两套来源，逻辑复杂且容易漏掉一种来源。

因此，正确修改方向是新增一个“统一历史接口”，把兼容、迁移、去重、排序、诊断都收敛到 local-backend。

## 新接口契约

### 请求

```text
GET /api/local/biaoshu/history
```

### 响应示例

```json
{
  "projects": [
    {
      "projectId": "bp_ce4292dba6e33a10",
      "runId": "agent_run_25",
      "projectName": "养护3",
      "bidFilePath": "E:\\yhbs\\招标文件\\招标文件_converted.docx",
      "outputDir": "E:\\lingxi\\tangying-ai-operation-system\\biaoshu-tools\\output\\养护3",
      "status": "completed",
      "currentStage": "scoring_ready",
      "generatedCount": 3,
      "totalCount": 5,
      "updatedAt": "2026-07-01T10:54:58+08:00",
      "source": "legacy_temp_managed",
      "hasManagedManifest": true,
      "hasLegacyRun": false
    }
  ],
  "sources": {
    "currentManaged": 0,
    "legacyTempManaged": 6,
    "legacyRuns": 6,
    "outputRecovered": 3
  },
  "warnings": []
}
```

### 字段说明

- `projectId`：managed project 的稳定 ID；legacy-only 项目可为空。
- `runId`：最近一次运行 ID；没有运行 ID 时为空。
- `projectName`：列表展示名称，优先来自 manifest，其次来自 legacy record，其次从输出目录名推断。
- `bidFilePath`：招标文件路径。
- `outputDir`：本地输出目录。
- `status`：统一后的状态，使用 `completed`、`running`、`failed`、`paused`、`unknown`。
- `currentStage`：统一后的阶段，如 `analysis_ready`、`context_ready`、`scoring_ready`、`outline_ready`。
- `generatedCount` / `totalCount`：用于历史页“产物”列。
- `updatedAt`：用于倒序排序。
- `source`：最终采用的数据来源，便于诊断。
- `hasManagedManifest` / `hasLegacyRun`：说明该行由哪些来源合并而来。

## 实施步骤

### 1. 后端先补失败测试

新增文件：

```text
local-backend/internal/localagent/biaoshu_history_test.go
```

覆盖三个关键场景：

1. 当前 managed 根目录为空，但旧 Temp managed 根目录存在 `养护2`、`养护3`，新接口仍返回它们。
2. legacy run store 中只有 `养护`，新接口会把它与 managed 来源一起返回，而不是互相覆盖。
3. 同一个项目在多个来源中同时存在时，按“产物更完整、阶段更靠后、更新时间更新”的规则保留最好记录。

建议测试结构：

```go
func TestBuildBiaoshuHistoryIncludesLegacyTempManagedProjects(t *testing.T) {
    s := newTestServer(t)
    legacyRoot := t.TempDir()

    writeManagedProjectManifestForHistoryTest(t, legacyRoot, BiaoshuProject{
        ID:          "bp_yanghu3",
        ProjectName: "养护3",
        BidFilePath: `E:\yhbs\招标文件\招标文件_converted.docx`,
        OutputDir:   `E:\lingxi\tangying-ai-operation-system\biaoshu-tools\output\养护3`,
        CurrentStage: StageScoringReady,
        Status:      ProjectStatusCompleted,
    })

    resp, err := s.buildBiaoshuHistory([]string{legacyRoot})
    if err != nil {
        t.Fatalf("buildBiaoshuHistory returned error: %v", err)
    }
    if !historyContainsProjectName(resp.Projects, "养护3") {
        t.Fatalf("expected history to include 养护3, got %#v", resp.Projects)
    }
    if resp.Sources.LegacyTempManaged != 1 {
        t.Fatalf("expected legacy temp managed count 1, got %d", resp.Sources.LegacyTempManaged)
    }
}
```

再补充：

```go
func TestBuildBiaoshuHistoryMergesManagedAndLegacyRuns(t *testing.T)
```

断言最终项目名集合至少包含：

```text
养护
养护2
养护3
```

### 2. 新增后端聚合实现

新增文件：

```text
local-backend/internal/localagent/biaoshu_history.go
```

实现以下类型：

```go
type BiaoshuHistoryResponse struct {
    Projects []BiaoshuHistoryProject `json:"projects"`
    Sources  BiaoshuHistorySources   `json:"sources"`
    Warnings []string                `json:"warnings,omitempty"`
}

type BiaoshuHistorySources struct {
    CurrentManaged    int `json:"currentManaged"`
    LegacyTempManaged int `json:"legacyTempManaged"`
    LegacyRuns        int `json:"legacyRuns"`
    OutputRecovered   int `json:"outputRecovered"`
}

type BiaoshuHistoryProject struct {
    ProjectID          string    `json:"projectId,omitempty"`
    RunID              string    `json:"runId,omitempty"`
    ProjectName        string    `json:"projectName"`
    BidFilePath        string    `json:"bidFilePath,omitempty"`
    OutputDir          string    `json:"outputDir,omitempty"`
    Status             string    `json:"status"`
    CurrentStage       string    `json:"currentStage,omitempty"`
    GeneratedCount     int       `json:"generatedCount"`
    TotalCount         int       `json:"totalCount"`
    UpdatedAt          time.Time `json:"updatedAt"`
    Source             string    `json:"source"`
    HasManagedManifest bool      `json:"hasManagedManifest"`
    HasLegacyRun       bool      `json:"hasLegacyRun"`
}
```

实现这些函数：

```go
func (s *Server) handleBiaoshuHistory(w http.ResponseWriter, r *http.Request)

func (s *Server) buildBiaoshuHistory(legacyManagedRoots []string) (BiaoshuHistoryResponse, error)

func (s *Server) collectBiaoshuManagedHistoryFromRoot(root string, source string) ([]BiaoshuHistoryProject, error)

func biaoshuHistoryFromManagedProject(project BiaoshuProject, source string) BiaoshuHistoryProject

func biaoshuHistoryFromLegacyProject(project BiaoshuLegacyProject) BiaoshuHistoryProject

func mergeBiaoshuHistoryProjects(items []BiaoshuHistoryProject) []BiaoshuHistoryProject

func biaoshuHistoryProjectKey(item BiaoshuHistoryProject) string

func biaoshuHistoryProjectScore(item BiaoshuHistoryProject) int
```

合并规则：

- 第一优先级：`projectId` 相同则合并。
- 第二优先级：`normalized(projectName) + normalized(bidFilePath)` 相同则合并。
- 第三优先级：`normalized(outputDir)` 相同则合并。
- 同 key 多条记录时，优先选择：
  1. 阶段更靠后的记录。
  2. `generatedCount` 更多的记录。
  3. 状态为 `completed` 的记录。
  4. `updatedAt` 更新的记录。

阶段权重建议：

```go
var biaoshuHistoryStageRank = map[string]int{
    string(StageCreated):      10,
    string(StageAnalysisReady): 20,
    string(StageContextReady):  30,
    string(StageScoringReady):  40,
    string(StageOutlineReady):  50,
    string(StageDraftReady):    60,
    string(StageFinalReady):    70,
}
```

### 3. 后端路由接入

修改文件：

```text
local-backend/internal/localagent/server.go
```

或当前集中注册标书路由的位置，增加：

```go
mux.HandleFunc("/api/local/biaoshu/history", s.handleBiaoshuHistory)
```

处理函数内部调用：

```go
resp, err := s.buildBiaoshuHistory(s.legacyManagedBiaoshuProjectRoots())
if err != nil {
    writeJSONError(w, http.StatusInternalServerError, err.Error())
    return
}
writeJSON(w, http.StatusOK, resp)
```

注意：新接口不应因为某一个来源读取失败就整体返回空。可恢复错误进入 `warnings`，只有当前数据目录完全不可读这类基础错误才返回 500。

### 4. 后端 OpenAPI 文档同步

修改文件：

```text
local-backend/internal/localagent/openapi.go
```

增加 `/api/local/biaoshu/history` 的 OpenAPI path、response schema。

随后运行：

```bash
cd local-backend && go run ./cmd/gen-local-apidocs
```

生成文件会更新：

```text
local-backend/docs/API_REFERENCE.md
```

### 5. 前端新增 API 客户端

修改文件：

```text
frontend/src/services/localAgent.ts
```

新增类型：

```ts
export interface BiaoshuHistoryResponse {
  projects: BiaoshuHistoryProject[];
  sources: {
    currentManaged: number;
    legacyTempManaged: number;
    legacyRuns: number;
    outputRecovered: number;
  };
  warnings?: string[];
}

export interface BiaoshuHistoryProject {
  projectId?: string;
  runId?: string;
  projectName: string;
  bidFilePath?: string;
  outputDir?: string;
  status: string;
  currentStage?: string;
  generatedCount: number;
  totalCount: number;
  updatedAt: string;
  source: string;
  hasManagedManifest: boolean;
  hasLegacyRun: boolean;
}

export async function fetchBiaoshuHistory(): Promise<BiaoshuHistoryResponse> {
  return localAgentRequest<BiaoshuHistoryResponse>("/api/local/biaoshu/history");
}
```

如果 `localAgentRequest` 当前已经自动拼接 `/api/local`，则实际路径使用项目内现有约定，不能重复拼接。

### 6. 前端历史页改为只读新接口

修改文件：

```text
frontend/src/pages/BiaoshuWorkbench.tsx
```

把历史项目初始化逻辑改成：

```ts
const history = await fetchBiaoshuHistory();
setProjectHistory(history.projects.map(biaoshuHistoryProjectToViewItem));
```

删除历史页对以下接口的直接依赖：

```text
fetchBiaoshuManagedProjects()
fetchBiaoshuProjects()
```

保留工作台当前项目恢复逻辑时，可以从新接口选择最佳项目：

```ts
const best = selectBestBiaoshuHistoryProject(history.projects);
```

选择规则：

- 优先有 `projectId` 的 managed 项目。
- 优先阶段靠后。
- 优先 `updatedAt` 更新。

### 7. 前端视图模型转换

修改文件：

```text
frontend/src/pages/biaoshuProjectSystem.ts
```

新增：

```ts
export function biaoshuHistoryProjectToViewItem(project: BiaoshuHistoryProject): BiaoshuProjectHistoryItem
```

转换规则：

- `name` 使用 `project.projectName`。
- `bidFilePath` 使用 `project.bidFilePath ?? project.outputDir ?? ""`。
- `status` 从后端 `status` 映射为页面现有状态。
- `artifactProgress` 使用 `${project.generatedCount}/${project.totalCount}`。
- `runId` 使用 `project.runId`。
- `projectId` 使用 `project.projectId`。
- 保留 `source`，便于调试显示或控制台诊断。

### 8. “查看产物”行为

历史页点击 `查看产物` 时：

- 如果记录有 `projectId`，进入 managed project 的产物视图。
- 如果没有 `projectId` 但有 `runId`，使用 legacy run 产物视图。
- 如果只有 `outputDir`，展示输出目录扫描出的产物列表，不能直接显示“无产物”。

这一步避免旧记录虽然能显示在历史页，但点击后无法进入产物列表。

### 9. 验证命令

后端：

```bash
cd local-backend && go test ./...
```

前端：

```bash
cd frontend && npm run build
```

接口手动验证：

```powershell
Invoke-RestMethod http://127.0.0.1:18080/api/local/biaoshu/history | ConvertTo-Json -Depth 6
```

期望结果：

- `projects` 至少包含 `养护`、`养护2`、`养护3`。
- `sources.legacyTempManaged` 大于 0。
- `projects[].source` 能看出每条记录来自 `current_managed`、`legacy_temp_managed`、`legacy_run` 或合并结果。

### 10. 回归检查清单

- 历史项目页刷新后不为空。
- 重启本地后端后历史项目仍存在。
- 当前 `%LOCALAPPDATA%` 无 managed manifest 时，仍能从旧 Temp managed 根恢复。
- 旧 `/api/local/biaoshu-projects` 只存在 `养护` 时，不会覆盖或挤掉 `养护2`、`养护3`。
- `产物` 页能打开 recovered managed 项目的产物。
- 新接口返回 warnings 时，前端控制台能输出诊断信息，页面不直接失败。

## 预期改动文件

```text
local-backend/internal/localagent/biaoshu_history.go
local-backend/internal/localagent/biaoshu_history_test.go
local-backend/internal/localagent/server.go
local-backend/internal/localagent/openapi.go
local-backend/docs/API_REFERENCE.md
frontend/src/services/localAgent.ts
frontend/src/pages/BiaoshuWorkbench.tsx
frontend/src/pages/biaoshuProjectSystem.ts
frontend/scripts/biaoshu-artifact-logic-check.mjs
```

## 完成标准

完成后，历史项目的数据来源应从“前端分别调用多个旧接口并猜测如何合并”变为“后端统一返回完整历史索引”。前端只负责展示和跳转，后端负责兼容旧数据、旧目录和 managed project 清单。

最关键的验收标准是：

```text
GET /api/local/biaoshu/history
```

返回的项目列表中同时包含 `养护`、`养护2`、`养护3`，并且前端历史项目页完整显示这些项目。
