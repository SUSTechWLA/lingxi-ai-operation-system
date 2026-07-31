# Editorial Studio Scene Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render a rigged non-human presenter inside a reusable dark editorial Blender studio so the character and set share physically coherent lighting, shadows, reflections, depth of field, and camera changes.

**Architecture:** `render_talking_video` accepts an optional `.blend` scene and emits a deterministic camera plan. Blender opens that scene before importing the character, normalizes the character at `IP_Character_Spawn`, preserves the character rig/materials, selects authored cameras and lights, and renders the complete opaque scene in one pass. Existing static-plate and transparent-avatar modes remain available as compatibility fallbacks.

**Tech Stack:** Python 3, FastMCP, Blender 5.x Python API, Eevee Next/Cycles, FFmpeg, `unittest`.

## Global Constraints

- Preserve existing GLB armatures, weights, materials, actions, and viseme shape keys.
- A `.blend` scene takes precedence over `backgroundPath`; static plate composition remains unchanged when no scene is supplied.
- The default production scene is a restrained dark editorial/news-analysis studio for knowledge, opinion, and current-affairs A-roll.
- Scene contracts use `IP_Character_Spawn`, `IP_Focus_Head`, `Camera_Wide`, `Camera_Medium`, and `Camera_Close`.
- Video defaults remain 1920x1080 at 30 fps and must continue to support local dry runs.

---

### Task 1: MCP Scene Contract

**Files:**
- Modify: `mcp/ip_avatar_3d/test_server.py`
- Modify: `mcp/ip_avatar_3d/server.py`

**Interfaces:**
- Consumes: optional `sceneBlendPath`, `cameraPreset`, `lightingPreset`, `renderEngine`, and `targetCharacterHeight`.
- Produces: render input fields `backgroundMode=blender_scene`, `cameraPlan`, and resolved scene metadata.

- [ ] Write tests proving `.blend` validation, scene precedence, profile defaults, and deterministic automatic camera cuts.
- [ ] Run the focused tests and confirm they fail because scene parameters and `build_camera_plan` do not exist.
- [ ] Add argument validation, profile resolution, camera-plan generation, and response metadata.
- [ ] Re-run the focused tests and the complete Python suite.

### Task 2: Blender Scene Integration

**Files:**
- Modify: `mcp/ip_avatar_3d/blender_renderer.py`

**Interfaces:**
- Consumes: scene render input from Task 1.
- Produces: a scene containing the imported character at the authored spawn marker with camera markers bound to authored cameras.

- [ ] Add a Blender smoke-test payload that currently fails because the renderer clears the supplied scene.
- [ ] Load the `.blend` before GLB import, read target height from the spawn marker, and keep scene collections intact.
- [ ] Parent the completed rig to a placement root, align it to `IP_Character_Spawn`, update `IP_Focus_Head`, and configure camera timeline markers.
- [ ] Reuse authored lights, apply the selected energy preset, and configure Eevee Next or Cycles without replacing the scene world.
- [ ] Record scene/camera/lighting details in the rig report and render the full scene with an opaque film.

### Task 3: Editorial Studio Asset

**Files:**
- Create: `mcp/ip_avatar_3d/editorial_studio_builder.py`
- Create: `ip形象/main_ip/scenes/editorial-news-studio.blend`
- Create: `ip形象/main_ip/scenes/editorial-news-studio-preview.png`
- Modify: `ip形象/main_ip/character-profile.json`

**Interfaces:**
- Consumes: no character-specific geometry.
- Produces: a reusable metric `.blend` with named collections, spawn/focus markers, three cameras, and an authored light rig.

- [ ] Build a real floor, wall system, recessed editorial display, acoustic slats, practical luminaires, and procedural PBR materials.
- [ ] Add a soft neutral key, restrained fill, cool/warm rims, low-strength world lighting, and custom base-energy metadata.
- [ ] Author wide, medium, and close cameras with depth of field focused on `IP_Focus_Head`.
- [ ] Generate the `.blend` and scene preview through Blender CLI.
- [ ] Point the main-IP profile at the scene while retaining the existing plate as fallback.

### Task 4: End-to-End Verification

**Files:**
- Modify: `mcp/ip_avatar_3d/README.md`
- Output: `outputs/ip_avatar_3d_editorial_scene_test/`

**Interfaces:**
- Consumes: an existing rigged regression GLB and the new studio scene.
- Produces: MP4, preview PNG, scene-containing `.blend`, character-only GLB, and render/rig reports.

- [ ] Run all Python MCP tests and local Go MCP tests.
- [ ] Verify the stdio MCP schema exposes all new scene parameters.
- [ ] Render a short 640x360 interaction test in the authored scene.
- [ ] Inspect the preview/contact sheet for grounding, shared shadows, material preservation, framing, and nonblank camera cuts.
- [ ] Inspect reports and media metadata, then document exact paths and any remaining limitation that the new main-IP GLB is still pending.
