# Main IP High-Quality A-roll Master Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce a stable, reference-faithful sloth master asset with independently articulated three-segment digits, presenter-close face and material quality, a reusable A-roll action pack, and a pinned human-like Mandarin production voice.

**Architecture:** Character-specific geometry refinement runs once from the source FBX and saves a master Blend. Tangying's MCP loads that approved master into authored studio scenes, builds motion and viseme timelines from shared A-roll pose definitions, and fails closed when the pinned production TTS provider is unavailable. Generic IP onboarding remains conservative and never applies the sloth-specific retopology profile without explicit configuration.

**Tech Stack:** Blender 5.1 Python API and `bmesh`, Python 3.11+, FastMCP, glTF/FBX, FFmpeg/FFprobe, HeyGen Starfish or ElevenLabs multilingual TTS, `unittest`, Blender background integration tests.

---

## File Structure

- Create `mcp/ip_avatar_3d/hand_refinement.py`: character hand-region analysis, support-loop insertion, three-segment finger bones, weights, and hand articulation metrics.
- Create `mcp/ip_avatar_3d/aroll_actions.py`: shared digit-chain helpers and reusable A-roll Action pose specifications.
- Create `mcp/ip_avatar_3d/master_asset.py`: append/save master character collections and validate master metadata.
- Create `mcp/ip_avatar_3d/voice_policy.py`: pure production/preview voice routing and audition manifest logic.
- Create `mcp/ip_avatar_3d/render_aroll_master_qa.py`: deterministic hand, face, medium-close, and full-body QA rendering.
- Create `mcp/ip_avatar_3d/test_blender_hand_refinement.py`: focused Blender integration tests for topology, bones, weights, controls, and independent digit movement.
- Create `mcp/ip_avatar_3d/test_voice_policy.py`: pure-Python voice policy tests.
- Modify `mcp/ip_avatar_3d/rig_semantics.py`: middle-segment role aliases.
- Modify `mcp/ip_avatar_3d/blender_renderer.py`: delegate hand/action/master operations, refine eye regions, load masters, and emit richer reports.
- Modify `mcp/ip_avatar_3d/server.py`: master preparation tool, master-aware render input, production voice policy, and audition tool.
- Modify `mcp/ip_avatar_3d/test_server.py`: MCP contract, profile, production-failure, and audition tests.
- Modify `mcp/ip_avatar_3d/test_blender_character_rig.py`: existing character expectations move from two to three finger segments and include eyelid/material checks.
- Modify `ip形象/main_ip/character-profile.json`: master path, three-segment requirements, A-roll quality tier, production voice policy, and candidate voice IDs.
- Modify `mcp/ip_avatar_3d/README.md` and `docs/mcp-providers.md`: document the approved master workflow and fail-closed production voice policy.

## Task 1: Extend Finger Semantics To Three Segments

**Files:**
- Modify: `mcp/ip_avatar_3d/rig_semantics.py`
- Modify: `mcp/ip_avatar_3d/test_server.py`

- [ ] **Step 1: Write the failing semantic test**

Add to `IPAvatar3DMCPTests`:

```python
def test_resolves_three_segment_three_digit_hand_roles(self) -> None:
    semantics = load_rig_semantics()
    names = [
        f"Finger_{digit:02d}_{segment}.{side}"
        for side in ("L", "R")
        for digit in (1, 2, 3)
        for segment in ("Proximal", "Middle", "Distal")
    ]

    mapping = semantics.resolve_bone_roles(names)

    for side in ("l", "r"):
        for digit in (1, 2, 3):
            assert mapping[f"finger_{digit}_{side}"] == f"Finger_{digit:02d}_Proximal.{side.upper()}"
            assert mapping[f"finger_{digit}_mid_{side}"] == f"Finger_{digit:02d}_Middle.{side.upper()}"
            assert mapping[f"finger_{digit}_tip_{side}"] == f"Finger_{digit:02d}_Distal.{side.upper()}"
```

- [ ] **Step 2: Run the test and verify RED**

Run:

```bash
python3 -m unittest mcp.ip_avatar_3d.test_server.IPAvatar3DMCPTests.test_resolves_three_segment_three_digit_hand_roles
```

Expected: FAIL because `finger_*_mid_*` roles are absent.

- [ ] **Step 3: Add middle-segment aliases**

Extend the role table in `rig_semantics.py` so each digit resolves all three chains. Keep existing proximal and tip role names backward-compatible:

```python
for side, side_words in (("l", ("left", "l")), ("r", ("right", "r"))):
    for digit in (1, 2, 3):
        ROLE_ALIASES[f"finger_{digit}_mid_{side}"] = (
            f"finger{digit}middle{side}",
            f"finger{digit:02d}middle{side}",
            f"finger_{digit:02d}_middle.{side}",
            *(f"{word}finger{digit}middle" for word in side_words),
        )
```

Normalize punctuation/case through the existing name-normalization path; do not special-case exact source names outside the alias table.

- [ ] **Step 4: Run semantic and MCP tests**

Run:

```bash
python3 mcp/ip_avatar_3d/test_server.py
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add mcp/ip_avatar_3d/rig_semantics.py mcp/ip_avatar_3d/test_server.py
git commit -m "feat: resolve three-segment IP fingers"
```

## Task 2: Add Focused Hand-Refinement Tests

**Files:**
- Create: `mcp/ip_avatar_3d/test_blender_hand_refinement.py`
- Modify: `mcp/ip_avatar_3d/test_blender_character_rig.py`

- [ ] **Step 1: Create the focused Blender test harness**

Reuse `load_enhanced_fbx_character()` from `test_blender_character_rig.py` and add helpers:

```python
def digit_roles(side: str, digit: int) -> tuple[str, str, str]:
    return (
        f"finger_{digit}_{side}",
        f"finger_{digit}_mid_{side}",
        f"finger_{digit}_tip_{side}",
    )


def world_tip(armature, bone_map, side: str, digit: int):
    distal = armature.pose.bones[bone_map[f"finger_{digit}_tip_{side}"]]
    return armature.matrix_world @ distal.tail
```

Add tests that require:

```python
def test_main_ip_has_three_segments_per_digit_and_clean_weights():
    objects, _, armature, stats, bone_map, _ = load_enhanced_fbx_character()
    assert stats["fingerBoneCount"] == 18
    assert stats["fingerSegmentCount"] == 3
    assert stats["handJointSupportLoopCount"] >= 24
    assert stats["maxVertexInfluences"] <= 4
    assert stats["unweightedVertexCount"] == 0
    for side in ("l", "r"):
        for digit in (1, 2, 3):
            assert all(role in bone_map for role in digit_roles(side, digit))


def test_each_digit_moves_independently_and_fist_closes():
    objects, _, armature, stats, bone_map, _ = load_enhanced_fbx_character()
    open_tips = sample_open_tips(armature, bone_map)
    fist_tips = sample_fist_tips(armature, bone_map)
    for side in ("l", "r"):
        for digit in (1, 2, 3):
            rest_length = digit_chain_length(armature, bone_map, side, digit)
            assert (fist_tips[side, digit] - open_tips[side, digit]).length >= rest_length * 0.25

    for selected in (1, 2, 3):
        before, after = sample_single_digit_curl(armature, bone_map, "r", selected)
        assert (after[selected] - before[selected]).length >= digit_chain_length(
            armature, bone_map, "r", selected
        ) * 0.18
        for other in {1, 2, 3} - {selected}:
            assert (after[other] - before[other]).length <= digit_chain_length(
                armature, bone_map, "r", other
            ) * 0.06
```

- [ ] **Step 2: Update the old two-segment expectation**

Rename `test_rigged_fbx_gains_two_segment_three_digit_hands_with_valid_weights` to `test_rigged_fbx_gains_three_segment_three_digit_hands_with_valid_weights`. Expect `Proximal`, `Middle`, and `Distal` names and 18 added bones.

- [ ] **Step 3: Run the focused tests and verify RED**

Run:

```bash
/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_hand_refinement.py
```

Expected: FAIL on missing middle roles, 12-vs-18 bones, support-loop count, and closure displacement.

- [ ] **Step 4: Commit the RED tests**

```bash
git add mcp/ip_avatar_3d/test_blender_hand_refinement.py mcp/ip_avatar_3d/test_blender_character_rig.py
git commit -m "test: define close-shot hand articulation contract"
```

## Task 3: Implement Character-Specific Local Hand Refinement

**Files:**
- Create: `mcp/ip_avatar_3d/hand_refinement.py`
- Modify: `mcp/ip_avatar_3d/blender_renderer.py`

- [ ] **Step 1: Move hand analysis into a focused module**

Define immutable analysis records:

```python
@dataclass(frozen=True)
class HandVertexRecord:
    object_name: str
    vertex_index: int
    world: Vector
    projection: float
    hand_weight: float


@dataclass
class DigitRegion:
    side: str
    index: int
    records: list[HandVertexRecord]
    axis: Vector
    base: Vector
    joint_1: Vector
    joint_2: Vector
    tip: Vector
    minimum: float
    maximum: float
    feature_center: Vector
```

Port the current clustering behavior into `analyze_three_digit_hands(...)`, preserving the lower-digit ordering rule. Replace tuple dictionaries with these records.

- [ ] **Step 2: Insert support cuts around both knuckles**

Before face Shape Keys exist, use `bmesh` to cut only faces whose vertices belong predominantly to one digit. For each digit axis, bisect at two knuckles plus support offsets:

```python
JOINT_PROGRESS = (0.34, 0.68)
SUPPORT_OFFSET = 0.045

for progress in JOINT_PROGRESS:
    for offset in (-SUPPORT_OFFSET, 0.0, SUPPORT_OFFSET):
        cut_point = region.base.lerp(region.tip, progress + offset)
        bmesh.ops.bisect_plane(
            bm,
            geom=eligible_geom,
            plane_co=object_to_local @ cut_point,
            plane_no=(object_to_local.to_3x3() @ region.axis).normalized(),
            clear_inner=False,
            clear_outer=False,
        )
```

Run a low-strength local smooth pass on new hand vertices while pinning wrist-boundary vertices and the outermost fingertip band:

```python
bmesh.ops.smooth_vert(
    bm,
    verts=smoothable,
    factor=0.12,
    use_axis_x=True,
    use_axis_y=True,
    use_axis_z=True,
)
```

Record cut counts and before/after vertex counts in the rig report.

- [ ] **Step 3: Create three deform segments per digit**

Create bones at `base -> joint_1 -> joint_2 -> tip`:

```python
segment_names = (
    f"Finger_{index:02d}_Proximal.{side}",
    f"Finger_{index:02d}_Middle.{side}",
    f"Finger_{index:02d}_Distal.{side}",
)
points = (region.base, region.joint_1, region.joint_2, region.tip)
for segment_index, name in enumerate(segment_names):
    bone = edit_bones.new(name)
    bone.head = world_to_armature @ points[segment_index]
    bone.tail = world_to_armature @ points[segment_index + 1]
    bone.parent = parent
    bone.use_connect = segment_index > 0
    bone.use_deform = True
    parent = bone
```

The proximal parent is the source wrist/hand bone. Middle and distal segments connect to their predecessor.

- [ ] **Step 4: Assign narrow three-segment weights**

Use smooth triangular membership centered at progress `0.22`, `0.54`, and `0.84`. Keep a palm share near the base:

```python
centers = (0.22, 0.54, 0.84)
widths = (0.34, 0.30, 0.28)
raw = [max(0.0, 1.0 - abs(progress - center) / width) ** 2 for center, width in zip(centers, widths)]
segment_total = hand_weight * smoothstep(0.08, 0.38, progress)
weights = normalize(raw, total=segment_total * digit_membership)
hand_group_weight = max(0.0, hand_weight - sum(weights))
```

Run the existing four-influence limiter and normalized-weight collector after all hands are assigned. Keep Armature preserve-volume enabled.

- [ ] **Step 5: Delegate from the renderer**

Replace `_cluster_hand_vertices`, `_subdivide_weighted_hand_regions`, and the finger-specific body of `enhance_existing_presenter_rig` with:

```python
stats, bone_map = enhance_three_segment_hands(
    armature=armature,
    objects=objects,
    dimensions=dimensions,
    stats=stats,
    resolve_roles=resolve_bone_roles,
    maximum_influences=4,
)
```

Keep the generated-cartoon-rig path compatible; only the curated existing-rig path receives the sloth close-shot refinement profile.

- [ ] **Step 6: Run the focused Blender tests**

Run:

```bash
/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_hand_refinement.py
```

Expected: bone/topology/weight tests PASS; movement tests may remain RED until Task 4.

- [ ] **Step 7: Commit**

```bash
git add mcp/ip_avatar_3d/hand_refinement.py mcp/ip_avatar_3d/blender_renderer.py
git commit -m "feat: refine main IP hand topology and bones"
```

## Task 4: Implement Independent Digit Poses And Correct Closure

**Files:**
- Create: `mcp/ip_avatar_3d/aroll_actions.py`
- Modify: `mcp/ip_avatar_3d/blender_renderer.py`
- Modify: `mcp/ip_avatar_3d/test_blender_hand_refinement.py`

- [ ] **Step 1: Add failing pose tests**

Test open, fist, and each isolated digit at representative frames. Require three distinct segment rotations:

```python
assert proximal.z > 0.34
assert middle.z > 0.42
assert distal.z > 0.28
```

For open hand, require curls below `0.04` and outer-digit splay signs to differ. For the lower digit, require non-zero opposition during pinch.

- [ ] **Step 2: Verify RED**

Run the focused Blender test command. Expected: FAIL because current distal scaling is `0.32`, no middle segment exists in pose helpers, and maximum curl is `0.34`.

- [ ] **Step 3: Add shared chain helpers**

Create explicit semantic rotations so Action creation and procedural timelines cannot drift:

```python
@dataclass(frozen=True)
class DigitPose:
    proximal: float
    middle: float
    distal: float
    splay: float = 0.0
    opposition: float = 0.0


OPEN = DigitPose(0.015, 0.01, 0.005)
RELAXED = DigitPose(0.10, 0.12, 0.06)
FIST = DigitPose(0.38, 0.48, 0.32)


def chain_eulers(side: str, pose: DigitPose) -> dict[str, tuple[float, float, float]]:
    sign = 1.0 if side == "r" else -1.0
    return {
        "proximal": (pose.splay, pose.opposition, sign * pose.proximal),
        "middle": (pose.splay * 0.22, 0.0, sign * pose.middle),
        "distal": (pose.splay * 0.08, 0.0, sign * pose.distal),
    }
```

- [ ] **Step 4: Define readable hand poses**

Provide `open_hand`, `relaxed_hand`, `fist`, `pinch`, `count_one`, `count_two`, `count_three`, `point`, and `finger_roll` dictionaries. Use the lower digit's `opposition` only in grip/pinch poses. Keep all values below tested deformation limits.

- [ ] **Step 5: Key all three segments explicitly**

Update `create_action_library` and `animate` to set proximal, middle, and distal roles directly. Remove automatic distal mirroring for three-segment rigs; retain it only for legacy two-segment models.

- [ ] **Step 6: Run focused and existing Blender tests**

Run:

```bash
/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_hand_refinement.py
/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_character_rig.py
```

Expected: all hand movement, existing rig, and generated-rig tests PASS.

- [ ] **Step 7: Commit**

```bash
git add mcp/ip_avatar_3d/aroll_actions.py mcp/ip_avatar_3d/blender_renderer.py mcp/ip_avatar_3d/test_blender_hand_refinement.py
git commit -m "feat: add independent three-segment hand poses"
```

## Task 5: Build The A-roll Action Pack And Planner Mapping

**Files:**
- Modify: `mcp/ip_avatar_3d/aroll_actions.py`
- Modify: `mcp/ip_avatar_3d/blender_renderer.py`
- Modify: `mcp/ip_avatar_3d/server.py`
- Modify: `mcp/ip_avatar_3d/test_server.py`
- Modify: `mcp/ip_avatar_3d/test_blender_hand_refinement.py`

- [ ] **Step 1: Write failing Action-name tests**

Require all confirmed names:

```python
AROLL_ACTIONS = {
    "Aroll_Idle_Listening", "Aroll_Greeting_Wave", "Aroll_OpenPalm_Explain",
    "Aroll_Explain_Left", "Aroll_Explain_Right", "Aroll_Count_One",
    "Aroll_Count_Two", "Aroll_Count_Three", "Aroll_Point_Left",
    "Aroll_Point_Right", "Aroll_Pinch_Detail", "Aroll_Emphasis_SoftFist",
    "Aroll_Think", "Aroll_Agree_Nod", "Aroll_Disagree_Shake",
    "Aroll_Transition_Reset",
}
assert AROLL_ACTIONS.issubset(set(report["actions"]))
```

Also test that `build_motion_plan` maps greeting, enumeration, detail, explanation, agreement, disagreement, and emphasis keywords to compatible motion events without overlapping conflicting right-hand gestures.

- [ ] **Step 2: Verify RED**

Run MCP and focused Blender tests. Expected: missing A-roll Actions and planner events.

- [ ] **Step 3: Define Actions from shared poses**

Add `build_aroll_action_specs(source_rig: bool, fps: int)` in `aroll_actions.py`. Each spec contains reset, anticipation, readable hold, and release keyframes. For example:

```python
"Aroll_Count_One": [
    (1, RESET),
    (12, count_pose(1, anticipation=True)),
    (24, count_pose(1)),
    (48, count_pose(1)),
    (60, RESET),
]
```

Keep the hand outside the face-safe box and use a chest-height arm stage for count/pinch actions.

- [ ] **Step 4: Add planner keyword mapping and conflict groups**

Add semantic event names and a `gestureGroup` field (`right_hand`, `left_hand`, `head`, `body`). Coalesce or time-shift events that overlap in the same group. Greeting defaults to `Aroll_Greeting_Wave`; enumerations map to count Actions; small-detail language maps to pinch; conclusions map to soft-fist emphasis.

- [ ] **Step 5: Run MCP and Blender tests**

```bash
python3 mcp/ip_avatar_3d/test_server.py
/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_hand_refinement.py
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add mcp/ip_avatar_3d/aroll_actions.py mcp/ip_avatar_3d/blender_renderer.py mcp/ip_avatar_3d/server.py mcp/ip_avatar_3d/test_server.py mcp/ip_avatar_3d/test_blender_hand_refinement.py
git commit -m "feat: add reusable IP A-roll action pack"
```

## Task 6: Refine Eyelids, Mouth Corners, And Existing PBR Materials

**Files:**
- Modify: `mcp/ip_avatar_3d/blender_renderer.py`
- Modify: `mcp/ip_avatar_3d/test_blender_character_rig.py`

- [ ] **Step 1: Add failing face and material tests**

Require:

```python
assert face_mesh["true_eyelid_topology"] is True
assert face_mesh["eyelid_loop_vertex_count_l"] >= 96
assert face_mesh["eyelid_loop_vertex_count_r"] >= 96
assert blink_contact_gap(face_mesh, "L") <= dimensions["height"] * 0.002
assert blink_contact_gap(face_mesh, "R") <= dimensions["height"] * 0.002
assert source_material.node_tree.nodes.get("Normal Map") is not None
assert source_material.node_tree.nodes.get("Image Texture - Roughness") is not None
```

Add mouth-corner stability assertions comparing `Mouth_Smile`, `Mouth_E`, and `Mouth_MBP` against a maximum lateral displacement based on head width.

- [ ] **Step 2: Verify RED**

Run the character Blender test. Expected: eyelid topology and blink-contact assertions FAIL.

- [ ] **Step 3: Increase local eye-region sampling before Shape Keys**

Use the resolved `Eye.L`/`Eye.R` weighted regions while the source mesh has no Shape Keys. Subdivide region edges once, preserve UV custom data, and keep the eye-region boundary fixed. Classify upper/lower lid vertices by eye center and store counts in object properties.

- [ ] **Step 4: Rebuild blink shapes from upper/lower lid sets**

Move upper and lower lids toward a shared curved contact line with a stronger upper-lid contribution. Do not scale the eye object or entire head region. Keep left/right blinks independent.

- [ ] **Step 5: Restrain mouth-corner shapes**

Clamp corner displacement and blend its falloff into cheek vertices. Preserve jaw-driven vertical opening and oral interior visibility.

- [ ] **Step 6: Tune, do not replace, source PBR maps**

The source already contains 4K base color, metallic, normal, and roughness textures. Verify correct non-color color-space settings for metallic/normal/roughness, set normal strength to a restrained close-shot value, and tune Principled roughness/specular without adding global procedural bump that would damage eyes or fabric.

- [ ] **Step 7: Run Blender tests**

```bash
/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_character_rig.py
/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_scene_contract.py
```

Expected: all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add mcp/ip_avatar_3d/blender_renderer.py mcp/ip_avatar_3d/test_blender_character_rig.py
git commit -m "feat: refine close-shot face and PBR detail"
```

## Task 7: Add Stable Master-Blend Preparation And Loading

**Files:**
- Create: `mcp/ip_avatar_3d/master_asset.py`
- Modify: `mcp/ip_avatar_3d/blender_renderer.py`
- Modify: `mcp/ip_avatar_3d/server.py`
- Modify: `mcp/ip_avatar_3d/test_server.py`
- Modify: `ip形象/main_ip/character-profile.json`

- [ ] **Step 1: Write failing profile and dry-run tests**

Require `model.masterBlendPath`, `qualityTier="aroll_close"`, `fingerTopology="three_digits_three_segments"`, and all middle-segment bone roles. Verify a render dry run writes `masterBlendPath` into `render_input.json` and does not request source retopology when the approved master exists.

- [ ] **Step 2: Verify RED**

Run `python3 mcp/ip_avatar_3d/test_server.py`. Expected: profile/master input assertions FAIL.

- [ ] **Step 3: Implement master collection helpers**

Create constants and helpers:

```python
MASTER_COLLECTION = "IP_Character_Master"


def save_master_collection(*, character_objects, armature, output_path: Path) -> dict:
    collection = ensure_master_collection(character_objects, armature)
    collection["ip_aroll_master_version"] = 1
    bpy.ops.wm.save_as_mainfile(filepath=str(output_path))
    return {"collection": collection.name, "path": str(output_path)}


def append_master_collection(master_path: Path, scene_collection) -> list[bpy.types.Object]:
    with bpy.data.libraries.load(str(master_path), link=False) as (source, target):
        if MASTER_COLLECTION not in source.collections:
            raise RuntimeError(f"missing {MASTER_COLLECTION} in {master_path}")
        target.collections = [MASTER_COLLECTION]
    scene_collection.children.link(target.collections[0])
    return list(target.collections[0].all_objects)
```

Reject duplicate Armatures and missing master metadata.

- [ ] **Step 4: Add the MCP preparation tool**

Add:

```python
@mcp.tool()
def prepare_character_master(
    sourceModel: str,
    characterProfilePath: str,
    outputDir: str = "",
    qualityTier: str = "aroll_close",
    dryRun: bool = False,
) -> dict[str, Any]:
    ...
```

Reuse the Blender renderer's asset-only path, but write the approved master, GLB, rig report, and QA input under deterministic output names.

- [ ] **Step 5: Make video rendering master-aware**

When `masterBlendPath` is configured and exists, pass it into Blender and append the master after opening the authored studio. Skip source FBX import, hand refinement, and face retopology. Preserve existing Actions, Shape Keys, materials, and Armature.

- [ ] **Step 6: Update the character profile**

Add:

```json
{
  "model": {
    "masterBlendPath": "models/main-ip-aroll-master.blend",
    "qualityTier": "aroll_close",
    "fingerTopology": "three_digits_three_segments"
  }
}
```

Add all six `finger_*_mid_*` roles to `requiredBoneRoles`.

- [ ] **Step 7: Run MCP tests**

```bash
python3 mcp/ip_avatar_3d/test_server.py
```

Expected: all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add mcp/ip_avatar_3d/master_asset.py mcp/ip_avatar_3d/blender_renderer.py mcp/ip_avatar_3d/server.py mcp/ip_avatar_3d/test_server.py ip形象/main_ip/character-profile.json
git commit -m "feat: load stable A-roll character masters"
```

## Task 8: Enforce Human-Like Production Voice Policy

**Files:**
- Create: `mcp/ip_avatar_3d/voice_policy.py`
- Create: `mcp/ip_avatar_3d/test_voice_policy.py`
- Modify: `mcp/ip_avatar_3d/server.py`
- Modify: `mcp/ip_avatar_3d/test_server.py`
- Modify: `ip形象/main_ip/character-profile.json`

- [ ] **Step 1: Write failing fail-closed tests**

Test these policies:

```python
import unittest


class VoicePolicyTests(unittest.TestCase):
    def test_production_rejects_preview_voice(self):
        with self.assertRaises(ProductionVoiceUnavailable):
            resolve_voice(
                mode="production",
                provider="apple",
                voice_id="Eddy (中文（中国大陆）)",
                fallback_policy="error",
            )

    def test_production_pins_provider_and_voice(self):
        resolved = resolve_voice(
            mode="production",
            provider="heygen",
            voice_id="dMkR1XwIkarpNqWUJLnX",
            fallback_policy="error",
        )
        self.assertEqual(resolved.provider, "heygen")
        self.assertEqual(resolved.voice_id, "dMkR1XwIkarpNqWUJLnX")
        self.assertFalse(resolved.allow_preview_fallback)
```

- [ ] **Step 2: Verify RED**

```bash
python3 mcp/ip_avatar_3d/test_voice_policy.py
```

Expected: module/import failure.

- [ ] **Step 3: Implement pure policy resolution**

Define:

```python
PRODUCTION_PROVIDERS = frozenset({"heygen", "elevenlabs"})
PREVIEW_PROVIDERS = frozenset({"apple", "kokoro"})

@dataclass(frozen=True)
class ResolvedVoice:
    provider: str
    voice_id: str
    language: str
    speed: float
    production_ready: bool
    allow_preview_fallback: bool
```

Production mode requires a pinned provider and voice ID, rejects preview providers, and uses `fallbackPolicy="error"`. Preview mode may use Apple/Kokoro and must return `production_ready=False`.

- [ ] **Step 4: Apply policy before `ensure_audio`**

Add `renderMode="production" | "preview"` to the MCP tool. Resolve the voice before calling the shared audio engine. Keep the existing provider-specific synthesis path; remove the incorrect assumption that Apple mastering makes the source human-like.

- [ ] **Step 5: Update profile policy without prematurely pinning an audition winner**

Set:

```json
{
  "voice": {
    "renderMode": "production",
    "provider": "heygen",
    "voiceId": "",
    "fallbackPolicy": "error",
    "preview": {
      "provider": "apple",
      "voiceId": "Eddy (中文（中国大陆）)"
    },
    "productionVoiceCandidates": [
      "dMkR1XwIkarpNqWUJLnX",
      "046dacc3502347eea0c796f97399632e",
      "5c1ade5e514c4c6c900b0ded224970fd"
    ]
  }
}
```

An empty production `voiceId` blocks production renders until Task 9 selects a winner.

- [ ] **Step 6: Run tests**

```bash
python3 mcp/ip_avatar_3d/test_voice_policy.py
python3 mcp/ip_avatar_3d/test_server.py
```

Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add mcp/ip_avatar_3d/voice_policy.py mcp/ip_avatar_3d/test_voice_policy.py mcp/ip_avatar_3d/server.py mcp/ip_avatar_3d/test_server.py ip形象/main_ip/character-profile.json
git commit -m "feat: fail closed on non-production IP voices"
```

## Task 9: Add Voice Auditions And Pin The Winner

**Files:**
- Modify: `mcp/ip_avatar_3d/server.py`
- Modify: `mcp/ip_avatar_3d/test_server.py`
- Modify: `ip形象/main_ip/character-profile.json`
- Create during execution: `outputs/MainIP_Sloth_Voice_Auditions/manifest.json`

- [ ] **Step 1: Write a mocked audition test**

Patch `ensure_audio` and verify one normalized file per candidate plus a label-neutral manifest:

```python
result = server.generate_voice_auditions(
    script="今天我们不追热点，只讲清楚一个真正重要的变化。",
    characterProfilePath=str(profile),
    outputDir=str(root / "auditions"),
)
assert [item["label"] for item in result["candidates"]] == ["A", "B", "C"]
assert all("voiceId" not in item for item in result["publicCandidates"])
assert result["requiresUserSelection"] is True
```

- [ ] **Step 2: Verify RED**

Run the single MCP test. Expected: missing tool/function.

- [ ] **Step 3: Implement the audition tool**

Add a FastMCP tool that iterates the profile's candidate IDs, calls the shared audio engine with explicit `provider="heygen"`, then normalizes every candidate to the same target:

```text
loudnorm=I=-16:TP=-1.5:LRA=7
```

Write private provider IDs to `manifest.json` and return label-only paths for blind review.

- [ ] **Step 4: Run the mocked test**

```bash
python3 mcp/ip_avatar_3d/test_server.py
```

Expected: all tests PASS.

- [ ] **Step 5: Run provider preflight and generate real auditions**

Run:

```bash
npx hyperframes auth status
node ~/.agents/skills/hyperframes-media/scripts/heygen-tts.mjs --list
```

Only continue when the shared credential and API are reachable. Then call `generate_voice_auditions` with the fixed audition script. If provider connectivity still times out, mark the voice deliverable blocked and do not generate Apple/Kokoro substitutes.

- [ ] **Step 6: Present blind A/B/C audio and pin the selected voice**

After user selection, write the winning HeyGen `voiceId` into `character-profile.json`, retain the warm-male persona, and keep `fallbackPolicy="error"`.

- [ ] **Step 7: Verify final voice metadata**

Generate a short production audio file and assert provider `heygen`, the pinned ID, approximately `-16 LUFS`, and true peak no higher than `-1.5 dBTP`.

- [ ] **Step 8: Commit**

```bash
git add mcp/ip_avatar_3d/server.py mcp/ip_avatar_3d/test_server.py ip形象/main_ip/character-profile.json
git commit -m "feat: pin the approved human-like IP voice"
```

## Task 10: Build Deterministic A-roll QA And Pre-Shoot Reels

**Files:**
- Create: `mcp/ip_avatar_3d/render_aroll_master_qa.py`
- Modify: `mcp/ip_avatar_3d/test_blender_hand_refinement.py`

- [ ] **Step 1: Add QA-contract tests**

Require the QA script's sample list to include every confirmed Action, both `Camera_Medium` and `Camera_Wide`, and stable hand-close framing. Add pixel-difference assertions for open-vs-fist and each Finger Roll phase.

- [ ] **Step 2: Implement the QA renderer**

The script accepts:

```text
blender master.blend --python render_aroll_master_qa.py -- output_dir
```

It renders:

- `hand/open.png`, `hand/fist.png`, `hand/pinch.png`, `hand/count_1.png`, `hand/count_2.png`, `hand/count_3.png`.
- Three isolated right-digit curls and three isolated left-digit curls.
- Neutral, happy, serious, blink, `A`, `E`, `O`, and `MBP` face crops.
- A medium-close and full-body frame for every A-roll Action.
- A JSON report containing bone rotations, fingertip displacement ratios, camera, Action, and output path.

- [ ] **Step 3: Add silhouette and duplicate-frame checks**

Use FFmpeg/framemd5 for adjacent duplicates and a deterministic alpha-mask crop comparison for open/fist. Fail QA if the open/fist hand-crop difference is below the approved threshold or any required file is missing.

- [ ] **Step 4: Run the QA script at low resolution first**

```bash
/Applications/Blender.app/Contents/MacOS/Blender -b outputs/MainIP_Sloth_Aroll_Master.blend --python mcp/ip_avatar_3d/render_aroll_master_qa.py -- tmp/ip_avatar_3d/aroll_master_qa
```

Expected: report status `ready`, all pose metrics pass, and no malformed hand/face crop.

- [ ] **Step 5: Render production reels**

Render and encode:

- `outputs/MainIP_Sloth_Aroll_Action_Reel_2K.mp4`
- `outputs/MainIP_Sloth_Aroll_Hand_QA_2K.mp4`
- `outputs/MainIP_Sloth_Aroll_Face_QA.png`

Use 2560x1440, 30fps, AgX, authored editorial studio lighting, H.264 CRF 18 or better, and the approved voice for the action reel.

- [ ] **Step 6: Commit QA code**

```bash
git add mcp/ip_avatar_3d/render_aroll_master_qa.py mcp/ip_avatar_3d/test_blender_hand_refinement.py
git commit -m "test: add A-roll master visual QA"
```

## Task 11: Build And Validate The Final Master Outputs

**Files:**
- Modify: `mcp/ip_avatar_3d/README.md`
- Modify: `docs/mcp-providers.md`
- Generated: `outputs/MainIP_Sloth_Aroll_Master.blend`
- Generated: `outputs/MainIP_Sloth_Aroll_Rigged.glb`
- Generated: `outputs/MainIP_Sloth_Aroll_Rig_Report.json`

- [ ] **Step 1: Run the complete source test suite**

```bash
python3 mcp/ip_avatar_3d/test_server.py
python3 mcp/ip_avatar_3d/test_voice_policy.py
/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_hand_refinement.py
/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_character_rig.py
/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_scene_contract.py
go test ./...
```

Run `go test ./...` from `local-backend/`. Expected: all commands PASS.

- [ ] **Step 2: Prepare the production master**

Call `prepare_character_master` with the source FBX, main profile, and `qualityTier="aroll_close"`. Save deterministic output paths from the design specification.

- [ ] **Step 3: Validate the delivery Blend directly**

Open `outputs/MainIP_Sloth_Aroll_Master.blend` in background mode and assert:

- one master Armature,
- 18 finger deform bones,
- every middle-segment semantic role,
- zero unweighted vertices,
- at most four influences,
- all required A-roll Actions,
- all required mouth/eye Shape Keys,
- `trueLipTopology=true`,
- `trueEyelidTopology=true`,
- `IP_Character_Master` collection metadata.

- [ ] **Step 4: Validate GLB reimport**

Run `validate_character_asset` against `outputs/MainIP_Sloth_Aroll_Rigged.glb`. Require `readyForTalkingVideo=true`, 18 finger bones, no missing required roles/Shape Keys, and all exported animations.

- [ ] **Step 5: Verify video and audio artifacts**

Use FFprobe to require 2560x1440, 30fps constant frame rate, expected frame count, and AAC 48kHz audio. Use framemd5 to require zero adjacent exact duplicate frames. Use `loudnorm` analysis to verify the voice target.

- [ ] **Step 6: Review contact sheets against the reference**

Inspect hand, face, medium-close, and full-body contact sheets. Reject the master if the hand looks like a blob, a neighboring digit visibly follows an isolated curl, the fist does not close, eyelids intersect eyes, mouth corners tear, or the face materially drifts from `front.png`.

- [ ] **Step 7: Document the final MCP workflow**

Document:

- one-time `prepare_character_master`,
- profile `masterBlendPath`,
- normal `render_talking_video` usage,
- production-vs-preview voice policy,
- generated reports and QA artifacts,
- conservative behavior for unknown IP topology.

- [ ] **Step 8: Commit documentation and profile state**

```bash
git add mcp/ip_avatar_3d/README.md docs/mcp-providers.md ip形象/main_ip/character-profile.json
git commit -m "docs: publish the main IP A-roll master workflow"
```

## Task 12: Final Regression Render From A Real Script

**Files:**
- Generated: `outputs/MainIP_Sloth_Aroll_Release_Test_2K.mp4`
- Generated: `outputs/MainIP_Sloth_Aroll_Release_Report.json`

- [ ] **Step 1: Use a representative knowledge-sharing script**

Use a 20-30 second Mandarin script containing greeting, enumeration, explanation, contrast, and conclusion so the planner exercises wave, count, open-palm, point, emphasis, nod, gaze, blink, and visemes.

- [ ] **Step 2: Render through the MCP tool, not a private Blender shortcut**

Call `render_talking_video` with the main profile and editorial studio. Require the tool to load `masterBlendPath`, use the pinned production voice, and emit motion/voice/render provenance.

- [ ] **Step 3: Run release QA**

Verify:

- video technical contract,
- no duplicate frames,
- no gesture conflicts,
- no hand/body/face intersections,
- readable independent digits in count/pinch shots,
- natural mouth/voice timing,
- reference-faithful appearance under medium-close and full-body cuts.

- [ ] **Step 4: Keep the previous approved master on any failure**

Only replace the profile's approved `masterBlendPath` and release video after every automated and visual gate passes. Otherwise retain the prior master and report the failing pose/provider explicitly.
