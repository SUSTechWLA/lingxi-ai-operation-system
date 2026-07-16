# Main IP Oral And Three-Digit Hand Refinement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace bead-like teeth and the low-detail tongue with continuous production oral geometry, refine the existing three-digit source hands for cleaner presenter-shot silhouettes, and publish a validated 1080p A-roll master and demo.

**Architecture:** Keep oral mesh construction in a focused Blender helper and keep source-hand analysis/weighting in `hand_refinement.py`. `blender_renderer.py` orchestrates both passes without replacing the source face or rig. Blender-side contract tests validate topology, weights, collisions, and preserved capabilities before the staged master is rendered or published.

**Tech Stack:** Python 3.11, Blender 5.1.2 Python API, `mathutils`, Eevee Next, unittest, FFmpeg/FFprobe.

## Global Constraints

- Work only on `hotfix/ip-aroll-production-pipeline` in `/Users/wanglian/.config/superpowers/worktrees/tangying-ai-operation-system/sloth-warm-studio-integration`.
- Preserve the source face, lips, UVs, source PBR materials, body, clothing, canonical rig, three-digit anatomy, eighteen finger deform bones, wrist controls, Actions, cameras, and permanent voice.
- Do not add five-finger hands, individual realistic teeth, a mouth card, curve mouth, texture patch, detached hand, or replacement facial overlay.
- Work on staged copies; never overwrite the source FBX or the only approved master before QA passes.
- Production validation remains 1920x1080, 30 fps CFR, Eevee Next, and a 15-30 second front-facing A-roll demo.
- Every production code change follows RED-GREEN-REFACTOR under Blender's Python runtime.
- Generated `.blend`, image, report, audio, and video artifacts remain untracked unless an existing tracked asset is explicitly versioned.
- Do not stage unrelated `.superpowers/sdd/task-4-report.md`, model, voice, turnaround, backup, output, or temporary files.

---

### Task 1: Continuous Oral Geometry Contract

**Files:**
- Create: `mcp/ip_avatar_3d/oral_refinement.py`
- Create: `mcp/ip_avatar_3d/test_blender_oral_refinement.py`
- Modify: `mcp/ip_avatar_3d/blender_renderer.py`

**Interfaces:**
- Consumes: `source_face`, `armature`, `bone_map`, `dimensions`, and the existing material/binding helpers.
- Produces: `create_refined_oral_interior(...) -> dict[str, bpy.types.Object]` with `oral_cavity`, `upper_teeth`, `lower_teeth`, `upper_gum`, `lower_gum`, and `tongue` roles.

- [ ] **Step 1: Write failing topology and binding tests**

Add Blender-side tests that call `create_refined_oral_interior` in a minimal mouth fixture and assert:

```python
def mesh_component_count(obj) -> int:
    adjacency = {vertex.index: set() for vertex in obj.data.vertices}
    for edge in obj.data.edges:
        adjacency[edge.vertices[0]].add(edge.vertices[1])
        adjacency[edge.vertices[1]].add(edge.vertices[0])
    remaining = set(adjacency)
    components = 0
    while remaining:
        components += 1
        stack = [remaining.pop()]
        while stack:
            current = stack.pop()
            neighbors = adjacency[current] & remaining
            remaining.difference_update(neighbors)
            stack.extend(neighbors)
    return components


def test_dental_arches_are_connected_and_follow_expected_bones():
    result, armature = build_oral_fixture()
    assert mesh_component_count(result["upper_teeth"]) == 1
    assert mesh_component_count(result["lower_teeth"]) == 1
    assert set(group.name for group in result["upper_teeth"].vertex_groups) == {"Head"}
    assert set(group.name for group in result["lower_teeth"].vertex_groups) == {"Jaw"}


def test_tongue_is_connected_tapered_and_weighted_to_three_bones():
    result, _ = build_oral_fixture()
    tongue = result["tongue"]
    assert mesh_component_count(tongue) == 1
    assert {group.name for group in tongue.vertex_groups} == {"Tongue_01", "Tongue_02", "Tongue_03"}
    assert tongue["ip_tongue_longitudinal_rings"] >= 9
    assert tongue["ip_tongue_tip_width_ratio"] < 0.72
    assert tongue["ip_tongue_center_groove"] is True
```

- [ ] **Step 2: Run tests to verify RED**

Run:

```bash
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --python mcp/ip_avatar_3d/test_blender_oral_refinement.py
```

Expected: fail because `oral_refinement` and `create_refined_oral_interior` do not exist.

- [ ] **Step 3: Implement connected dental, gum, cavity, and tongue builders**

Create `oral_refinement.py` with focused parametric builders:

```python
def create_refined_oral_interior(
    source_face: bpy.types.Object,
    armature: bpy.types.Object,
    bone_map: dict[str, str],
    dimensions: dict[str, Any],
    *,
    material_factory: Callable[..., bpy.types.Material],
    bind_object: Callable[..., None],
) -> dict[str, bpy.types.Object]:
    frame = OralFrame.from_source(source_face, dimensions)
    upper_teeth = build_rounded_arch("IP_UpperTeeth", frame, upper=True)
    lower_teeth = build_rounded_arch("IP_LowerTeeth", frame, upper=False)
    upper_gum = build_gum_arch("IP_UpperGum", frame, upper=True)
    lower_gum = build_gum_arch("IP_LowerGum", frame, upper=False)
    cavity = build_oral_cup("IP_OralCavity", frame)
    tongue = build_articulated_tongue("IP_Tongue", frame)
    bind_object(upper_teeth, armature, rigid_weights(upper_teeth, bone_map["head"]))
    bind_object(upper_gum, armature, rigid_weights(upper_gum, bone_map["head"]))
    bind_object(lower_teeth, armature, rigid_weights(lower_teeth, bone_map["jaw"]))
    bind_object(lower_gum, armature, rigid_weights(lower_gum, bone_map["jaw"]))
    bind_object(cavity, armature, rigid_weights(cavity, bone_map["head"]))
    bind_object(tongue, armature, tongue_weights(tongue, bone_map))
    return {
        "oral_cavity": cavity,
        "upper_teeth": upper_teeth,
        "lower_teeth": lower_teeth,
        "upper_gum": upper_gum,
        "lower_gum": lower_gum,
        "tongue": tongue,
    }
```

Use connected quad grids with rounded end caps. Store measurable metadata for
component count, ring count, tip ratio, and groove presence. Use smooth shading,
warm ivory teeth, muted gums, dark rough cavity, and desaturated rose tongue.

- [ ] **Step 4: Integrate the builder and remove cluster generation from the production path**

Import `oral_refinement` in `blender_renderer.py` and replace
`create_integrated_oral_interior` with a compatibility wrapper that delegates to
`create_refined_oral_interior`. Existing refined roles are reused only when
their `ip_oral_refinement_version` matches the current version; older tooth and
tongue objects are removed from the staged copy before regeneration.

- [ ] **Step 5: Run oral and existing face tests to verify GREEN**

Run:

```bash
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --python mcp/ip_avatar_3d/test_blender_oral_refinement.py
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --python mcp/ip_avatar_3d/test_blender_character_rig.py
```

Expected: all oral and character-rig tests pass with no Blender traceback.

- [ ] **Step 6: Commit**

```bash
git add mcp/ip_avatar_3d/oral_refinement.py \
  mcp/ip_avatar_3d/test_blender_oral_refinement.py \
  mcp/ip_avatar_3d/blender_renderer.py
git commit -m "feat: refine sloth oral geometry"
```

### Task 2: Three-Digit Source-Hand Aesthetic Refinement

**Files:**
- Modify: `mcp/ip_avatar_3d/hand_refinement.py`
- Modify: `mcp/ip_avatar_3d/aroll_actions.py`
- Modify: `mcp/ip_avatar_3d/test_blender_hand_refinement.py`

**Interfaces:**
- Consumes: existing `analyze_three_digit_hands(...)` regions and the canonical three-segment finger chains.
- Produces: `refine_three_digit_surface(...) -> dict[str, Any]` metadata and updated source vertices/weights without a replacement hand object.

- [ ] **Step 1: Write failing hand-shape contract tests**

Add a production fixture test that checks the rest and deformed source surface:

```python
def test_refined_hands_preserve_three_digits_and_taper_each_tip():
    armature, character, dimensions, bone_map = build_source_hand_fixture()
    report = hand_refinement.enhance_three_segment_hands(
        armature, [character], dimensions, bone_map
    )
    assert report["handAestheticVersion"] == "three_digit_refined_v2"
    for side in ("l", "r"):
        assert report["sides"][side]["digitCount"] == 3
        for digit in report["sides"][side]["digits"]:
            assert digit["tipWidth"] / digit["rootWidth"] <= 0.78
            assert digit["rootTransitionContinuity"] >= 0.82
            assert digit["wristBoundaryMaxDisplacement"] <= dimensions["width"] * 0.002


def test_refined_hand_weights_remain_normalized_and_isolated():
    report = build_refined_hand_report()
    assert report["maxInfluences"] <= 4
    assert report["unnormalizedVertices"] == 0
    assert report["unweightedVertices"] == 0
    assert report["neighborTipLeakageMax"] <= 0.08
```

- [ ] **Step 2: Run the tests to verify RED**

Run:

```bash
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --python mcp/ip_avatar_3d/test_blender_hand_refinement.py
```

Expected: fail because the aesthetic metadata and local surface shaping do not exist.

- [ ] **Step 3: Implement conservative source-surface shaping**

Add `refine_three_digit_surface` after digit-region analysis. For each digit,
derive a centerline from the proximal, middle, and distal bone chain. Pin wrist
vertices, preserve palm volume, taper only the outer 72 percent of each digit,
and apply bounded Laplacian smoothing inside each digit region. Store before and
after root width, tip width, wrist displacement, and continuity metrics. Do not
create a replacement mesh or change UV indices.

Use a bounded displacement function:

```python
def tapered_radial_scale(progress: float) -> float:
    amount = max(0.0, min(1.0, (progress - 0.28) / 0.72))
    eased = amount * amount * (3.0 - 2.0 * amount)
    return 1.0 - 0.24 * eased
```

Run the existing isolated banded weighting after shaping, normalize affected
groups, and enforce the existing four-influence cap.

- [ ] **Step 4: Tune semantic hand poses for the refined silhouette**

Keep action names and rig roles unchanged. Adjust `open_hand`, `relaxed_hand`,
`fist`, `soft_curl`, `pinch`, `point`, and count poses only where the refined
surface needs slightly reduced splay or opposition. Camera-facing wave must show
the palm and separated rounded tips without hyperextension.

- [ ] **Step 5: Run hand, action, and rig regression tests to verify GREEN**

Run:

```bash
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --python mcp/ip_avatar_3d/test_blender_hand_refinement.py
python3 mcp/ip_avatar_3d/test_aroll_actions.py -v
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --python mcp/ip_avatar_3d/test_blender_character_rig.py
```

Expected: all tests pass; no required action or rig role changes.

- [ ] **Step 6: Commit**

```bash
git add mcp/ip_avatar_3d/hand_refinement.py \
  mcp/ip_avatar_3d/test_blender_hand_refinement.py \
  mcp/ip_avatar_3d/aroll_actions.py
git commit -m "feat: refine three-digit hand surfaces"
```

### Task 3: Staged Master Build And Render QA

**Files:**
- Modify: `mcp/ip_avatar_3d/master_asset.py`
- Modify: `mcp/ip_avatar_3d/render_aroll_master_qa.py`
- Modify: `mcp/ip_avatar_3d/test_blender_master_asset.py`
- Modify: `mcp/ip_avatar_3d/test_blender_hand_refinement.py`

**Interfaces:**
- Consumes: refined oral and hand metadata from Tasks 1-2.
- Produces: staged refined master, oral/hand QA report, comparison contact sheet, and an atomic publish decision.

- [ ] **Step 1: Write a failing master capability gate**

Require the refined metadata and fail closed on legacy clusters:

```python
def test_refined_master_requires_connected_oral_and_hand_metadata():
    report = master_asset.validate_master_capabilities(build_master_fixture())
    assert report["oralRefinementVersion"] == "continuous_arch_v1"
    assert report["handAestheticVersion"] == "three_digit_refined_v2"
    assert report["legacyToothClusterCount"] == 0
    assert report["tongueConnectedComponents"] == 1
```

- [ ] **Step 2: Run the master test to verify RED**

Run:

```bash
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --python mcp/ip_avatar_3d/test_blender_master_asset.py
```

Expected: fail because the refined capability fields are not validated.

- [ ] **Step 3: Add staged build and atomic publication gates**

Extend the master report with topology versions, component counts, material
roles, weight statistics, preserved source UV/material hashes, and pose-sampled
collision findings. Build to
`ip形象/main_ip/models/staging/main-ip-aroll-master-refined.blend`; only copy to
`ip形象/main_ip/models/main-ip-aroll-master-refined.blend` after all report gates
are true. Never overwrite `main-ip-aroll-master.blend` in this task.

- [ ] **Step 4: Expand fixed QA samples**

Add face samples for Rest, MBP, A, E, O, U, Smile, and Surprise, plus hand
samples for relaxed, open, fist, pinch, count one/two/three, point, and
camera-facing wave. Render with the approved medium-close and hand QA cameras.
The report must record object visibility, dental exposure, tongue exposure,
component counts, and extrema-frame intersections.

- [ ] **Step 5: Run master and QA tests to verify GREEN**

Run:

```bash
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --python mcp/ip_avatar_3d/test_blender_master_asset.py
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --python mcp/ip_avatar_3d/test_blender_hand_refinement.py
```

Expected: all master and QA contracts pass.

- [ ] **Step 6: Build the staged refined master and render evidence**

Run the existing master builder with the canonical source FBX and refined output
path, then render QA samples into `outputs/qa/main-ip-refined/`. Generate
`outputs/MainIP_Sloth_Oral_Hand_QA.png` and
`outputs/MainIP_Sloth_Oral_Hand_Comparison.png` from fixed crops. Inspect both
images at full resolution before publication.

- [ ] **Step 7: Commit**

```bash
git add mcp/ip_avatar_3d/master_asset.py \
  mcp/ip_avatar_3d/render_aroll_master_qa.py \
  mcp/ip_avatar_3d/test_blender_master_asset.py \
  mcp/ip_avatar_3d/test_blender_hand_refinement.py
git commit -m "test: gate refined sloth master publication"
```

### Task 4: Front-Facing A-roll Production Verification

**Files:**
- Modify only if a regression is found: `mcp/ip_avatar_3d/render_front_talking_demo.py`
- Modify only if a regression is found: `mcp/ip_avatar_3d/test_front_talking_demo.py`

**Interfaces:**
- Consumes: approved refined master, warm studio, permanent IP voice, and existing front-facing demo pipeline.
- Produces: `outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p.mp4` and final media/visual QA evidence.

- [ ] **Step 1: Run the focused non-rendering regression suite**

```bash
python3 -m unittest discover -s mcp/ip_avatar_3d -p 'test_*.py' -v
go test ./service/localmcp ./gateway/api ./service/agentbridge
```

Expected: all tests pass. If a failure is caused by this refinement, add a
minimal failing regression test before changing production code.

- [ ] **Step 2: Render one 15-30 second front-facing demo**

Use the existing demo publisher with 1920x1080, 30 fps, the refined master,
warm-studio scene, and permanent IP voice. The motion plan must include neutral
speech, A/E/O/MBP visemes, camera-facing wave, open-palm explanation, count,
pinch, and a relaxed reset.

- [ ] **Step 3: Validate encoded media**

Use FFprobe to assert 1920x1080, 30 fps CFR, H.264 video, AAC 48 kHz audio,
15-30 second duration, and no missing frames. Run the existing audio sync,
loudness, motion continuity, and duplicate-frame checks.

- [ ] **Step 4: Perform render-based visual review**

Inspect the first, middle, and last frames plus all gesture and viseme extrema.
Reject bead-like dental silhouettes, exposed teeth at rest, oral intersections,
finger collapse, palm inversion, wrist seams, five-finger-like silhouettes,
black limbs, clipped hands, or camera framing that hides the tested motion.

- [ ] **Step 5: Run final regressions and commit any required demo fix**

Re-run the focused Python, Blender, Go, and media tests. If no demo-code change
was required, do not create an empty commit. If a regression fix was required:

```bash
git add mcp/ip_avatar_3d/render_front_talking_demo.py \
  mcp/ip_avatar_3d/test_front_talking_demo.py
git commit -m "fix: validate refined sloth a-roll demo"
```
