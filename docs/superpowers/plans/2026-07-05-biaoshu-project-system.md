# Biaoshu Project System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a complete local project management system for the Biaoshu workbench so project identity, stages, artifacts, recovery, and history no longer depend on cloud run records, inferred file names, or frontend memory.

**Architecture:** Add a local-backend Biaoshu project manifest as the source of truth under the app data directory, with an optional mirror `project.manifest.json` in each output directory for backup and migration reference. The frontend reads and writes projects through local-backend APIs, while cloud `agent_run` IDs become execution references only. Artifact generation and edits register into the manifest immediately; startup and history opening restore from manifest first, then verify files exist.

**Tech Stack:** Go local-backend `net/http`, JSON files under the local app data directory, React 18, TypeScript, existing local-agent service layer, existing cloud Biaoshu generation endpoints.

---

## Non-Negotiable Design Decisions

- Biaoshu projects use a stable `projectId`; `runId`, project name, bid file name, and output directory name are not project identity.
- The primary manifest is stored under local-backend app data, for example `TangyingAIOS/projects/biaoshu/<projectId>/project.json`.
- The output directory may contain `project.manifest.json`, but this is a mirror copy only. It does not make artifact files read-only and is not the runtime source of truth.
- Artifact files remain normal writable files. Users can regenerate, preview, revise, and write back Markdown or Word outputs.
- Cloud `agent_run` records are optional execution history. A missing cloud run must not prevent opening a local project.
- Artifact progress comes from manifest entries and file existence verification, not from scanning guessed folders.
- Directory scanning is a recovery fallback only, and it must verify ownership using `projectId`, `sourceFile`, or manifest metadata before adopting files.

---

## Target Data Model

```ts
type BiaoshuProjectStage =
  | 'created'
  | 'raw_parsed'
  | 'analysis_ready'
  | 'context_ready'
  | 'outline_ready'
  | 'chapters_ready'
  | 'wordcheck_ready'
  | 'draft_merged'
  | 'word_exported'
  | 'failed'

type BiaoshuArtifactKind =
  | 'BID_RAW_TEXT'
  | 'BID_ANALYSIS'
  | 'BID_PROJECT_CONTEXT'
  | 'BID_OUTLINE'
  | 'BID_CHAPTERS'
  | 'WORD_COUNT_REPORT'
  | 'MERGED_DRAFT'
  | 'TECHNICAL_BID_DOCX'

interface BiaoshuProjectManifest {
  schemaVersion: 'biaoshu.project.v1'
  projectId: string
  projectName: string
  status: 'CREATED' | 'RUNNING' | 'SUCCESS' | 'FAILED' | 'UNKNOWN'
  currentStage: BiaoshuProjectStage
  createdAt: string
  updatedAt: string
  sourceFiles: Array<{
    id: string
    path: string
    originalName: string
    sha256?: string
    addedAt: string
  }>
  outputDir: string
  runs: Array<{
    runId: string
    cloudTaskId?: string
    status: string
    startedAt: string
    endedAt?: string
    cloudAvailable?: boolean
  }>
  artifacts: Array<{
    id: string
    kind: BiaoshuArtifactKind
    name: string
    status: 'valid' | 'pending' | 'running' | 'failed' | 'missing' | 'stale'
    storageRef: string
    mimeType: string
    sourceFileId?: string
    dependsOn: string[]
    createdAt: string
    updatedAt: string
    metadata: Record<string, unknown>
  }>
  stageEvents: Array<{
    id: string
    type: string
    at: string
    message: string
    artifactId?: string
    runId?: string
  }>
}
```

---

## File Structure

- Create: `local-backend/internal/localagent/biaoshu_project_model.go`
  - Own manifest structs, constants, stage calculation, and validation.

- Create: `local-backend/internal/localagent/biaoshu_project_store.go`
  - Own paths, JSON read/write, atomic updates, mirror write, list summary, and migration from legacy history.

- Create: `local-backend/internal/localagent/biaoshu_project_handler.go`
  - Own HTTP handlers for project CRUD, artifact registration, artifact verification, and run registration.

- Modify: `local-backend/internal/localagent/server.go`
  - Register project-system routes.
  - Keep existing legacy routes during migration.

- Modify: `local-backend/internal/localagent/openapi.go`
  - Document new local Biaoshu project APIs.

- Create: `local-backend/internal/localagent/biaoshu_project_store_test.go`
  - Test project creation, artifact registration, stage updates, mirror write, and legacy migration.

- Create: `local-backend/internal/localagent/biaoshu_project_handler_test.go`
  - Test route behavior and file-existence verification.

- Modify: `frontend/src/services/localAgent.ts`
  - Add TypeScript types and API client functions for the project system.

- Create: `frontend/src/pages/biaoshuProjectSystem.ts`
  - Own frontend stage mapping, manifest-to-artifact conversion, and display summaries.

- Modify: `frontend/src/pages/biaoshuArtifactLogic.ts`
  - Keep artifact display helpers, but consume manifest-derived artifacts instead of guessed local paths.

- Modify: `frontend/src/pages/BiaoshuWorkbench.tsx`
  - Use `projectId` and manifest APIs for create/open/history/artifact registration.

- Modify: `frontend/scripts/biaoshu-artifact-logic-check.mjs`
  - Add focused checks for manifest-to-artifact conversion and stage summaries.

- Modify: `local-backend/docs/API_REFERENCE.md`
  - Regenerate via `go run ./cmd/gen-local-apidocs` after updating local OpenAPI.

---

### Task 1: Backend Manifest Model

**Files:**
- Create: `local-backend/internal/localagent/biaoshu_project_model.go`
- Create: `local-backend/internal/localagent/biaoshu_project_store_test.go`

- [ ] **Step 1: Write model tests**

Create `local-backend/internal/localagent/biaoshu_project_store_test.go` with:

```go
package localagent

import (
	"testing"
	"time"
)

func TestBiaoshuProjectStageFromArtifacts(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	manifest := BiaoshuProjectManifest{
		SchemaVersion: "biaoshu.project.v1",
		ProjectID:     "bp_123",
		ProjectName:   "养护",
		Status:        "SUCCESS",
		CreatedAt:     now,
		UpdatedAt:     now,
		Artifacts: []BiaoshuProjectArtifact{
			{ID: "a1", Kind: ArtifactKindBidAnalysis, Status: "valid", StorageRef: "analysis.md"},
			{ID: "a2", Kind: ArtifactKindProjectContext, Status: "valid", StorageRef: "context.md"},
		},
	}

	stage := biaoshuStageFromArtifacts(manifest.Artifacts, manifest.Status)
	if stage != StageContextReady {
		t.Fatalf("stage = %s, want %s", stage, StageContextReady)
	}
}

func TestValidateBiaoshuProjectManifestRejectsMissingIdentity(t *testing.T) {
	err := validateBiaoshuProjectManifest(BiaoshuProjectManifest{
		SchemaVersion: "biaoshu.project.v1",
		ProjectName:   "养护",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run:

```powershell
Set-Location local-backend
go test ./internal/localagent -run TestBiaoshuProject -count=1
```

Expected result:

```text
FAIL
undefined: BiaoshuProjectManifest
undefined: biaoshuStageFromArtifacts
```

- [ ] **Step 3: Add the manifest model**

Create `local-backend/internal/localagent/biaoshu_project_model.go`:

```go
package localagent

import (
	"errors"
	"fmt"
	"strings"
)

const BiaoshuProjectSchemaVersion = "biaoshu.project.v1"

type BiaoshuProjectStage string

const (
	StageCreated       BiaoshuProjectStage = "created"
	StageRawParsed     BiaoshuProjectStage = "raw_parsed"
	StageAnalysisReady BiaoshuProjectStage = "analysis_ready"
	StageContextReady  BiaoshuProjectStage = "context_ready"
	StageOutlineReady  BiaoshuProjectStage = "outline_ready"
	StageChaptersReady BiaoshuProjectStage = "chapters_ready"
	StageWordcheckReady BiaoshuProjectStage = "wordcheck_ready"
	StageDraftMerged   BiaoshuProjectStage = "draft_merged"
	StageWordExported  BiaoshuProjectStage = "word_exported"
	StageFailed        BiaoshuProjectStage = "failed"
)

type BiaoshuArtifactKind string

const (
	ArtifactKindRawText        BiaoshuArtifactKind = "BID_RAW_TEXT"
	ArtifactKindBidAnalysis    BiaoshuArtifactKind = "BID_ANALYSIS"
	ArtifactKindProjectContext BiaoshuArtifactKind = "BID_PROJECT_CONTEXT"
	ArtifactKindOutline        BiaoshuArtifactKind = "BID_OUTLINE"
	ArtifactKindChapters       BiaoshuArtifactKind = "BID_CHAPTERS"
	ArtifactKindWordCount      BiaoshuArtifactKind = "WORD_COUNT_REPORT"
	ArtifactKindMergedDraft    BiaoshuArtifactKind = "MERGED_DRAFT"
	ArtifactKindDocx           BiaoshuArtifactKind = "TECHNICAL_BID_DOCX"
)

type BiaoshuProjectManifest struct {
	SchemaVersion string                   `json:"schemaVersion"`
	ProjectID     string                   `json:"projectId"`
	ProjectName   string                   `json:"projectName"`
	Status        string                   `json:"status"`
	CurrentStage  BiaoshuProjectStage      `json:"currentStage"`
	CreatedAt     string                   `json:"createdAt"`
	UpdatedAt     string                   `json:"updatedAt"`
	SourceFiles   []BiaoshuProjectSourceFile `json:"sourceFiles"`
	OutputDir     string                   `json:"outputDir"`
	Runs          []BiaoshuProjectRun       `json:"runs"`
	Artifacts     []BiaoshuProjectArtifact  `json:"artifacts"`
	StageEvents   []BiaoshuProjectStageEvent `json:"stageEvents"`
}

type BiaoshuProjectSourceFile struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	OriginalName string `json:"originalName"`
	SHA256       string `json:"sha256,omitempty"`
	AddedAt      string `json:"addedAt"`
}

type BiaoshuProjectRun struct {
	RunID          string `json:"runId"`
	CloudTaskID    string `json:"cloudTaskId,omitempty"`
	Status         string `json:"status"`
	StartedAt      string `json:"startedAt"`
	EndedAt        string `json:"endedAt,omitempty"`
	CloudAvailable bool   `json:"cloudAvailable,omitempty"`
}

type BiaoshuProjectArtifact struct {
	ID           string                 `json:"id"`
	Kind         BiaoshuArtifactKind    `json:"kind"`
	Name         string                 `json:"name"`
	Status       string                 `json:"status"`
	StorageRef   string                 `json:"storageRef"`
	MimeType     string                 `json:"mimeType"`
	SourceFileID string                 `json:"sourceFileId,omitempty"`
	DependsOn    []string               `json:"dependsOn"`
	CreatedAt    string                 `json:"createdAt"`
	UpdatedAt    string                 `json:"updatedAt"`
	Metadata     map[string]interface{} `json:"metadata"`
}

type BiaoshuProjectStageEvent struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	At         string `json:"at"`
	Message    string `json:"message"`
	ArtifactID string `json:"artifactId,omitempty"`
	RunID      string `json:"runId,omitempty"`
}

func validateBiaoshuProjectManifest(manifest BiaoshuProjectManifest) error {
	if manifest.SchemaVersion != BiaoshuProjectSchemaVersion {
		return fmt.Errorf("unsupported schemaVersion: %s", manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.ProjectID) == "" {
		return errors.New("projectId is required")
	}
	if strings.TrimSpace(manifest.ProjectName) == "" {
		return errors.New("projectName is required")
	}
	if strings.TrimSpace(manifest.OutputDir) == "" {
		return errors.New("outputDir is required")
	}
	return nil
}

func biaoshuStageFromArtifacts(artifacts []BiaoshuProjectArtifact, status string) BiaoshuProjectStage {
	if status == "FAILED" {
		return StageFailed
	}
	valid := map[BiaoshuArtifactKind]bool{}
	for _, artifact := range artifacts {
		if artifact.Status == "valid" && strings.TrimSpace(artifact.StorageRef) != "" {
			valid[artifact.Kind] = true
		}
	}
	switch {
	case valid[ArtifactKindDocx]:
		return StageWordExported
	case valid[ArtifactKindMergedDraft]:
		return StageDraftMerged
	case valid[ArtifactKindWordCount]:
		return StageWordcheckReady
	case valid[ArtifactKindChapters]:
		return StageChaptersReady
	case valid[ArtifactKindOutline]:
		return StageOutlineReady
	case valid[ArtifactKindProjectContext]:
		return StageContextReady
	case valid[ArtifactKindBidAnalysis]:
		return StageAnalysisReady
	case valid[ArtifactKindRawText]:
		return StageRawParsed
	default:
		return StageCreated
	}
}
```

- [ ] **Step 4: Run the model tests**

Run:

```powershell
Set-Location local-backend
go test ./internal/localagent -run TestBiaoshuProject -count=1
```

Expected result:

```text
ok  	github.com/tangying-ai/aios-core/local-backend/internal/localagent
```

---

### Task 2: Backend Manifest Store

**Files:**
- Create: `local-backend/internal/localagent/biaoshu_project_store.go`
- Modify: `local-backend/internal/localagent/biaoshu_project_store_test.go`

- [ ] **Step 1: Add store tests**

Append to `local-backend/internal/localagent/biaoshu_project_store_test.go`:

```go
func TestBiaoshuProjectStoreCreateAndRegisterArtifact(t *testing.T) {
	dataDir := t.TempDir()
	s := NewServer(Config{DataDir: dataDir})
	if err := s.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	manifest, err := s.createBiaoshuProject(BiaoshuProjectCreateRequest{
		ProjectName: "养护",
		BidFilePath: `E:\yhbs\招标文件\招标文件_converted.docx`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ProjectID == "" {
		t.Fatal("projectId should be generated")
	}
	if manifest.CurrentStage != StageCreated {
		t.Fatalf("stage = %s, want %s", manifest.CurrentStage, StageCreated)
	}

	updated, err := s.registerBiaoshuProjectArtifact(manifest.ProjectID, BiaoshuArtifactRegisterRequest{
		Kind:       string(ArtifactKindBidAnalysis),
		Name:       "招标文件解析报告",
		StorageRef: manifest.OutputDir + `/00_招标文件解析报告.md`,
		MimeType:   "text/markdown",
		Status:     "valid",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.CurrentStage != StageAnalysisReady {
		t.Fatalf("stage = %s, want %s", updated.CurrentStage, StageAnalysisReady)
	}
	if len(updated.Artifacts) != 1 {
		t.Fatalf("artifact count = %d, want 1", len(updated.Artifacts))
	}
}
```

- [ ] **Step 2: Run the store test and confirm it fails**

Run:

```powershell
Set-Location local-backend
go test ./internal/localagent -run TestBiaoshuProjectStoreCreateAndRegisterArtifact -count=1
```

Expected result:

```text
FAIL
undefined: BiaoshuProjectCreateRequest
```

- [ ] **Step 3: Implement the store**

Create `local-backend/internal/localagent/biaoshu_project_store.go`:

```go
package localagent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const biaoshuProjectDirName = "biaoshu"
const biaoshuProjectManifestFilename = "project.json"
const biaoshuProjectMirrorFilename = "project.manifest.json"

type BiaoshuProjectCreateRequest struct {
	ProjectName string `json:"projectName"`
	BidFilePath string `json:"bidFilePath"`
	OutputDir   string `json:"outputDir,omitempty"`
}

type BiaoshuArtifactRegisterRequest struct {
	ID         string                 `json:"id,omitempty"`
	Kind       string                 `json:"kind"`
	Name       string                 `json:"name"`
	Status     string                 `json:"status"`
	StorageRef string                 `json:"storageRef"`
	MimeType   string                 `json:"mimeType"`
	DependsOn  []string               `json:"dependsOn,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

func (s *Server) biaoshuProjectRoot() string {
	return filepath.Join(s.paths.ProjectDir, biaoshuProjectDirName)
}

func (s *Server) biaoshuProjectDir(projectID string) string {
	return filepath.Join(s.biaoshuProjectRoot(), projectID)
}

func (s *Server) biaoshuProjectManifestPath(projectID string) string {
	return filepath.Join(s.biaoshuProjectDir(projectID), biaoshuProjectManifestFilename)
}

func (s *Server) createBiaoshuProject(req BiaoshuProjectCreateRequest) (BiaoshuProjectManifest, error) {
	projectName := strings.TrimSpace(req.ProjectName)
	bidFilePath := strings.TrimSpace(req.BidFilePath)
	if projectName == "" {
		return BiaoshuProjectManifest{}, errors.New("projectName is required")
	}
	if bidFilePath == "" {
		return BiaoshuProjectManifest{}, errors.New("bidFilePath is required")
	}
	projectID := newBiaoshuProjectID()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	outputDir := strings.TrimSpace(req.OutputDir)
	if outputDir == "" {
		outputDir = filepath.Join(s.paths.ArtifactDir, "biaoshu", projectID)
	}
	manifest := BiaoshuProjectManifest{
		SchemaVersion: BiaoshuProjectSchemaVersion,
		ProjectID:     projectID,
		ProjectName:   projectName,
		Status:        "CREATED",
		CurrentStage:  StageCreated,
		CreatedAt:     now,
		UpdatedAt:     now,
		OutputDir:     outputDir,
		SourceFiles: []BiaoshuProjectSourceFile{{
			ID:           "source_1",
			Path:         bidFilePath,
			OriginalName: filepath.Base(bidFilePath),
			AddedAt:      now,
		}},
		Artifacts: []BiaoshuProjectArtifact{},
		Runs:      []BiaoshuProjectRun{},
		StageEvents: []BiaoshuProjectStageEvent{{
			ID:      "event_" + projectID,
			Type:    "project_created",
			At:      now,
			Message: "项目已创建",
		}},
	}
	if err := s.writeBiaoshuProjectManifest(manifest); err != nil {
		return BiaoshuProjectManifest{}, err
	}
	return manifest, nil
}

func (s *Server) readBiaoshuProjectManifest(projectID string) (BiaoshuProjectManifest, error) {
	if !isSafePathSegment(projectID) {
		return BiaoshuProjectManifest{}, errors.New("projectId is required and must be a safe path segment")
	}
	data, err := os.ReadFile(s.biaoshuProjectManifestPath(projectID))
	if err != nil {
		return BiaoshuProjectManifest{}, err
	}
	var manifest BiaoshuProjectManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return BiaoshuProjectManifest{}, err
	}
	return manifest, validateBiaoshuProjectManifest(manifest)
}

func (s *Server) writeBiaoshuProjectManifest(manifest BiaoshuProjectManifest) error {
	manifest.CurrentStage = biaoshuStageFromArtifacts(manifest.Artifacts, manifest.Status)
	manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := validateBiaoshuProjectManifest(manifest); err != nil {
		return err
	}
	path := s.biaoshuProjectManifestPath(manifest.ProjectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := writeIndentedJSON(path, manifest); err != nil {
		return err
	}
	return s.writeBiaoshuProjectManifestMirror(manifest)
}

func (s *Server) writeBiaoshuProjectManifestMirror(manifest BiaoshuProjectManifest) error {
	if strings.TrimSpace(manifest.OutputDir) == "" {
		return nil
	}
	if err := os.MkdirAll(manifest.OutputDir, 0o755); err != nil {
		return err
	}
	return writeIndentedJSON(filepath.Join(manifest.OutputDir, biaoshuProjectMirrorFilename), manifest)
}

func (s *Server) registerBiaoshuProjectArtifact(projectID string, req BiaoshuArtifactRegisterRequest) (BiaoshuProjectManifest, error) {
	manifest, err := s.readBiaoshuProjectManifest(projectID)
	if err != nil {
		return BiaoshuProjectManifest{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	artifactID := strings.TrimSpace(req.ID)
	if artifactID == "" {
		artifactID = "artifact_" + strings.ToLower(string(req.Kind))
	}
	artifact := BiaoshuProjectArtifact{
		ID:         artifactID,
		Kind:       BiaoshuArtifactKind(req.Kind),
		Name:       strings.TrimSpace(req.Name),
		Status:     strings.TrimSpace(req.Status),
		StorageRef: strings.TrimSpace(req.StorageRef),
		MimeType:   strings.TrimSpace(req.MimeType),
		DependsOn:  req.DependsOn,
		CreatedAt:  now,
		UpdatedAt:  now,
		Metadata:   req.Metadata,
	}
	if artifact.Name == "" {
		artifact.Name = string(artifact.Kind)
	}
	if artifact.Status == "" {
		artifact.Status = "valid"
	}
	if artifact.MimeType == "" {
		artifact.MimeType = "text/markdown"
	}
	if artifact.Metadata == nil {
		artifact.Metadata = map[string]interface{}{}
	}
	replaced := false
	for i, existing := range manifest.Artifacts {
		if existing.Kind == artifact.Kind {
			artifact.CreatedAt = existing.CreatedAt
			manifest.Artifacts[i] = artifact
			replaced = true
			break
		}
	}
	if !replaced {
		manifest.Artifacts = append(manifest.Artifacts, artifact)
	}
	manifest.StageEvents = append(manifest.StageEvents, BiaoshuProjectStageEvent{
		ID:         fmt.Sprintf("event_%d", time.Now().UTC().UnixNano()),
		Type:       "artifact_registered",
		At:         now,
		Message:    "产物已登记: " + artifact.Name,
		ArtifactID: artifact.ID,
	})
	if artifact.Status == "valid" {
		manifest.Status = "SUCCESS"
	}
	if err := s.writeBiaoshuProjectManifest(manifest); err != nil {
		return BiaoshuProjectManifest{}, err
	}
	return s.readBiaoshuProjectManifest(projectID)
}

func (s *Server) listBiaoshuProjectManifests() ([]BiaoshuProjectManifest, error) {
	root := s.biaoshuProjectRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []BiaoshuProjectManifest{}, nil
		}
		return nil, err
	}
	manifests := []BiaoshuProjectManifest{}
	for _, entry := range entries {
		if !entry.IsDir() || !isSafePathSegment(entry.Name()) {
			continue
		}
		manifest, err := s.readBiaoshuProjectManifest(entry.Name())
		if err == nil {
			manifests = append(manifests, manifest)
		}
	}
	sort.SliceStable(manifests, func(i, j int) bool {
		return manifests[i].UpdatedAt > manifests[j].UpdatedAt
	})
	return manifests, nil
}

func newBiaoshuProjectID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("bp_%d", time.Now().UTC().UnixNano())
	}
	return "bp_" + hex.EncodeToString(b[:])
}
```

- [ ] **Step 4: Run store tests**

Run:

```powershell
Set-Location local-backend
go test ./internal/localagent -run TestBiaoshuProject -count=1
```

Expected result:

```text
ok  	github.com/tangying-ai/aios-core/local-backend/internal/localagent
```

---

### Task 3: Backend HTTP API

**Files:**
- Create: `local-backend/internal/localagent/biaoshu_project_handler.go`
- Modify: `local-backend/internal/localagent/server.go`
- Create: `local-backend/internal/localagent/biaoshu_project_handler_test.go`

- [ ] **Step 1: Write handler tests**

Create `local-backend/internal/localagent/biaoshu_project_handler_test.go`:

```go
package localagent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBiaoshuProjectRoutesCreateGetAndRegisterArtifact(t *testing.T) {
	s := NewServer(Config{DataDir: t.TempDir()})
	if err := s.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	createBody := bytes.NewBufferString(`{"projectName":"养护","bidFilePath":"E:/yhbs/招标文件/招标文件_converted.docx"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/local/biaoshu/projects", createBody)
	createRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Project BiaoshuProjectManifest `json:"project"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	registerBody := bytes.NewBufferString(`{"kind":"BID_ANALYSIS","name":"招标文件解析报告","storageRef":"E:/out/00_招标文件解析报告.md","mimeType":"text/markdown","status":"valid"}`)
	registerReq := httptest.NewRequest(http.MethodPost, "/api/local/biaoshu/projects/"+created.Project.ProjectID+"/artifacts", registerBody)
	registerRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(registerRec, registerReq)
	if registerRec.Code != http.StatusOK {
		t.Fatalf("register status = %d body=%s", registerRec.Code, registerRec.Body.String())
	}
	var updated struct {
		Project BiaoshuProjectManifest `json:"project"`
	}
	if err := json.Unmarshal(registerRec.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Project.CurrentStage != StageAnalysisReady {
		t.Fatalf("stage = %s, want %s", updated.Project.CurrentStage, StageAnalysisReady)
	}
}
```

- [ ] **Step 2: Run the handler test and confirm it fails**

Run:

```powershell
Set-Location local-backend
go test ./internal/localagent -run TestBiaoshuProjectRoutes -count=1
```

Expected result:

```text
FAIL
create status = 404
```

- [ ] **Step 3: Add route registration**

In `local-backend/internal/localagent/server.go`, add these routes in `routes()` near the existing Biaoshu routes:

```go
	s.mux.HandleFunc("/api/local/biaoshu/projects", s.handleBiaoshuProjectCollection)
	s.mux.HandleFunc("/api/local/biaoshu/projects/", s.handleBiaoshuProjectResource)
```

- [ ] **Step 4: Add HTTP handlers**

Create `local-backend/internal/localagent/biaoshu_project_handler.go`:

```go
package localagent

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Server) handleBiaoshuProjectCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		projects, err := s.listBiaoshuProjectManifests()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"projects": projects})
	case http.MethodPost:
		var req BiaoshuProjectCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid biaoshu project payload")
			return
		}
		project, err := s.createBiaoshuProject(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{"project": project})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleBiaoshuProjectResource(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/local/biaoshu/projects/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || !isSafePathSegment(parts[0]) {
		writeError(w, http.StatusBadRequest, "projectId is required and must be a safe path segment")
		return
	}
	projectID := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		project, err := s.readBiaoshuProjectManifest(projectID)
		if err != nil {
			writeError(w, http.StatusNotFound, "biaoshu project not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"project": project})
		return
	}
	if len(parts) == 2 && parts[1] == "artifacts" {
		s.handleBiaoshuProjectArtifacts(w, r, projectID)
		return
	}
	writeError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleBiaoshuProjectArtifacts(w http.ResponseWriter, r *http.Request, projectID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req BiaoshuArtifactRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid biaoshu artifact payload")
		return
	}
	project, err := s.registerBiaoshuProjectArtifact(projectID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"project": project})
}
```

- [ ] **Step 5: Run handler tests**

Run:

```powershell
Set-Location local-backend
go test ./internal/localagent -run TestBiaoshuProjectRoutes -count=1
```

Expected result:

```text
ok  	github.com/tangying-ai/aios-core/local-backend/internal/localagent
```

---

### Task 4: Frontend Local Agent Client

**Files:**
- Modify: `frontend/src/services/localAgent.ts`

- [ ] **Step 1: Add frontend project types**

In `frontend/src/services/localAgent.ts`, add:

```ts
export type BiaoshuProjectStage =
  | 'created'
  | 'raw_parsed'
  | 'analysis_ready'
  | 'context_ready'
  | 'outline_ready'
  | 'chapters_ready'
  | 'wordcheck_ready'
  | 'draft_merged'
  | 'word_exported'
  | 'failed'

export interface BiaoshuManagedArtifact {
  id: string
  kind: string
  name: string
  status: string
  storageRef: string
  mimeType: string
  sourceFileId?: string
  dependsOn: string[]
  createdAt: string
  updatedAt: string
  metadata: Record<string, unknown>
}

export interface BiaoshuProjectManifest {
  schemaVersion: 'biaoshu.project.v1'
  projectId: string
  projectName: string
  status: BiaoshuProjectStatus
  currentStage: BiaoshuProjectStage
  createdAt: string
  updatedAt: string
  sourceFiles: Array<{
    id: string
    path: string
    originalName: string
    sha256?: string
    addedAt: string
  }>
  outputDir: string
  runs: Array<{
    runId: string
    cloudTaskId?: string
    status: string
    startedAt: string
    endedAt?: string
    cloudAvailable?: boolean
  }>
  artifacts: BiaoshuManagedArtifact[]
  stageEvents: Array<{
    id: string
    type: string
    at: string
    message: string
    artifactId?: string
    runId?: string
  }>
}

export interface CreateBiaoshuManagedProjectRequest {
  projectName: string
  bidFilePath: string
  outputDir?: string
}

export interface RegisterBiaoshuManagedArtifactRequest {
  id?: string
  kind: string
  name: string
  status?: string
  storageRef: string
  mimeType?: string
  dependsOn?: string[]
  metadata?: Record<string, unknown>
}
```

- [ ] **Step 2: Add client functions**

Append these functions after the existing Biaoshu project history functions:

```ts
export async function fetchBiaoshuManagedProjects(): Promise<{ projects: BiaoshuProjectManifest[] }> {
  const response = await fetch(localAgentUrl('/api/local/biaoshu/projects'))
  if (!response.ok) {
    throw new Error(await errorMessage(response, '读取标书项目失败'))
  }
  return response.json() as Promise<{ projects: BiaoshuProjectManifest[] }>
}

export async function createBiaoshuManagedProject(
  payload: CreateBiaoshuManagedProjectRequest
): Promise<{ project: BiaoshuProjectManifest }> {
  const response = await fetch(localAgentUrl('/api/local/biaoshu/projects'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!response.ok) {
    throw new Error(await errorMessage(response, '创建标书项目失败'))
  }
  return response.json() as Promise<{ project: BiaoshuProjectManifest }>
}

export async function fetchBiaoshuManagedProject(
  projectId: string
): Promise<{ project: BiaoshuProjectManifest }> {
  const response = await fetch(localAgentUrl(`/api/local/biaoshu/projects/${encodeURIComponent(projectId)}`))
  if (!response.ok) {
    throw new Error(await errorMessage(response, '读取标书项目失败'))
  }
  return response.json() as Promise<{ project: BiaoshuProjectManifest }>
}

export async function registerBiaoshuManagedArtifact(
  projectId: string,
  payload: RegisterBiaoshuManagedArtifactRequest
): Promise<{ project: BiaoshuProjectManifest }> {
  const response = await fetch(localAgentUrl(`/api/local/biaoshu/projects/${encodeURIComponent(projectId)}/artifacts`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!response.ok) {
    throw new Error(await errorMessage(response, '登记标书产物失败'))
  }
  return response.json() as Promise<{ project: BiaoshuProjectManifest }>
}
```

- [ ] **Step 3: Run TypeScript build**

Run:

```powershell
Set-Location frontend
npm run build
```

Expected result:

```text
TypeScript may still fail until later tasks wire imports. No syntax errors should point to localAgent.ts.
```

---

### Task 5: Frontend Manifest Mapping Logic

**Files:**
- Create: `frontend/src/pages/biaoshuProjectSystem.ts`
- Modify: `frontend/scripts/biaoshu-artifact-logic-check.mjs`

- [ ] **Step 1: Add logic tests**

In `frontend/scripts/biaoshu-artifact-logic-check.mjs`, import the new module after the current artifact import:

```js
  const {
    biaoshuProjectToArtifacts,
    biaoshuProjectSummary,
  } = await import(pathToFileURL(new URL('../src/pages/biaoshuProjectSystem.ts', import.meta.url)))
```

Append before the final console log:

```js
  const projectSummary = biaoshuProjectSummary({
    projectId: 'bp_1',
    projectName: '养护',
    status: 'SUCCESS',
    currentStage: 'context_ready',
    artifacts: [
      { id: 'a1', kind: 'BID_ANALYSIS', name: '解析报告', status: 'valid', storageRef: 'E:/out/a.md', mimeType: 'text/markdown', dependsOn: [], createdAt: '2026-07-05T00:00:00Z', updatedAt: '2026-07-05T00:00:00Z', metadata: {} },
      { id: 'a2', kind: 'BID_PROJECT_CONTEXT', name: '背景确认表', status: 'valid', storageRef: 'E:/out/c.md', mimeType: 'text/markdown', dependsOn: [], createdAt: '2026-07-05T00:00:00Z', updatedAt: '2026-07-05T00:00:00Z', metadata: {} },
    ],
    updatedAt: '2026-07-05T00:00:00Z',
  })
  assert.equal(projectSummary.validArtifactCount, 2)
  assert.equal(projectSummary.artifactCount, 2)

  const projectArtifacts = biaoshuProjectToArtifacts({
    projectId: 'bp_1',
    projectName: '养护',
    status: 'SUCCESS',
    currentStage: 'context_ready',
    artifacts: [
      { id: 'a2', kind: 'BID_PROJECT_CONTEXT', name: '背景确认表', status: 'valid', storageRef: 'E:/out/c.md', mimeType: 'text/markdown', dependsOn: [], createdAt: '2026-07-05T00:00:00Z', updatedAt: '2026-07-05T00:00:00Z', metadata: {} },
    ],
    updatedAt: '2026-07-05T00:00:00Z',
  })
  assert.equal(projectArtifacts[0].kind, 'BID_PROJECT_CONTEXT')
  assert.equal(projectArtifacts[0].storageRef, 'E:/out/c.md')
```

- [ ] **Step 2: Create the frontend mapping module**

Create `frontend/src/pages/biaoshuProjectSystem.ts`:

```ts
import type { BiaoshuProjectManifest } from '../services/localAgent'
import type { BiaoshuArtifactRecord, BiaoshuArtifactStatus } from './biaoshuArtifactLogic'

export function biaoshuProjectToArtifacts(
  project: Pick<BiaoshuProjectManifest, 'artifacts' | 'updatedAt'>,
): BiaoshuArtifactRecord[] {
  return project.artifacts.map((artifact) => ({
    id: artifact.id,
    name: artifact.name || artifact.kind,
    kind: artifact.kind,
    version: '-',
    status: normalizeArtifactStatus(artifact.status),
    owner: ownerForArtifactKind(artifact.kind),
    updatedAt: formatProjectTime(artifact.updatedAt || project.updatedAt),
    storageRef: artifact.storageRef,
    summary: typeof artifact.metadata?.summary === 'string' ? artifact.metadata.summary : '-',
    sourceTool: sourceToolForArtifactKind(artifact.kind),
    metadata: artifact.metadata,
  }))
}

export function biaoshuProjectSummary(project: Pick<BiaoshuProjectManifest, 'artifacts' | 'status' | 'currentStage'>) {
  const artifactCount = project.artifacts.length
  const validArtifactCount = project.artifacts.filter((artifact) => artifact.status === 'valid').length
  return {
    status: project.status,
    currentStage: project.currentStage,
    artifactCount,
    validArtifactCount,
  }
}

function normalizeArtifactStatus(status: string): BiaoshuArtifactStatus {
  if (status === 'valid' || status === 'review' || status === 'pending' || status === 'running' || status === 'failed' || status === 'missing') {
    return status
  }
  if (status === 'stale') return 'pending'
  return 'pending'
}

function ownerForArtifactKind(kind: string): string {
  if (kind === 'BID_RAW_TEXT') return '文件解析'
  if (kind === 'BID_ANALYSIS') return 'AI分析'
  if (kind === 'BID_PROJECT_CONTEXT') return '信息确认'
  if (kind === 'BID_OUTLINE') return '大纲规划'
  if (kind === 'BID_CHAPTERS') return '章节编写'
  if (kind === 'WORD_COUNT_REPORT') return '质量检查'
  if (kind === 'MERGED_DRAFT') return '成稿整合'
  if (kind === 'TECHNICAL_BID_DOCX') return 'Word 导出'
  return '标书项目'
}

function sourceToolForArtifactKind(kind: string): string {
  if (kind === 'BID_RAW_TEXT') return 'parse_bid_files'
  if (kind === 'BID_ANALYSIS') return 'bid_analysis_report'
  if (kind === 'BID_PROJECT_CONTEXT') return 'project_context_report'
  if (kind === 'BID_OUTLINE') return 'outline_generator'
  if (kind === 'BID_CHAPTERS') return 'chapter_writer'
  if (kind === 'WORD_COUNT_REPORT') return 'chapter_word_checker'
  if (kind === 'MERGED_DRAFT') return 'merge_chapters'
  if (kind === 'TECHNICAL_BID_DOCX') return 'convert_to_word'
  return 'biaoshu_project'
}

function formatProjectTime(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}
```

- [ ] **Step 3: Run focused frontend logic check**

Run:

```powershell
Set-Location frontend
node scripts/biaoshu-artifact-logic-check.mjs
```

Expected result:

```text
biaoshuArtifactLogic tests passed
```

---

### Task 6: Frontend Workbench Integration

**Files:**
- Modify: `frontend/src/pages/BiaoshuWorkbench.tsx`

- [ ] **Step 1: Add project-system imports**

Add these imports:

```ts
import {
  createBiaoshuManagedProject,
  fetchBiaoshuManagedProject,
  fetchBiaoshuManagedProjects,
  registerBiaoshuManagedArtifact,
  type BiaoshuProjectManifest,
} from '../services/localAgent'
import { biaoshuProjectToArtifacts, biaoshuProjectSummary } from './biaoshuProjectSystem'
```

If names conflict with existing localAgent imports, merge them into the existing import block.

- [ ] **Step 2: Add manifest state**

In `BiaoshuWorkbench`, add:

```ts
  const [activeProject, setActiveProject] = useState<BiaoshuProjectManifest | null>(null)
```

- [ ] **Step 3: Derive artifacts from manifest first**

Replace the `artifacts` memo with:

```ts
  const manifestArtifacts = useMemo(
    () => activeProject ? biaoshuProjectToArtifacts(activeProject) : [],
    [activeProject],
  )
  const artifacts = useMemo(
    () => activeProject
      ? mergeManualBiaoshuArtifacts(manifestArtifacts, manualArtifacts)
      : mergeManualBiaoshuArtifacts(baseArtifacts, manualArtifacts),
    [activeProject, baseArtifacts, manifestArtifacts, manualArtifacts],
  )
```

- [ ] **Step 4: Create a managed project before starting a run**

At the beginning of `handleStart`, after validation and before `startAgentRun`, add:

```ts
      const managed = await createBiaoshuManagedProject({
        projectName: projectName || '未命名项目',
        bidFilePath,
      })
      setActiveProject(managed.project)
```

Use `managed.project.projectId` in later artifact registrations.

- [ ] **Step 5: Register generated artifacts**

In `handleReportGenerated`, after `upsertManualArtifact(...)`, register with local-backend when `activeProject` is available:

```ts
    if (activeProject?.projectId && filePath) {
      registerBiaoshuManagedArtifact(activeProject.projectId, {
        kind: kind || 'BID_ANALYSIS',
        name: String(artifact?.name || filePath.split(/[\\/]/).pop() || kind || '标书产物'),
        storageRef: filePath,
        mimeType: 'text/markdown',
        status: 'valid',
        metadata: {
          ...(typeof artifact?.metadata === 'object' && artifact.metadata ? artifact.metadata : {}),
          sourceFile,
        },
      }).then((response) => {
        setActiveProject(response.project)
      }).catch((err: unknown) => {
        addLog(`产物登记失败: ${err instanceof Error ? err.message : String(err)}`)
      })
    }
```

- [ ] **Step 6: Open history from manifest**

Replace the legacy history load in the initial `useEffect` with `fetchBiaoshuManagedProjects()` first:

```ts
    fetchBiaoshuManagedProjects().then((response) => {
      const latest = response.projects[0]
      if (!latest) return
      setActiveProject(latest)
      setProjectName(latest.projectName || DEFAULT_PROJECT_NAME)
      setBidFilePath(latest.sourceFiles[0]?.path || DEFAULT_BID_FILE_PATH)
      setActiveView('artifacts')
    }).catch(() => {
      // Keep legacy history fallback until migration is complete.
    })
```

Keep the old `fetchBiaoshuProjects()` path as a fallback until migration task completes.

- [ ] **Step 7: Open a history row by projectId**

When the history row is backed by `BiaoshuProjectManifest`, call:

```ts
      const response = await fetchBiaoshuManagedProject(item.projectId)
      setActiveProject(response.project)
      setProjectName(response.project.projectName)
      setBidFilePath(response.project.sourceFiles[0]?.path || DEFAULT_BID_FILE_PATH)
      setRun(null)
      setTrace(null)
      setReviews([])
      setManualArtifacts([])
      setActiveView('artifacts')
```

Cloud `refreshRunData` should only run if the user explicitly opens run diagnostics later.

---

### Task 7: Legacy History Migration

**Files:**
- Modify: `local-backend/internal/localagent/biaoshu_project_store.go`
- Modify: `local-backend/internal/localagent/biaoshu_project_store_test.go`

- [ ] **Step 1: Add migration test**

Append to `local-backend/internal/localagent/biaoshu_project_store_test.go`:

```go
func TestMigrateLegacyBiaoshuProjectsCreatesManifests(t *testing.T) {
	s := NewServer(Config{DataDir: t.TempDir()})
	if err := s.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	_, _, err := s.upsertBiaoshuProject(BiaoshuProjectRecord{
		RunID:       "agent_run_1",
		ProjectName: "养护",
		BidFilePath: "E:/yhbs/招标文件/招标文件_converted.docx",
		Status:      "SUCCESS",
		CreatedAt:   "2026-07-04T07:20:18Z",
		UpdatedAt:   "2026-07-04T08:20:18Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.migrateLegacyBiaoshuProjects()
	if err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}
	projects, err := s.listBiaoshuProjectManifests()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("project count = %d, want 1", len(projects))
	}
	if projects[0].Runs[0].RunID != "agent_run_1" {
		t.Fatalf("runId = %s, want agent_run_1", projects[0].Runs[0].RunID)
	}
}
```

- [ ] **Step 2: Implement migration**

Add to `local-backend/internal/localagent/biaoshu_project_store.go`:

```go
func (s *Server) migrateLegacyBiaoshuProjects() (int, error) {
	legacy, err := s.readBiaoshuProjects()
	if err != nil {
		return 0, err
	}
	existing, err := s.listBiaoshuProjectManifests()
	if err != nil {
		return 0, err
	}
	seenRuns := map[string]bool{}
	for _, project := range existing {
		for _, run := range project.Runs {
			seenRuns[run.RunID] = true
		}
	}
	created := 0
	for _, item := range legacy {
		if seenRuns[item.RunID] {
			continue
		}
		manifest, err := s.createBiaoshuProject(BiaoshuProjectCreateRequest{
			ProjectName: item.ProjectName,
			BidFilePath: item.BidFilePath,
		})
		if err != nil {
			return created, err
		}
		manifest.Status = item.Status
		manifest.CreatedAt = item.CreatedAt
		manifest.UpdatedAt = item.UpdatedAt
		manifest.Runs = append(manifest.Runs, BiaoshuProjectRun{
			RunID:          item.RunID,
			Status:         item.Status,
			StartedAt:      item.CreatedAt,
			EndedAt:        item.UpdatedAt,
			CloudAvailable: false,
		})
		if err := s.writeBiaoshuProjectManifest(manifest); err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}
```

- [ ] **Step 3: Call migration before listing managed projects**

In `handleBiaoshuProjectCollection`, before `listBiaoshuProjectManifests()` in the GET branch, add:

```go
		_, _ = s.migrateLegacyBiaoshuProjects()
```

- [ ] **Step 4: Run migration tests**

Run:

```powershell
Set-Location local-backend
go test ./internal/localagent -run TestMigrateLegacyBiaoshuProjectsCreatesManifests -count=1
```

Expected result:

```text
ok  	github.com/tangying-ai/aios-core/local-backend/internal/localagent
```

---

### Task 8: OpenAPI And Docs

**Files:**
- Modify: `local-backend/internal/localagent/openapi.go`
- Modify: `local-backend/docs/API_REFERENCE.md`

- [ ] **Step 1: Add OpenAPI paths**

In `local-backend/internal/localagent/openapi.go`, add docs for:

```text
GET  /api/local/biaoshu/projects
POST /api/local/biaoshu/projects
GET  /api/local/biaoshu/projects/{projectId}
POST /api/local/biaoshu/projects/{projectId}/artifacts
```

Use response schema names:

```text
BiaoshuProjectManifest
BiaoshuProjectListResponse
BiaoshuProjectCreateRequest
BiaoshuArtifactRegisterRequest
```

- [ ] **Step 2: Regenerate local API docs**

Run:

```powershell
Set-Location local-backend
go run ./cmd/gen-local-apidocs
```

Expected result:

```text
local-backend/docs/API_REFERENCE.md is regenerated with the new Biaoshu project APIs.
```

---

### Task 9: Verification

**Files:**
- Verify: `local-backend/internal/localagent/*.go`
- Verify: `frontend/src/pages/*.tsx`
- Verify: `frontend/src/services/localAgent.ts`

- [ ] **Step 1: Run local backend tests**

Run:

```powershell
Set-Location local-backend
go test ./...
```

Expected result:

```text
ok for all local-backend packages.
```

- [ ] **Step 2: Run frontend logic check**

Run:

```powershell
Set-Location frontend
node scripts/biaoshu-artifact-logic-check.mjs
```

Expected result:

```text
biaoshuArtifactLogic tests passed
```

- [ ] **Step 3: Run frontend build**

Run:

```powershell
Set-Location frontend
npm run build
```

Expected result:

```text
TypeScript and Vite build complete successfully.
```

- [ ] **Step 4: Manual acceptance path**

Run local backend and frontend:

```powershell
bash scripts/start-local-backend.sh
bash scripts/start-frontend.sh
```

Manual checks:

```text
1. Create a new Biaoshu project from a bid file.
2. Confirm local-backend creates TangyingAIOS/projects/biaoshu/<projectId>/project.json.
3. Generate the analysis report and confirm the manifest records BID_ANALYSIS.
4. Generate the project context report and confirm the manifest records BID_PROJECT_CONTEXT.
5. Restart the frontend and local backend.
6. Open history and confirm the project opens from manifest without cloud run lookup.
7. Confirm artifacts still open and can be edited.
8. Confirm the output directory has project.manifest.json as a mirror copy.
```

---

## Self-Review

- Scope: The plan is limited to the Biaoshu workbench and avoids broad video/self-media project generalization.
- Identity: `projectId` is the stable identity; file names, project names, and run IDs are references only.
- Persistence: The primary manifest is local-backend app data; the output-dir manifest is a mirror for migration and backup reference.
- Artifact mutability: Artifact files remain writable. The mirror manifest does not impose read-only behavior.
- Recovery: History opens from manifest first; cloud run loss no longer blocks local projects.
- Testing: Backend model, store, handler, migration, frontend mapping, and full build verification are covered.
