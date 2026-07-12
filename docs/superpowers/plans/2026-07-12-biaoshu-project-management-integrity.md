# 标书项目管理完整性修复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让标书项目历史、项目清单和章节文件在重启、批量生成、扩写和恢复后保持可发现、可恢复且不丢失记录。

**Architecture:** 保留本地 JSON 项目清单架构，不引入数据库。为每个项目的清单写入提供项目级锁和原子文件替换；批量章节登记在单个事务式操作中完成；目录对账从受控章节文件恢复清单并去除伪重复。开发启动必须固定使用仓库 `local-backend/data`，本地代理若只能解析到临时目录则应失败并提示显式传入 `-data-dir`，而不是创建一套空历史。

**Tech Stack:** Go `net/http`、标准库 `sync`/`os`/`path/filepath`、React/TypeScript、Node `node:test`、PowerShell。

## Global Constraints

- 不向 `local-backend` 引入数据库、Docker、Redis、Kafka、MinIO 或云端 LLM API Key。
- `local-backend/data/projects/biaoshu/<projectId>/project.json` 是唯一权威项目清单；`outputDir/project.manifest.json` 只能是从权威清单同步得到的镜像。
- 不重生成、不覆盖、不删除既有章节 Markdown；恢复只修改项目清单及其镜像。
- 普通 `GET /api/local/biaoshu/projects*` 必须保持纯读取；修复只通过显式 `POST` 触发。
- 初稿身份固定为 `chapter-<N>`，扩写稿身份固定为 `chapter-<N>-expanded`；同一 `(chapterNumber, draftStage, storageRef)` 只能有一个清单记录。
- 开发模式本地代理数据目录固定为 `E:\lingxi\tangying-ai-operation-system\local-backend\data`；禁止未显式配置时回退到临时目录。

---

## 文件结构与职责

| 文件 | 职责 |
| --- | --- |
| `local-backend/internal/localagent/server.go` | 为 JSON 写入提供原子替换，并在 `Server` 上维护项目级锁。 |
| `local-backend/internal/localagent/biaoshu_project_store.go` | 清单校验、单条/批量 upsert、稳定 ID 和权威镜像写入。 |
| `local-backend/internal/localagent/biaoshu_chapter_reconcile.go` | 扫描章节目录、解析任务书、恢复缺失记录及去重。 |
| `local-backend/internal/localagent/biaoshu_project_handler.go` | 暴露批量登记和章节对账 HTTP 端点。 |
| `local-backend/cmd/local-agent/main.go` | 拒绝未显式配置且会落入临时目录的启动。 |
| `local-backend/internal/localagent/*_test.go` | 并发、原子、批量、对账、路由与数据目录回归测试。 |
| `frontend/src/services/localAgent.ts` | 声明并调用批量登记、对账和本地数据目录健康信息。 |
| `frontend/src/pages/BiaoshuWorkbench.tsx` | 用一次批量登记替代并发单章登记；项目加载后按需触发显式对账。 |
| `frontend/src/pages/biaoshuProjectSystem.ts` | 以服务端返回的权威清单更新工作区，不从旧响应反向覆盖。 |
| `frontend/scripts/biaoshuChapterWorkspace.test.ts` | 验证恢复后的九章列表和真实版本聚合。 |
| `frontend/electron/local-agent-data-dir.cjs`、`frontend/electron/local-agent-data-dir.test.cjs` | 保持 Electron 开发模式与仓库数据目录一致。 |
| `启动说明.md` | 作为唯一 Windows 启动手册，包含四个组件与数据目录检查。 |

## Task 1: 建立可串行化、原子化的项目清单存储

**Files:**
- Modify: `local-backend/internal/localagent/server.go:28-33,710-720`
- Modify: `local-backend/internal/localagent/biaoshu_project_store.go:155-244`
- Modify: `local-backend/internal/localagent/biaoshu_project_store_test.go`

**Interfaces:**
- Produces: `func (s *Server) withBiaoshuProjectLock(projectID string, fn func() error) error`.
- Produces: `func writeIndentedJSONAtomically(path string, value interface{}) error`.
- Produces: `func (s *Server) mutateBiaoshuProject(projectID string, mutate func(*BiaoshuProjectManifest) error) (BiaoshuProjectManifest, error)`.
- Consumes: existing `readBiaoshuProjectManifest`, `writeBiaoshuProjectManifest`, `BiaoshuProjectManifest`.

- [ ] **Step 1: Write failing concurrency and atomic-write tests**

Add a test that creates one project, starts nine goroutines simultaneously, and registers artifacts `chapter-1` through `chapter-9`. Use a start channel so all goroutines read at the same time in the old implementation.

Add these helpers to `biaoshu_project_store_test.go` (and import `fmt` plus `sync`):

```go
func newTestBiaoshuProject(t *testing.T) (*Server, BiaoshuProjectManifest) {
    t.Helper()
    server := NewServer(Config{DataDir: t.TempDir(), BiaoshuOutputDir: t.TempDir()})
    if err := server.EnsureDirs(); err != nil { t.Fatal(err) }
    project, err := server.createBiaoshuProject(BiaoshuProjectCreateRequest{
        ProjectName: "concurrent-chapters", BidFilePath: "E:/bid/source.docx",
    })
    if err != nil { t.Fatal(err) }
    return server, project
}

func chapterRegisterRequest(project BiaoshuProjectManifest, chapter int) BiaoshuArtifactRegisterRequest {
    return BiaoshuArtifactRegisterRequest{
        ID: fmt.Sprintf("chapter-%d", chapter), Kind: string(ArtifactKindChapters),
        Name: fmt.Sprintf("第%d章", chapter), Status: "valid", MimeType: "text/markdown",
        StorageRef: filepath.Join(project.OutputDir, "章节", fmt.Sprintf("%02d_第%d章_初稿.md", chapter, chapter)),
        Metadata: map[string]interface{}{"chapterNumber": chapter, "draftStage": "initial"},
    }
}
```

```go
func TestRegisterBiaoshuProjectArtifactKeepsConcurrentUpdates(t *testing.T) {
    server, project := newTestBiaoshuProject(t)
    start := make(chan struct{})
    var wg sync.WaitGroup
    for chapter := 1; chapter <= 9; chapter++ {
        chapter := chapter
        wg.Add(1)
        go func() {
            defer wg.Done()
            <-start
            _, err := server.registerBiaoshuProjectArtifact(project.ProjectID, chapterRegisterRequest(project, chapter))
            if err != nil { t.Errorf("register chapter %d: %v", chapter, err) }
        }()
    }
    close(start)
    wg.Wait()
    actual, err := server.readBiaoshuProjectManifest(project.ProjectID)
    if err != nil { t.Fatal(err) }
    if got := len(actual.Artifacts); got != 9 { t.Fatalf("artifacts = %d, want 9", got) }
}
```

Add a second test that calls `writeIndentedJSONAtomically` with a manifest, reads the file back, and asserts valid JSON plus an absent `*.tmp` file.

- [ ] **Step 2: Run the targeted tests to verify the concurrency test fails before the lock exists**

Run:

```powershell
Set-Location 'E:\lingxi\tangying-ai-operation-system\local-backend'
$env:GOCACHE = 'E:\lingxi\tangying-ai-operation-system\.gocache-local'
go test ./internal/localagent -run 'TestRegisterBiaoshuProjectArtifactKeepsConcurrentUpdates|TestWriteIndentedJSONAtomically' -count=20
```

Expected: the concurrent registration test intermittently reports fewer than nine artifacts on the old implementation.

- [ ] **Step 3: Add project-level locking and atomic JSON replacement**

Extend `Server` with a lock registry:

```go
type Server struct {
    cfg               Config
    paths             Paths
    biaoshuHistoryDir string
    mux               *http.ServeMux
    biaoshuLocks      sync.Map // map[string]*sync.Mutex
}

func (s *Server) withBiaoshuProjectLock(projectID string, fn func() error) error {
    value, _ := s.biaoshuLocks.LoadOrStore(projectID, &sync.Mutex{})
    lock := value.(*sync.Mutex)
    lock.Lock()
    defer lock.Unlock()
    return fn()
}
```

Replace truncating writes with a same-directory temporary file, `Sync`, `Close`, and `os.Rename`:

```go
func writeIndentedJSONAtomically(path string, value interface{}) error {
    data, err := json.MarshalIndent(value, "", "  ")
    if err != nil { return err }
    dir := filepath.Dir(path)
    temp, err := os.CreateTemp(dir, ".project-*.tmp")
    if err != nil { return err }
    tempPath := temp.Name()
    defer os.Remove(tempPath)
    if _, err = temp.Write(append(data, '\n')); err != nil { temp.Close(); return err }
    if err = temp.Sync(); err != nil { temp.Close(); return err }
    if err = temp.Close(); err != nil { return err }
    return os.Rename(tempPath, path)
}
```

Route all `writeBiaoshuProjectManifest` calls through this writer. Implement `mutateBiaoshuProject` so it acquires the project lock, reads the latest manifest inside the lock, applies the callback, writes canonical then mirror manifests, and returns a fresh canonical read. Refactor `registerBiaoshuProjectArtifact` to use this method.

- [ ] **Step 4: Run store tests and race detection**

Run:

```powershell
Set-Location 'E:\lingxi\tangying-ai-operation-system\local-backend'
$env:GOCACHE = 'E:\lingxi\tangying-ai-operation-system\.gocache-local'
go test ./internal/localagent -count=20
go test -race ./internal/localagent
```

Expected: all tests pass; race detector reports no data race.

- [ ] **Step 5: Commit the storage integrity change**

```bash
git add local-backend/internal/localagent/server.go local-backend/internal/localagent/biaoshu_project_store.go local-backend/internal/localagent/biaoshu_project_store_test.go
git commit -m "fix(local): make biaoshu manifest writes atomic"
```

## Task 2: 增加批量章节登记并消除前端并发写入

**Files:**
- Modify: `local-backend/internal/localagent/biaoshu_project_store.go:26-35,185-244`
- Modify: `local-backend/internal/localagent/biaoshu_project_handler.go:46-119`
- Modify: `local-backend/internal/localagent/biaoshu_project_handler_test.go`
- Modify: `frontend/src/services/localAgent.ts:324-338`
- Modify: `frontend/src/pages/BiaoshuWorkbench.tsx:1523-1526,169-200`

**Interfaces:**
- Produces: `type BiaoshuArtifactBatchRegisterRequest struct { Artifacts []BiaoshuArtifactRegisterRequest \`json:"artifacts"\` }`.
- Produces: `func (s *Server) registerBiaoshuProjectArtifacts(projectID string, requests []BiaoshuArtifactRegisterRequest) (BiaoshuProjectManifest, error)`.
- Produces: `POST /api/local/biaoshu/projects/:projectId/artifacts/batch`.
- Produces: `registerBiaoshuManagedArtifacts(projectId, payload)` in TypeScript.

- [ ] **Step 1: Write failing batch API tests**

Add a handler test posting nine chapter requests to `/api/local/biaoshu/projects/<id>/artifacts/batch` and asserting HTTP 200 and nine chapter artifacts in the returned manifest. Add a second test that posts duplicate `id: "chapter-1"` values and asserts HTTP 400 plus an unchanged manifest.

Add this helper to `biaoshu_project_handler_test.go` (and import `fmt`):

```go
func chapterBatchRequest(project BiaoshuProjectManifest, chapter int) BiaoshuArtifactRegisterRequest {
    return BiaoshuArtifactRegisterRequest{
        ID: fmt.Sprintf("chapter-%d", chapter), Kind: string(ArtifactKindChapters),
        Name: fmt.Sprintf("第%d章", chapter), Status: "valid", MimeType: "text/markdown",
        StorageRef: filepath.Join(project.OutputDir, "章节", fmt.Sprintf("%02d_第%d章_初稿.md", chapter, chapter)),
        Metadata: map[string]interface{}{"chapterNumber": chapter, "draftStage": "initial"},
    }
}
```

```go
payload := BiaoshuArtifactBatchRegisterRequest{Artifacts: []BiaoshuArtifactRegisterRequest{
    chapterBatchRequest(project, 1), chapterBatchRequest(project, 2), chapterBatchRequest(project, 3),
    chapterBatchRequest(project, 4), chapterBatchRequest(project, 5), chapterBatchRequest(project, 6),
    chapterBatchRequest(project, 7), chapterBatchRequest(project, 8), chapterBatchRequest(project, 9),
}}
body, err := json.Marshal(payload)
if err != nil { t.Fatal(err) }
req := httptest.NewRequest(http.MethodPost, "/api/local/biaoshu/projects/"+project.ProjectID+"/artifacts/batch", bytes.NewReader(body))
req.Header.Set("Content-Type", "application/json")
rec := httptest.NewRecorder()
server.Handler().ServeHTTP(rec, req)
if rec.Code != http.StatusOK { t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String()) }
```

- [ ] **Step 2: Run the batch tests to verify the route does not exist**

Run:

```powershell
Set-Location 'E:\lingxi\tangying-ai-operation-system\local-backend'
$env:GOCACHE = 'E:\lingxi\tangying-ai-operation-system\.gocache-local'
go test ./internal/localagent -run 'TestBiaoshuProjectArtifactsBatch' -count=1
```

Expected: FAIL with HTTP 404 before the handler and store operation are added.

- [ ] **Step 3: Implement all-or-nothing batch validation and route dispatch**

Implement `registerBiaoshuProjectArtifacts` using `mutateBiaoshuProject` from Task 1. Before appending any item, validate all requests:

```go
seen := map[string]struct{}{}
for _, req := range requests {
    if err := validateBiaoshuArtifactRegisterRequest(manifest, req); err != nil { return err }
    id := canonicalArtifactID(req)
    if _, exists := seen[id]; exists { return fmt.Errorf("duplicate artifact id: %s", id) }
    seen[id] = struct{}{}
}
```

`validateBiaoshuArtifactRegisterRequest` must require non-empty `kind`, `storageRef`, and `mimeType`; require `chapterNumber > 0` for `BID_CHAPTERS`; clean the path and reject any path outside `manifest.OutputDir`; and only accept `.md` for `BID_CHAPTERS`. Append or replace all validated items in memory, then write once.

In `handleBiaoshuProjectResource`, reserve `artifacts/batch` for POST before the generic `artifacts/:artifactID` content branch. Add a dedicated handler that returns `{ "project": project }`.

In `localAgent.ts`, add:

```ts
export async function registerBiaoshuManagedArtifacts(
  projectId: string,
  artifacts: RegisterBiaoshuManagedArtifactRequest[],
): Promise<{ project: BiaoshuProjectManifest }> {
  const response = await fetch(localAgentUrl(`/api/local/biaoshu/projects/${encodeURIComponent(projectId)}/artifacts/batch`), {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ artifacts }),
  })
  if (!response.ok) throw new Error(await errorMessage(response, '批量登记标书产物失败'))
  return response.json() as Promise<{ project: BiaoshuProjectManifest }>
}
```

In `handleGenerateChapters`, construct the successful chapter records first and issue one awaited batch request. Do not call the existing `onReportGenerated` chapter branch for that batch, because it starts independent unawaited single-record requests. After the single response, update `manualArtifacts`, `activeProject`, `managedProjects`, and `projectHistory` from the returned project once.

- [ ] **Step 4: Run batch, component and frontend build verification**

Run:

```powershell
Set-Location 'E:\lingxi\tangying-ai-operation-system\local-backend'
$env:GOCACHE = 'E:\lingxi\tangying-ai-operation-system\.gocache-local'
go test ./internal/localagent -run 'TestBiaoshuProjectArtifactsBatch|TestRegisterBiaoshuProjectArtifactKeepsConcurrentUpdates'

Set-Location 'E:\lingxi\tangying-ai-operation-system\frontend'
npm.cmd run test:biaoshu-chapters
npm.cmd run build
```

Expected: batch endpoint accepts nine unique records in one response; frontend TypeScript build passes.

- [ ] **Step 5: Commit the batch registration change**

```bash
git add local-backend/internal/localagent/biaoshu_project_store.go local-backend/internal/localagent/biaoshu_project_handler.go local-backend/internal/localagent/biaoshu_project_handler_test.go frontend/src/services/localAgent.ts frontend/src/pages/BiaoshuWorkbench.tsx
git commit -m "fix(biaoshu): register generated chapters atomically"
```

## Task 3: 实现章节目录对账、恢复与去重

**Files:**
- Create: `local-backend/internal/localagent/biaoshu_chapter_reconcile.go`
- Create: `local-backend/internal/localagent/biaoshu_chapter_reconcile_test.go`
- Modify: `local-backend/internal/localagent/biaoshu_project_handler.go`
- Modify: `frontend/src/services/localAgent.ts`
- Modify: `frontend/src/pages/BiaoshuWorkbench.tsx`
- Modify: `frontend/scripts/biaoshuChapterWorkspace.test.ts`

**Interfaces:**
- Produces: `type BiaoshuChapterReconcileResponse struct { Project BiaoshuProjectManifest; Added []string; Deduplicated []string; Unclassified []string; MissingTargets []int; Changed bool }`.
- Produces: `func (s *Server) reconcileBiaoshuProjectChapters(projectID string) (BiaoshuChapterReconcileResponse, error)`.
- Produces: `POST /api/local/biaoshu/projects/:projectId/artifacts/reconcile-chapters`.
- Produces: `reconcileBiaoshuManagedChapters(projectId)` in TypeScript.

- [ ] **Step 1: Write failing reconciliation tests with the current incident as a fixture**

Create a temporary output directory containing `04_章节写作任务书.md` with chapters 1–9 and files `01_..._初稿.md` through `09_..._初稿.md`. Seed the manifest with chapter records `chapter-1`, `chapter-2`, `artifact_bid_chapters` for chapter 7, `chapter-7` for the same chapter-7 file, and `chapter-9`.

```go
response, err := server.reconcileBiaoshuProjectChapters(project.ProjectID)
if err != nil { t.Fatal(err) }
if got := chapterNumbers(response.Project.Artifacts); !slices.Equal(got, []int{1,2,3,4,5,6,7,8,9}) {
    t.Fatalf("chapter numbers = %v, want 1..9", got)
}
if !slices.Contains(response.Deduplicated, "artifact_bid_chapters") {
    t.Fatalf("expected duplicate chapter 7 to be removed: %#v", response.Deduplicated)
}
```

Add tests for: an unrecognized file (`notes.md`) being returned in `Unclassified` without registration; a second reconciliation making `Changed == false`; and a source file whose path is outside `outputDir` being rejected.

- [ ] **Step 2: Run reconciliation tests to verify the function is absent**

Run:

```powershell
Set-Location 'E:\lingxi\tangying-ai-operation-system\local-backend'
$env:GOCACHE = 'E:\lingxi\tangying-ai-operation-system\.gocache-local'
go test ./internal/localagent -run 'TestReconcileBiaoshuProjectChapters' -count=1
```

Expected: FAIL because `reconcileBiaoshuProjectChapters` and its route do not exist.

- [ ] **Step 3: Implement constrained scan, metadata restoration and deterministic de-duplication**

In `biaoshu_chapter_reconcile.go`, define two exact filename expressions:

```go
var initialChapterFilename = regexp.MustCompile(`^(\\d{2})_(.+)_初稿\\.md$`)
var expandedChapterFilename = regexp.MustCompile(`^(\\d{2})_(.+)_扩写稿\\.md$`)
```

Only scan direct files in `filepath.Join(manifest.OutputDir, "章节")`. Parse `04_章节写作任务书.md` into `map[int]chapterTaskMetadata` by matching `### 第[一二三四五六七八九十]+章：` and the following `estimatedWordCount` value. Derive each recovered artifact as follows:

```go
id := fmt.Sprintf("chapter-%d", number)
if stage == "expanded" { id += "-expanded" }
metadata := map[string]interface{}{
    "chapterNumber": number,
    "chapterTitle": title,
    "draftStage": stage,
    "wordCount": countBiaoshuMarkdownWords(content),
    "targetWords": task.TargetWords,
}
```

Use the same project lock/mutation operation from Task 1. For duplicate manifest records with equal cleaned `storageRef`, chapter number and draft stage, retain the entry with the later `UpdatedAt`; if timestamps tie, retain the one already using the stable ID. Normalize the retained record to the stable ID. Never delete or rewrite a chapter file.

Add the `reconcile-chapters` route before generic artifact-content routing. Return the full response described by the interface.

In the frontend, trigger this POST only after a managed project is loaded and only when the user explicitly chooses the new `修复章节记录` action or a dedicated non-blocking banner reports a mismatch. Do not issue this mutation during a plain project list GET. On success replace `activeProject` with `response.project` and show `已恢复 X 章，已清理 Y 条重复记录`.

- [ ] **Step 4: Extend workspace regression coverage and execute recovery verification**

Extend `frontend/scripts/biaoshuChapterWorkspace.test.ts` with recovered initial records for chapters 1–9 and one valid expanded chapter 7. Assert:

```ts
assert.deepEqual(workspace.chapters.map((item) => item.chapterNumber), [1,2,3,4,5,6,7,8,9])
assert.equal(workspace.chapters[6].versions.length, 2)
```

Run:

```powershell
Set-Location 'E:\lingxi\tangying-ai-operation-system\local-backend'
$env:GOCACHE = 'E:\lingxi\tangying-ai-operation-system\.gocache-local'
go test ./internal/localagent -run 'TestReconcileBiaoshuProjectChapters|TestBiaoshuProjectArtifactsBatch'

Set-Location 'E:\lingxi\tangying-ai-operation-system\frontend'
npm.cmd run test:biaoshu-chapters
```

Expected: recovery fixture shows nine unique initial chapters; only a true expanded file creates a second version.

- [ ] **Step 5: Commit the recovery workflow**

```bash
git add local-backend/internal/localagent/biaoshu_chapter_reconcile.go local-backend/internal/localagent/biaoshu_chapter_reconcile_test.go local-backend/internal/localagent/biaoshu_project_handler.go frontend/src/services/localAgent.ts frontend/src/pages/BiaoshuWorkbench.tsx frontend/scripts/biaoshuChapterWorkspace.test.ts
git commit -m "fix(biaoshu): reconcile missing chapter manifests"
```

## Task 4: 固化数据目录并让错误启动立即可见

**Files:**
- Modify: `local-backend/cmd/local-agent/main.go:19-38`
- Modify: `local-backend/internal/localagent/server.go:1025-1056`
- Modify: `local-backend/internal/localagent/server_test.go`
- Modify: `frontend/electron/main.cjs:40-76`
- Modify: `frontend/electron/local-agent-data-dir.cjs`
- Modify: `frontend/electron/local-agent-data-dir.test.cjs`
- Modify: `启动说明.md`

**Interfaces:**
- Produces: `func resolveLocalAgentDataDir(explicitDataDir, workspaceRoot string) (string, error)` in Go startup code.
- Produces: a startup error when no explicit data directory is supplied and the only fallback is under `os.TempDir()`.
- Consumes: existing Electron `resolveLocalAgentDataDir({ appIsPackaged, workspaceRoot, userDataDir, environment })` resolver.

- [ ] **Step 1: Write failing data-directory tests**

Add Go tests that set an inaccessible home environment and an empty `TANGYING_LOCAL_DATA_DIR`, then assert the CLI resolver returns an error containing `-data-dir` rather than `os.TempDir()/TangyingAIOS`. Add a second test asserting explicit `TANGYING_LOCAL_DATA_DIR` wins. Keep the existing Electron test that development resolves to `workspaceRoot/local-backend/data` and add an assertion that the spawned local-agent environment receives that exact value.

```go
func TestResolveLocalAgentDataDirRejectsEphemeralFallback(t *testing.T) {
    t.Setenv("TANGYING_LOCAL_DATA_DIR", "")
    _, err := resolveLocalAgentDataDir("", "")
    if err == nil || !strings.Contains(err.Error(), "-data-dir") {
        t.Fatalf("err = %v, want explicit data-dir guidance", err)
    }
}
```

- [ ] **Step 2: Run the data-directory tests to verify the old fallback is unsafe**

Run:

```powershell
Set-Location 'E:\lingxi\tangying-ai-operation-system\local-backend'
$env:GOCACHE = 'E:\lingxi\tangying-ai-operation-system\.gocache-local'
go test ./internal/localagent -run 'Test.*DataDir|TestResolveLocalAgentDataDir' -count=1

Set-Location 'E:\lingxi\tangying-ai-operation-system\frontend'
node --test electron/local-agent-data-dir.test.cjs
```

Expected: the new Go resolver test fails before startup validation is implemented; Electron resolver tests already pass or reveal any mismatch.

- [ ] **Step 3: Implement persistent-directory selection and startup diagnostics**

Keep this exact priority:

1. Explicit `-data-dir` or `TANGYING_LOCAL_DATA_DIR`.
2. Electron development resolver: `<workspaceRoot>/local-backend/data`.
3. Electron packaged resolver: `<userDataDir>/local-agent`.
4. Persistent OS application-data directory when available.
5. Otherwise return an error instructing the user to supply `-data-dir`; never silently use `os.TempDir()`.

Update `cmd/local-agent/main.go` to resolve and validate the directory before `localagent.NewServer`. Preserve the existing explicit command-line value. In `main.cjs`, retain the resolver module already introduced in the worktree and ensure `startLocalAgent` passes the resolved value both through `TANGYING_LOCAL_DATA_DIR` and the `-data-dir` argument, so the child process cannot ignore inherited environment behavior.

Update `启动说明.md` to name the same canonical development directory, start the local agent with both environment variable and flag, and require the `/api/local/health` `dataDir` check before opening the frontend.

- [ ] **Step 4: Run startup resolver and full local verification**

Run:

```powershell
Set-Location 'E:\lingxi\tangying-ai-operation-system\local-backend'
$env:GOCACHE = 'E:\lingxi\tangying-ai-operation-system\.gocache-local'
go test ./...

Set-Location 'E:\lingxi\tangying-ai-operation-system\frontend'
node --test electron/local-agent-data-dir.test.cjs
npm.cmd run build
```

Then manually start the local agent using `启动说明.md` and verify:

```powershell
Invoke-RestMethod 'http://127.0.0.1:18080/api/local/health' | Select-Object dataDir,status
Invoke-RestMethod 'http://127.0.0.1:18080/api/local/biaoshu/projects' | Select-Object -ExpandProperty projects
```

Expected: `dataDir` is the repository `local-backend/data` directory and historical projects are returned.

- [ ] **Step 5: Commit startup-directory consistency changes**

```bash
git add local-backend/cmd/local-agent/main.go local-backend/internal/localagent/server.go local-backend/internal/localagent/server_test.go frontend/electron/main.cjs frontend/electron/local-agent-data-dir.cjs frontend/electron/local-agent-data-dir.test.cjs 启动说明.md
git commit -m "fix(local): require persistent project data directory"
```

## Task 5: 迁移当前项目并完成端到端验收

**Files:**
- Modify: `local-backend/internal/localagent/biaoshu_project_handler_test.go`
- Modify: `docs/superpowers/specs/2026-07-11-biaoshu-chapter-manifest-consistency-design.md` only if implementation discovers a contract change.

**Interfaces:**
- Consumes: Task 3 `POST /api/local/biaoshu/projects/:projectId/artifacts/reconcile-chapters`.
- Produces: a repaired authoritative manifest for project `bp_404d75fe6c670710` without modifying any chapter Markdown.

- [ ] **Step 1: Add a route-level idempotency test**

Call the reconcile endpoint twice against the nine-file fixture. Assert the first response has `Changed == true`, five additions and one deduplication; assert the second response has `Changed == false`, empty `Added`, and empty `Deduplicated`.

- [ ] **Step 2: Run the idempotency test before migration**

Run:

```powershell
Set-Location 'E:\lingxi\tangying-ai-operation-system\local-backend'
$env:GOCACHE = 'E:\lingxi\tangying-ai-operation-system\.gocache-local'
go test ./internal/localagent -run 'TestReconcileBiaoshuProjectChaptersRouteIsIdempotent' -count=1
```

Expected: PASS after Task 3; this test is the gate that prevents a recovery endpoint from repeatedly mutating valid projects.

- [ ] **Step 3: Back up and reconcile the current project only after all automated tests pass**

With the local agent started using the canonical data directory, copy only the authoritative manifest before calling the new endpoint:

```powershell
$projectId = 'bp_404d75fe6c670710'
$projectDir = "E:\lingxi\tangying-ai-operation-system\local-backend\data\projects\biaoshu\$projectId"
Copy-Item "$projectDir\project.json" "$projectDir\project.pre-reconcile.json"
Invoke-RestMethod -Method Post "http://127.0.0.1:18080/api/local/biaoshu/projects/$projectId/artifacts/reconcile-chapters"
```

Verify the source directory still has exactly nine `_初稿.md` files, and verify the returned project has initial chapter numbers 1 through 9. Do not call any generation endpoint during this task.

- [ ] **Step 4: Run end-to-end verification**

Run:

```powershell
Set-Location 'E:\lingxi\tangying-ai-operation-system\local-backend'
$env:GOCACHE = 'E:\lingxi\tangying-ai-operation-system\.gocache-local'
go test ./...
go test -race ./...

Set-Location 'E:\lingxi\tangying-ai-operation-system\frontend'
npm.cmd run test:biaoshu-chapters
npm.cmd run build
```

Manual acceptance criteria:

- The chapter workspace displays chapters 1–9.
- Chapter 7 has one initial version unless a real `_扩写稿.md` exists.
- The history page still displays all existing projects after restarting with the documented command.
- No chapter Markdown has a changed modification time from the reconciliation action.

- [ ] **Step 5: Commit tests and recovery documentation only; do not commit local project data**

```bash
git add local-backend/internal/localagent/biaoshu_project_handler_test.go docs/superpowers/specs/2026-07-11-biaoshu-chapter-manifest-consistency-design.md
git commit -m "test(biaoshu): cover manifest recovery workflow"
```

Do not stage `local-backend/data/`, `biaoshu-tools/output/`, `project.json`, `project.manifest.json`, or `project.pre-reconcile.json`.

## Plan Self-Review

- Spec coverage: Task 1 handles serial and crash-safe writes; Task 2 handles atomic batch registration; Task 3 handles recovery, metadata and duplicate cleanup; Task 4 handles history-directory consistency; Task 5 repairs and verifies the current project.
- Placeholder scan: no deferred implementation markers are used; every task names files, interfaces, tests and commands.
- Type consistency: the batch request uses `BiaoshuArtifactRegisterRequest` in Go and `RegisterBiaoshuManagedArtifactRequest` in TypeScript; reconciliation consistently returns `BiaoshuChapterReconcileResponse`.
- Scope check: the plan keeps JSON storage and adds no database or cloud dependency, so it remains within the local-backend boundary.
