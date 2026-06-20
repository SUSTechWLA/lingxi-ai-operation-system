# Local Asset Storage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move user-generated artifact payloads out of cloud database/object storage and establish local artifact storage as the default runtime boundary.

**Architecture:** Cloud artifact rows become metadata/index records only: kind, stage, hash, size, and deterministic `local://...` references. Local desktop agent owns actual user asset files under the OS application data directory. This patch handles the artifact layer first and documents remaining node-output/media upload migration risk.

**Tech Stack:** Go stdlib local agent, Go AIOS Core artifact service, PostgreSQL metadata tables, React/Electron client.

---

### Task 1: Cloud Artifact Metadata-Only Storage

**Files:**
- Modify: `cloud-backend/internal/core/artifact/service.go`
- Modify: `cloud-backend/internal/core/artifact/materializer.go`
- Test: `cloud-backend/internal/core/artifact/service_test.go`
- Test: `cloud-backend/internal/core/artifact/materializer_test.go`

- [ ] **Step 1: Write failing tests**
  - Assert `CreateArtifact` does not copy request data into `InlineJSON`.
  - Assert generated artifact requests use `StorageType=local` and `StorageRef=local://...`.

- [ ] **Step 2: Run tests to verify RED**

```bash
env GOCACHE=/private/tmp/tangying-go-build go test ./internal/core/artifact
```

- [ ] **Step 3: Implement metadata-only storage**
  - Preserve `ContentHash` and `SizeBytes`.
  - Store only local reference metadata.
  - Keep existing DB schema compatible by leaving `inline_json` empty.

- [ ] **Step 4: Run tests to verify GREEN**

```bash
env GOCACHE=/private/tmp/tangying-go-build go test ./internal/core/artifact
```

### Task 2: Local Artifact Store API

**Files:**
- Modify: `local-backend/internal/localagent/server.go`
- Modify: `local-backend/internal/localagent/server_test.go`

- [ ] **Step 1: Write failing tests**
  - POST an artifact payload and assert a local file is written.
  - GET the artifact and assert the payload is read back.

- [ ] **Step 2: Run tests to verify RED**

```bash
env GOCACHE=/private/tmp/tangying-go-build go test ./...
```

- [ ] **Step 3: Implement local artifact endpoints**
  - `POST /api/local/artifacts`
  - `GET /api/local/artifacts/:id`
  - Store content under `artifacts/<projectId>/<artifactId>/content`.
  - Store metadata beside it as `metadata.json`.

- [ ] **Step 4: Run tests to verify GREEN**

```bash
env GOCACHE=/private/tmp/tangying-go-build go test ./...
```

### Task 3: Documentation And Verification

**Files:**
- Modify: `docs/ARCHITECTURE.md`
- Modify: `docs/LOCAL_USAGE.md`
- Modify: `docs/CLOUD_DEPLOYMENT.md`

- [ ] **Step 1: Update docs**
  - Replace inline/minio default strategy with local-first user asset policy.
  - Mark cloud node-output/media upload persistence as a known follow-up risk until local execution is fully adopted.

- [ ] **Step 2: Run verification**

```bash
env GOCACHE=/private/tmp/tangying-go-build go test ./...       # cloud-backend
env GOCACHE=/private/tmp/tangying-go-build go test ./...       # local-backend
npm run build                                                  # frontend
```
