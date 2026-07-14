# Task 4 Report: Real Seated Base Pose

## Status

Complete. The renderer now layers a calibrated seated lower-body/root pose
under the existing speech timeline, preserves head, mouth, wrist, and finger
animation, and suppresses seated-incompatible bounce, step, and standing
weight-shift motion.

## Changes

- Added `presentation_pose(mode, source_rig)` and `Aroll_Seated_Idle`.
- Added five-phase seated action keyframes using the shared A-roll protocol.
- Passed normalized `presentationMode` into `animate()`.
- Applied the base pose on every procedural frame before speech motion.
- Converted semantic world-space root offsets into source-rig pose-local
  coordinates, accounting for the imported armature scale and root axes.
- Disabled idle leg drift plus `happy_bounce`, `leg_step`, and `weight_shift`
  body/leg contributions while seated.
- Kept head, jaw/viseme, wrist, and three-segment digit paths unchanged.
- Updated character-rig and hand/QA regressions for the seventeenth A-roll
  action and its intentional nonzero lower-body channels.

## TDD Evidence

Initial pure RED:

```text
AttributeError: module 'ip_avatar_3d_aroll_actions' has no attribute 'presentation_pose'
Ran 90 tests; FAILED (errors=1, skipped=2)
```

Initial Blender RED on the real source FBX:

```text
TypeError: animate() got an unexpected keyword argument 'presentation_mode'
```

The brief's initial Euler values then failed real world-space calibration. The
source root's local Z translation lowered the pelvis by only about 0.00027 m,
and equal left/right leg Z rotations sent the knees in opposite world-space
directions. The imported armature scale was 0.0137426 and root local Y was the
axis closest to world Z.

The first automatically green pose still looked too close to standing in the
front render. A stricter visual RED required pelvis drop above 0.30 m, knee
angles below 105 degrees, pelvis-to-knee height below 0.16 m, and mirrored
world-space knee/foot positions. It failed the old pose at 0.230 m pelvis drop
and approximately 111.6-degree knees.

The hand action-pack RED then caught the initial two-key seated action:

```text
AssertionError: ('Aroll_Seated_Idle', [1, 60])
```

GREEN uses the standard `[1, 12, 24, 46, 60]` five-phase frames.

## Final Calibration

Semantic root location is a world-space offset; rotations are pose-local XYZ.

| Role | Source rig | Generated rig |
| --- | --- | --- |
| root location | `(0.0, 0.12, -0.34)` | `(0.0, 0.12, -0.34)` |
| body | `(0.08, 0.0, 0.0)` | `(0.08, 0.0, 0.0)` |
| leg L | `(0.0, 0.0, 1.35)` | `(1.02, 0.0, 0.0)` |
| leg R | `(0.05, 0.197, -1.30)` | `(1.02, 0.0, 0.0)` |
| shin L | `(0.0, 0.0, -1.55)` | `(-1.16, 0.0, 0.0)` |
| shin R | `(-0.047, -0.0595, 1.52)` | `(-1.16, 0.0, 0.0)` |
| foot L | `(0.0, 0.0, 0.05)` | `(0.18, 0.0, 0.0)` |
| foot R | `(-0.07175, -0.2125, -0.02)` | `(0.18, 0.0, 0.0)` |

The asymmetric source values are intentional. They mirror the evaluated limb
chains in world space despite different left/right local axes and rest-pose
alignment.

## World-Space Metrics

The real source FBX was sampled at frames 1, 15, and 29 while the timeline also
contained nod, wrist twist, finger roll, happy bounce, leg step, weight shift,
and Mouth_A lip sync.

| Frame | Pelvis Z seated | Pelvis drop | Knee L/R | Lowest foot-tail Z |
| ---: | ---: | ---: | ---: | ---: |
| 1 | 0.7612 m | 0.3399 m | 89.34 / 91.21 deg | -0.0010 m |
| 15 | 0.7683 m | 0.4097 m | 89.34 / 91.21 deg | -0.0024 m |
| 29 | 0.7546 m | 0.3545 m | 89.34 / 91.21 deg | -0.0163 m |

- Pelvis range across samples: 0.0137 m.
- Maximum knee-angle L/R delta: 1.87 degrees.
- Knee mirror errors: below 0.015 m on every axis threshold.
- Foot mirror errors: below 0.020 m on every axis threshold.
- Head, wrist, finger, jaw, and Mouth_A controls all exceeded their motion
  thresholds at the middle frame.
- Front and side renders at all three frames showed horizontal thighs,
  near-vertical shins, grounded shoes, stable lower-body silhouette, and the
  preserved speech/hand gesture.

## Verification

Python suite:

```text
python3 mcp/ip_avatar_3d/test_server.py
Ran 90 tests in 0.178s
OK (skipped=2)
```

Complete Blender character-rig suite:

```text
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --python-exit-code 1 --python mcp/ip_avatar_3d/test_blender_character_rig.py
25 direct-runner tests passed; exit 0
```

Complete Blender hand-refinement/visual-QA suite:

```text
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --python-exit-code 1 --python mcp/ip_avatar_3d/test_blender_hand_refinement.py
11 focused tests passed; 54 QA samples rendered; exit 0
```

`python3 -m py_compile` passed for all five changed Python files.
`git diff --check` passed.

## Commit

Implementation: `23509e14` (`feat: add seated A-roll base action`).

## Self-Review

No blocking findings. The committed implementation/test scope is limited to
`aroll_actions.py`, `blender_renderer.py`, and their related Python/Blender
regressions. Existing upper-body, facial, standing, export/reimport, and hand
tests remain green.

## Concerns

- The brief names `run_blender_character_rig_tests.py` and
  `run_blender_hand_refinement_tests.py`, but neither wrapper exists at baseline
  `db5c7b8e`. The equivalent direct-runner test files were executed with
  `--python-exit-code 1`.
- The generated-rig values retain the brief's semantic fallback. Real visual
  calibration was performed on the production source FBX, while generated-rig
  behavior is covered by the full character and QA suites.
- Blender 5.1 emits existing `Material.use_nodes` Blender 6.0 deprecation
  warnings. They do not affect test outcomes.
- Canonical master, rigged GLB, source FBX, and the copied source GLB used for
  regression remain untracked and are not included in either Task 4 commit.
