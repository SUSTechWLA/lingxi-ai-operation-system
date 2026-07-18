# Stylized Sloth Eye Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current toy-like `CXR_` eyes with smaller, warmer, more deeply seated target-faithful eyes while preserving every production mesh, rig, Shape Key, UV, and action invariant.

**Architecture:** The production mesh remains immutable. One idempotent Blender Python stage updates only independent `CXR_` eye transforms, materials, lid curves, expression drivers, and two new secondary catchlights; review and validation scripts then compare the result against the Gate 0 manifest and save versioned copies.

**Tech Stack:** Blender 4.x Python API (`bpy`, `bmesh`, `mathutils`), Blender MCP `execute_blender_code`, JSON validation reports, EEVEE/AgX neutral review renders.

## Global Constraints

- Never overwrite `ip形象/main_ip/models/main-ip-aroll-master.blend` or `ip形象/main_ip/scenes/editorial-news-studio.blend`.
- Preserve 5478 vertices, 11336 edges, 5846 faces, vertex order, `UVMap`, all 26 original Shape Key hashes/order, all 43 vertex groups, all 52 bones/hierarchy, all 60 actions/ranges, and `Armature → IP_Render_CorrectiveSmooth → IP_Render_Subdivision`.
- Do not edit the main mesh, original source material, Armature, actions, frame rate, production cameras, or color management.
- Blender mutations must run through MCP, use repository-local generated scripts, expose `main()`, be idempotent, and tag new data with `CXR_`.
- Cornea remains hidden unless a neutral EEVEE render proves it does not render black.

---

### Task 1: Disk Baseline and Pre-Gate Checkpoint

**Files:**
- Read: `ip形象/main_ip/models/reports/refinement_manifest.json`
- Read: `ip形象/main_ip/models/reports/final_independent_verification.json`
- Execute: `.codex/skills/codex_blender_character_refine/blender_scripts/01_checkpoint.py`

**Interfaces:**
- Consumes: canonical final blend `main-ip-aroll-master_20260718_100418_final_refined.blend`.
- Produces: timestamped `pre_gate6_eye_redesign` checkpoint path printed as `CHECKPOINT_OK`.

- [ ] **Step 1: Reload the canonical final blend through Blender MCP**

Run `bpy.ops.wm.open_mainfile(filepath=CANONICAL_FINAL)` and print `GATE6_SOURCE_OPEN_OK`.
Expected: active file path ends in `main-ip-aroll-master_20260718_100418_final_refined.blend`.

- [ ] **Step 2: Run the existing checkpoint helper**

Load `01_checkpoint.py` with `__name__="cxr_checkpoint"`, set `STAGE="pre_gate6_eye_redesign"`, and invoke `main()`.
Expected: `CHECKPOINT_OK` and a new compressed `.blend` copy; active file path remains the canonical final.

- [ ] **Step 3: Reconfirm the disk invariants before mutation**

Run `52_independent_verify.py` against the reloaded file.
Expected: `INDEPENDENT_VERIFY_OK` with zero failures.

### Task 2: Idempotent CXR Eye Reconstruction

**Files:**
- Create: `.codex/skills/codex_blender_character_refine/blender_scripts/generated/60_gate6_eye_redesign.py`
- Create: `ip形象/main_ip/models/reports/stage_06_validation.json`
- Create: `ip形象/main_ip/models/reports/stage_06_summary.md`

**Interfaces:**
- Consumes: `Armature`, `Eye.L/R` bones, `part_00000001.001` Shape Keys, existing Gate 2 `CXR_` eye objects/materials, and `refinement_manifest.json`.
- Produces: `main() -> None`, `GATE6_EYE_REDIGN_OK` on structural pass, redesigned CXR eye layers, and a stage validation report.

- [ ] **Step 1: Implement safe transform and material helpers**

Create `set_eye_sphere(name, center, scale, material, bone_name)` that removes only the object's `scale[2]` and `location[0]` drivers, assigns `matrix_world = Matrix.Translation(center) @ Matrix.Diagonal((*scale, 1.0))`, restores the named CXR material, bone parent, and `CXR_gate6_role` tag.

Create `configure_principled(name, base_color, roughness, coat)` that updates only the named CXR material's Principled inputs and diffuse preview color.

- [ ] **Step 2: Apply the approved neutral proportions**

For each side, derive the bone head world center and add an inward X offset of 0.007 m. Use these world-space dimensions:

```python
sclera = (0.0440, 0.0320, 0.0460)
iris_ring = (0.0285, 0.0028, 0.0285)
iris = (0.0252, 0.0025, 0.0252)
iris_inner = (0.0160, 0.0021, 0.0160)
pupil = (0.0122, 0.0019, 0.0122)
primary_catchlight = (0.0044, 0.0011, 0.0050)
secondary_catchlight = (0.0018, 0.0009, 0.0020)
```

Place the sclera center at bone center Y minus 0.010 m; place successive front layers at Y offsets `-0.0300`, `-0.0315`, `-0.0327`, `-0.0340`, and catchlights at `-0.0362` from the revised sclera center.

- [ ] **Step 3: Rebuild the lid and catchlight hierarchy**

Recreate `CXR_LidUpper.L/R` as 17-point Bezier arcs with radii 0.0445 m × 0.0435 m, angles 18–162 degrees, front offset -0.0345 m, and bevel depth 0.00165 m. Keep lower lids, tearlines, and corneas hidden. Create or update `CXR_CatchlightSecondary.L/R` as bone-parented UV spheres with `CXR_Catchlight` material and `CXR_gate6_role="secondary_corneal_highlight"`.

- [ ] **Step 4: Rebuild expression and gaze drivers**

For sclera, iris ring, iris, inner iris, pupil, and both catchlights, recreate `scale[2]` drivers using:

```python
f"{base_z:.9f}*(1.0-0.64*squint+0.07*wide)"
```

For iris ring, iris, inner iris, and pupil, create `location[0]` drivers with two Shape Key variables and a calibrated 0.70 local-unit offset:

```python
f"{base_x:.9f}+0.70*look_left-0.70*look_right"
```

If the front render shows reversed gaze, invert the two signs and rerender before checkpointing.

- [ ] **Step 5: Apply target-faithful CXR material calibration**

Use deep warm brown socket and upper lid, warm ivory sclera, near-black brown limbal ring, richer amber iris, golden inner iris, glossy dark pupil, and white catchlights. Required values:

```python
socket=(0.105, 0.025, 0.006, 1.0), roughness=0.58, coat=0.02
sclera=(0.82, 0.74, 0.64, 1.0), roughness=0.30, coat=0.18
ring=(0.012, 0.003, 0.001, 1.0), roughness=0.22, coat=0.32
iris=(0.31, 0.075, 0.004, 1.0), roughness=0.24, coat=0.30
inner=(0.58, 0.19, 0.012, 1.0), roughness=0.21, coat=0.34
pupil=(0.0025, 0.0015, 0.0010, 1.0), roughness=0.10, coat=0.58
lid=(0.12, 0.026, 0.007, 1.0), roughness=0.52, coat=0.03
catchlight=(1.0, 0.96, 0.88, 1.0), roughness=0.04, coat=0.90
```

- [ ] **Step 6: Validate structural isolation**

Compare main counts, Basis hash, UV hash, original Shape Key hashes/order, vertex groups, bone hierarchy, action ranges, and modifier stack with the manifest. Confirm the secondary catchlights exist, all eye objects use valid eye bones, corneas/lower lids/tearlines are hidden, and no main Shape Key driver was added.
Expected: `GATE6_EYE_REDIGN_OK`; otherwise `GATE6_EYE_REDIGN_FAIL` and do not save the approved checkpoint.

### Task 3: Neutral and Expression Review Loop

**Files:**
- Create: `ip形象/main_ip/models/renders/gate06_eye_redesign/*.png`
- Update: `ip形象/main_ip/models/reports/stage_06_validation.json`

**Interfaces:**
- Consumes: redesigned eye layers and `CXR_REVIEW` scene.
- Produces: named 1024×1024 AgX/None/0/1 PNG renders and visual acceptance decision.

- [ ] **Step 1: Render the required eye review set**

Render `before_neutral`, `after_neutral`, `after_3q_left`, `after_3q_right`, `after_smile`, `after_blink`, `after_wide`, `after_gaze_left`, and `after_gaze_right` using the 85 mm face camera. Preserve 1024×1024, EEVEE, AgX, Look None, exposure 0, gamma 1.

- [ ] **Step 2: Review against explicit failure conditions**

Reject the result if neutral shows a complete black lower outline, large white crescent, circular startled stare, crossed gaze, asymmetric centers, disconnected highlights, black cornea, or lid/eyeball intersection.

- [ ] **Step 3: Tune only approved CXR parameters if rejected**

Adjust only eye center X/Y, layer X/Z scales, lid radii/bevel, gaze-driver sign/magnitude, and CXR material colors/roughness. Rerun the full nine-render set after every tuning pass.

### Task 4: Full Regression, Studio Integration, and Delivery

**Files:**
- Create: `.codex/skills/codex_blender_character_refine/blender_scripts/generated/61_gate6_validate_deliver.py`
- Create: `ip形象/main_ip/models/reports/stage_06_final_validation.json`
- Create: `ip形象/main_ip/models/reports/stage_06_independent_verification.json`
- Create: `ip形象/main_ip/models/checkpoints/main-ip-aroll-master_<timestamp>_eye_redesign_final.blend`
- Create: `ip形象/main_ip/scenes/production_versions/editorial-news-studio_<timestamp>_eye_redesign.blend`

**Interfaces:**
- Consumes: approved Gate 6 eye result, Gate 0 manifest, Gate 5 pose/action set, and original studio source.
- Produces: canonical eye-redesign character and studio copies plus independent validation reports.

- [ ] **Step 1: Run the full character regression**

Render neutral front/three-quarter, smile, blink, wide, gaze left/right, wave, fist, wrist twist, both-arms-forward, and head left/right. Evaluate meshes for finite coordinates and healthy bounding-box extents.
Expected: all pose records `status="pass"`.

- [ ] **Step 2: Recompute production invariants**

Recompute main counts, Basis/UV/original Shape Key hashes, vertex groups, bone hierarchy, modifier order, 60 action ranges, packed texture presence, eye-driver counts, and original source/studio metadata.
Expected: exact Gate 0 match and zero validation errors.

- [ ] **Step 3: Save the new character delivery copy**

Use `bpy.ops.wm.save_as_mainfile(filepath=target, copy=True, compress=True)` with suffix `_eye_redesign_final.blend`.
Expected: target exists and original character source metadata remains unchanged.

- [ ] **Step 4: Append the revised collection into a fresh studio copy**

Open the untouched studio source, append `IP_Character_Master` from the Gate 6 character file, preserve `Camera_Medium`, 1920×1080, 30 fps, and existing neutralized-light calibration values, render Wide/Medium/Close, and save only to `production_versions`.
Expected: three studio renders exist and original studio metadata remains unchanged.

- [ ] **Step 5: Reload both delivery files from disk and independently verify**

Reload the character delivery and recompute all invariants without trusting the in-memory Gate 6 report. Confirm the integrated studio copy exists and contains `IP_Character_Master_Refined` with the two secondary catchlights.
Expected: `GATE6_INDEPENDENT_VERIFY_OK` with an empty failures list.

### Task 5: Documentation and Handoff

**Files:**
- Update: `ip形象/main_ip/models/reports/final_delivery_index.md`
- Create: `ip形象/main_ip/models/renders/gate06_eye_redesign/contact_sheet.png`

**Interfaces:**
- Consumes: final validation reports and renders.
- Produces: one canonical artifact index and visual review sheet.

- [ ] **Step 1: Generate a contact sheet**

Combine neutral, three-quarter, smile, blink, wide, gaze, action, and studio close-up renders into a single PNG without altering source renders.

- [ ] **Step 2: Update the delivery index**

Record the new canonical character and studio files, Gate 6 status, eye-specific changes, preserved invariants, and the unchanged pre-existing 33 boundary/non-manifold edges.

- [ ] **Step 3: Report the result with evidence**

Provide clickable links to the new character, studio, validation, independent verification, and contact sheet. Explicitly disclose any remaining visual limitation rather than claiming identity with the 2D target.
