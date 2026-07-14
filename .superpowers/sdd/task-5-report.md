# Task 5 Report: Mode Placement, Framing, And Geometry QA

## Status

Complete from baseline `56f0403d`. The implementation commit is `ce61a848`
(`feat: place sloth in warm studio by presentation mode`).

## Changes

- Added strict `resolve_scene_mode_objects()` resolution for standing and seated
  spawn, focus, seat, mode-specific foot targets, and mode-specific cameras.
- Kept the legacy shared-marker path only for old standing-only authored scenes.
  Seated mode never falls back to `IP_Foot_Target.*` or another standing alias.
- Made target height, placement root, DOF focus, and camera cuts mode-aware.
- Persisted mode, marker names, camera names, placement root, and post-placement
  world bounds in scene stats.
- Applied a measured `-0.01` vertical shift to the selected medium camera so the
  sampled two-hand present gesture enters frame without replacing the authored
  camera.
- Added evaluated-mesh QA for shoe clearance, medium-frame visibility, visible
  desk/chair/hand intersections, and cross-frame deformation spikes.
- Added the recorded Minor regression: shoe sampling fails unless its Armature
  modifier targets the tested armature and has `show_render=true`.

## TDD Evidence

The initial mode-resolution Blender run failed on the real studio with:

```text
AttributeError: module 'blender_renderer' has no attribute 'resolve_scene_mode_objects'
```

The validator cycles then failed in order for the missing module, missing
`finalize_mode_report`, and missing `validate_modes`. The first real integrated
report correctly failed standing with `chairIntersectionCount=3153` and no
left-hand geometry in frame. Investigation showed those chair intersections
were lower-body triangles outside the medium frustum. Filtering to visible
character triangles reduced all sampled collision counts to zero. A two-hand
present sample plus the measured medium-camera shift moved five evaluated
vertices from each standing hand into the middle-frame boundary.

The standalone CLI initially failed because Blender `--python` did not add the
script directory to `sys.path`; adding the same explicit script-directory setup
used by the Blender tests made the unchanged command pass.

## Blender Commands

Scene contract and real canonical integration:

```bash
/Applications/Blender.app/Contents/MacOS/Blender --background \
  ip形象/main_ip/scenes/warm-sloth-studio-v1.blend \
  --python-exit-code 1 \
  --python mcp/ip_avatar_3d/test_blender_scene_contract.py
```

Standalone clearance/collision/visibility report:

```bash
/Applications/Blender.app/Contents/MacOS/Blender --background \
  ip形象/main_ip/scenes/warm-sloth-studio-v1.blend \
  --python-exit-code 1 \
  --python mcp/ip_avatar_3d/validate_warm_studio_character.py -- \
  --master ip形象/main_ip/models/main-ip-aroll-master.blend \
  --output /tmp/task-5-warm-studio-character-validation.json \
  --frames 1,15,29
```

Both commands exited `0`. The validator top-level result was `success=true`.

## Geometry Results

All values come from the actual canonical master appended into the actual warm
studio. Shoe values use Armature-deformed, foot-weighted downward sole vertices;
collisions use render-visible evaluated triangles; visibility uses evaluated
head/hand surface vertices projected through the selected medium camera.

| Mode | Frames | Left clearance m | Right clearance m | Desk | Chair | Hand | Spikes |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: |
| standing | 1/15/29 | `0.00001185 / 0.00667456 / -0.00842813` | `0.00001482 / 0.00238133 / -0.01244198` | 0 | 0 | 0 | 0 |
| seated | 1/15/29 | `0.00114307 / 0.00114308 / 0.00114297` | `0.00102375 / 0.00102376 / 0.00102376` | 0 | 0 | 0 | 0 |

The blocking floor threshold is `-0.020 m`. Standing minima were `-8.43 mm`
left and `-12.44 mm` right; seated minima were positive `1.143 mm` left and
`1.024 mm` right.

## Visibility Results

| Mode / region | Sampled points | Inside medium frame | Per-frame inside counts |
| --- | ---: | ---: | --- |
| standing head | 7506 | 6218 | `2076 / 2061 / 2081` |
| standing left hand | 1065 | 5 | `0 / 5 / 0` |
| standing right hand | 1065 | 5 | `0 / 5 / 0` |
| seated head | 7506 | 6257 | `2076 / 2102 / 2079` |
| seated left hand | 1065 | 25 | `2 / 19 / 4` |
| seated right hand | 1065 | 191 | `29 / 123 / 39` |

Standing used `Camera_Standing_Medium`; seated used `Camera_Seated_Medium`.
Resolved foot markers were `IP_Standing_Foot_Target.*` and
`IP_Seated_Foot_Target.*`, respectively.

## Verification

```text
Blender scene contract: 14 PASS, exit 0
Standalone both-mode validator: success=true, exit 0
python3 mcp/ip_avatar_3d/test_server.py: Ran 90, OK (skipped=2)
python3 -m py_compile <three Task 5 Python files>: exit 0
git diff --check: exit 0
```

## Self-Review

No blocking finding remains. The implementation commit contains only the owned
renderer, scene contract test, and new validator. Character profile, voice,
studio blend, canonical master, and Task 4 files were not changed or reverted.

## Concerns

- The collision gate follows the brief's "visible sampled geometry" wording:
  character triangles wholly outside the selected medium frustum do not count.
  Their raw off-frame lower-body/chair overlap is therefore diagnostic, not a
  publish blocker.
- Standing hands enter the medium frame only during the sampled present phase
  and have a five-vertex margin per side. A materially different action library
  or camera contract should rerun this validator rather than reuse these values.
- Maximum evaluated edge-length ratios were `4.7881` standing and `4.7876`
  seated, but no edge also exceeded the independent `0.02 m` absolute-growth
  threshold, so `deformationSpikeCount` remained zero.
- Blender 5.1 prints the existing BlenderMCP background-mode warning. It does
  not affect the test or validator exit status.
- Canonical model/turnaround assets remain untracked and were not included in
  the implementation commit.
