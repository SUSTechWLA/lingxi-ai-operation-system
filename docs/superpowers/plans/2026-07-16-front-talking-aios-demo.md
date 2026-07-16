# Front Talking AIOS Demo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce a publishable 15-30 second, 1080p, front-facing IP talking-head demo through the current hotfix AIOS pipeline and fix the camera bug that sends close shots to a three-quarter camera.

**Architecture:** Keep the existing authored warm studio and action library. Add a front close camera role to the shared studio contract, make automatic talking-head cuts resolve only to front cameras, then add a deterministic demo request and QA report that enters through the AIOS agent-run API and is executed by the hotfix Cloud, Local Agent, IP Avatar MCP, and HyperFrames services.

**Tech Stack:** Python 3/unittest, Blender Python, Go services/tests, HyperFrames/TypeScript, ffmpeg/ffprobe, Tangying AIOS Cloud and Local Agent.

## Global Constraints

- The demo is 1920x1080 at 30 fps and lasts 15-30 seconds.
- The IP character remains front-facing; the demo uses a centered wide opening and a stable centered medium shot, with no close or three-quarter cut that can crop the face or hands.
- A-roll remains continuous and dominant; B-roll occupies no more than 20 percent of the timeline.
- The demo uses the production GPT-SoVITS voice policy and must not silently fall back to a system voice.
- Existing user changes and untracked character, voice, scene, and output assets are preserved.
- Existing services on ports 8080 and 8787 remain untouched; hotfix services use isolated alternate ports.

---

### Task 1: Front Camera Contract

**Files:**
- Modify: `mcp/ip_avatar_3d/test_warm_studio_contract.py`
- Modify: `mcp/ip_avatar_3d/test_blender_scene_contract.py`
- Modify: `mcp/ip_avatar_3d/warm_studio_contract.py`
- Modify: `mcp/ip_avatar_3d/blender_renderer.py`

**Interfaces:**
- Consumes: `MODE_CAMERA_SPECS[mode]` and `configure_camera_plan(data, mode_objects)`.
- Produces: a mode-specific `close` camera role and correct `Camera_Close -> close` alias resolution.

- [x] **Step 1: Write failing contract tests**

Assert that both presentation modes expose `wide`, `medium`, `close`, `three_quarter`, and `transition`; the three front roles have `location.x == 0.0`; and `Camera_Close` binds to the mode-specific close camera.

- [x] **Step 2: Verify the tests fail for the missing close role**

Run: `python3 -m unittest mcp/ip_avatar_3d/test_warm_studio_contract.py -v`

Expected: FAIL because `MODE_CAMERA_SPECS` has no `close` role.

- [x] **Step 3: Implement the minimum front close contract**

Add centered standing and seated close camera specifications and map `Camera_Close` to `close` in `configure_camera_plan`.

- [x] **Step 4: Run the contract and Blender scene tests**

Run: `python3 -m unittest mcp/ip_avatar_3d/test_warm_studio_contract.py -v`

Run the Blender scene contract with the repository's configured Blender executable.

Expected: all relevant tests PASS and the selected close camera is centered on the character.

### Task 2: Front Talking Camera Plan

**Files:**
- Modify: `mcp/ip_avatar_3d/test_server.py`
- Modify: `mcp/ip_avatar_3d/server.py`
- Modify: `mcp/ip_avatar_3d/README.md`

**Interfaces:**
- Consumes: `build_camera_plan(duration_sec, fps, camera_preset)`.
- Produces: `front_talking` preset and a safe front wide-to-medium camera plan.

- [x] **Step 1: Write failing plan tests**

Assert that `front_talking` produces restrained front wide-to-medium cuts, never returns `Camera_Close`, `Camera_ThreeQuarter`, or `Camera_Transition`, and that short clips remain on `Camera_Medium`.

- [x] **Step 2: Verify the tests fail because the preset is unsupported**

Run: `python3 -m unittest mcp.ip_avatar_3d.test_server.IPAvatar3DMCPTests.test_front_talking_camera_plan_is_restrained -v`

Expected: FAIL with the unsupported `cameraPreset` validation error.

- [x] **Step 3: Implement the preset without changing transition-shot behavior**

Add `front_talking` to the accepted preset set and return a centered wide-to-medium plan. Keep automatic close cuts and explicit `transition` and `three_quarter` presets intact for non-demo use and physical action QA.

- [x] **Step 4: Run the complete MCP unit suite**

Run: `python3 -m unittest discover -s mcp/ip_avatar_3d -p 'test_*.py'`

Expected: PASS with zero failures.

### Task 3: Formal Front Demo Job

**Files:**
- Create: `mcp/ip_avatar_3d/front_talking_demo.py`
- Create: `mcp/ip_avatar_3d/test_front_talking_demo.py`

**Interfaces:**
- Produces: a deterministic topic, script, restrained action sequence, AIOS run context, and final media QA helper.
- Consumes: the public AIOS `/api/agent/runs` API and final local artifact metadata.

- [x] **Step 1: Write failing demo-contract tests**

Assert 15-30 second duration, `talking_head` profile, `front_talking` camera preset, standing presentation, no stand/sit transition actions, maximum 20 percent B-roll coverage, 1920x1080 resolution, and 30 fps.

- [x] **Step 2: Verify the tests fail because the module does not exist**

Run: `python3 -m unittest mcp/ip_avatar_3d/test_front_talking_demo.py -v`

Expected: FAIL because `front_talking_demo` is missing.

- [x] **Step 3: Implement the deterministic request and QA report**

Use the standalone topic `为什么 AI 视频不要每三秒换画面`, the approved knowledge-sharing script, and restrained greeting, explanation, key-point, nod, and conclusion actions. Implement ffprobe checks for size, fps, duration, video/audio codecs, and continuity evidence.

- [x] **Step 4: Run demo-contract tests**

Run: `python3 -m unittest mcp/ip_avatar_3d/test_front_talking_demo.py -v`

Expected: PASS with zero failures.

### Task 4: Isolated Hotfix Runtime and Real AIOS Run

**Files:**
- Runtime outputs only: `outputs/front-talking-aios-demo/`

**Interfaces:**
- Cloud: `http://127.0.0.1:18088`
- Local Agent: `http://127.0.0.1:18081`
- HyperFrames: `http://127.0.0.1:18787`
- IP Avatar MCP: launched by Local Agent provider configuration.

- [x] **Step 1: Start dependencies and build current-worktree binaries**

Start existing Docker dependencies, build the Cloud and Local Agent from the hotfix worktree, and start HyperFrames from the same worktree on isolated ports.

- [x] **Step 2: Verify service provenance and health**

Record process working directories, commit SHA, health endpoints, MCP provider preflight, Blender executable, ffmpeg, and GPT-SoVITS readiness in `outputs/front-talking-aios-demo/runtime-evidence.json`.

- [x] **Step 3: Register a temporary AIOS user and submit the approved topic**

Create a project with `profileId=talking_head`, submit `/api/agent/runs` with `cameraPreset=front_talking`, and let Local Agent poll and execute the resulting plan.

- [x] **Step 4: Wait for terminal run status and collect artifacts**

Fail if any stage is skipped, emulated, or handled by the old services. Record run ID, project ID, plan, MCP outputs, HyperFrames manifest, and final artifact path.

### Task 5: Media QA, Regression Verification, and Delivery

**Files:**
- Runtime outputs: `outputs/front-talking-aios-demo/Front_Talking_AIOS_Demo_1080p.mp4`
- Runtime outputs: `outputs/front-talking-aios-demo/Front_Talking_AIOS_Demo_ContactSheet.png`
- Runtime outputs: `outputs/front-talking-aios-demo/Front_Talking_AIOS_Demo_QA.json`

**Interfaces:**
- Consumes: the final AIOS artifact.
- Produces: release evidence and the demo video.

- [x] **Step 1: Run objective media checks**

Use ffprobe to confirm 1920x1080, constant 30 fps, 15-30 second duration, H.264 video, AAC audio, and one-second GOP compatibility.

- [x] **Step 2: Inspect representative frames**

Generate a contact sheet covering every camera cut and gesture peak. Reject side angles, clipped hands/head, hand-body intersections, malformed limbs, underlit hands/feet, or unreadable mouth movement.

- [x] **Step 3: Run repository regression checks**

Run the MCP Python tests, Cloud Go tests for plan compilation, Local Agent IP A-roll tests, HyperFrames tests/build, and repository beta smoke checks.

- [x] **Step 4: Commit and update the hotfix PR**

Stage only the files created or modified by this plan, commit with a focused message, push `hotfix/ip-aroll-production-pipeline`, and verify PR checks.
