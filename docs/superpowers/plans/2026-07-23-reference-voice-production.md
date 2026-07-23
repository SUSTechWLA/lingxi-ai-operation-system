# Reference Voice Production Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the approved sloth reference voice the default A-roll voice and support authorized project-scoped reference recordings through a standard local MCP synthesis tool.

**Architecture:** Keep GPT-SoVITS as the pinned production engine and extract its existing synthesis/mastering path into a reusable reference-voice service used by both `synthesize_reference_voice` and `render_talking_video`. The creator stores only project-scoped local artifact identity and voice policy; the local runner resolves and verifies audio before an MCP call. ChatTTS is an optional fail-closed adapter behind the same MCP contract.

**Tech Stack:** React 19 and TypeScript, Go cloud/local runners, Python MCP SDK, GPT-SoVITS API v2, optional ChatTTS, FFmpeg, JSON Schema, Electron.

## Global Constraints

- The default main IP voice source is SHA-256 `0304b63d381c065e90b34ad373c6438f8b71123ffa74781f66fe191043570dd9`.
- Existing MCP tool schemas remain unchanged; `ip_avatar_3d.synthesize_reference_voice` is appended.
- User recordings stay local and are never sent to remote ASR or TTS by default.
- Reference cloning requires exact reference text, `referenceTextVerified=true`, and `usageRightsConfirmed=true`.
- Production synthesis never silently falls back to Apple, Kokoro, another voice, or another provider.
- Cloud state never contains absolute local paths or audio bytes.
- Files and new identifiers use English names.

---

### Task 1: Lock the canonical IP voice and remove the preview-voice override

**Files:**
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`
- Modify: `frontend/src/features/creator-studio/logic.ts`
- Modify: `ip-assets/main-ip/character-profile.json`
- Modify: `ip-assets/main-ip/manifests/default-aroll-assets.json`
- Modify: `mcp/ip_avatar_3d/test_default_aroll_assets.py`
- Modify: `mcp/ip_avatar_3d/test_warm_studio_contract.py`

**Interfaces:**
- Produces: `buildCreationRequest()` output with `ipRenderMode: "production"` and `voiceSelection.mode: "default_ip"` for talking-head creation.
- Produces: asset provenance fields `approvedSourceSha256`, `approvedSourceDurationSec`, and `derivedReference`.

- [ ] **Step 1: Write failing frontend and Python contract assertions**

Add assertions equivalent to:

```js
assert.equal(talkingHeadRequest.project.config.ipRenderMode, 'production')
assert.deepEqual(talkingHeadRequest.project.config.voiceSelection, {
  mode: 'default_ip',
  provider: 'gpt_sovits_local',
  voiceId: 'main_ip_warm_knowledge_host_v1',
})
```

```python
self.assertEqual(
    profile["voice"]["sourceProvenance"]["approvedSourceSha256"],
    "0304b63d381c065e90b34ad373c6438f8b71123ffa74781f66fe191043570dd9",
)
self.assertEqual(manifest["assets"]["voice"]["provider"], "gpt_sovits_local")
```

- [ ] **Step 2: Verify red**

Run:

```bash
cd frontend && npm run test:creator
cd .. && python3 mcp/ip_avatar_3d/test_default_aroll_assets.py
```

Expected: assertions fail because creator requests still use preview mode and the manifest has no formal voice asset.

- [ ] **Step 3: Implement the minimal default policy and provenance**

Change the talking-head request builder to:

```ts
const ipRenderMode = productionRoute === 'talking_head' ? 'production' : undefined
const voiceSelection = productionRoute === 'talking_head'
  ? { mode: 'default_ip' as const, provider: 'gpt_sovits_local', voiceId: 'main_ip_warm_knowledge_host_v1' }
  : undefined
```

Record the source recording hash/duration and the derived 8.542-second English-named reference in the profile and default asset manifest.

- [ ] **Step 4: Verify green**

Run the same focused commands and expect exit zero.

### Task 2: Add the reusable reference-voice synthesis service and MCP tool

**Files:**
- Create: `mcp/ip_avatar_3d/reference_voice.py`
- Create: `mcp/ip_avatar_3d/chattts_client.py`
- Create: `mcp/ip_avatar_3d/test_reference_voice.py`
- Modify: `mcp/ip_avatar_3d/server.py`
- Modify: `mcp/ip_avatar_3d/test_server.py`
- Modify: `mcp/ip_avatar_3d/README.md`
- Modify: `scripts/test_mcp_contracts.py`

**Interfaces:**
- Produces:

```python
def synthesize_reference_voice_service(
    *,
    text: str,
    provider: str,
    voice_id: str,
    reference_audio_path: str,
    reference_text: str,
    reference_text_verified: bool,
    usage_rights_confirmed: bool,
    language: str,
    speed: float,
    output_dir: str,
    character_profile_path: str = "",
) -> dict[str, Any]
```

- Produces MCP tool `synthesize_reference_voice(...) -> dict[str, Any]`.
- Produces optional `ChatTTSClient.preflight()` and `ChatTTSClient.synthesize()` with no import-time ChatTTS dependency.

- [ ] **Step 1: Write failing service tests**

Cover:

```python
with self.assertRaisesRegex(ValueError, "usage rights"):
    synthesize_reference_voice_service(..., usage_rights_confirmed=False)
with self.assertRaisesRegex(ValueError, "verified reference text"):
    synthesize_reference_voice_service(..., reference_text_verified=False)
```

Mock the existing GPT-SoVITS client and FFmpeg mastering to assert a ready result contains:

```python
{
    "schemaVersion": "tangying-reference-voice/v1",
    "status": "ready",
    "ttsProvider": "gpt_sovits_local",
    "productionReady": True,
    "usageRightsConfirmed": True,
}
```

Assert `chattts_local` reports `status=blocked` when `ChatTTS` cannot be imported and never switches provider.

- [ ] **Step 2: Verify red**

Run:

```bash
python3 mcp/ip_avatar_3d/test_reference_voice.py
```

Expected: FAIL because `reference_voice.py` does not exist.

- [ ] **Step 3: Implement the service and append the MCP tool**

Use the existing `GPTSoVITSClient`, hash checks, output validation, and `_master_gpt_sovits_audio`. Keep `ChatTTS` behind dynamic import:

```python
try:
    import ChatTTS
except ImportError as exc:
    return {"status": "blocked", "provider": "chattts_local", "reason": str(exc)}
```

Append, without changing existing tool signatures:

```python
@mcp.tool()
def synthesize_reference_voice(
    text: str,
    referenceAudioPath: str,
    referenceText: str,
    outputDir: str,
    provider: str = "gpt_sovits_local",
    voiceId: str = "",
    referenceTextVerified: bool = False,
    usageRightsConfirmed: bool = False,
    language: str = "zh",
    speed: float = 1.0,
    characterProfilePath: str = "",
) -> dict[str, Any]:
    ...
```

- [ ] **Step 4: Verify the MCP contract**

Run:

```bash
python3 mcp/ip_avatar_3d/test_reference_voice.py
python3 mcp/ip_avatar_3d/test_server.py
python3 scripts/test_mcp_contracts.py
```

Expected: all pass and the tool appears after existing tools.

### Task 3: Resolve project voice artifacts safely in the local runner

**Files:**
- Modify: `local-backend/internal/localtool/mcp_tool_call.go`
- Modify: `local-backend/internal/localtool/mcp_tool_call_test.go`
- Modify: `local-backend/internal/localtool/bootstrap.go`

**Interfaces:**
- Produces:

```go
func resolveProjectVoiceArtifact(dataDir, projectID, artifactID, expectedHash string) (string, error)
```

- Consumes MCP arguments `referenceAudioArtifactId`, `referenceAudioSha256`, and `recordedNarrationArtifactId`.
- Supplies MCP-only local arguments `referenceAudioPath` or `audioPath`.

- [ ] **Step 1: Write failing containment and integrity tests**

Create a test artifact at `artifacts/vp-1/voice-ref/content` with metadata containing matching project, MIME, storage reference, and hash. Assert the executor passes its path to MCP.

Add rejection cases for:

```go
[]string{
    "../escape",
    "artifact-from-another-project",
    "metadata-project-mismatch",
    "content-hash-mismatch",
    "non-audio-mime",
}
```

- [ ] **Step 2: Verify red**

Run:

```bash
cd local-backend
go test ./internal/localtool -run 'VoiceArtifact|ReferenceVoice' -count=1
```

Expected: FAIL because the executor forwards unresolved artifact identity.

- [ ] **Step 3: Implement path hydration before `CallTool`**

For `ip_avatar_3d.synthesize_reference_voice` and `ip_avatar_3d.render_talking_video` only:

```go
resolved, err := resolveProjectVoiceArtifact(e.dataDir, projectID, artifactID, expectedHash)
if err != nil { return nil, err }
args["referenceAudioPath"] = resolved
delete(args, "referenceAudioArtifactId")
```

Resolve output directories to `projects/<projectId>/voice/<stable-id>` and reject cloud-provided absolute paths.

- [ ] **Step 4: Verify green**

Run focused local tests and expect exit zero.

### Task 4: Carry voice selection from the creator UI into the project run

**Files:**
- Modify: `frontend/src/features/creator-studio/types.ts`
- Modify: `frontend/src/features/creator-studio/logic.ts`
- Modify: `frontend/src/features/creator-studio/StartCreationPage.tsx`
- Modify: `frontend/src/index.css`
- Modify: `frontend/scripts/creator-studio-logic-check.mjs`

**Interfaces:**
- Produces:

```ts
export type CreatorVoiceMode = 'default_ip' | 'reference_clone' | 'recorded_narration'

export interface CreatorVoiceSelection {
  mode: CreatorVoiceMode
  provider: 'gpt_sovits_local' | 'chattts_local'
  voiceId?: string
  referenceArtifactId?: string
  referenceStorageRef?: string
  referenceContentHash?: string
  referenceMimeType?: string
  referenceText?: string
  referenceTextVerified?: boolean
  usageRightsConfirmed?: boolean
}
```

- Extends `CreationRequestInput` with `voiceSelection`.

- [ ] **Step 1: Write failing creator logic and source-contract tests**

Assert:

```js
assert.equal(logic.validateCreatorVoiceSelection({ mode: 'default_ip' }), undefined)
assert.match(
  logic.validateCreatorVoiceSelection({ mode: 'reference_clone', referenceText: '', usageRightsConfirmed: false }),
  /录音原文/,
)
```

Assert the UI has the three Chinese mode labels, audio-only file inputs, exact reference text, and required authorization checkbox.

- [ ] **Step 2: Verify red**

Run:

```bash
cd frontend
npm run test:creator
```

Expected: FAIL because the voice selection types and controls do not exist.

- [ ] **Step 3: Implement the controls and upload role**

Keep generic project materials separate. Add one voice file state and upload it with:

```ts
metadata: {
  artifactType: voiceMode === 'recorded_narration' ? 'recorded_narration' : 'voice_reference',
  source: 'creator_studio',
  localOnly: true,
  usageRightsConfirmed,
}
```

After the project ID exists, append the returned artifact ID/storage reference/hash to `agentRun.context.voiceSelection`. Disable start until the selected custom mode is complete.

- [ ] **Step 4: Verify green and accessibility**

Run `npm run test:creator`, `npm run build`, and `npm run lint`; expect exit zero.

### Task 5: Compile custom voice policy into A-roll generation

**Files:**
- Modify: `cloud-backend/internal/core/agentruntime/plan_compiler.go`
- Modify: `cloud-backend/internal/core/agentruntime/plan_compiler_test.go`
- Modify: `cloud-backend/internal/core/agentruntime/runner.go`
- Modify: `cloud-backend/internal/core/agentruntime/runner_test.go`
- Modify: `cloud-backend/internal/agents/video/service/audio_master.go`
- Modify: `cloud-backend/internal/agents/video/service/audio_master_test.go`

**Interfaces:**
- Consumes `voiceSelection` from the task’s immutable request context.
- Produces default A-roll arguments:

```json
{
  "renderMode": "production",
  "voiceProvider": "gpt_sovits_local",
  "voiceId": "main_ip_warm_knowledge_host_v1",
  "fallbackPolicy": "error"
}
```

- Produces custom clone arguments with project-scoped artifact identity and verified transcript.

- [ ] **Step 1: Write failing compiler tests**

Test default, reference clone, and recorded narration plans. For clone, require:

```go
if got := step.Arguments["referenceAudioArtifactId"]; got != "voice-ref-1" { ... }
if got := step.Arguments["usageRightsConfirmed"]; got != true { ... }
```

Assert no absolute local path appears in plan JSON.

- [ ] **Step 2: Verify red**

Run:

```bash
cd cloud-backend
go test ./internal/core/agentruntime ./internal/agents/video/service -run 'Voice|IPAroll|AudioMaster' -count=1
```

Expected: FAIL because the compiler ignores `voiceSelection`.

- [ ] **Step 3: Implement compiler and revision fingerprints**

Default mode passes pinned production voice arguments. Custom clone passes reference artifact identity, hash, transcript, and consent into the render tool, whose shared service performs synthesis. Recorded narration passes its artifact identity plus explicit `voiceInputMode=recorded_narration`.

Include these fields in audio-master fingerprint input:

```go
VoiceMode, VoiceProvider, VoiceProfileID, VoiceProfileVersion,
ReferenceContentHash, ReferenceTranscriptHash, SynthesisSettingsHash
```

- [ ] **Step 4: Verify green**

Run the focused cloud tests and expect exit zero.

### Task 6: Permit only verified custom production audio in the renderer

**Files:**
- Modify: `mcp/ip_avatar_3d/server.py`
- Modify: `mcp/ip_avatar_3d/test_server.py`
- Modify: `mcp/ip_avatar_3d/render_warm_studio_demo.py`

**Interfaces:**
- Extends `render_talking_video` only with optional new arguments:

```python
voiceInputMode: str = "default_ip"
referenceAudioPath: str = ""
referenceText: str = ""
referenceTextVerified: bool = False
usageRightsConfirmed: bool = False
referenceAudioSha256: str = ""
audioProvenance: dict[str, Any] | None = None
```

- Existing arguments retain their meaning.

- [ ] **Step 1: Write failing production policy tests**

Assert arbitrary production `audioPath` remains rejected. Assert a verified `recorded_narration` input with matching hash/project provenance is accepted. Assert reference clone calls the shared service and returns its provider/voice/hash provenance.

- [ ] **Step 2: Verify red**

Run:

```bash
python3 mcp/ip_avatar_3d/test_server.py
```

Expected: new cases fail because production audio is always rejected and clone fields are absent.

- [ ] **Step 3: Implement minimal verified branches**

Route:

```python
if voice_input_mode == "reference_clone":
    synthesized = synthesize_reference_voice_service(...)
elif voice_input_mode == "recorded_narration":
    verified_audio = validate_recorded_narration_provenance(...)
else:
    use pinned profile voice
```

Never interpret a bare `audioPath` as verified production input.

- [ ] **Step 4: Verify green**

Run the full `mcp/ip_avatar_3d` Python suite and expect exit zero.

### Task 7: Documentation, full verification, packaging, and UI smoke test

**Files:**
- Modify: `README.md`
- Modify: `docs/mcp-providers.md`
- Modify: `docs/local-ip-talking-avatar-render.md`
- Modify: `docs/PROJECT_INTRODUCTION_EN.md`
- Verify: `frontend/release/Tangying-AI-Video-Creator-0.2.1-mac-arm64.dmg`

**Interfaces:**
- Documents the new standard tool, default IP voice, custom voice modes, privacy behavior, and optional ChatTTS installation boundary.

- [ ] **Step 1: Update English file references and user documentation**

Document the MCP JSON contract and state that ChatTTS is optional, local, and fail-closed. Do not document the original Chinese filename as a runtime dependency.

- [ ] **Step 2: Run all relevant suites**

Run:

```bash
cd cloud-backend && go test ./internal/agents/video/... ./internal/core/agentruntime/... ./internal/core/apispec/... -count=1
cd ../local-backend && go test ./... -count=1
cd .. && python3 scripts/test_mcp_contracts.py
python3 mcp/ip_avatar_3d/test_reference_voice.py
python3 mcp/ip_avatar_3d/test_server.py
python3 mcp/ip_avatar_3d/test_default_aroll_assets.py
cd frontend && npm run test:creator && npm run test:settings
node --test electron/local-agent-runtime.test.cjs
npm run build && npm run lint
git diff --check
```

Expected: every command exits zero.

- [ ] **Step 3: Build and verify the desktop package**

Run:

```bash
bash scripts/build-local-desktop.sh --skip-install
hdiutil verify frontend/release/Tangying-AI-Video-Creator-0.2.1-mac-arm64.dmg
codesign --verify --deep --strict --verbose=2 "frontend/release/mac-arm64/Tangying AI Video Creation Assistant.app"
```

Expected: DMG checksum and app signature are valid.

- [ ] **Step 4: Install recoverably and smoke test**

Back up the currently installed app, install the new bundle, and verify with Computer Use:

- default `IP 口播视频` displays `默认 IP 音色`;
- custom clone cannot start without reference text and authorization;
- a valid local recording uploads and remains local;
- `我的视频` continues to open finished videos;
- MCP provider status lists `ip_avatar_3d.synthesize_reference_voice`.
