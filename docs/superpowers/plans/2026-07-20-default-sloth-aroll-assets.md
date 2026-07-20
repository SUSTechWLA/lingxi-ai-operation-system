# Default Sloth A-roll Assets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish the approved sloth and warm studio as the portable, verified default A-roll identity used whenever the Tangying avatar provider receives no explicit character profile.

**Architecture:** Export a character-only master and studio-only template from the immutable approved integrated Blend, then point the bundled character profile at those versioned files. A small Blender-independent manifest validator makes provenance and hash checks testable in CI, while Blender scripts reopen and audit the binary assets and produce a smoke-render report.

**Tech Stack:** Python 3.11+, Blender 4.x `bpy`, `unittest`, JSON, SHA-256, Git.

## Global Constraints

- Preserve the validated Blender source files and all existing release assets.
- Do not depend on a developer worktree or an absolute local path at runtime.
- Keep exactly one formal character and one formal armature in the character master.
- Keep the studio template free of a formal character so the renderer cannot import a duplicate.
- Preserve the character's rig, actions, shape keys, drivers, UVs, materials, groom data, and vertex order.
- Use versioned, immutable Blender artifacts and a manifest containing hashes.
- Build from `develop_go/release@ffbc5f176a2f0591518533ad224610082d2a8a02`.
- Approved source Blend: `/Users/wanglian/.config/superpowers/worktrees/tangying-ai-operation-system/codex-warm-sloth-studio/outputs/warm_sloth_fullbody_demo_v1/blend/warm_sloth_fullbody_demo_v1.blend`.
- Approved source SHA-256: `e606afb61454c58895f2c81ce5308218b324f62bcbd7505df226ebf3c63c358c`.
- Default presentation: `front_talking` camera with `standing` mode; `wide`, `close`, `three_quarter`, `transition`, and `seated` remain overrides using the same assets.

---

## File Structure

- Create `mcp/ip_avatar_3d/default_aroll_assets.py`: load and validate the bundled asset manifest without importing Blender.
- Create `mcp/ip_avatar_3d/test_default_aroll_assets.py`: TDD coverage for schema, relative paths, hashes, role separation, and bundled defaults.
- Create `ip形象/main_ip/scripts/publish_default_aroll_assets.py`: Blender-only, non-destructive character/studio exporter and structural auditor.
- Create `ip形象/main_ip/scripts/render_default_aroll_smoke.py`: Blender-only smoke render and render report generator.
- Create `ip形象/main_ip/models/main-ip-aroll-master-20260720.blend`: immutable character-only master.
- Create `ip形象/main_ip/scenes/warm-sloth-studio-20260720.blend`: immutable studio-only template.
- Create `ip形象/main_ip/manifests/default-aroll-assets.json`: paths, hashes, provenance, defaults, and validation evidence.
- Create `ip形象/main_ip/reports/default-aroll-character-audit.json`: reopened character audit.
- Create `ip形象/main_ip/reports/default-aroll-studio-audit.json`: reopened studio audit.
- Create `ip形象/main_ip/reports/default-aroll-smoke.json`: smoke-render configuration and result.
- Create `ip形象/main_ip/renders/default-aroll-smoke.png`: 640x360 evidence frame.
- Modify `ip形象/main_ip/character-profile.json`: versioned master/studio paths and manifest link.
- Modify `mcp/ip_avatar_3d/test_warm_studio_contract.py`: assert the versioned default pair and approved presets.
- Modify `mcp/ip_avatar_3d/test_server.py`: assert omitted profile/model resolves to the bundled pair.
- Modify `docs/local-ip-talking-avatar-render.md`: document the default identity and supported overrides.

---

### Task 1: Blender-independent default asset contract

**Files:**
- Create: `mcp/ip_avatar_3d/default_aroll_assets.py`
- Create: `mcp/ip_avatar_3d/test_default_aroll_assets.py`

**Interfaces:**
- Consumes: a JSON manifest path and optional repository root.
- Produces: `validate_manifest(path: Path, repo_root: Path | None = None) -> dict[str, Any]` with `success`, `errors`, `assets`, and `manifestPath`.

- [ ] **Step 1: Write the failing unit tests**

Test exact schema rejection, absolute path rejection, missing asset rejection, SHA mismatch rejection, duplicate-role rejection, and success with two temporary files. The success fixture uses:

```python
manifest = {
    "schemaVersion": "tangying-default-aroll-assets/v1",
    "characterId": "main_ip_sloth",
    "assets": {
        "characterMaster": {"path": "models/character.blend", "sha256": file_sha(character)},
        "studioTemplate": {"path": "scenes/studio.blend", "sha256": file_sha(studio)},
    },
}
result = validate_manifest(manifest_path, repo_root=root)
self.assertTrue(result["success"])
```

- [ ] **Step 2: Run the test and confirm failure**

Run: `python3 -m unittest mcp/ip_avatar_3d/test_default_aroll_assets.py -v`

Expected: FAIL because `default_aroll_assets` does not exist.

- [ ] **Step 3: Implement the validator**

Implement constants and helpers:

```python
SCHEMA_VERSION = "tangying-default-aroll-assets/v1"
REQUIRED_ROLES = ("characterMaster", "studioTemplate")

def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()

def validate_manifest(path: Path, repo_root: Path | None = None) -> dict[str, Any]:
    # Fail closed for schema, roles, relative paths, existence, and hashes.
```

The validator must never expand an asset outside `repo_root`; compare
`candidate.relative_to(root)` and emit an error on `ValueError`.

- [ ] **Step 4: Run the tests**

Run: `python3 -m unittest mcp/ip_avatar_3d/test_default_aroll_assets.py -v`

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add mcp/ip_avatar_3d/default_aroll_assets.py mcp/ip_avatar_3d/test_default_aroll_assets.py
git commit -m "feat: validate default a-roll asset manifests"
```

### Task 2: Non-destructive Blender publication scripts

**Files:**
- Create: `ip形象/main_ip/scripts/publish_default_aroll_assets.py`
- Create: `ip形象/main_ip/scripts/render_default_aroll_smoke.py`

**Interfaces:**
- Consumes: Blender's current file plus arguments after `--`.
- Produces: one `.blend` and one JSON audit per publication invocation; the smoke script produces PNG and JSON.

- [ ] **Step 1: Save the publication script before executing Blender**

The CLI is:

```text
blender -b SOURCE.blend --python publish_default_aroll_assets.py -- \
  --kind character --output OUTPUT.blend --report REPORT.json
```

For `character`, recursively collect objects in `COL_CHR_SLOTH_FINAL`, link that collection into a fresh `SCENE_CHARACTER_MASTER`, remove every non-character object and other scene, purge orphans, save, reopen, and assert:

```python
assert formal_collection_count == 1
assert armature_count == 1
assert camera_count == 0
assert light_count == 0
assert shape_key_count > 0
assert uv_layer_count > 0
```

For `studio`, remove all objects recursively contained by `COL_CHR_SLOTH_FINAL`, remove the formal collection, purge orphan character data, save, reopen, and assert:

```python
assert formal_collection_count == 0
assert armature_count == 0
assert required_markers == {"IP_Character_Spawn", "IP_Focus_Head"}
assert camera_count >= 3
assert light_count >= 3
```

Both reports record Blender/Python versions, source path/hash, output path/hash,
object/datablock counts, names, actions, drivers, shape-key order, UV counts,
vertex-count fingerprints, render engine, resolution, fps, and `status`.

- [ ] **Step 2: Save the smoke-render script**

The smoke script opens the studio, appends `COL_CHR_SLOTH_FINAL` from the master,
places its root at `IP_Character_Spawn`, targets `Camera_Medium`, sets Eevee,
AgX, 640x360, 1 frame, and writes both files:

```text
ip形象/main_ip/renders/default-aroll-smoke.png
ip形象/main_ip/reports/default-aroll-smoke.json
```

It fails if the scene contains anything other than one formal collection and
one armature after import.

- [ ] **Step 3: Syntax-check scripts without Blender**

Run:

```bash
python3 -m py_compile ip形象/main_ip/scripts/publish_default_aroll_assets.py
python3 -m py_compile ip形象/main_ip/scripts/render_default_aroll_smoke.py
```

Expected: both commands exit 0.

- [ ] **Step 4: Commit scripts**

```bash
git add ip形象/main_ip/scripts/publish_default_aroll_assets.py ip形象/main_ip/scripts/render_default_aroll_smoke.py
git commit -m "feat: add default a-roll Blender publisher"
```

### Task 3: Publish and reopen the approved Blender pair

**Files:**
- Create: `ip形象/main_ip/models/main-ip-aroll-master-20260720.blend`
- Create: `ip形象/main_ip/scenes/warm-sloth-studio-20260720.blend`
- Create: `ip形象/main_ip/reports/default-aroll-character-audit.json`
- Create: `ip形象/main_ip/reports/default-aroll-studio-audit.json`

**Interfaces:**
- Consumes: Task 2 scripts and the immutable approved source Blend.
- Produces: two independently reopenable formal assets and structural reports used by Task 4.

- [ ] **Step 1: Verify immutable source provenance**

Run:

```bash
shasum -a 256 /Users/wanglian/.config/superpowers/worktrees/tangying-ai-operation-system/codex-warm-sloth-studio/outputs/warm_sloth_fullbody_demo_v1/blend/warm_sloth_fullbody_demo_v1.blend
```

Expected: `e606afb61454c58895f2c81ce5308218b324f62bcbd7505df226ebf3c63c358c`.

- [ ] **Step 2: Publish character and studio in separate Blender processes**

Run the Task 2 command twice with `/Applications/Blender.app/Contents/MacOS/Blender`, once for each kind and output/report pair.

Expected: both processes exit 0 and each report has `"status": "PASS"`.

- [ ] **Step 3: Reopen each output and rerun audit mode**

Run the publisher with `--audit-only` for each output.

Expected: all assertions pass, the character has one Armature and the studio has none.

- [ ] **Step 4: Verify the approved source was not modified**

Run the source SHA command again.

Expected: identical `e606...` digest.

- [ ] **Step 5: Commit binary assets and audits**

```bash
git add ip形象/main_ip/models/main-ip-aroll-master-20260720.blend \
  ip形象/main_ip/scenes/warm-sloth-studio-20260720.blend \
  ip形象/main_ip/reports/default-aroll-character-audit.json \
  ip形象/main_ip/reports/default-aroll-studio-audit.json
git commit -m "feat: publish approved sloth a-roll assets"
```

### Task 4: Make the published pair the bundled default

**Files:**
- Create: `ip形象/main_ip/manifests/default-aroll-assets.json`
- Modify: `ip形象/main_ip/character-profile.json`
- Modify: `mcp/ip_avatar_3d/test_default_aroll_assets.py`
- Modify: `mcp/ip_avatar_3d/test_warm_studio_contract.py`
- Modify: `mcp/ip_avatar_3d/test_server.py`

**Interfaces:**
- Consumes: output hashes and audit reports from Task 3.
- Produces: the bundled default profile and CI-verifiable manifest.

- [ ] **Step 1: Write failing bundled-default assertions**

Assert exactly:

```python
self.assertEqual(profile["model"]["masterBlendPath"], "models/main-ip-aroll-master-20260720.blend")
self.assertEqual(profile["render"]["sceneBlendPath"], "scenes/warm-sloth-studio-20260720.blend")
self.assertEqual(profile["defaultAssetManifest"], "manifests/default-aroll-assets.json")
self.assertEqual(profile["render"]["cameraPreset"], "front_talking")
self.assertEqual(profile["render"]["presentationMode"], "standing")
```

Add a server test that clears `TANGYING_IP_AVATAR_PROFILE`, calls
`render_talking_video(..., dryRun=True)` with no model/profile, then verifies
the result and render input resolve the two versioned files.

- [ ] **Step 2: Run tests and confirm profile-path failures**

Run:

```bash
python3 -m unittest mcp/ip_avatar_3d/test_default_aroll_assets.py mcp/ip_avatar_3d/test_warm_studio_contract.py mcp/ip_avatar_3d/test_server.py -v
```

Expected: only new versioned-path/manifest assertions fail.

- [ ] **Step 3: Write manifest and update profile**

Generate the manifest from the measured audit values:

```python
character_audit = json.loads(Path("ip形象/main_ip/reports/default-aroll-character-audit.json").read_text())
studio_audit = json.loads(Path("ip形象/main_ip/reports/default-aroll-studio-audit.json").read_text())
manifest = {
    "schemaVersion": "tangying-default-aroll-assets/v1",
    "characterId": "main_ip_sloth",
    "releaseId": "main-ip-sloth-warm-studio-20260720",
    "source": {
        "approvedBlendSha256": "e606afb61454c58895f2c81ce5308218b324f62bcbd7505df226ebf3c63c358c"
    },
    "defaults": {"cameraPreset": "front_talking", "presentationMode": "standing"},
    "assets": {
        "characterMaster": {
            "path": "models/main-ip-aroll-master-20260720.blend",
            "sha256": character_audit["outputSha256"],
        },
        "studioTemplate": {
            "path": "scenes/warm-sloth-studio-20260720.blend",
            "sha256": studio_audit["outputSha256"],
        },
    },
}
```

Add audit report paths and supported presentation/camera overrides before serializing with `json.dumps(..., ensure_ascii=False, indent=2)`.

- [ ] **Step 4: Run the contract and server tests**

Run the Step 2 command.

Expected: all tests PASS.

- [ ] **Step 5: Commit default wiring**

```bash
git add ip形象/main_ip/manifests/default-aroll-assets.json \
  ip形象/main_ip/character-profile.json \
  mcp/ip_avatar_3d/test_default_aroll_assets.py \
  mcp/ip_avatar_3d/test_warm_studio_contract.py \
  mcp/ip_avatar_3d/test_server.py
git commit -m "feat: default a-roll to approved sloth studio"
```

### Task 5: Smoke render, documentation, and release verification

**Files:**
- Create: `ip形象/main_ip/renders/default-aroll-smoke.png`
- Create: `ip形象/main_ip/reports/default-aroll-smoke.json`
- Modify: `docs/local-ip-talking-avatar-render.md`

**Interfaces:**
- Consumes: bundled profile, manifest, character master, studio template.
- Produces: visual evidence and final release instructions.

- [ ] **Step 1: Run the Blender smoke render**

Run `render_default_aroll_smoke.py` with the published master and studio.

Expected: PNG exists, is 640x360, and the report records `status=PASS`, Eevee,
AgX, one formal character, one armature, and `Camera_Medium`.

- [ ] **Step 2: Document default and overrides**

Add a section stating that omitted `characterProfilePath`/`modelPath` resolves to
`main_ip_sloth`, with `front_talking` + `standing`. Document that `wide`,
`close`, `three_quarter`, `transition`, and `seated` are framing overrides using
the same pair, not separate character versions.

- [ ] **Step 3: Run full verification**

Run:

```bash
python3 -m unittest discover -s mcp/ip_avatar_3d -p 'test_*.py' -v
npm run test:beta-smoke
(cd local-backend && go test ./...)
(cd cloud-backend && go test ./...)
(cd hyperframes-render-service && npm test)
git diff --check
git status --short
```

Expected: all tests PASS, `git diff --check` emits nothing, and status contains only the intended smoke evidence/documentation before commit.

- [ ] **Step 4: Revalidate manifest hashes and source immutability**

Run the manifest validator and both source/published SHA commands.

Expected: validator `success=true`; source remains `e606...`; published hashes match the manifest.

- [ ] **Step 5: Commit final evidence**

```bash
git add ip形象/main_ip/renders/default-aroll-smoke.png \
  ip形象/main_ip/reports/default-aroll-smoke.json \
  docs/local-ip-talking-avatar-render.md
git commit -m "docs: verify default sloth a-roll release"
```

- [ ] **Step 6: Final clean-tree check**

Run: `git status --short --branch`

Expected: clean `codex/default-sloth-aroll` branch ahead of `develop_go/release` only by the reviewed commits.
