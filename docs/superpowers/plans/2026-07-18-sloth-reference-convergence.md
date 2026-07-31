# Sloth Reference-Convergence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Blender mutations are serialized because they target one interactive Blender process.

**Goal:** Improve the current formal sloth asset toward `target_character.png` without changing production topology, rig compatibility, or asset uniqueness.

**Architecture:** Audit and checkpoint the current final file, then apply small idempotent Blender-Python stages for facial form, eyes/mask, surface detail, and camera/lookdev. Each stage writes a JSON report, renders fixed views, saves a new checkpoint, and is accepted only if invariant checks pass.

**Tech Stack:** Blender 5.1 Python API, Blender MCP socket bridge, Cycles, AgX, Shape Keys, procedural materials, Hair/Groom objects, PNG/EXR review renders, JSON validation.

## Global Constraints

- Do not modify the untouched source checkpoint or overwrite it.
- Keep `GEO_HeadBody` at 5,478 vertices and preserve its vertex order, UVMap, original Shape Keys, vertex groups, and modifier relative order.
- Preserve the single `RIG_Sloth`, 52 bones, 60 Actions, constraints, drivers, animation timing, and production scene.
- Do not remesh, decimate, voxel-remesh, join/split the production mesh, download assets, call external 3D services, or execute network/shell/subprocess operations from Blender.
- Save all Blender Python to `ip形象/main_ip/scripts/` before execution and save a checkpoint before every mutation stage.
- Use only the one formal `COL_CHR_SLOTH_FINAL`; clean temporary and superseded data before delivery.

---

### Task 1: Audit and safety checkpoint

**Files:**
- Create: `ip形象/main_ip/scripts/11a_reference_refine_audit_checkpoint.py`
- Create: `ip形象/main_ip/reports/reference_refine_audit.json`
- Create: `ip形象/main_ip/checkpoints/sloth_090_pre_reference_refine.blend`

- [ ] Execute the saved script through Blender MCP.
- [ ] Confirm the open file is `character_sloth_final.blend` and record object, material, rig, Shape Key, modifier, Groom, eye, clothing, and camera state.
- [ ] Verify the checkpoint exists and the current working filepath remains unchanged.

### Task 2: Reference proportion and facial-form pass

**Files:**
- Create: `ip形象/main_ip/scripts/11b_reference_form_refine.py`
- Create: `ip形象/main_ip/reports/reference_form_refine.json`
- Create: `ip形象/main_ip/checkpoints/sloth_091_reference_form.blend`

- [ ] Compute safe spatial masks from Basis coordinates and existing vertex groups.
- [ ] Add or update one reversible formal corrective layer for cheek, muzzle, chin, forehead, and compact head/neck transitions without changing topology.
- [ ] Render front, three-quarter, and profile clay views and compare silhouette/landmarks.
- [ ] Verify all original expression deltas, vertex order, UVs, Actions, and rig structure remain unchanged.

### Task 3: Eye, eyelid, facial-mask, and mouth polish

**Files:**
- Create: `ip形象/main_ip/scripts/11c_reference_face_eye_polish.py`
- Create: `ip形象/main_ip/reports/reference_face_eye_polish.json`
- Create: `ip形象/main_ip/checkpoints/sloth_092_reference_face_eyes.blend`

- [ ] Preserve recessed eye centers and refine only iris/pupil hierarchy, limbal gradient, catchlight scale, lid silhouette, tearline restraint, and gaze.
- [ ] Soften white/brown facial transition with off-white color, roughness, short-fur density/length, and boundary breakup.
- [ ] Increase perceived muzzle/cheek/mouth-corner volume and refine nose response without changing head topology.
- [ ] Render neutral, blink, smile, wide-eye, mouth-open, three-quarter, and strict profile close-ups.

### Task 4: Groom, clothing, hand, and material detail

**Files:**
- Create: `ip形象/main_ip/scripts/11d_reference_surface_detail.py`
- Create: `ip形象/main_ip/reports/reference_surface_detail.json`
- Create: `ip形象/main_ip/checkpoints/sloth_093_reference_surface.blend`

- [ ] Refine formal Groom region roughness, root/tip color, silhouette breakup, face exclusions, brow/tuft direction, and mask transition.
- [ ] Add correctly scaled cardigan knit, hoodie fleece, trouser weave, seam/rib/button response, and restrained fold contrast through existing formal objects/materials.
- [ ] Improve visible nail material/seating and wrist-to-sleeve readability without topology changes.
- [ ] Validate neutral, raised arm, wrist bend, open hand, and fist poses.

### Task 5: Reference-like camera, renders, cleanup, and validation

**Files:**
- Create: `ip形象/main_ip/scripts/11e_reference_camera_final_render.py`
- Create: `ip形象/main_ip/scripts/11f_reference_refine_final_validation.py`
- Create: `ip形象/main_ip/reports/reference_camera_match.json`
- Create: `ip形象/main_ip/reports/reference_refine_final_validation.json`
- Create: `ip形象/main_ip/checkpoints/sloth_094_reference_refined_final.blend`

- [ ] Match an 80 mm frontal product portrait with target-like headroom, eye line, neutral warm-white background, soft key/fill/rim, and no perspective cheating.
- [ ] Render final front, front three-quarter, side, back, face close-up, blink, smile, and production-scene evidence in Cycles/AgX.
- [ ] Remove temporary and superseded objects/materials/Groom while retaining the one formal character system.
- [ ] Re-run topology, UV, Shape Key, driver, Armature, Action, material, Groom, scene-sharing, and file-output checks.
- [ ] Save `character_sloth_final.blend` only after the new checkpoint passes all checks.

## Self-review

- The plan covers every requested visual area and explicitly prevents camera-only cheating.
- Each mutation is isolated, checkpointed, rendered, and validated.
- No placeholder, topology migration, external asset, or duplicate formal-character path remains.

