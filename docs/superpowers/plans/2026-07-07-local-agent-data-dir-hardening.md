# Local Agent Data Directory Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 根治 local-agent 在 Windows 上反复出现 `mkdir C:\Users\86183\AppData\Local\TangyingAIOS\projects\biaoshu: Access is denied` 的问题，并让标书项目输出目录稳定指向仓库内 `biaoshu-tools/output`。

**Architecture:** local-agent 启动时显式解析并验证 `DataDir`、`WorkspaceRoot` 和 `BiaoshuOutputDir`，避免依赖 `os.Getwd()` 或 Go 临时构建目录。项目列表接口改为纯读，所有迁移/导入写入动作从 GET 路径移出。启动时做可写性探针，目录不可写时 fail fast，并返回可操作错误。

**Tech Stack:** Go 1.x local-backend, net/http handlers, Windows filesystem, PowerShell/bash startup scripts, existing `go test ./...` test suite.

## Global Constraints

- 不给 `local-backend` 增加数据库、Docker、Redis、Kafka、MinIO 或 LLM API key 依赖。
- 不删除用户原始招标文件 `bidFilePath`。
- 标书输出文件必须落在 `biaoshu-tools/output/<项目名>` 或显式配置的 `BIAOSHU_OUTPUT_DIR/<项目名>`。
- `GET /api/local/biaoshu/projects` 必须纯读，不允许隐式创建/迁移项目。
- 保留现有 API 兼容性，新增字段和配置必须向后兼容。

---

## File Structure

- Modify: `local-backend/internal/localagent/server.go`
  - Add `WorkspaceRoot` and `BiaoshuOutputDir` to `Config`.
  - Resolve stable paths in `NewServer`.
  - Add `ValidateWritablePaths()` and `writeProbeFile()`.
  - Add better health/path diagnostics.

- Modify: `local-backend/cmd/local-agent/main.go`
  - Add `-workspace-root` and `-biaoshu-output-dir` flags.
  - Pass new config into `localagent.NewServer`.
  - Run `ValidateWritablePaths()` after `EnsureDirs()` and fail fast.

- Modify: `local-backend/internal/localagent/biaoshu_project_store.go`
  - Stop using `os.Getwd()` for `biaoshuOutputRoot()`.
  - Use `s.cfg.BiaoshuOutputDir`.
  - Add path validation helpers for output root.

- Modify: `local-backend/internal/localagent/biaoshu_project_handler.go`
  - Make `GET /api/local/biaoshu/projects` list only current manifests.
  - Remove implicit `importManagedBiaoshuProjectsFromRoots()` and `migrateLegacyBiaoshuProjects()` from GET.

- Modify: `local-backend/internal/localagent/server_test.go`
  - Add config resolution and writable probe tests.

- Modify: `local-backend/internal/localagent/biaoshu_project_store_test.go`
  - Add output-root tests and create-project tests.

- Modify: `local-backend/internal/localagent/biaoshu_project_handler_test.go`
  - Add pure-read GET test.

- Modify: `scripts/start-local-backend.sh`
  - Export stable `TANGYING_LOCAL_DATA_DIR` and `BIAOSHU_OUTPUT_DIR`.
  - Start local backend from `local-backend` with explicit flags.

---

### Task 1: Stabilize Local Agent Configuration

**Files:**
- Modify: `local-backend/internal/localagent/server.go`
- Modify: `local-backend/internal/localagent/server_test.go`

**Interfaces:**
- Produces: `Config.WorkspaceRoot string`
- Produces: `Config.BiaoshuOutputDir string`
- Produces: `func defaultWorkspaceRoot() string`
- Produces: `func defaultBiaoshuOutputDir(workspaceRoot string) string`
- Consumes: existing `defaultDataDir()`, `NewServer(Config)`, `Paths()`

- [ ] **Step 1: Write failing tests for stable config resolution**

Add these tests to `local-backend/internal/localagent/server_test.go`:

```go
func TestNewServerUsesExplicitWorkspaceAndBiaoshuOutputDir(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	workspaceRoot := filepath.Join(root, "workspace")
	outputDir := filepath.Join(root, "workspace", "biaoshu-tools", "output")

	server := NewServer(Config{
		DataDir:         dataDir,
		WorkspaceRoot:   workspaceRoot,
		BiaoshuOutputDir: outputDir,
	})

	if server.Paths().DataDir != dataDir {
		t.Fatalf("dataDir = %q, want %q", server.Paths().DataDir, dataDir)
	}
	if server.cfg.WorkspaceRoot != workspaceRoot {
		t.Fatalf("workspaceRoot = %q, want %q", server.cfg.WorkspaceRoot, workspaceRoot)
	}
	if server.cfg.BiaoshuOutputDir != outputDir {
		t.Fatalf("biaoshuOutputDir = %q, want %q", server.cfg.BiaoshuOutputDir, outputDir)
	}
}

func TestDefaultBiaoshuOutputDirIsDerivedFromWorkspaceRoot(t *testing.T) {
	workspaceRoot := filepath.Join(t.TempDir(), "repo")
	got := defaultBiaoshuOutputDir(workspaceRoot)
	want := filepath.Join(workspaceRoot, "biaoshu-tools", "output")
	if got != want {
		t.Fatalf("defaultBiaoshuOutputDir() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd local-backend && go test ./internal/localagent -run 'TestNewServerUsesExplicitWorkspaceAndBiaoshuOutputDir|TestDefaultBiaoshuOutputDirIsDerivedFromWorkspaceRoot' -count=1
```

Expected: FAIL because `Config.WorkspaceRoot`, `Config.BiaoshuOutputDir`, and `defaultBiaoshuOutputDir` do not exist or are not populated.

- [ ] **Step 3: Extend `Config` and `NewServer`**

In `local-backend/internal/localagent/server.go`, change `Config`:

```go
type Config struct {
	DataDir           string
	CloudAPIBase      string
	BiaoshuHistoryDir string
	WorkspaceRoot     string
	BiaoshuOutputDir  string
}
```

Add helpers near `defaultDataDir()`:

```go
func defaultWorkspaceRoot() string {
	if root := strings.TrimSpace(os.Getenv("TANGYING_WORKSPACE_ROOT")); root != "" {
		return root
	}
	cwd, err := os.Getwd()
	if err == nil {
		if root, ok := findWorkspaceRoot(cwd); ok {
			return root
		}
	}
	return cwd
}

func defaultBiaoshuOutputDir(workspaceRoot string) string {
	if dir := strings.TrimSpace(os.Getenv("BIAOSHU_OUTPUT_DIR")); dir != "" {
		return dir
	}
	return filepath.Join(workspaceRoot, "biaoshu-tools", "output")
}

func findWorkspaceRoot(start string) (string, bool) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		if fileExists(filepath.Join(dir, "go.work")) ||
			fileExists(filepath.Join(dir, "AGENTS.md")) ||
			dirExists(filepath.Join(dir, "local-backend")) && dirExists(filepath.Join(dir, "biaoshu-tools")) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
```

Update `NewServer` after `DataDir` defaulting:

```go
	if strings.TrimSpace(cfg.WorkspaceRoot) == "" {
		cfg.WorkspaceRoot = defaultWorkspaceRoot()
	}
	if strings.TrimSpace(cfg.BiaoshuOutputDir) == "" {
		cfg.BiaoshuOutputDir = defaultBiaoshuOutputDir(cfg.WorkspaceRoot)
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
cd local-backend && go test ./internal/localagent -run 'TestNewServerUsesExplicitWorkspaceAndBiaoshuOutputDir|TestDefaultBiaoshuOutputDirIsDerivedFromWorkspaceRoot' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add local-backend/internal/localagent/server.go local-backend/internal/localagent/server_test.go
git commit -m "fix(local-agent): stabilize workspace and output config"
```

---

### Task 2: Add Startup Writable Probes

**Files:**
- Modify: `local-backend/internal/localagent/server.go`
- Modify: `local-backend/internal/localagent/server_test.go`
- Modify: `local-backend/cmd/local-agent/main.go`

**Interfaces:**
- Consumes: `Server.Paths()`, `Server.cfg.BiaoshuOutputDir`
- Produces: `func (s *Server) ValidateWritablePaths() error`
- Produces: `func writeProbeFile(dir string) error`

- [ ] **Step 1: Write failing tests for writable probes**

Add to `local-backend/internal/localagent/server_test.go`:

```go
func TestValidateWritablePathsCreatesProbeFiles(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{
		DataDir:         filepath.Join(root, "data"),
		WorkspaceRoot:   root,
		BiaoshuOutputDir: filepath.Join(root, "biaoshu-tools", "output"),
	})
	if err := server.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := server.ValidateWritablePaths(); err != nil {
		t.Fatalf("ValidateWritablePaths returned error: %v", err)
	}
	for _, dir := range []string{
		server.Paths().ProjectDir,
		server.Paths().ArtifactDir,
		server.Paths().CacheDir,
		server.Paths().LogDir,
		server.Paths().DiagnosticsDir,
		server.cfg.BiaoshuOutputDir,
	} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("expected writable directory %q to exist: %v", dir, err)
		}
	}
}

func TestValidateWritablePathsReportsDirectoryContext(t *testing.T) {
	root := t.TempDir()
	blockingFile := filepath.Join(root, "not-a-dir")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer(Config{
		DataDir:         filepath.Join(root, "data"),
		WorkspaceRoot:   root,
		BiaoshuOutputDir: blockingFile,
	})

	err := server.ValidateWritablePaths()
	if err == nil {
		t.Fatal("expected writable path error")
	}
	if !strings.Contains(err.Error(), "biaoshu output dir") {
		t.Fatalf("error should name failing path role, got %v", err)
	}
	if !strings.Contains(err.Error(), blockingFile) {
		t.Fatalf("error should include failing path, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd local-backend && go test ./internal/localagent -run 'TestValidateWritablePaths' -count=1
```

Expected: FAIL because `ValidateWritablePaths` does not exist.

- [ ] **Step 3: Implement writable probes**

Add to `local-backend/internal/localagent/server.go` after `EnsureDirs()`:

```go
func (s *Server) ValidateWritablePaths() error {
	checks := []struct {
		role string
		dir  string
	}{
		{role: "data dir", dir: s.paths.DataDir},
		{role: "project dir", dir: s.paths.ProjectDir},
		{role: "artifact dir", dir: s.paths.ArtifactDir},
		{role: "cache dir", dir: s.paths.CacheDir},
		{role: "log dir", dir: s.paths.LogDir},
		{role: "diagnostics dir", dir: s.paths.DiagnosticsDir},
		{role: "biaoshu output dir", dir: s.cfg.BiaoshuOutputDir},
	}
	for _, check := range checks {
		if err := os.MkdirAll(check.dir, 0o755); err != nil {
			return fmt.Errorf("%s is not creatable (%s): %w", check.role, check.dir, err)
		}
		if err := writeProbeFile(check.dir); err != nil {
			return fmt.Errorf("%s is not writable (%s): %w", check.role, check.dir, err)
		}
	}
	return nil
}

func writeProbeFile(dir string) error {
	name := fmt.Sprintf(".tangying-write-probe-%d.tmp", time.Now().UTC().UnixNano())
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("ok"), 0o600); err != nil {
		return err
	}
	return os.Remove(path)
}
```

In `local-backend/cmd/local-agent/main.go`, after `server.EnsureDirs()` add:

```go
	if err := server.ValidateWritablePaths(); err != nil {
		log.Fatalf("validate local writable directories: %v", err)
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:

```bash
cd local-backend && go test ./internal/localagent -run 'TestValidateWritablePaths' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add local-backend/internal/localagent/server.go local-backend/internal/localagent/server_test.go local-backend/cmd/local-agent/main.go
git commit -m "fix(local-agent): fail fast on unwritable local paths"
```

---

### Task 3: Make Biaoshu Output Root Stable

**Files:**
- Modify: `local-backend/internal/localagent/biaoshu_project_store.go`
- Modify: `local-backend/internal/localagent/biaoshu_project_store_test.go`
- Modify: `local-backend/internal/localagent/server.go`

**Interfaces:**
- Consumes: `Server.cfg.BiaoshuOutputDir`
- Produces: `func (s *Server) biaoshuOutputRoot() string`
- Produces: `func (s *Server) ensurePathWithinBiaoshuOutputRoot(path string) error`

- [ ] **Step 1: Write failing tests for output root**

Add to `local-backend/internal/localagent/biaoshu_project_store_test.go`:

```go
func TestBiaoshuOutputRootUsesConfigNotWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	outputRoot := filepath.Join(root, "stable-output")
	server := NewServer(Config{
		DataDir:         filepath.Join(root, "data"),
		WorkspaceRoot:   filepath.Join(root, "workspace"),
		BiaoshuOutputDir: outputRoot,
	})

	if got := server.biaoshuOutputRoot(); got != outputRoot {
		t.Fatalf("biaoshuOutputRoot = %q, want %q", got, outputRoot)
	}
}

func TestCreateBiaoshuProjectUsesConfiguredOutputRoot(t *testing.T) {
	root := t.TempDir()
	outputRoot := filepath.Join(root, "biaoshu-output")
	server := NewServer(Config{
		DataDir:         filepath.Join(root, "data"),
		WorkspaceRoot:   filepath.Join(root, "workspace"),
		BiaoshuOutputDir: outputRoot,
	})
	if err := server.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	project, err := server.createBiaoshuProject(BiaoshuProjectCreateRequest{
		ProjectName: "养护",
		BidFilePath: "E:/bid/招标文件.docx",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantOutput := filepath.Join(outputRoot, "养护")
	if project.OutputDir != wantOutput {
		t.Fatalf("outputDir = %q, want %q", project.OutputDir, wantOutput)
	}
	if _, err := os.Stat(filepath.Join(wantOutput, biaoshuProjectMirrorFilename)); err != nil {
		t.Fatalf("expected mirror manifest in output dir: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify current behavior**

Run:

```bash
cd local-backend && go test ./internal/localagent -run 'TestBiaoshuOutputRootUsesConfigNotWorkingDirectory|TestCreateBiaoshuProjectUsesConfiguredOutputRoot' -count=1
```

Expected before implementation: FAIL if `biaoshuOutputRoot()` still reads env/`os.Getwd()` instead of config.

- [ ] **Step 3: Replace `biaoshuOutputRoot()` implementation**

In `local-backend/internal/localagent/biaoshu_project_store.go`, replace:

```go
func (s *Server) biaoshuOutputRoot() string {
	if dir := os.Getenv("BIAOSHU_OUTPUT_DIR"); strings.TrimSpace(dir) != "" {
		return strings.TrimSpace(dir)
	}
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, "..", "biaoshu-tools", "output")
}
```

with:

```go
func (s *Server) biaoshuOutputRoot() string {
	return s.cfg.BiaoshuOutputDir
}
```

Remove the `os` import from `biaoshu_project_store.go` only if no remaining code in that file uses it.

- [ ] **Step 4: Add output-root containment helper**

In `local-backend/internal/localagent/biaoshu_project_store.go`, add:

```go
func (s *Server) ensurePathWithinBiaoshuOutputRoot(path string) error {
	root, err := filepath.Abs(s.biaoshuOutputRoot())
	if err != nil {
		return err
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel)) {
		return nil
	}
	return fmt.Errorf("path %s is outside biaoshu output root %s", target, root)
}
```

Use this helper in project deletion where current code compares strings with `strings.HasPrefix`.

- [ ] **Step 5: Run tests**

Run:

```bash
cd local-backend && go test ./internal/localagent -run 'TestBiaoshuOutputRootUsesConfigNotWorkingDirectory|TestCreateBiaoshuProjectUsesConfiguredOutputRoot' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add local-backend/internal/localagent/biaoshu_project_store.go local-backend/internal/localagent/biaoshu_project_store_test.go
git commit -m "fix(biaoshu): use stable configured output root"
```

---

### Task 4: Make Project List GET Pure Read

**Files:**
- Modify: `local-backend/internal/localagent/biaoshu_project_handler.go`
- Modify: `local-backend/internal/localagent/biaoshu_project_handler_test.go`

**Interfaces:**
- Consumes: `listBiaoshuProjectManifests()`
- Removes from GET path: `importManagedBiaoshuProjectsFromRoots(...)`
- Removes from GET path: `migrateLegacyBiaoshuProjects()`

- [ ] **Step 1: Write failing test proving GET does not create project directories**

Add to `local-backend/internal/localagent/biaoshu_project_handler_test.go`:

```go
func TestBiaoshuProjectCollectionGetIsPureRead(t *testing.T) {
	root := t.TempDir()
	server := NewServer(Config{
		DataDir:         root,
		WorkspaceRoot:   t.TempDir(),
		BiaoshuOutputDir: filepath.Join(t.TempDir(), "output"),
	})
	if err := server.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/local/biaoshu/projects", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(server.biaoshuProjectRoot()); err == nil {
		t.Fatalf("GET /biaoshu/projects must not create %s", server.biaoshuProjectRoot())
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected stat error for %s: %v", server.biaoshuProjectRoot(), err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails on current handler**

Run:

```bash
cd local-backend && go test ./internal/localagent -run TestBiaoshuProjectCollectionGetIsPureRead -count=1
```

Expected before implementation: FAIL if GET still triggers migration/import that writes `projects/biaoshu`.

- [ ] **Step 3: Remove side effects from GET handler**

In `local-backend/internal/localagent/biaoshu_project_handler.go`, change GET case from:

```go
	case http.MethodGet:
		_, _ = s.importManagedBiaoshuProjectsFromRoots(s.legacyManagedBiaoshuProjectRoots())
		_, _ = s.migrateLegacyBiaoshuProjects()
		projects, err := s.listBiaoshuProjectManifests()
```

to:

```go
	case http.MethodGet:
		projects, err := s.listBiaoshuProjectManifests()
```

Do not delete `importManagedBiaoshuProjectsFromRoots` or `migrateLegacyBiaoshuProjects`; they remain available for explicit future maintenance commands or tests.

- [ ] **Step 4: Run test to verify it passes**

Run:

```bash
cd local-backend && go test ./internal/localagent -run TestBiaoshuProjectCollectionGetIsPureRead -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add local-backend/internal/localagent/biaoshu_project_handler.go local-backend/internal/localagent/biaoshu_project_handler_test.go
git commit -m "fix(biaoshu): make project list read-only"
```

---

### Task 5: Improve API Error Context for Directory Writes

**Files:**
- Modify: `local-backend/internal/localagent/biaoshu_project_store.go`
- Modify: `local-backend/internal/localagent/biaoshu_project_handler_test.go`

**Interfaces:**
- Produces clearer error text from `writeBiaoshuProjectManifest`
- Consumes: existing `writeError(w, status, message)`

- [ ] **Step 1: Write failing test for actionable create-project errors**

Add to `local-backend/internal/localagent/biaoshu_project_handler_test.go`:

```go
func TestCreateBiaoshuProjectReportsManifestDirectoryRole(t *testing.T) {
	root := t.TempDir()
	blockingFile := filepath.Join(root, "projects")
	if err := os.WriteFile(blockingFile, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewServer(Config{
		DataDir:         root,
		WorkspaceRoot:   t.TempDir(),
		BiaoshuOutputDir: filepath.Join(t.TempDir(), "output"),
	})

	body := bytes.NewBufferString(`{"projectName":"养护","bidFilePath":"E:/bid/招标文件.docx"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/local/biaoshu/projects", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "biaoshu project manifest dir") {
		t.Fatalf("error should name manifest directory role, got %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd local-backend && go test ./internal/localagent -run TestCreateBiaoshuProjectReportsManifestDirectoryRole -count=1
```

Expected: FAIL because the current error only exposes raw `mkdir ...` text.

- [ ] **Step 3: Wrap directory errors with role context**

In `local-backend/internal/localagent/biaoshu_project_store.go`, change `writeBiaoshuProjectManifest`:

```go
	path := s.biaoshuProjectManifestPath(manifest.ProjectID)
	manifestDir := filepath.Dir(path)
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		return fmt.Errorf("biaoshu project manifest dir is not creatable (%s): %w", manifestDir, err)
	}
	if err := writeIndentedJSON(path, manifest); err != nil {
		return fmt.Errorf("write biaoshu project manifest %s: %w", path, err)
	}
	if err := s.writeBiaoshuProjectManifestMirror(manifest); err != nil {
		return fmt.Errorf("write biaoshu output manifest mirror: %w", err)
	}
	return nil
```

Change `writeBiaoshuProjectManifestMirror`:

```go
	if err := os.MkdirAll(manifest.OutputDir, 0o755); err != nil {
		return fmt.Errorf("biaoshu output dir is not creatable (%s): %w", manifest.OutputDir, err)
	}
	return writeIndentedJSON(filepath.Join(manifest.OutputDir, biaoshuProjectMirrorFilename), manifest)
```

- [ ] **Step 4: Run test to verify it passes**

Run:

```bash
cd local-backend && go test ./internal/localagent -run TestCreateBiaoshuProjectReportsManifestDirectoryRole -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add local-backend/internal/localagent/biaoshu_project_store.go local-backend/internal/localagent/biaoshu_project_handler_test.go
git commit -m "fix(biaoshu): report manifest write path failures clearly"
```

---

### Task 6: Wire Stable Paths Through Startup Script

**Files:**
- Modify: `scripts/start-local-backend.sh`
- Modify: `local-backend/cmd/local-agent/main.go`

**Interfaces:**
- Produces CLI flag: `-workspace-root`
- Produces CLI flag: `-biaoshu-output-dir`
- Consumes env vars: `TANGYING_WORKSPACE_ROOT`, `TANGYING_LOCAL_DATA_DIR`, `BIAOSHU_OUTPUT_DIR`

- [ ] **Step 1: Add flags in main**

In `local-backend/cmd/local-agent/main.go`, add after `dataDir` flag:

```go
	workspaceRoot := flag.String("workspace-root", os.Getenv("TANGYING_WORKSPACE_ROOT"), "workspace repository root")
	biaoshuOutputDir := flag.String("biaoshu-output-dir", os.Getenv("BIAOSHU_OUTPUT_DIR"), "biaoshu output directory")
```

Change server creation:

```go
	server := localagent.NewServer(localagent.Config{
		DataDir:          *dataDir,
		CloudAPIBase:     *cloudAPIBase,
		WorkspaceRoot:    *workspaceRoot,
		BiaoshuOutputDir: *biaoshuOutputDir,
	})
```

- [ ] **Step 2: Update startup script**

In `scripts/start-local-backend.sh`, compute repo root from the script location and export stable paths:

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

export TANGYING_WORKSPACE_ROOT="${TANGYING_WORKSPACE_ROOT:-${REPO_ROOT}}"
export TANGYING_LOCAL_DATA_DIR="${TANGYING_LOCAL_DATA_DIR:-${REPO_ROOT}/local-backend/data}"
export BIAOSHU_OUTPUT_DIR="${BIAOSHU_OUTPUT_DIR:-${REPO_ROOT}/biaoshu-tools/output}"

mkdir -p "${TANGYING_LOCAL_DATA_DIR}" "${BIAOSHU_OUTPUT_DIR}"

cd "${REPO_ROOT}/local-backend"
go run ./cmd/local-agent \
  -addr "${TANGYING_LOCAL_AGENT_ADDR:-127.0.0.1:18080}" \
  -workspace-root "${TANGYING_WORKSPACE_ROOT}" \
  -data-dir "${TANGYING_LOCAL_DATA_DIR}" \
  -biaoshu-output-dir "${BIAOSHU_OUTPUT_DIR}"
```

If the existing script also passes cloud runner flags, preserve those arguments and only add the three stable path exports and flags.

- [ ] **Step 3: Verify script syntax**

Run:

```bash
bash -n scripts/start-local-backend.sh
```

Expected: exit 0.

- [ ] **Step 4: Verify local-agent package compiles**

Run:

```bash
cd local-backend && go test ./cmd/local-agent ./internal/localagent -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add local-backend/cmd/local-agent/main.go scripts/start-local-backend.sh
git commit -m "fix(local-agent): start with stable local paths"
```

---

### Task 7: End-to-End Regression Test

**Files:**
- Modify: `local-backend/internal/localagent/biaoshu_project_handler_test.go`

**Interfaces:**
- Consumes: POST `/api/local/biaoshu/projects`
- Consumes: GET `/api/local/biaoshu/projects`
- Consumes: DELETE `/api/local/biaoshu/projects/:projectId`

- [ ] **Step 1: Add create-delete-recreate regression test**

Add to `local-backend/internal/localagent/biaoshu_project_handler_test.go`:

```go
func TestBiaoshuProjectCreateDeleteRecreateSameName(t *testing.T) {
	root := t.TempDir()
	outputRoot := filepath.Join(root, "output")
	server := NewServer(Config{
		DataDir:         filepath.Join(root, "data"),
		WorkspaceRoot:   root,
		BiaoshuOutputDir: outputRoot,
	})
	if err := server.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := server.ValidateWritablePaths(); err != nil {
		t.Fatal(err)
	}

	create := func() BiaoshuProjectManifest {
		body := bytes.NewBufferString(`{"projectName":"养护","bidFilePath":"E:/bid/招标文件.docx"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/local/biaoshu/projects", body)
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Project BiaoshuProjectManifest `json:"project"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		return resp.Project
	}

	first := create()
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/local/biaoshu/projects/"+first.ProjectID, nil)
	deleteRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteRec.Code, deleteRec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(outputRoot, "养护")); !os.IsNotExist(err) {
		t.Fatalf("output dir should be deleted, err=%v", err)
	}

	second := create()
	if second.ProjectID == first.ProjectID {
		t.Fatalf("recreated project should have a new projectId")
	}
}
```

- [ ] **Step 2: Run regression test**

Run:

```bash
cd local-backend && go test ./internal/localagent -run TestBiaoshuProjectCreateDeleteRecreateSameName -count=1
```

Expected: PASS.

- [ ] **Step 3: Run full local backend tests**

Run:

```bash
cd local-backend && go test ./...
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add local-backend/internal/localagent/biaoshu_project_handler_test.go
git commit -m "test(biaoshu): cover create delete recreate lifecycle"
```

---

## Manual Verification

After all tasks are implemented:

- [ ] Stop any existing local-agent process on `127.0.0.1:18080`.
- [ ] Start with:

```bash
bash scripts/start-local-backend.sh
```

- [ ] Verify paths:

```bash
curl http://127.0.0.1:18080/api/local/paths
```

Expected JSON contains:

```json
{
  "dataDir": "E:\\lingxi\\tangying-ai-operation-system\\local-backend\\data"
}
```

Path separators may be `/` if the script runs through Git Bash; this is acceptable if the path resolves to the same directory.

- [ ] Create project:

```bash
curl -i -X POST http://127.0.0.1:18080/api/local/biaoshu/projects \
  -H "Content-Type: application/json" \
  --data "{\"projectName\":\"养护\",\"bidFilePath\":\"E:/bid/招标文件.docx\"}"
```

Expected: `201 Created`, response has `project.outputDir` under `E:\lingxi\tangying-ai-operation-system\biaoshu-tools\output\养护`.

- [ ] List projects:

```bash
curl http://127.0.0.1:18080/api/local/biaoshu/projects
```

Expected: response includes exactly the created project.

- [ ] Delete the created project:

```bash
curl -i -X DELETE http://127.0.0.1:18080/api/local/biaoshu/projects/<projectId>
```

Expected: `200 OK`, output directory removed.

- [ ] Recreate same project name:

```bash
curl -i -X POST http://127.0.0.1:18080/api/local/biaoshu/projects \
  -H "Content-Type: application/json" \
  --data "{\"projectName\":\"养护\",\"bidFilePath\":\"E:/bid/招标文件.docx\"}"
```

Expected: `201 Created`.

---

## Plan Self-Review

- Spec coverage: The plan addresses DataDir instability, `os.Getwd()` output-root instability, GET side effects, clearer errors, startup scripts, and create/delete/recreate behavior.
- Placeholder scan: The plan contains no unresolved marker words or unspecified “handle edge cases” instructions.
- Type consistency: `Config.WorkspaceRoot`, `Config.BiaoshuOutputDir`, `ValidateWritablePaths`, `writeProbeFile`, `defaultBiaoshuOutputDir`, and `findWorkspaceRoot` are defined before later tasks use them.
