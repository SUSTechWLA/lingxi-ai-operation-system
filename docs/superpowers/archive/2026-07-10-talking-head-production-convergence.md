# Talking Head Production Convergence Implementation Plan

> **For agentic workers:** Execute inline in the current worktree. Do not create a worktree, commit, reset, clean, or overwrite unrelated local changes.

**Goal:** Close one production-grade talking-head slice from canonical profile selection through an audio-master-derived shot plan, layer-scoped invalidation, strict candidate acceptance, and final-assembly rejection.

**Architecture:** Keep the dynamic Agent `PlanCompiler` as the Director Studio runtime DAG source. Extend the existing video `ShotUnit`, `ShotCandidate`, artifact provenance, repair, and final-assembly contracts; legacy YAML/Go templates remain compatibility surfaces and must not become a second runtime. Persist canonical profile metadata while preserving legacy project modes.

**Tech Stack:** Go 1.25 cloud backend, Go 1.24 local backend, React/TypeScript frontend, existing HyperFrames and local runner adapters.

## Global Constraints

- Preserve all pre-existing modified and untracked files.
- Use additive JSON-compatible fields with schema versions and legacy normalization.
- Store and calculate canonical video time in integer milliseconds; seconds remain adapter fields only.
- Production strict mode rejects fixture, fallback, placeholder, stale, or revision-mismatched candidates.
- Do not create a parallel workflow engine, artifact store, candidate model, or renderer.
- Write failing tests before production behavior changes.

---

### Task 1: Canonical profile and runtime source

**Files:**
- Modify: `cloud-backend/internal/agents/video/model/project.go`
- Modify: `cloud-backend/internal/agents/video/model/creation.go`
- Modify: `cloud-backend/internal/agents/video/service/profile.go`
- Modify: `cloud-backend/internal/agents/video/service/project_service.go`
- Modify: `cloud-backend/internal/core/agentruntime/plan_compiler.go`
- Modify: `cloud-backend/internal/core/localrunner/preflight.go`
- Modify: `frontend/src/pages/directorStudioLogic.ts`
- Modify: `frontend/src/pages/DirectorStudioPage.tsx`
- Test: existing adjacent Go tests and `frontend/scripts/director-studio-logic-check.mjs`

**Produces:** `talking_head` and `cinematic_story` canonical IDs; `voice_visual`, `aigc_shot`, workflow IDs, and historical names normalize without changing persisted project mode semantics.

- [ ] Add failing alias-normalization and project compatibility tests.
- [ ] Add failing compiler/preflight parity assertions proving `dynamic-agent-video-creation` is the runtime source.
- [ ] Implement shared normalization and canonical profile metadata.
- [ ] Change Director Studio selection IDs to canonical IDs while continuing to persist legacy modes.
- [ ] Run focused Go and frontend logic tests.

### Task 2: Strict execution provenance gate

**Files:**
- Modify: `cloud-backend/internal/agents/video/model/creation.go`
- Modify: `cloud-backend/internal/agents/video/service/shot_repair.go`
- Modify: `cloud-backend/internal/agents/video/service/final_assembly.go`
- Modify: `cloud-backend/internal/core/artifact/materializer.go`
- Modify: `local-backend/internal/localtool/hyperframes_render.go`
- Test: `cloud-backend/internal/agents/video/service/closed_beta_pipeline_test.go`
- Test: artifact materializer and local HyperFrames render tests

**Produces:** normalized `executionMode`, `productionEligible`, fallback reason, and strict/draft assembly policy.

- [ ] Add failing tests for fallback/placeholder candidate acceptance and final assembly.
- [ ] Add failing tests proving placeholder manifests are not `valid` production artifacts.
- [ ] Implement provenance normalization and strict gates.
- [ ] Preserve fixture diagnostics through an explicit non-production policy.
- [ ] Run focused cloud/local tests.

### Task 3: Audio Master and millisecond adapters

**Files:**
- Create: `cloud-backend/internal/agents/video/model/talking_head.go`
- Create: `cloud-backend/internal/agents/video/service/audio_master.go`
- Modify: `cloud-backend/internal/agents/video/service/time_window_plan.go`
- Modify: `cloud-backend/internal/core/worker/tool/builtin/video_creation_external_tools.go`
- Modify: `cloud-backend/internal/core/agentruntime/plan_compiler.go`
- Test: new adjacent service tests and compiler tests

**Produces:** schema-versioned estimated/real Audio Master timeline, `int64` millisecond shot windows, seconds adapters, timeline revision/fingerprint, and a reachable `audio_master` compiler stage.

- [ ] Add failing serialization and seconds-to-milliseconds compatibility tests.
- [ ] Add failing Audio Master validation and 3–15 second shot tests.
- [ ] Add failing compiler test requiring `audio_master` before `time_window`.
- [ ] Implement estimated Audio Master creation without claiming synthesized audio.
- [ ] Make time-window output reference the Audio Master revision.
- [ ] Run focused tests.

### Task 4: Existing ShotUnit layer graph and scoped repair

**Files:**
- Modify: `cloud-backend/internal/agents/video/model/talking_head.go`
- Create: `cloud-backend/internal/agents/video/service/layer_state.go`
- Modify: `cloud-backend/internal/agents/video/service/shot_repair.go`
- Modify: `cloud-backend/internal/agents/video/service/final_assembly.go`
- Test: new layer-state tests and closed-beta integration fixture

**Produces:** audio/IP/text/B-roll/composition revisions on `ShotUnit`, deterministic invalidation scopes, accepted-candidate invalidation, layer fingerprinting, and minimal repair routing.

- [ ] Add failing tests for script, subtitle-style, B-roll, and IP-motion invalidation.
- [ ] Add failing tests for timeline/layer revision mismatch at the accepted and final gates.
- [ ] Implement the minimal dependency graph and repair mapping.
- [ ] Verify unrelated layers remain current.

### Task 5: Talking-head policy, manifest, and UI evidence

**Files:**
- Create: `cloud-backend/internal/agents/video/service/talking_head_policy.go`
- Modify: `cloud-backend/internal/agents/video/model/talking_head.go`
- Create: `frontend/src/features/director-studio/talking-head/selectors.ts`
- Modify: `frontend/src/pages/DirectorStudioPage.tsx`
- Modify: `frontend/scripts/director-studio-logic-check.mjs`

**Produces:** deterministic visual-mode policy, B-roll manifest validation, canonical HyperFrames naming, and compact project/layer/provenance summaries without adding more logic to the page.

- [ ] Add failing policy/manifest/frontend selector tests.
- [ ] Implement deterministic low-confidence manual review and B-roll license blocking.
- [ ] Replace visible HyperGen aliases with HyperFrames while keeping backend legacy aliases readable.
- [ ] Show execution mode, eligibility, timeline revision, and stale reason from backend metadata.

### Task 6: Documentation and verification

**Files:**
- Create: `docs/talking-head-runtime.md`
- Modify only task-relevant release documentation if needed.

- [ ] Document the actual Director Studio → dynamic compiler → local runner path and historical adapter status.
- [ ] Run targeted tests, cloud Go tests, local Go tests, frontend director test/typecheck/build, HyperFrames build, and smoke tests when environment permits.
- [ ] Run `git diff --check`, `git diff --stat`, `git diff --name-only`, and compare against the captured baseline.
- [ ] Report fixture-only versus real-provider verification explicitly.
