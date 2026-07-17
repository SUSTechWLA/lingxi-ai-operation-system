# Sloth Hand Perspective Quality Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the 2–3 second hand enlargement artifact by preserving a long-lens medium camera, authoring sloth-specific three-digit poses, and enforcing render-derived hand perspective gates.

**Architecture:** Keep the existing master asset and source FBX immutable. Add pure pose constraints in `aroll_actions.py`, replace lens-only framing with a fixed-lens dolly search in `blender_renderer.py`, and validate the generated timeline with pure QA plus Blender projection tests before rendering a new candidate asset and 1080p demo.

**Tech Stack:** Python 3.12, Blender 5.1 Python API, `mathutils`, `bpy_extras.object_utils`, `unittest`, FFmpeg/FFprobe.

## Global Constraints

- Never overwrite the source FBX or the current production master before candidate QA passes.
- Medium cameras must use at least `48mm` focal length.
- Hand and finger local/matrix scale must remain within `1.0 ± 0.001`.
- Open-explain hand screen area growth must not exceed `35%` relative to its relaxed pre-action sample.
- `open_hand` outer splay is bounded by `±0.08rad`; `count_three` outer splay is bounded by `±0.12rad`.
- Final delivery remains `1920x1080`, `30fps`, approximately 15 seconds.

---

### Task 1: Lock Sloth-Specific Three-Digit Pose Semantics

**Files:**
- Modify: `mcp/ip_avatar_3d/test_aroll_actions.py`
- Modify: `mcp/ip_avatar_3d/aroll_actions.py`

**Interfaces:**
- Consumes: `hand_pose(name: str) -> Mapping[int, DigitPose]`.
- Produces: bounded `open_hand` and `count_three` poses reused by the existing action catalog.

- [ ] **Step 1: Write the failing pose contract test**

```python
def test_sloth_open_and_count_three_keep_low_splay_and_hooked_tips(self) -> None:
    open_pose = aroll_actions.hand_pose("open_hand")
    count_pose = aroll_actions.hand_pose("count_three")
    self.assertLessEqual(abs(open_pose[1].splay), 0.08)
    self.assertLessEqual(abs(open_pose[3].splay), 0.08)
    self.assertGreater(open_pose[1].distal, open_pose[1].proximal)
    self.assertGreater(open_pose[2].distal, open_pose[2].proximal)
    self.assertGreater(open_pose[3].distal, open_pose[3].proximal)
    self.assertLessEqual(abs(count_pose[1].splay), 0.12)
    self.assertLessEqual(abs(count_pose[3].splay), 0.12)
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
PYTHONPATH=mcp/ip_avatar_3d python3 -m unittest mcp.ip_avatar_3d.test_aroll_actions.ArollActionContractTests.test_sloth_open_and_count_three_keep_low_splay_and_hooked_tips -v
```

Expected: FAIL because current outer splay is `0.22/0.32` and distal curl is below proximal curl.

- [ ] **Step 3: Implement the minimal sloth pose values**

```python
SLOTH_OPEN = DigitPose(0.035, 0.075, 0.115)

"open_hand": _digits(
    DigitPose(SLOTH_OPEN.proximal, SLOTH_OPEN.middle, SLOTH_OPEN.distal, splay=0.08),
    SLOTH_OPEN,
    DigitPose(SLOTH_OPEN.proximal, SLOTH_OPEN.middle, SLOTH_OPEN.distal, splay=-0.08),
),
"count_three": _digits(
    DigitPose(0.04, 0.07, 0.10, splay=0.12),
    DigitPose(0.04, 0.07, 0.10),
    DigitPose(0.04, 0.07, 0.10, splay=-0.12),
),
```

- [ ] **Step 4: Run action tests and verify GREEN**

Run: `PYTHONPATH=mcp/ip_avatar_3d python3 -m unittest -v mcp/ip_avatar_3d/test_aroll_actions.py`

Expected: all action tests pass and semantic-distinctness remains at least `0.08`.

- [ ] **Step 5: Commit the pose contract**

```bash
git add mcp/ip_avatar_3d/aroll_actions.py mcp/ip_avatar_3d/test_aroll_actions.py
git commit -m "fix: author sloth-specific three-digit poses"
```

### Task 2: Replace Wide-Angle Lens Reduction with Fixed-Lens Dolly Calibration

**Files:**
- Modify: `mcp/ip_avatar_3d/test_blender_scene_contract.py`
- Modify: `mcp/ip_avatar_3d/blender_renderer.py`

**Interfaces:**
- Consumes: `calibrate_mode_medium_camera(character_objects, bone_map, mode_objects, sample_frames)`.
- Produces: the existing report plus `authoredLocation`, `location`, and `dollyDistance`; camera focal length never below `48mm`.

- [ ] **Step 1: Add a failing Blender camera calibration test**

Create a synthetic animated armature/mesh fixture whose hands exceed the safe frame at the authored camera position, then assert:

```python
report = blender_renderer.calibrate_mode_medium_camera(
    character_objects, bone_map, mode_objects, sample_frames=(1, 30)
)
assert report["lens"] >= 48.0
assert report["dollyDistance"] > 0.0
assert tuple(report["location"]) != tuple(report["authoredLocation"])
assert blender_renderer._medium_frame_candidate_is_safe(
    {item["frame"]: {key: value for key, value in item.items() if key != "frame"} for item in report["frames"]}
)
```

- [ ] **Step 2: Run the focused Blender test and verify RED**

Run:

```bash
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup --python mcp/ip_avatar_3d/test_blender_scene_contract.py
```

Expected: the new assertion fails because the current implementation lowers focal length at a fixed location.

- [ ] **Step 3: Implement fixed-lens dolly candidate search**

Add constants and helpers:

```python
MIN_MEDIUM_CAMERA_LENS_MM = 48.0
MAX_MEDIUM_CAMERA_DOLLY_M = 3.5
MEDIUM_CAMERA_DOLLY_STEP_M = 0.05

def _camera_backward_axis(camera: bpy.types.Object) -> Vector:
    return (camera.matrix_world.to_quaternion() @ Vector((0.0, 0.0, 1.0))).normalized()
```

Search authored focal length down to `48mm`, dolly distances `0.0..3.5m`, and existing shift deltas. Restore authored transform before every candidate. Score safe candidates by focal deviation, dolly distance, vertical span error, center error, and shift delta. Persist the selected location and report both transforms.

- [ ] **Step 4: Run the full Blender scene contract suite**

Run the Blender command from Step 2.

Expected: all scene contract tests pass; calibrated medium cameras remain `>=48mm`.

- [ ] **Step 5: Commit camera calibration**

```bash
git add mcp/ip_avatar_3d/blender_renderer.py mcp/ip_avatar_3d/test_blender_scene_contract.py
git commit -m "fix: preserve long-lens a-roll framing"
```

### Task 3: Add Fail-Closed Hand Perspective QA

**Files:**
- Modify: `mcp/ip_avatar_3d/test_aroll_performance_qa.py`
- Modify: `mcp/ip_avatar_3d/aroll_performance_qa.py`
- Modify: `mcp/ip_avatar_3d/test_blender_hand_refinement.py`
- Modify: `mcp/ip_avatar_3d/blender_renderer.py`

**Interfaces:**
- Produces: `validate_hand_perspective_metrics(metrics) -> dict[str, object]`.
- Metrics: `cameraLensMm`, `maxLocalScaleError`, `maxMatrixScaleError`, `leftAreaGrowth`, `rightAreaGrowth`, `minimumFrameMargin`.

- [ ] **Step 1: Write pure failing threshold tests**

```python
def test_hand_perspective_metrics_enforce_long_lens_unit_scale_and_growth(self):
    passing = {
        "cameraLensMm": 50.0, "maxLocalScaleError": 0.0002,
        "maxMatrixScaleError": 0.0003, "leftAreaGrowth": 0.30,
        "rightAreaGrowth": 0.35, "minimumFrameMargin": 0.06,
    }
    self.assertTrue(aroll_performance_qa.validate_hand_perspective_metrics(passing)["success"])
    for key, value in (
        ("cameraLensMm", 47.999), ("maxLocalScaleError", 0.0011),
        ("maxMatrixScaleError", 0.0011), ("leftAreaGrowth", 0.351),
        ("rightAreaGrowth", 0.351), ("minimumFrameMargin", 0.049),
    ):
        failing = dict(passing, **{key: value})
        self.assertFalse(aroll_performance_qa.validate_hand_perspective_metrics(failing)["success"])
```

- [ ] **Step 2: Verify pure tests fail**

Run: `PYTHONPATH=mcp/ip_avatar_3d python3 -m unittest -v mcp/ip_avatar_3d/test_aroll_performance_qa.py`

Expected: ERROR because the validator does not exist.

- [ ] **Step 3: Implement the pure validator**

Add `HAND_PERSPECTIVE_LIMITS`, required metric names, finite-number parsing through `_required_metrics`, explicit inclusive threshold checks, and a structured passed/failed report consistent with existing transition/viseme validators.

- [ ] **Step 4: Add Blender-derived projection evidence**

Reuse `world_to_camera_view` and semantic hand vertex groups to sample relaxed and hold frames. Store the six metrics under `arollPerformanceQa.handPerspective`. Do not accept caller-provided booleans.

- [ ] **Step 5: Add a real timeline regression test**

Build the programmatic main-IP fixture, run `Aroll_OpenPalm_Explain`, calibrate the medium camera, and assert the Blender-derived report passes with all hand/hand-finger bone scales inside tolerance.

- [ ] **Step 6: Run QA suites and commit**

```bash
PYTHONPATH=mcp/ip_avatar_3d python3 -m unittest -v mcp/ip_avatar_3d/test_aroll_performance_qa.py
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup --python mcp/ip_avatar_3d/test_blender_hand_refinement.py
git add mcp/ip_avatar_3d/aroll_performance_qa.py mcp/ip_avatar_3d/blender_renderer.py mcp/ip_avatar_3d/test_aroll_performance_qa.py mcp/ip_avatar_3d/test_blender_hand_refinement.py
git commit -m "test: gate sloth hand perspective"
```

Expected: pure and Blender hand suites pass.

### Task 4: Rebuild the Candidate Master and Diagnostic Segment

**Files:**
- Create: `ip形象/main_ip/models/main-ip-aroll-master-sloth-hands-v3.blend`
- Create: `outputs/final/sloth-hand-perspective-v3/`

**Interfaces:**
- Consumes: the existing refined master, current warm studio scene, and production GPT-SoVITS voice bundle.
- Produces: immutable candidate master, render input/report, projection QA report, and `2.0s–3.6s` diagnostic frames/video.

- [ ] **Step 1: Run source and hardware preflight**

Verify Blender 5.1, source/master hashes, one armature, required three-segment digit bones, scene cameras, available render engine, and free disk space. Record JSON evidence.

- [ ] **Step 2: Build candidate without overwriting current production files**

Run the existing master preparation pipeline with the new code and a distinct output path. Confirm the candidate action library contains all existing standing/seated actions.

- [ ] **Step 3: Render the focused diagnostic range**

Render frames `61..109` at `1920x1080`, `30fps`, using the production warm-studio scene and medium camera.

- [ ] **Step 4: Validate and visually inspect the range**

Require the hand perspective report to pass, render a 10-frame contact sheet, and inspect the first relaxed frame, action anticipation, maximum hand extent, hold, and return frames. Reject blocky fingers, face occlusion, edge contact, or abrupt hand-size changes.

### Task 5: Render and Publish the New 1080p Demo

**Files:**
- Create: `outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p_v3.mp4`
- Create: `outputs/final/sloth-hand-perspective-v3/media-evidence.json`

**Interfaces:**
- Consumes: the candidate master approved in Task 4.
- Produces: final 15-second demo and signed evidence; current demo remains available until approval.

- [ ] **Step 1: Render the complete 15-second production timeline**

Use the same script, voice, lighting, scene, CRF, and 30fps contract as the prior demo, changing only the approved hand/camera behavior.

- [ ] **Step 2: Run media verification**

```bash
ffprobe -v error -show_streams -show_format -of json outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p_v3.mp4
ffmpeg -v error -i outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p_v3.mp4 -f null -
ffmpeg -v error -i outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p_v3.mp4 -map 0:v:0 -f framemd5 outputs/final/sloth-hand-perspective-v3/final.framemd5
```

Require H.264 High/yuv420p, `1920x1080`, constant `30fps`, complete decode, zero adjacent duplicate frames, AAC `48kHz`, integrated loudness near `-16 LUFS`, and true peak below `-1.5 dBTP`.

- [ ] **Step 3: Run regression suites**

```bash
python3 -m unittest -v mcp.ip_avatar_3d.test_front_talking_demo mcp.ip_avatar_3d.test_server
PYTHONPATH=mcp/ip_avatar_3d python3 -m unittest -v mcp/ip_avatar_3d/test_aroll_actions.py mcp/ip_avatar_3d/test_aroll_performance_qa.py
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup --python mcp/ip_avatar_3d/test_blender_character_rig.py
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup --python mcp/ip_avatar_3d/test_blender_hand_refinement.py
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup --python mcp/ip_avatar_3d/test_front_talking_demo.py
```

Expected: all suites pass and the new perspective gate is included in the production report.

- [ ] **Step 4: Independent review and publication**

Request an independent P1/P2 review of the code and evidence. Fix any finding, rerun affected tests, then publish the candidate master/demo through the existing evidence-bound MCP publication flow.
