# Stylized Sloth Character Refinement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Blender scene mutations are serialized because all tasks share one interactive Blender process.

**Goal:** Refine `main-ip-aroll-master.blend` toward `target_character.png` while preserving the original Armature, animations, 26 existing Shape Keys, UVs, original vertex order, and production scene.

**Architecture:** Keep the original production mesh and rig as the immutable compatibility core. Add only reversible `CXR_` corrective layers, independent eye/fur/detail objects, isolated review-scene data, and versioned `.blend` copies. Every gate is implemented by one idempotent Blender Python script, executed through Blender MCP, followed by mesh validation, invariant comparison, review renders, and a checkpoint copy.

**Tech Stack:** Blender 5.1.2 Python API, Blender MCP 1.28.1, EEVEE, Shape Keys, Armature modifiers, Geometry Nodes, procedural material nodes, PNG review renders, JSON validation reports.

## Global Constraints

- Never save over `ip形象/main_ip/models/main-ip-aroll-master.blend`.
- Existing main-mesh vertex count is 5,478 and vertex order must not change.
- Preserve `UVMap`, all 43 original vertex groups, all 26 original Shape Keys, all 52 bones, all 60 Actions, 24 fps, and animation frame ranges.
- Do not remesh, decimate, voxel-remesh, join/split the production mesh, apply subdivision, or reorder existing modifiers.
- Existing modifier relative order remains Armature → Corrective Smooth → Simple Subdivision.
- New data names use the `CXR_` prefix.
- Generated scripts are idempotent, contain `main()`, and remain under `.codex/skills/codex_blender_character_refine/blender_scripts/generated/`.
- No external asset download, 3D generation, Poly Haven, Sketchfab, Hyper3D, Hunyuan3D, network, or subprocess calls.
- Git worktree isolation is not used because required `.blend`, reference images, and skill files are untracked workspace assets. Isolation is provided by timestamped `.blend` checkpoints on the non-main branch `hotfix/tool-system-convergence`.

---

### Task 1: Safety Baseline and Neutral Review Harness

**Files:**
- Create: `.codex/skills/codex_blender_character_refine/blender_scripts/generated/05_review_harness.py`
- Create: `ip形象/main_ip/models/reports/refinement_manifest.json`
- Modify: `ip形象/main_ip/models/reports/stage_00_validation.json`

**Interfaces:**
- Consumes: Gate 0 `asset_map.json` and `scene_audit.json`.
- Produces: `CXR_REVIEW` Scene, `CXR_ReviewCamera`, neutral key/fill/rim lights, gray ground/background, and baseline render paths used by every later gate.

- [ ] Run `01_checkpoint.py` through MCP with `STAGE="gate0_approved"`; expect `CHECKPOINT_OK` under `models/checkpoints/`.
- [ ] Create an idempotent review harness that links `IP_Character_Master` into `CXR_REVIEW`, creates an 80 mm full-frame camera, neutral gray World/ground, and three neutral area lights without touching Scene, Camera_Medium, Camera_Wide, or World.
- [ ] Render baseline front, left, right, back, front-three-quarter-left, front-three-quarter-right, neutral face, and smile face PNGs under `models/renders/gate00_baseline/`.
- [ ] Record original vertex-coordinate hashes, UV loop hash, Shape Key names/counts, bone names, Action frame ranges, modifier stack, material assignments, cameras, render settings, and source file mtime in `refinement_manifest.json`.
- [ ] Verify the original master mtime remains `2026-07-14 10:10:05` and render paths exist.

### Task 2: Gate 1 Macro Form

**Files:**
- Create: `.codex/skills/codex_blender_character_refine/blender_scripts/generated/10_gate1_macro.py`
- Create: `ip形象/main_ip/models/reports/stage_01_summary.md`
- Create: `ip形象/main_ip/models/reports/stage_01_validation.json`

**Interfaces:**
- Consumes: main mesh `part_00000001.001`, original Head/Jaw/hand groups, and review harness.
- Produces: reversible `CXR_MacroForm` corrective Shape Key plus review renders and Gate 1 checkpoint.

- [ ] Run `01_checkpoint.py` with `STAGE="pre_gate1_macro"`.
- [ ] Generate `CXR_MacroForm` from Basis coordinates using smooth spatial masks: increase cranial volume, muzzle projection, cheek fullness, brow transition, neck-to-hood continuity, palm volume, finger taper, and torso silhouette without changing topology.
- [ ] Keep all original Shape Key blocks byte-for-byte named and in original order; append only `CXR_MacroForm` with value 1.0.
- [ ] Render all fixed body views plus neutral/smile face views to `models/renders/gate01_macro/`.
- [ ] Execute `02_validate_meshes.py`; compare manifest invariants and representative `Face_Neutral`, `Face_Happy`, `Face_Surprised`, `Gesture_Wave`, and `Gesture_Fist` poses.
- [ ] Save `main-ip-aroll-master_<timestamp>_gate1_macro.blend` and write Stage 1 reports.

### Task 3: Gate 2 Layered Eyes and Mouth

**Files:**
- Create: `.codex/skills/codex_blender_character_refine/blender_scripts/generated/20_gate2_eyes_mouth.py`
- Create: `ip形象/main_ip/models/reports/stage_02_summary.md`
- Create: `ip形象/main_ip/models/reports/stage_02_validation.json`

**Interfaces:**
- Consumes: Eye.L/Eye.R bone world transforms, head mesh eye-group bounds, oral objects, and `CXR_MacroForm`.
- Produces: separate `CXR_Eye.L/R`, `CXR_Iris.L/R`, `CXR_Pupil.L/R`, `CXR_Cornea.L/R`, tearline objects/materials, and `CXR_MouthCornerVolume` corrective Shape Key.

- [ ] Run `01_checkpoint.py` with `STAGE="pre_gate2_eyes_mouth"`.
- [ ] Derive eye centers and radii from Eye.L/Eye.R weighted vertices and bones; create layered spherical components with dedicated sclera, iris, pupil, and cornea materials.
- [ ] Parent or Armature-bind each eye layer to its existing eye bone; keep gaze driven by existing Eye.L/Eye.R animation.
- [ ] Position new eye layers slightly in front of the embedded eye surface and add restrained upper/lower lid rim overlays without deleting original faces.
- [ ] Add `CXR_MouthCornerVolume` to deepen muzzle-to-mouth transition, lip corners, and smile volume while retaining all original viseme deltas.
- [ ] Render neutral, blink/squint, smile, wide-eye, left/right gaze, and Mouth_A/E/O/U/MBP close-ups to `models/renders/gate02_face_eyes/`.
- [ ] Validate no eye/lid penetration, consistent gaze, oral-object parenting, unchanged original Shape Keys/Actions, and unchanged production mesh topology.
- [ ] Save Gate 2 checkpoint and reports.

### Task 4: Gate 3A Layered Fur

**Files:**
- Create: `.codex/skills/codex_blender_character_refine/blender_scripts/generated/30_gate3_fur.py`
- Create: `ip形象/main_ip/models/reports/stage_03_fur_summary.md`
- Create: `ip形象/main_ip/models/reports/stage_03_fur_validation.json`

**Interfaces:**
- Consumes: packed base-color texture, UVMap, Head/Hand/Foot weights, Armature, review harness.
- Produces: `CXR_FurEmitter`, `CXR_FurFiber`, `CXR_FurNodes`, soft mask-transition fibers, brows, and controlled head tuft.

- [ ] Run `01_checkpoint.py` with `STAGE="pre_gate3_fur"`.
- [ ] Build a non-rendering emitter duplicate from selected face copies classified by spatial region, bone weights, UV-sampled base color, and exclusion masks for eyes, nose, mouth, clothing, and nails.
- [ ] Copy only the required Armature weights to the emitter and add an Armature modifier targeting the existing Armature.
- [ ] Create a short tapered fiber prototype and Geometry Nodes instancing system with separate viewport/render density, normal-aligned rotation, deterministic random seed, length variation, restrained clumping, and brown/white material variation.
- [ ] Add shorter facial fibers, feathered white-mask boundary fibers, eyebrow fibers, and a controlled longer head tuft; keep all objects independent of the production mesh.
- [ ] Test neutral and raised-arm poses; render close-ups and all silhouettes to `models/renders/gate03_fur/`.
- [ ] Validate no eye/nose/mouth coverage, no clothing penetration in tested poses, stable Armature deformation, and unchanged base topology.
- [ ] Save fur checkpoint and reports.

### Task 5: Gate 3B Clothing, Hands, and Nails

**Files:**
- Create: `.codex/skills/codex_blender_character_refine/blender_scripts/generated/31_gate3_clothing_hands.py`
- Create: `ip形象/main_ip/models/reports/stage_03_clothing_hands_summary.md`
- Create: `ip形象/main_ip/models/reports/stage_03_clothing_hands_validation.json`

**Interfaces:**
- Consumes: existing cardigan/hoodie/trouser surface, hand/finger bones and weights, fur exclusion masks.
- Produces: `CXR_Seams`, `CXR_Ribbing`, `CXR_Drawstrings`, enhanced button/pocket overlays, independent nail overlays, and `CXR_HandWristRefine` corrective Shape Key.

- [ ] Run `01_checkpoint.py` with `STAGE="pre_gate3_clothing_hands"`.
- [ ] Add separate curve/mesh overlays for cardigan opening, pocket seams, hoodie drawstrings, buttons, cuffs, hem ribbing, and trouser seam/fold accents; bind overlays to appropriate torso/limb bones.
- [ ] Add `CXR_HandWristRefine` for palm fullness, finger taper, knuckle transition, nail seating, and wrist-to-sleeve continuity without altering existing weights or topology.
- [ ] Create separate nail-cap overlays only where the existing shared mesh cannot supply clean seating; parent them to distal finger/toe bones.
- [ ] Render relaxed, raised arm, fist, open hand, pointing, and wrist-twist poses to `models/renders/gate03_clothing_hands/`.
- [ ] Validate no sleeve/wrist gaps, nail detachment, clothing/fur intersection, or action timing changes.
- [ ] Save clothing/hands checkpoint and reports.

### Task 6: Gate 4 Materials and Rendering

**Files:**
- Create: `.codex/skills/codex_blender_character_refine/blender_scripts/generated/40_gate4_materials_render.py`
- Create: `ip形象/main_ip/models/reports/stage_04_summary.md`
- Create: `ip形象/main_ip/models/reports/stage_04_validation.json`

**Interfaces:**
- Consumes: `Material.001`, packed PBR textures, CXR eye/fur/clothing/nail objects, CXR_REVIEW Scene.
- Produces: non-destructive material-node enhancement, preview/final review presets, neutral comparisons, and production-room integration copy.

- [ ] Run `01_checkpoint.py` with `STAGE="pre_gate4_materials"`.
- [ ] Duplicate `Material.001` to `CXR_Character_Master_Material`, preserve original packed textures, and add position/color masks for fur, white muzzle, cloth, trousers, eyes/nose, and nails.
- [ ] Add restrained region-specific micro-normal, roughness, sheen, coat, and subsurface response without hiding geometric defects.
- [ ] Finalize dedicated sclera/iris/pupil/cornea, fur, cloth-detail, button/drawstring, and nail materials.
- [ ] Keep AgX / Look None / Exposure 0 / Gamma 1; create preview EEVEE and final-quality EEVEE presets only in CXR_REVIEW.
- [ ] Render neutral before/after comparisons, all fixed views, face close-ups, and material diagnostics to `models/renders/gate04_materials/`.
- [ ] Open `editorial-news-studio.blend` only after the character checkpoint exists; create a versioned production-scene copy, integrate the refined character collection, and adapt light energy/color without overwriting the original studio file.
- [ ] Render production-room comparison frames and save Gate 4 asset/studio checkpoints and reports.

### Task 7: Gate 5 Rig and Final Validation

**Files:**
- Create: `.codex/skills/codex_blender_character_refine/blender_scripts/generated/50_gate5_validate.py`
- Create: `ip形象/main_ip/models/reports/stage_05_summary.md`
- Create: `ip形象/main_ip/models/reports/stage_05_validation.json`
- Create: `ip形象/main_ip/models/renders/final_contact_sheet.png`

**Interfaces:**
- Consumes: refinement manifest, every Gate checkpoint/report, 60 Actions, original mesh/rig, all CXR additions.
- Produces: final regression report, contact sheet, and final versioned `.blend`.

- [ ] Run `01_checkpoint.py` with `STAGE="pre_gate5_validation"`.
- [ ] Compare original vertex order/coordinates for Basis where unchanged by approved CXR corrective keys, UV hash, original Shape Key names/order/deltas, original vertex groups, bone hierarchy, modifier relative order, Action names/frame ranges, fps, production camera, and color management.
- [ ] Test neutral, raised arm, both arms forward, elbow bend, wrist bend, open/closed hands, head turns, blink, smile, wide eyes, gaze, and Mouth_A/E/O/U/MBP.
- [ ] Detect and report mesh intersections, weight collapse, clothing clipping, detached fur roots, eye/lid penetration, broken Shape Keys, changed timing, missing materials, and missing render assets.
- [ ] Fix only regressions introduced by CXR stages; rerun the full validation after every fix.
- [ ] Render final neutral and production contact sheets and assemble `final_contact_sheet.png`.
- [ ] Save `main-ip-aroll-master_<timestamp>_final_refined.blend` without overwriting the source master.
- [ ] Verify the source master mtime/size remain unchanged and every stage report/checkpoint/render exists.

## Self-Review Result

- Spec coverage: All Gate 0 findings, seven visual gaps, Gate 1–5 requirements, checkpoints, validation, fixed review views, and production-room integration are mapped to tasks.
- Placeholder scan: No deferred implementation placeholders are present.
- Interface consistency: Every generated object/script uses `CXR_`; every task consumes the same original object names from `asset_map.json`; every stage writes its own checkpoint, report, validation, and render folder.
- Scope decision: Work remains one serialized Blender production pipeline because all stages operate on the same rigged asset and must share checkpoints.
