# Production Reference Voice Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every IP talking-head project use the approved sloth voice in production, while supporting an authorized user reference recording or an already-recorded narration without ChatTTS or preview-voice fallback.

**Architecture:** Keep the existing `ip_avatar_3d.render_talking_video` MCP schema byte-for-byte stable. Append one standard MCP tool, `ip_avatar_3d.synthesize_reference_voice`, and let the local MCP executor resolve the project-scoped voice selection before it invokes the existing renderer. Default and reference-clone modes call the new GPT-SoVITS tool; recorded narration is deterministically mastered locally. Both paths produce a hash-bound provenance sidecar that the existing renderer validates before accepting production `audioPath`.

**Tech Stack:** TypeScript/React, Go, Python 3, FastMCP, GPT-SoVITS `api_v2.py`, FFmpeg/ffprobe, Electron, repository contract tests.

## Global Constraints

- Preserve the owner recording provenance: `ip形象/ip音频.wav`, SHA-256 `0304b63d381c065e90b34ad373c6438f8b71123ffa74781f66fe191043570dd9`, 26.679729 seconds, 48 kHz mono PCM16.
- Runtime code uses `ip-assets/main-ip/voice/reference/main_ip_voice_ref_v1.wav`, not the Chinese source filename.
- Provider is exactly `gpt_sovits_local`; voice ID is exactly `main_ip_warm_knowledge_host_v1`.
- Do not add ChatTTS, macOS `say`, a remote voice provider, or any production fallback.
- Do not modify existing MCP tool names or input schemas. Only append `synthesize_reference_voice`.
- A running task continues to use its snapshotted ordered tool catalog. New tasks may discover the appended tool.
- Never place audio bytes or absolute local paths in cloud project state, plan prompts, logs, or model context.
- Preserve all unrelated and pre-existing worktree changes. Stage only the files named by the current task before each commit.

---

### Task 1: Lock the canonical voice asset and remove ChatTTS from creator contracts

**Files:**
- Modify: `mcp/ip_avatar_3d/test_default_aroll_assets.py`
- Modify: `frontend/src/features/creator-studio/types.ts`
- Modify: `frontend/src/features/creator-studio/logic.ts`
- Modify: `frontend/src/features/creator-studio/StartCreationPage.tsx`
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`

**Interfaces:**

```ts
export type CreatorVoiceProvider = 'gpt_sovits_local'

export interface CreatorVoiceSelection {
  mode: 'default_ip' | 'reference_clone' | 'recorded_narration'
  provider?: CreatorVoiceProvider
  voiceId?: string
  referenceArtifactId?: string
  referenceStorageRef?: string
  referenceContentHash?: string
  referenceMimeType?: string
  recordedNarrationArtifactId?: string
  recordedNarrationStorageRef?: string
  recordedNarrationContentHash?: string
  recordedNarrationMimeType?: string
  referenceText?: string
  referenceTextVerified?: boolean
  usageRightsConfirmed?: boolean
}
```

- [ ] **Step 1: Add failing asset and creator-contract assertions**

Extend `BundledDefaultArollAssetsTests` to assert:

```python
self.assertEqual(profile["voice"]["provider"], "gpt_sovits_local")
self.assertEqual(profile["voice"]["voiceId"], "main_ip_warm_knowledge_host_v1")
self.assertEqual(
    profile["voice"]["gptSovitsLocal"]["referenceAudioPath"],
    "voice/reference/main_ip_voice_ref_v1.wav",
)
self.assertEqual(
    _sha256(self.profile_root / "voice/reference/main_ip_voice_ref_v1.wav"),
    profile["voice"]["gptSovitsLocal"]["expectedReferenceAudioSha256"],
)
```

In `creator-studio-logic-check.mjs`, assert that the source contains the three voice modes, the default pinned provider/voice ID, audio-only upload handling, exact-reference-text verification, and authorization; assert it does not contain `chattts`, `macos_say`, or `say` as a selectable provider.

- [ ] **Step 2: Run the focused tests and observe the ChatTTS assertion fail**

Run:

```bash
python3 mcp/ip_avatar_3d/test_default_aroll_assets.py
cd frontend && npm run test:creator
```

Expected: the asset assertions pass; the creator source contract fails because `chattts_local` and the ChatTTS option still exist.

- [ ] **Step 3: Make GPT-SoVITS the only synthesis provider**

Remove `chattts_local` from `CreatorVoiceProvider`, remove the provider selector and provider state from `StartCreationPage`, and hard-code `provider: 'gpt_sovits_local'` only for `default_ip` and `reference_clone`. Keep `recorded_narration` provider-free because it bypasses TTS.

Preserve the existing local upload metadata:

```ts
metadata: {
  artifactType: voiceMode === 'recorded_narration' ? 'recorded_narration' : 'voice_reference',
  source: 'creator_studio',
  localOnly: true,
  usageRightsConfirmed,
}
```

- [ ] **Step 4: Verify the frontend and asset contracts**

Run:

```bash
python3 mcp/ip_avatar_3d/test_default_aroll_assets.py
cd frontend && npm run test:creator && npm run build && npm run lint
```

Expected: all commands exit zero.

- [ ] **Step 5: Commit only Task 1 files**

```bash
git add mcp/ip_avatar_3d/test_default_aroll_assets.py frontend/src/features/creator-studio/types.ts frontend/src/features/creator-studio/logic.ts frontend/src/features/creator-studio/StartCreationPage.tsx frontend/scripts/creator-studio-logic-check.mjs
git commit -m "feat: lock creator voice modes to gpt sovits"
```

### Task 2: Append the standard `synthesize_reference_voice` MCP tool

**Files:**
- Create: `mcp/ip_avatar_3d/reference_voice.py`
- Create: `mcp/ip_avatar_3d/test_reference_voice.py`
- Modify: `mcp/ip_avatar_3d/server.py`
- Modify: `mcp/ip_avatar_3d/test_server.py`
- Modify: `scripts/test_mcp_contracts.py`
- Modify: `mcp/ip_avatar_3d/README.md`

**Input contract:**

```python
def synthesize_reference_voice(
    text: str,
    outputDir: str,
    mode: str = "default_ip",
    characterProfilePath: str = "",
    provider: str = "gpt_sovits_local",
    voiceId: str = "",
    referenceAudioPath: str = "",
    referenceText: str = "",
    referenceTextVerified: bool = False,
    usageRightsConfirmed: bool = False,
    language: str = "zh",
    speed: float = 1.0,
    expectedReferenceAudioSha256: str = "",
) -> dict[str, Any]:
```

**Output contract:**

```json
{
  "schemaVersion": "tangying-reference-voice-result/v1",
  "status": "ready",
  "success": true,
  "mode": "default_ip",
  "provider": "gpt_sovits_local",
  "voiceId": "main_ip_warm_knowledge_host_v1",
  "audioPath": "/project-scoped/output/narration_master.wav",
  "provenancePath": "/project-scoped/output/narration_master.provenance.json",
  "durationSec": 29.84,
  "inputTextSha256": "hex-sha256",
  "referenceAudioSha256": "hex-sha256",
  "masteredFileSha256": "hex-sha256",
  "productionReady": true,
  "usageRightsConfirmed": true,
  "voice": {}
}
```

- [ ] **Step 1: Write failing service tests**

Cover these cases in `test_reference_voice.py`:

1. `default_ip` loads the pinned profile and deployable reference.
2. `reference_clone` requires non-empty exact reference text, `referenceTextVerified=true`, and `usageRightsConfirmed=true` before the client is called.
3. Provider values other than `gpt_sovits_local` fail before network I/O.
4. Reference hash mismatch fails before synthesis.
5. Output outside `outputDir` is rejected.
6. A successful fake client response is mastered to 48 kHz mono PCM16, and the audio hash matches the provenance sidecar.
7. The result and sidecar omit reference text, local model paths, and audio bytes.

In `test_server.py`, assert that `server.synthesize_reference_voice(...)` delegates to the shared service and returns the versioned result.

- [ ] **Step 2: Verify red**

Run:

```bash
python3 mcp/ip_avatar_3d/test_reference_voice.py
python3 mcp/ip_avatar_3d/test_server.py
```

Expected: imports or tool calls fail because the service and MCP tool do not exist.

- [ ] **Step 3: Implement the shared service and thin MCP wrapper**

Move no existing tool schema. Extract the shared GPT-SoVITS profile resolution, bundle arguments, and mastering helpers from `server.py` into `reference_voice.py`; import them back into `server.py`. `reference_voice.py` must not import `server.py`, which prevents a circular module dependency. The service must normalize `mode` to `default_ip` or `reference_clone`, build a temporary raw WAV, atomically publish `narration_master.wav`, and write `narration_master.provenance.json` only after mastering succeeds.

For `reference_clone`, override only:

```python
config["referenceAudioPath"] = verified_reference_audio
config["promptText"] = reference_text
config["promptTextVerified"] = True
```

The pinned GPT and SoVITS weights, model version, seed, settings, and loopback endpoint still come from the approved character profile.

- [ ] **Step 4: Append the tool to the MCP contract**

Add `synthesize_reference_voice` immediately after `check_gpt_sovits_voice` in `server.py`. Add exactly `"synthesize_reference_voice"` to the `ip_avatar_3d` expected tool set in `scripts/test_mcp_contracts.py`; do not reorder or edit any existing tool definition.

- [ ] **Step 5: Verify service, server, and stdio discovery**

Run:

```bash
python3 mcp/ip_avatar_3d/test_reference_voice.py
python3 mcp/ip_avatar_3d/test_server.py
python3 scripts/test_mcp_contracts.py
```

Expected: the service tests pass, the server tests pass, and MCP `tools/list` reports the original tools plus `synthesize_reference_voice` with a valid object JSON Schema.

- [ ] **Step 6: Commit only Task 2 files**

```bash
git add mcp/ip_avatar_3d/reference_voice.py mcp/ip_avatar_3d/test_reference_voice.py mcp/ip_avatar_3d/server.py mcp/ip_avatar_3d/test_server.py scripts/test_mcp_contracts.py mcp/ip_avatar_3d/README.md
git commit -m "feat: add standard reference voice mcp tool"
```

### Task 3: Resolve project-local voice artifacts and master recorded narration

**Files:**
- Create: `local-backend/internal/localtool/reference_voice.go`
- Create: `local-backend/internal/localtool/reference_voice_test.go`
- Modify: `local-backend/internal/localtool/mcp_tool_call.go`
- Modify: `local-backend/internal/localtool/mcp_tool_call_test.go`

**Local resolver contract:**

```go
type resolvedProjectVoiceArtifact struct {
    ProjectID   string
    ArtifactID  string
    StorageRef  string
    MIMEType    string
    ContentHash string
    ContentPath string
}

func resolveProjectVoiceArtifact(
    dataDir, projectID, artifactID, storageRef, expectedHash string,
    allowedMIME map[string]struct{},
) (resolvedProjectVoiceArtifact, error)
```

- [ ] **Step 1: Write failing resolver tests**

Create uploaded-artifact fixtures at:

```text
<dataDir>/artifacts/<projectId>/<artifactId>/content
<dataDir>/artifacts/<projectId>/<artifactId>/metadata.json
```

Assert rejection of traversal, remote URLs, absolute paths, mismatched project IDs, mismatched artifact IDs, mismatched `storageRef`, mismatched SHA-256, missing authorization, non-audio MIME, and unreadable content. Assert success only for WAV, MP3, M4A, or FLAC with matching metadata and computed content hash.

- [ ] **Step 2: Write failing recorded-narration mastering tests**

Inject an FFmpeg runner and assert that `masterRecordedNarration`:

- decodes the project-local input;
- writes 48 kHz mono PCM16 WAV;
- applies the same `-16 LUFS`, `-1.5 dBTP`, `LRA 7` policy as GPT-SoVITS output;
- writes a `tangying-production-audio-provenance/v1` sidecar containing source mode, input hash, output hash, duration, loudness, consent, and `productionReady=true`;
- never calls a TTS provider.

- [ ] **Step 3: Verify red**

Run:

```bash
cd local-backend
go test ./internal/localtool -run 'ProjectVoiceArtifact|RecordedNarrationMaster' -count=1
```

Expected: compilation fails because the resolver and mastering functions are absent.

- [ ] **Step 4: Implement project-scoped resolution and mastering**

Read `metadata.json`, validate `projectId`, `id`, `storageRef`, `mimeType`, and `contentHash`, then recompute SHA-256 over `content`. Resolve no path from cloud input. Build paths exclusively from validated safe segments under `<dataDir>/artifacts`.

Publish mastered output and provenance atomically under:

```text
<dataDir>/projects/<projectId>/voice/<voice-run-id>/narration_master.wav
<dataDir>/projects/<projectId>/voice/<voice-run-id>/narration_master.provenance.json
```

- [ ] **Step 5: Add the voice preprocessor to the existing MCP executor**

Before calling `ip_avatar_3d.render_talking_video`, inspect and then delete the internal-only `voiceSelection` argument:

```go
prepared, err := e.prepareIPArollVoice(operationCtx, client, projectID, voiceSelection, script)
if err != nil { return nil, err }
args["audioPath"] = prepared.AudioPath
args["outputDir"] = prepared.OutputDir
delete(args, "voiceSelection")
```

Read `projectID` only from the executor job payload keys `projectId` or `videoProjectId`; reject a missing or unsafe value before resolving artifacts. Route `default_ip` and `reference_clone` to `client.CallTool(..., "synthesize_reference_voice", ...)`. Route `recorded_narration` to `masterRecordedNarration`. Do not pass absolute reference paths, hashes, or consent fields to `render_talking_video` after preparation.

- [ ] **Step 6: Verify that the renderer receives only its unchanged schema**

In `mcp_tool_call_test.go`, use a fake MCP server to assert:

- default mode calls `synthesize_reference_voice` then `render_talking_video`;
- reference mode supplies the locally resolved content path only to the synthesis call;
- recorded mode calls only `render_talking_video` after local mastering;
- render arguments contain `audioPath` and `outputDir` but not `voiceSelection`, `referenceStorageRef`, or consent fields;
- any resolver/mastering failure prevents the renderer call.

Run:

```bash
cd local-backend
go test ./internal/localtool -run 'ProjectVoiceArtifact|RecordedNarrationMaster|IPArollVoice' -count=1
```

Expected: all focused tests exit zero.

- [ ] **Step 7: Commit only Task 3 files**

```bash
git add local-backend/internal/localtool/reference_voice.go local-backend/internal/localtool/reference_voice_test.go local-backend/internal/localtool/mcp_tool_call.go local-backend/internal/localtool/mcp_tool_call_test.go
git commit -m "feat: prepare project scoped production narration"
```

### Task 4: Compile voice selection into the A-roll run without leaking private paths

**Files:**
- Modify: `cloud-backend/internal/core/agentruntime/runner.go`
- Modify: `cloud-backend/internal/core/agentruntime/runner_test.go`
- Modify: `cloud-backend/internal/core/agentruntime/plan_compiler.go`
- Modify: `cloud-backend/internal/core/agentruntime/plan_compiler_test.go`
- Modify: `cloud-backend/internal/core/agentruntime/tool_snapshot_test.go`
- Modify: `cloud-backend/internal/agents/video/service/audio_master.go`
- Modify: `cloud-backend/internal/agents/video/service/audio_master_test.go`

**Compiler behavior:**

```json
{
  "voiceSelection": {
    "mode": "reference_clone",
    "provider": "gpt_sovits_local",
    "voiceId": "project_reference_voice_<projectId>",
    "referenceArtifactId": "voice-1",
    "referenceStorageRef": "local://projects/<projectId>/artifacts/voice-1/<hash>/reference.wav",
    "referenceContentHash": "sha256:hex",
    "referenceMimeType": "audio/wav",
    "referenceText": "录音中实际说出的完整文字",
    "referenceTextVerified": true,
    "usageRightsConfirmed": true
  }
}
```

- [ ] **Step 1: Write failing runner allow-list tests**

Add `voiceSelection` to the expected safe context defaults. Assert that it is copied unchanged into the plan’s first-step arguments, while `modelProviders`, absolute-path keys, audio bytes, and unrelated context remain excluded.

- [ ] **Step 2: Write failing compiler tests for all modes**

Add `TestPlanCompiler_ProjectsVoiceSelectionIntoIPAroll` as a table-driven test. Assert:

- missing selection on talking-head projects becomes pinned `default_ip`;
- `reference_clone` preserves project-scoped identifiers, hash, MIME, exact transcript, verification, and consent;
- `recorded_narration` preserves only the recorded artifact fields and consent;
- the `ip_aroll_main` request contains `voiceSelection` for the local preprocessor;
- no absolute path or audio byte field appears in marshalled plan JSON;
- non-talking-head plans receive no implicit IP voice.

Add `TestRunnerKeepsExistingToolSnapshotWhenSynthesisToolRegisters` in `tool_snapshot_test.go`: attach a snapshot containing the original IP-avatar tools, append `ip_avatar_3d.synthesize_reference_voice` to the live resolver, and assert the first run keeps byte-identical canonical JSON while a newly started run receives a different snapshot containing the appended tool.

- [ ] **Step 3: Verify red**

Run:

```bash
cd cloud-backend
go test ./internal/core/agentruntime -run 'VoiceSelection|IPAroll|SynthesisToolRegisters' -count=1
```

Expected: tests fail because `voiceSelection` is not in the runner safe-context allow-list and the compiler does not project it.

- [ ] **Step 4: Implement strict projection and defaulting**

Add a `requestedVoiceSelection(plan)` helper that copies only the fields named in `CreatorVoiceSelection`. Validate mode/provider combinations and default the missing talking-head selection to:

```go
map[string]interface{}{
    "mode": "default_ip",
    "provider": "gpt_sovits_local",
    "voiceId": "main_ip_warm_knowledge_host_v1",
}
```

Place the safe map in `renderArguments["voiceSelection"]`; do not change the render MCP manifest.

- [ ] **Step 5: Extend audio-master fingerprints**

Add these request fields and JSON fingerprint members:

```go
VoiceMode                string
ReferenceContentHash     string
ReferenceTranscriptHash string
SynthesisSettingsHash    string
MasteredOutputHash       string
```

Add table-driven tests proving that changing any one of mode, voice profile, reference hash, transcript hash, synthesis settings, recorded-narration hash, or mastered output hash changes the audio-master fingerprint and revision.

- [ ] **Step 6: Verify cloud behavior**

Run:

```bash
cd cloud-backend
go test ./internal/core/agentruntime ./internal/agents/video/service -run 'VoiceSelection|IPAroll|SynthesisToolRegisters|AudioMaster' -count=1
```

Expected: all focused tests exit zero.

- [ ] **Step 7: Commit only Task 4 files**

```bash
git add cloud-backend/internal/core/agentruntime/runner.go cloud-backend/internal/core/agentruntime/runner_test.go cloud-backend/internal/core/agentruntime/plan_compiler.go cloud-backend/internal/core/agentruntime/plan_compiler_test.go cloud-backend/internal/core/agentruntime/tool_snapshot_test.go cloud-backend/internal/agents/video/service/audio_master.go cloud-backend/internal/agents/video/service/audio_master_test.go
git commit -m "feat: compile production voice selection into arroll"
```

### Task 5: Let the existing renderer accept only hash-bound production masters

**Files:**
- Modify: `mcp/ip_avatar_3d/server.py`
- Modify: `mcp/ip_avatar_3d/test_server.py`
- Modify: `mcp/ip_avatar_3d/render_warm_studio_demo.py`
- Modify: `mcp/ip_avatar_3d/test_warm_studio_demo.py`

**Invariant:** `render_talking_video` keeps exactly its current Python signature and MCP JSON Schema.

- [ ] **Step 1: Snapshot the existing render schema before changing behavior**

Add a contract assertion in `test_server.py` that compares `inspect.signature(server.render_talking_video).parameters` to this exact ordered tuple and keep it after implementation:

```python
(
    "script", "modelPath", "characterProfilePath", "audioPath", "subtitlePath",
    "backgroundPath", "sceneBlendPath", "outputDir", "characterId", "shotId",
    "durationSec", "fps", "width", "height", "transparent", "faceScreenMode",
    "rigMode", "preserveExistingRig", "enhanceExistingRig", "elbowRig",
    "mouthMode", "mouthHeightRatio", "mouthScale", "mouthStyle",
    "facialDetailMode", "facialTopologyMode", "backgroundBrightness",
    "cameraPreset", "lightingPreset", "renderEngine", "qualityPreset",
    "renderDetailMode", "targetCharacterHeight", "motionStyle", "voiceName",
    "speakingRate", "voiceProvider", "voiceId", "voiceLanguage", "voiceSpeed",
    "blenderTimeoutSec", "dryRun", "renderMode", "fallbackPolicy",
    "presentationMode", "actionSequence",
)
```

- [ ] **Step 2: Write failing provenance acceptance tests**

Cover:

1. A bare production `audioPath` remains rejected.
2. A production master plus adjacent `narration_master.provenance.json` is accepted only when the sidecar schema, output hash, source mode, consent, mastering format, loudness, and `productionReady` values all pass.
3. Tampered audio, path escape, preview-only provenance, missing consent for custom modes, or an unapproved provider is rejected.
4. Valid `default_ip`, `reference_clone`, and `recorded_narration` masters become the authoritative duration and skip internal TTS.

- [ ] **Step 3: Verify red**

Run:

```bash
python3 mcp/ip_avatar_3d/test_server.py
python3 mcp/ip_avatar_3d/test_warm_studio_demo.py
```

Expected: the new verified-master cases fail because all production `audioPath` values are currently rejected.

- [ ] **Step 4: Implement provenance validation without changing the tool schema**

Add `_validate_production_audio_master(audio_path, output_dir)` in `server.py`. Require the master and sidecar to be under the same resolved `outputDir`, recompute the file hash, verify 48 kHz mono PCM16, and return sanitized provenance. When it passes, set `audioSource` to `production_audio_master`, skip `ensure_audio`, and use audio duration as the timing authority.

Mirror the same rule in `render_warm_studio_demo.py` so both render entry points reject unverified production input consistently.

- [ ] **Step 5: Verify renderer behavior and schema stability**

Run:

```bash
python3 mcp/ip_avatar_3d/test_server.py
python3 mcp/ip_avatar_3d/test_warm_studio_demo.py
python3 scripts/test_mcp_contracts.py
```

Expected: all tests pass and the original `render_talking_video` schema is unchanged.

- [ ] **Step 6: Commit only Task 5 files**

```bash
git add mcp/ip_avatar_3d/server.py mcp/ip_avatar_3d/test_server.py mcp/ip_avatar_3d/render_warm_studio_demo.py mcp/ip_avatar_3d/test_warm_studio_demo.py
git commit -m "fix: accept only verified production narration masters"
```

### Task 6: Document, verify, package, and smoke-test the complete voice path

**Files:**
- Modify: `README.md`
- Modify: `docs/mcp-providers.md`
- Modify: `docs/local-ip-talking-avatar-render.md`
- Modify: `docs/PROJECT_INTRODUCTION_EN.md`
- Verify: `ip-assets/main-ip/character-profile.json`
- Verify: `ip-assets/main-ip/voice/reference/main_ip_voice_ref_v1.wav`
- Verify: `frontend/release/Tangying-AI-Video-Creator-0.2.1-mac-arm64.dmg`

- [ ] **Step 1: Update user and operator documentation**

Document the three user modes, the exact local privacy boundary, authorization requirements, the new MCP input/output contract, fail-closed behavior, and the fact that ChatTTS/macOS `say` are not production providers. Do not document the Chinese source filename as a runtime dependency.

- [ ] **Step 2: Run the complete relevant test matrix**

```bash
python3 mcp/ip_avatar_3d/test_default_aroll_assets.py
python3 mcp/ip_avatar_3d/test_gpt_sovits_client.py
python3 mcp/ip_avatar_3d/test_reference_voice.py
python3 mcp/ip_avatar_3d/test_voice_policy.py
python3 mcp/ip_avatar_3d/test_server.py
python3 mcp/ip_avatar_3d/test_warm_studio_demo.py
python3 scripts/test_mcp_contracts.py
cd cloud-backend && go test ./internal/core/agentruntime ./internal/agents/video/service -count=1
cd ../local-backend && go test ./internal/localtool ./internal/localagent -count=1
cd ../frontend && npm run test:creator && npm run test:settings && npm run build && npm run lint
node --test electron/local-agent-runtime.test.cjs
cd .. && git diff --check
```

Expected: every command exits zero.

- [ ] **Step 3: Build and verify the desktop package**

```bash
bash scripts/build-local-desktop.sh --skip-install
hdiutil verify frontend/release/Tangying-AI-Video-Creator-0.2.1-mac-arm64.dmg
codesign --verify --deep --strict --verbose=2 "frontend/release/mac-arm64/Tangying AI Video Creation Assistant.app"
```

Expected: packaging succeeds, the DMG verifies, and the application signature is valid.

- [ ] **Step 4: Smoke-test all three voice modes in the installed client**

Using a real local project and local GPT-SoVITS runtime:

1. Default IP mode produces a production master whose provider and voice ID match the pinned profile.
2. Reference clone cannot start without a file, exact transcript verification, and authorization.
3. Authorized reference clone produces a new mastered narration and preserves the reference file locally.
4. Recorded narration bypasses GPT-SoVITS, is mastered, and drives timing.
5. Stopping GPT-SoVITS makes default/reference modes fail with an actionable error and no preview fallback.
6. Completed video playback and subsequent regeneration still use the selected current audio revision.

Capture project ID, run ID, voice mode, provider, input/output hashes, MCP tool revision, and final video path in the smoke-test report; do not capture audio bytes or absolute private paths.

- [ ] **Step 5: Commit documentation only after the smoke test passes**

```bash
git add README.md docs/mcp-providers.md docs/local-ip-talking-avatar-render.md docs/PROJECT_INTRODUCTION_EN.md
git commit -m "docs: document production reference voice workflow"
```
