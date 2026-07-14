# Sloth Warm Studio Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Integrate the production sloth character with the packed warm Blender studio, expose standing and seated A-roll modes through MCP, and render publishable 1080p demos with the permanent local GPT-SoVITS voice.

**Architecture:** Start from the completed character pipeline and selectively restore only studio-owned files from codex/warm-sloth-studio. Extend the authored-scene contract with mode-specific markers and cameras, keep one canonical character master, apply standing or seated base pose offsets before speech gestures, and render character plus room together in Blender. A dedicated demo runner produces two standalone videos, a combined reel, contact sheets, and a provenance/QA report.

**Tech Stack:** Python 3.11, Blender 5.1.2 Python API, Eevee Next, Cycles still QA, FastMCP, FFmpeg/FFprobe, local GPT-SoVITS, unittest, Go local-MCP bridge tests.

## Global Constraints

- Work only on codex/sloth-warm-studio-integration in the isolated worktree.
- Do not merge the full codex/warm-sloth-studio branch; its character profile, 2K defaults, and HeyGen voice are stale.
- Production animation remains 1920x1080, 30 fps CFR, Eevee Next, H.264 plus AAC 48 kHz mono.
- Keep the canonical character master, source materials, facial topology, visemes, wrists, and independent three-segment fingers.
- standing, seated, and auto are the only accepted presentation modes; auto resolves to standing.
- Standing and seated are per-shot modes; do not implement an automated stand-to-sit transition.
- The room must remain approximately 1 to 1.5 stops below the face while retaining practical-light and material detail.
- Production demos must use gpt_sovits_local with main_ip_warm_knowledge_host_v1; no preview voice fallback is allowed.
- Generated character binaries and videos remain untracked output artifacts unless already tracked by the source studio branch.
- Publish demo files only after Blender, media, collision, and encoded-audio gates pass.

---

### Task 1: Import The Studio-Owned Baseline

**Files:**
- Create from source branch: mcp/ip_avatar_3d/warm_studio_contract.py
- Create from source branch: mcp/ip_avatar_3d/warm_sloth_studio_builder.py
- Create from source branch: mcp/ip_avatar_3d/validate_warm_studio.py
- Create from source branch: mcp/ip_avatar_3d/render_warm_studio_qa.py
- Create from source branch: mcp/ip_avatar_3d/test_warm_studio_contract.py
- Create from source branch: ip形象/main_ip/scenes/assets/warm-sloth-brand-icon.png
- Create from source branch: ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png
- Create from source branch: ip形象/main_ip/scenes/warm-sloth-studio-v1.blend
- Create from source branch: docs/superpowers/specs/2026-07-13-warm-sloth-studio-design.md

**Interfaces:**
- Consumes: final studio files at codex/warm-sloth-studio.
- Produces: warm_studio_contract, deterministic builder, validator, QA renderer, and packed empty scene.

- [ ] **Step 1: Verify the source artifacts**

~~~bash
git ls-tree -r --name-only codex/warm-sloth-studio -- mcp/ip_avatar_3d ip形象/main_ip/scenes
~~~

Expected: every file listed above exists on the source branch.

- [ ] **Step 2: Restore only studio-owned files**

~~~bash
git restore --source=codex/warm-sloth-studio -- \
  mcp/ip_avatar_3d/warm_studio_contract.py \
  mcp/ip_avatar_3d/warm_sloth_studio_builder.py \
  mcp/ip_avatar_3d/validate_warm_studio.py \
  mcp/ip_avatar_3d/render_warm_studio_qa.py \
  mcp/ip_avatar_3d/test_warm_studio_contract.py \
  ip形象/main_ip/scenes/assets/warm-sloth-brand-icon.png \
  ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png \
  ip形象/main_ip/scenes/warm-sloth-studio-v1.blend \
  docs/superpowers/specs/2026-07-13-warm-sloth-studio-design.md
~~~

- [ ] **Step 3: Run the imported contract tests**

~~~bash
python3 mcp/ip_avatar_3d/test_warm_studio_contract.py
~~~

Expected: all imported warm studio contract tests pass.

- [ ] **Step 4: Prove current character and voice code is untouched**

~~~bash
git diff --exit-code HEAD -- ip形象/main_ip/character-profile.json mcp/ip_avatar_3d/server.py mcp/ip_avatar_3d/voice_policy.py
~~~

Expected: exit code 0.

- [ ] **Step 5: Commit**

~~~bash
git add mcp/ip_avatar_3d/warm_* \
  ip形象/main_ip/scenes/assets/warm-sloth-brand-icon.png \
  ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png \
  ip形象/main_ip/scenes/warm-sloth-studio-v1.blend \
  docs/superpowers/specs/2026-07-13-warm-sloth-studio-design.md
git commit -m "feat: import warm sloth studio assets"
~~~

---

### Task 2: Define The Dual-Mode Studio Contract

**Files:**
- Modify: mcp/ip_avatar_3d/warm_studio_contract.py
- Modify: mcp/ip_avatar_3d/warm_sloth_studio_builder.py
- Modify: mcp/ip_avatar_3d/test_warm_studio_contract.py
- Modify: mcp/ip_avatar_3d/test_blender_scene_contract.py
- Modify generated: ip形象/main_ip/scenes/warm-sloth-studio-v1.blend

**Interfaces:**
- Consumes: existing studio collections, cameras, desk, chair, and marker helpers.
- Produces: PRESENTATION_MODES, MODE_MARKER_SPECS, MODE_CAMERA_SPECS, SUBJECT_LIGHT_PROFILE, and matching Blender objects.

- [ ] **Step 1: Write failing contract tests**

~~~python
def test_dual_mode_contract_is_explicit(self):
    self.assertEqual(contract.PRESENTATION_MODES, ("standing", "seated"))
    self.assertEqual(set(contract.MODE_MARKER_SPECS), {"standing", "seated"})
    self.assertEqual(set(contract.MODE_CAMERA_SPECS), {"standing", "seated"})
    for mode in contract.PRESENTATION_MODES:
        self.assertEqual(
            set(contract.MODE_MARKER_SPECS[mode]),
            {"spawn", "focus", "seat", "foot_l", "foot_r"},
        )
        self.assertEqual(
            set(contract.MODE_CAMERA_SPECS[mode]),
            {"wide", "medium", "three_quarter"},
        )

def test_subject_first_light_profile_is_bounded(self):
    profile = contract.SUBJECT_LIGHT_PROFILE
    self.assertEqual(profile["name"], "warm_subject_first_v1")
    self.assertLess(profile["worldStrength"], 0.20)
    self.assertEqual(profile["keyTemperatureK"], 4500)
    self.assertEqual(profile["rimTemperatureK"], 3200)
    self.assertGreaterEqual(profile["backgroundStopsBelowFace"], 1.0)
    self.assertLessEqual(profile["backgroundStopsBelowFace"], 1.5)
~~~

- [ ] **Step 2: Run RED**

~~~bash
python3 mcp/ip_avatar_3d/test_warm_studio_contract.py
~~~

Expected: missing dual-mode constants.

- [ ] **Step 3: Add the exact pure contract**

~~~python
PRESENTATION_MODES = ("standing", "seated")

MODE_MARKER_SPECS = MappingProxyType({
    "standing": MappingProxyType({
        "spawn": (0.0, 0.30, 0.0),
        "focus": (0.0, 0.30, 1.93),
        "seat": (0.0, 0.58, 0.62),
        "foot_l": (-0.22, 0.03, 0.0),
        "foot_r": (0.22, 0.03, 0.0),
    }),
    "seated": MappingProxyType({
        "spawn": (0.0, 0.47, 0.0),
        "focus": (0.0, 0.47, 1.58),
        "seat": (0.0, 0.58, 0.62),
        "foot_l": (-0.22, 0.02, 0.0),
        "foot_r": (0.22, 0.02, 0.0),
    }),
})

MODE_CAMERA_SPECS = MappingProxyType({
    "standing": MappingProxyType({
        "wide": ("Camera_Standing_Wide", (0.0, -2.74, 1.67), 24.0),
        "medium": ("Camera_Standing_Medium", (0.0, -2.15, 1.78), 50.0),
        "three_quarter": ("Camera_Standing_ThreeQuarter", (-2.25, -1.65, 1.82), 50.0),
    }),
    "seated": MappingProxyType({
        "wide": ("Camera_Seated_Wide", (0.0, -2.74, 1.55), 24.0),
        "medium": ("Camera_Seated_Medium", (0.0, -2.15, 1.60), 50.0),
        "three_quarter": ("Camera_Seated_ThreeQuarter", (-2.20, -1.60, 1.62), 50.0),
    }),
})

SUBJECT_LIGHT_PROFILE = MappingProxyType({
    "name": "warm_subject_first_v1",
    "worldStrength": 0.12,
    "keyTemperatureK": 4500,
    "rimTemperatureK": 3200,
    "practicalTemperatureK": 2700,
    "backgroundStopsBelowFace": 1.25,
})
~~~

- [ ] **Step 4: Build all markers and cameras**

In build_markers_and_cameras, create IP_Standing_Spawn,
IP_Standing_Focus_Head, IP_Seated_Spawn, IP_Seated_Focus_Head,
IP_Seat_Target, IP_Foot_Target.L, IP_Foot_Target.R, and all six mode cameras.
Keep IP_Character_Spawn and IP_Focus_Head at the standing transforms for
backward compatibility. Stamp:

~~~python
ctx.scene["ip_presentation_modes"] = json.dumps(list(contract.PRESENTATION_MODES))
ctx.scene["ip_subject_light_profile"] = contract.SUBJECT_LIGHT_PROFILE["name"]
ctx.scene["ip_background_stops_below_face"] = contract.SUBJECT_LIGHT_PROFILE[
    "backgroundStopsBelowFace"
]
~~~

- [ ] **Step 5: Rebuild and validate in Blender**

~~~bash
BLENDER_BIN=/Applications/Blender.app/Contents/MacOS/Blender
"$BLENDER_BIN" --background --factory-startup \
  --python mcp/ip_avatar_3d/warm_sloth_studio_builder.py -- \
  "$PWD/ip形象/main_ip/scenes/warm-sloth-studio-v1.blend" \
  "$PWD/ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png" \
  "$PWD/ip形象/main_ip/scenes/assets/warm-sloth-brand-icon.png"
"$BLENDER_BIN" --background \
  "$PWD/ip形象/main_ip/scenes/warm-sloth-studio-v1.blend" \
  --python mcp/ip_avatar_3d/test_blender_scene_contract.py
~~~

Expected: all required objects and cameras exist.

- [ ] **Step 6: Commit**

~~~bash
git add mcp/ip_avatar_3d/warm_studio_contract.py \
  mcp/ip_avatar_3d/warm_sloth_studio_builder.py \
  mcp/ip_avatar_3d/test_warm_studio_contract.py \
  mcp/ip_avatar_3d/test_blender_scene_contract.py \
  ip形象/main_ip/scenes/warm-sloth-studio-v1.blend \
  ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png
git commit -m "feat: add standing and seated studio contract"
~~~

---

### Task 3: Add Presentation Mode To MCP

**Files:**
- Modify: mcp/ip_avatar_3d/server.py
- Modify: mcp/ip_avatar_3d/test_server.py
- Modify: local-backend/internal/localmcp/client_test.go

**Interfaces:**
- Consumes: presentationMode request and profile render.presentationMode.
- Produces: normalized mode in render input, dry-run result, render report, and final result.

- [ ] **Step 1: Write failing MCP tests**

~~~python
def test_presentation_mode_is_written_to_render_input(self):
    server = load_server()
    with tempfile.TemporaryDirectory() as tmp:
        root = pathlib.Path(tmp)
        model = root / "avatar.glb"
        model.write_bytes(b"glTF placeholder")
        result = server.render_talking_video(
            script="坐姿口播测试。",
            modelPath=str(model),
            outputDir=str(root / "out"),
            presentationMode="seated",
            dryRun=True,
        )
        payload = json.loads(pathlib.Path(result["renderInputPath"]).read_text())
        self.assertEqual(payload["presentationMode"], "seated")
        self.assertEqual(result["presentationMode"], "seated")

def test_auto_presentation_mode_defaults_to_standing(self):
    server = load_server()
    self.assertEqual(server.resolve_presentation_mode("auto"), "standing")

def test_invalid_presentation_mode_fails(self):
    server = load_server()
    with self.assertRaisesRegex(ValueError, "presentationMode"):
        server.resolve_presentation_mode("crouching")
~~~

- [ ] **Step 2: Run RED**

~~~bash
cd mcp/ip_avatar_3d
python3 -m unittest test_server.IPAvatar3DMCPTests.test_presentation_mode_is_written_to_render_input -v
~~~

Expected: missing argument or resolver.

- [ ] **Step 3: Implement normalization**

~~~python
PRESENTATION_MODES = {"auto", "standing", "seated"}

def resolve_presentation_mode(value: str) -> str:
    selected = str(value or "auto").strip().lower()
    if selected not in PRESENTATION_MODES:
        raise ValueError("presentationMode must be one of: auto, seated, standing")
    return "standing" if selected == "auto" else selected
~~~

Add presentationMode: str = "auto" to render_talking_video. Read profile value
only while the request remains auto. Persist the normalized value in every
render input/report/result path.

- [ ] **Step 4: Extend bridge serialization coverage**

Add presentationMode: seated to the local-MCP tool-call fixture, then run:

~~~bash
python3 mcp/ip_avatar_3d/test_server.py
cd local-backend
go test ./internal/localmcp/...
~~~

Expected: Python and Go tests pass.

- [ ] **Step 5: Commit**

~~~bash
git add mcp/ip_avatar_3d/server.py mcp/ip_avatar_3d/test_server.py \
  local-backend/internal/localmcp/client_test.go
git commit -m "feat: expose A-roll presentation mode"
~~~

---

### Task 4: Implement The Seated Base Pose

**Files:**
- Modify: mcp/ip_avatar_3d/aroll_actions.py
- Modify: mcp/ip_avatar_3d/blender_renderer.py
- Modify: mcp/ip_avatar_3d/test_server.py
- Modify: mcp/ip_avatar_3d/test_blender_character_rig.py

**Interfaces:**
- Consumes: normalized presentationMode, semantic bone map, source-rig axis detection.
- Produces: presentation_pose, Aroll_Seated_Idle, and seated offsets layered below normal speech gestures.

- [ ] **Step 1: Write the failing pure pose test**

~~~python
def test_seated_pose_moves_only_lower_body_and_root(self):
    pose = aroll_actions.presentation_pose("seated", source_rig=True)
    self.assertEqual(
        set(pose),
        {"root", "body", "leg_l", "shin_l", "foot_l", "leg_r", "shin_r", "foot_r"},
    )
    self.assertLess(pose["root"]["location"][2], -0.20)
    self.assertEqual(pose["leg_l"]["rotation"], pose["leg_r"]["rotation"])
    self.assertNotIn("head", pose)
    self.assertNotIn("hand_l", pose)
    self.assertNotIn("hand_r", pose)
~~~

- [ ] **Step 2: Run RED**

~~~bash
python3 mcp/ip_avatar_3d/test_server.py
~~~

Expected: presentation_pose is missing.

- [ ] **Step 3: Implement semantic base values**

~~~python
def presentation_pose(mode: str, source_rig: bool) -> dict[str, dict[str, tuple[float, float, float]]]:
    if mode == "standing":
        return {}
    if mode != "seated":
        raise ValueError(f"unsupported presentation mode: {mode}")
    if source_rig:
        leg = (0.0, 0.0, 1.02)
        shin = (0.0, 0.0, -1.16)
        foot = (0.0, 0.0, 0.18)
    else:
        leg = (1.02, 0.0, 0.0)
        shin = (-1.16, 0.0, 0.0)
        foot = (0.18, 0.0, 0.0)
    return {
        "root": {"location": (0.0, 0.12, -0.34)},
        "body": {"rotation": (0.08, 0.0, 0.0)},
        "leg_l": {"rotation": leg},
        "shin_l": {"rotation": shin},
        "foot_l": {"rotation": foot},
        "leg_r": {"rotation": leg},
        "shin_r": {"rotation": shin},
        "foot_r": {"rotation": foot},
    }
~~~

Add Aroll_Seated_Idle to the action library. Pass presentation_mode to animate.
Initialize every frame from the base pose, add speech motion on top, and suppress
happy_bounce, leg_step, and standing weight_shift while seated.

- [ ] **Step 4: Add Blender pose assertions**

Sample standing and seated first/middle/last frames and require pelvis height
drop greater than 0.20 m, knee angle below 125 degrees, foot-floor clearance
above -0.02 m, symmetric legs, and preserved upper-body/facial controls. Tune
the exact constants from Blender evidence and write the accepted values back to
presentation_pose.

- [ ] **Step 5: Run body and hand regression**

~~~bash
BLENDER_BIN=/Applications/Blender.app/Contents/MacOS/Blender
"$BLENDER_BIN" --background --factory-startup \
  --python mcp/ip_avatar_3d/run_blender_character_rig_tests.py
"$BLENDER_BIN" --background --factory-startup \
  --python mcp/ip_avatar_3d/run_blender_hand_refinement_tests.py
~~~

Expected: seated checks and all existing standing/hand checks pass.

- [ ] **Step 6: Commit**

~~~bash
git add mcp/ip_avatar_3d/aroll_actions.py mcp/ip_avatar_3d/blender_renderer.py \
  mcp/ip_avatar_3d/test_server.py mcp/ip_avatar_3d/test_blender_character_rig.py
git commit -m "feat: add seated A-roll base action"
~~~

---

### Task 5: Place And Frame The Character By Mode

**Files:**
- Modify: mcp/ip_avatar_3d/blender_renderer.py
- Modify: mcp/ip_avatar_3d/test_blender_scene_contract.py
- Create: mcp/ip_avatar_3d/validate_warm_studio_character.py

**Interfaces:**
- Consumes: presentationMode, mode markers/cameras, character dimensions, semantic bones.
- Produces: resolve_scene_mode_objects, mode-aware placement/focus/camera cuts, and clearance JSON.

- [ ] **Step 1: Write failing Blender mode-resolution tests**

Require standing spawn IP_Standing_Spawn, seated spawn IP_Seated_Spawn,
seated focus IP_Seated_Focus_Head, and seated medium camera
Camera_Seated_Medium.

- [ ] **Step 2: Run RED**

~~~bash
/Applications/Blender.app/Contents/MacOS/Blender --background \
  "$PWD/ip形象/main_ip/scenes/warm-sloth-studio-v1.blend" \
  --python mcp/ip_avatar_3d/test_blender_scene_contract.py
~~~

Expected: mode resolver is missing.

- [ ] **Step 3: Implement mode-aware scene resolution**

~~~python
def resolve_scene_mode_objects(mode: str) -> dict[str, Any]:
    prefix = "Standing" if mode == "standing" else "Seated"
    names = {
        "spawn": f"IP_{prefix}_Spawn",
        "focus": f"IP_{prefix}_Focus_Head",
        "seat": "IP_Seat_Target",
        "foot_l": "IP_Foot_Target.L",
        "foot_r": "IP_Foot_Target.R",
    }
    resolved = {key: bpy.data.objects.get(name) for key, name in names.items()}
    resolved["cameras"] = {
        "wide": bpy.data.objects.get(f"Camera_{prefix}_Wide"),
        "medium": bpy.data.objects.get(f"Camera_{prefix}_Medium"),
        "three_quarter": bpy.data.objects.get(f"Camera_{prefix}_ThreeQuarter"),
    }
    missing = [names[key] for key in names if resolved[key] is None]
    missing.extend(role for role, camera in resolved["cameras"].items() if camera is None)
    if missing:
        raise RuntimeError(f"studio presentation mode {mode} is incomplete: {missing}")
    return resolved
~~~

Use selected spawn in scene_target_height and character placement, selected
focus for DOF, and selected cameras for timeline cuts. Persist marker names,
camera names, mode, and world bounds in scene_stats.

- [ ] **Step 4: Implement deterministic clearance validation**

validate_warm_studio_character.py must sample both modes and write mode,
sampleCount, left/right floor clearance, desk/chair/hand intersection counts,
deformationSpikeCount, cameraVisibility, and success. Fail when feet are more
than 0.02 m below the floor, visible sampled geometry intersects desk/chair,
deformation spikes occur, or the head and both hands are outside medium frame.

- [ ] **Step 5: Run both-mode validation**

Expected: standing and seated reports both contain success true.

- [ ] **Step 6: Commit**

~~~bash
git add mcp/ip_avatar_3d/blender_renderer.py \
  mcp/ip_avatar_3d/test_blender_scene_contract.py \
  mcp/ip_avatar_3d/validate_warm_studio_character.py
git commit -m "feat: place sloth in warm studio by presentation mode"
~~~

---

### Task 6: Calibrate Subject-First Lighting And Color

**Files:**
- Modify: mcp/ip_avatar_3d/warm_sloth_studio_builder.py
- Modify: mcp/ip_avatar_3d/render_warm_studio_qa.py
- Modify: mcp/ip_avatar_3d/validate_warm_studio.py
- Modify: mcp/ip_avatar_3d/test_blender_scene_contract.py
- Modify generated: ip形象/main_ip/scenes/warm-sloth-studio-v1.blend
- Modify generated: ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png

**Interfaces:**
- Consumes: SUBJECT_LIGHT_PROFILE and both head-focus markers.
- Produces: named subject key/fill/rim lights and luminance evidence.

- [ ] **Step 1: Write failing light assertions**

Require IP_Subject_Key role key, IP_Subject_Fill role fill,
IP_Subject_Rim role rim, scene profile warm_subject_first_v1, and AgX Medium
High Contrast.

- [ ] **Step 2: Build the subject light rig**

Create a broad 4500 K camera-left key at energy 520, neutral 5200 K fill at
energy 115, and 3200 K rear rim at energy 260. Aim key/fill at standing focus
with an area size broad enough for seated focus. Preserve 2700 K practicals,
set world strength to 0.12, start exposure at -0.6, and reduce room/window
sources before touching character materials.

- [ ] **Step 3: Add image evidence**

Render standing medium, seated medium, and empty-room control stills. Measure
linear face and background luminance, backgroundStopsBelowFace, and
highlightClipRatio. Accept separation 1.0 to 1.5 stops and clipped non-catchlight
pixels below 0.5 percent.

- [ ] **Step 4: Rebuild and visually inspect**

Inspect both modes at medium, three-quarter, and wide in Eevee, plus Cycles
medium comparison. Tune only authored light energy, world strength, exposure,
and framing until numeric and visual gates pass.

- [ ] **Step 5: Commit**

~~~bash
git add mcp/ip_avatar_3d/warm_sloth_studio_builder.py \
  mcp/ip_avatar_3d/render_warm_studio_qa.py \
  mcp/ip_avatar_3d/validate_warm_studio.py \
  mcp/ip_avatar_3d/test_blender_scene_contract.py \
  ip形象/main_ip/scenes/warm-sloth-studio-v1.blend \
  ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png
git commit -m "feat: calibrate warm studio for sloth A-roll"
~~~

---

### Task 7: Activate The Production Profile

**Files:**
- Modify: ip形象/main_ip/character-profile.json
- Modify: mcp/ip_avatar_3d/test_warm_studio_contract.py
- Modify: mcp/ip_avatar_3d/README.md
- Modify: docs/mcp-providers.md

**Interfaces:**
- Consumes: packed integrated studio and presentationMode.
- Produces: canonical warm-studio profile while preserving 1080p and GPT-SoVITS.

- [ ] **Step 1: Write a profile regression test**

~~~python
def test_main_profile_uses_warm_studio_without_regression(self):
    profile = json.loads(
        (contract.repo_root() / "ip形象/main_ip/character-profile.json").read_text()
    )
    render = profile["render"]
    voice = profile["voice"]
    self.assertEqual(render["sceneBlendPath"], "scenes/warm-sloth-studio-v1.blend")
    self.assertEqual(render["presentationMode"], "standing")
    self.assertEqual(render["qualityPreset"], "production_1080p")
    self.assertEqual(render["resolution"], {"width": 1920, "height": 1080})
    self.assertEqual(render["fps"], 30)
    self.assertEqual(voice["provider"], "gpt_sovits_local")
    self.assertEqual(voice["voiceId"], "main_ip_warm_knowledge_host_v1")
    self.assertEqual(voice["fallbackPolicy"], "error")
~~~

- [ ] **Step 2: Run RED**

Expected: profile still points at editorial-news-studio.blend.

- [ ] **Step 3: Update only intended render fields**

Set sceneBlendPath to scenes/warm-sloth-studio-v1.blend, presentationMode to
standing, lightingPreset to editorial_soft, renderEngine to
BLENDER_EEVEE_NEXT, and qualityPreset to production_1080p. Do not change model,
facial, or voice.gptSovitsLocal fields.

- [ ] **Step 4: Add direct standing/seated MCP examples**

Document identical requests differing only by presentationMode.

- [ ] **Step 5: Run regressions**

~~~bash
python3 mcp/ip_avatar_3d/test_warm_studio_contract.py
python3 mcp/ip_avatar_3d/test_voice_policy.py
python3 mcp/ip_avatar_3d/test_gpt_sovits_client.py
python3 mcp/ip_avatar_3d/test_server.py
~~~

Expected: all pass with no voice fallback.

- [ ] **Step 6: Commit**

~~~bash
git add ip形象/main_ip/character-profile.json \
  mcp/ip_avatar_3d/test_warm_studio_contract.py \
  mcp/ip_avatar_3d/README.md docs/mcp-providers.md
git commit -m "feat: activate warm studio for main IP A-roll"
~~~

---

### Task 8: Build The Two-Mode Demo Runner

**Files:**
- Create: mcp/ip_avatar_3d/render_warm_studio_demo.py
- Create: mcp/ip_avatar_3d/test_warm_studio_demo.py

**Interfaces:**
- Consumes: server.render_talking_video, canonical profile, packed studio, permanent voice.
- Produces: standing demo, seated demo, combined reel, contact sheets, lighting comparison, report.

- [ ] **Step 1: Write failing orchestration tests**

Mock render_talking_video and assert exactly two calls with modes standing and
seated and common values width 1920, height 1080, fps 30, quality preset
production_1080p, render mode production, provider gpt_sovits_local, voice ID
main_ip_warm_knowledge_host_v1, and fallback policy error. Assert a failed QA
never publishes a final file.

- [ ] **Step 2: Run RED**

~~~bash
python3 mcp/ip_avatar_3d/test_warm_studio_demo.py
~~~

Expected: module missing.

- [ ] **Step 3: Implement deterministic demo scripts**

~~~python
STANDING_SCRIPT = (
    "大家好，我是小唐。今天想和你分享一个判断：AI视频真正重要的，不只是生成速度，"
    "而是选题、脚本、画面和审核都能被理解、修改和复用。这样创作才会越来越稳定。"
)

SEATED_SCRIPT = (
    "换一个更安静的视角，我们继续聊。面对快速变化的信息，先确认事实，再形成观点，"
    "最后用清楚的结构表达出来。慢一点想明白，往往能让内容走得更远。"
)
~~~

Render each mode into staging, run existing media/audio probes, then atomically
publish its MP4. Build the combined reel with FFmpeg at 1920x1080/30 fps.
Generate one 4x3 contact sheet per mode and one side-by-side lighting comparison.

- [ ] **Step 4: Persist provenance**

Write Sloth_WarmStudio_Integration_Report.json with success, character and
studio paths/hashes, full voice provenance, mode QA, combined-reel QA, lighting
measurements, and collision report paths. Exit nonzero and leave final outputs
absent on any failed gate.

- [ ] **Step 5: Run tests**

~~~bash
python3 mcp/ip_avatar_3d/test_warm_studio_demo.py
python3 mcp/ip_avatar_3d/test_server.py
~~~

Expected: all pass.

- [ ] **Step 6: Commit**

~~~bash
git add mcp/ip_avatar_3d/render_warm_studio_demo.py \
  mcp/ip_avatar_3d/test_warm_studio_demo.py
git commit -m "feat: render standing and seated warm studio demos"
~~~

---

### Task 9: Render, Review, And Publish

**Files:**
- Create generated: outputs/Sloth_WarmStudio_Standing_Demo_1080p.mp4
- Create generated: outputs/Sloth_WarmStudio_Seated_Demo_1080p.mp4
- Create generated: outputs/Sloth_WarmStudio_DualMode_Reel_1080p.mp4
- Create generated: outputs/Sloth_WarmStudio_Standing_ContactSheet.png
- Create generated: outputs/Sloth_WarmStudio_Seated_ContactSheet.png
- Create generated: outputs/Sloth_WarmStudio_Lighting_Comparison.png
- Create generated: outputs/Sloth_WarmStudio_Integration_Report.json
- Create: .superpowers/sdd/sloth-warm-studio-integration-report.md
- Modify: docs/superpowers/plans/2026-07-14-sloth-warm-studio-integration.md

**Interfaces:**
- Consumes: complete dual-mode pipeline and local production voice service.
- Produces: final demos, evidence, and closure report.

- [ ] **Step 1: Copy canonical untracked assets into the isolated worktree**

~~~bash
mkdir -p ip形象/main_ip/models ip形象/main_ip/voice/reference
cp -p /Users/wanglian/Projects/tangying-ai-operation-system/outputs/MainIP_Sloth_Aroll_Master.blend \
  ip形象/main_ip/models/main-ip-aroll-master.blend
cp -p /Users/wanglian/Projects/tangying-ai-operation-system/outputs/MainIP_Sloth_Aroll_Rigged.glb \
  ip形象/main_ip/models/main-ip-rigged.glb
cp -p /Users/wanglian/Projects/tangying-ai-operation-system/ip形象/main_ip/voice/reference/main_ip_voice_ref_v1.wav \
  ip形象/main_ip/voice/reference/main_ip_voice_ref_v1.wav
~~~

Verify source and copy SHA-256 values match.

- [ ] **Step 2: Run production voice preflight**

Call check_gpt_sovits_voice using the canonical profile. Require bundleReady
true, matching reference/GPT/SoVITS hashes, and loopback endpoint availability.

- [ ] **Step 3: Render demos**

~~~bash
python3 mcp/ip_avatar_3d/render_warm_studio_demo.py \
  --profile "$PWD/ip形象/main_ip/character-profile.json" \
  --output-dir "$PWD/outputs"
~~~

Expected: all seven outputs exist and report success is true.

- [ ] **Step 4: Run media QA**

For every MP4 require H.264 1920x1080 at 30/1, AAC 48000 Hz mono, expected
duration within 0.1 seconds, zero unintended adjacent duplicate frames, decoded
audio at -16 plus or minus 0.5 LUFS, and true peak no higher than -1.5 dBTP.

- [ ] **Step 5: Perform visual review**

Inspect greeting, explain, emphasis, neutral, and closing frames in both modes.
Reject desk/chair/body intersections, floating or buried feet, clipped face or
cardigan, background brighter than face, unnatural rim color, soft eyes/mouth/
hands, unbalanced posture, or mechanically mirrored gestures.

- [ ] **Step 6: Run full regression**

~~~bash
python3 mcp/ip_avatar_3d/test_warm_studio_contract.py
python3 mcp/ip_avatar_3d/test_warm_studio_demo.py
python3 mcp/ip_avatar_3d/test_voice_policy.py
python3 mcp/ip_avatar_3d/test_gpt_sovits_client.py
python3 mcp/ip_avatar_3d/test_server.py
cd local-backend && go test ./...
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --python mcp/ip_avatar_3d/run_blender_character_rig_tests.py
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --python mcp/ip_avatar_3d/run_blender_hand_refinement_tests.py
~~~

Expected: all Python, Go, Blender, scene, hand, voice, and demo checks pass.

- [ ] **Step 7: Write closure evidence and check plan items**

Record exact test counts, asset/output hashes, probes, loudness, collisions,
lighting measurements, and accepted limitations in
.superpowers/sdd/sloth-warm-studio-integration-report.md. Change each completed
checkbox only after evidence exists.

- [ ] **Step 8: Commit tracked closure evidence**

~~~bash
git add -u
git add -f .superpowers/sdd/sloth-warm-studio-integration-report.md
git commit -m "docs: complete sloth warm studio integration"
~~~

Generated videos and copied local character/voice binaries remain untracked.
Confirm no unexpected tracked modifications before handoff.

