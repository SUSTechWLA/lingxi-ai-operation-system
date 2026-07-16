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

## Review Follow-up: Four Important Findings

The rejected review findings are addressed by implementation commit `e6939c58`
(`fix: address seated pose review findings`). The earlier foot-tail values in
this report are retained as historical RED/GREEN evidence, but they are
superseded for floor contact by the evaluated shoe-geometry measurements below.

### RED / GREEN

Tests were added before the review fixes. The first focused run failed against
the real source FBX because the old foot-bone-tail proxy hid shoe penetration:

```text
AssertionError: ...
left minimumZ/clearance  = -0.0151163 m
right minimumZ/clearance = -0.0319805 m
```

The same test also added downstream assertions that seated `happy_bounce`
produces zero `Cheek_Smile.L/R`, while the standing control exceeds `0.5`.
GREEN filters `happy_bounce` from the seated event stream before any body or
facial derivation, so future derivations cannot accidentally consume it.

GREEN additionally proves that a seated timeline containing `wave`, `present`,
`emphasis`, `happy_bounce`, `leg_step`, and `weight_shift`:

- moves the right shoulder, upper arm, and forearm relative to frame 1;
- preserves right-wrist motion;
- independently moves proximal, middle, and distal segments of right digit 2;
- preserves head, jaw, `Mouth_A`, and lip-sync motion;
- keeps all six seated leg/shin/foot local rotations unchanged to `1e-6`;
- keeps both seated cheek-smile channels below `1e-6`.

The new standing regression was GREEN without changing standing behavior. One
three-event plan, sampled against the same-frame neutral plan, measured:

| Standing effect | Measured delta | Assertion |
| --- | ---: | ---: |
| `happy_bounce` root lift | `0.0700 m` | `> 0.050 m` |
| `leg_step` max leg rotation | `0.1069 rad` | `> 0.070 rad` |
| `weight_shift` root side | `0.0750 m` | `> 0.060 m` |
| `weight_shift` body sway | `0.0550 rad` | `> 0.040 rad` |

### Final Calibration

The source and generated seated root world offset is now
`(0.0, 0.12, -0.301)`. Source-rig limb rotations remain:

```text
leg L  (0.0, 0.0, 1.35)       leg R  (0.05, 0.197, -1.30)
shin L (0.0, 0.0, -1.55)      shin R (-0.047, -0.0595, 1.52)
foot L (0.0, 0.0, 0.05)       foot R (-0.07175, -0.2125, -0.02)
```

Seated procedural root idle lift is disabled. Upper-body sway and explicit
upper-body events remain active; standing idle lift is unchanged.

### Evaluated Shoe Geometry

The test creates an actual Blender plane at world `z=0`. For each side it maps
semantic `leg`, `shin`, and `foot` roles through `boneMap`, then scans source
vertex-group weights. A vertex belongs to a side when its summed weight for
that side's three groups is positive and greater than the opposite side's sum.
The selected source indices are read from the Armature-evaluated mesh after
asserting topology/index preservation, transformed by evaluated
`matrix_world`, and reduced to the minimum world-space Z. No bone head or tail
is used for clearance.

| Frame | Pelvis Z | Knee L/R | Left sole clearance | Right sole clearance |
| ---: | ---: | ---: | ---: | ---: |
| 1 | `0.80019 m` | `89.344 / 91.213 deg` | `0.023884 m` | `0.007019 m` |
| 15 | `0.82015 m` | `89.344 / 91.213 deg` | `0.015102 m` | `0.000580 m` |
| 29 | `0.80437 m` | `89.344 / 91.213 deg` | `0.013404 m` | `-0.002285 m` |

- Floor plane Z: exactly `0.0 m` at all four evaluated plane vertices.
- Weighted source/evaluated vertices: left `815`, right `813`.
- Pelvis sampled range: `0.01996 m` while the overlapping `emphasis` event is
  active.
- Maximum left/right knee-angle delta: `1.87 deg`.
- Signed shoe clearance acceptance: `-0.003 m` through `0.025 m`; the minimum
  is a `2.29 mm` right-sole skin deformation below the mathematical plane.

### Verification After Review Fixes

```text
python3 mcp/ip_avatar_3d/test_server.py
Ran 90 tests in 0.156s
OK (skipped=2)

/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --python-exit-code 1 --python mcp/ip_avatar_3d/test_blender_character_rig.py
26 direct-runner tests passed; exit 0

/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --python-exit-code 1 --python mcp/ip_avatar_3d/test_blender_hand_refinement.py
11 focused tests passed; 54 QA samples rendered; exit 0

python3 -m py_compile <five Task 4 Python files>
exit 0

git diff --check
exit 0
```

### Follow-up Self-Review and Concerns

No blocking finding remains. The implementation change is limited to seated
event filtering, seated root-idle behavior, and the calibrated root offset;
the larger diff is regression evidence. Standing behavior is explicitly
compared with neutral same-frame samples rather than inferred from keyframes.

The real skin's right sole reaches `2.29 mm` below the mathematical floor at
frame 29, within the explicit `3 mm` contact tolerance. Raising the full root
farther pushed the opposite shoe beyond the accepted `25 mm` contact band and
reduced the required seated pelvis drop, so the reported value is the selected
world-space balance.
Blender 5.1's existing `Material.use_nodes` deprecation warnings remain
non-failing. Copied canonical/model assets remain untracked and are not part of
the implementation or report commits.

## Second Review Follow-up: Strict Contact and Isolated Events

Implementation and regression commit: `153db9b7`
(`fix: tighten seated contact regressions`). This section supersedes the first
follow-up's `-3 mm` penetration tolerance, `25 mm` upper band, and combined
upper-event evidence. Neither circular limit remains in the tests.

### RED / GREEN

The tightened foot-dominant test was written first and failed on the real FBX:

```text
frame 1 LeftFoot  clearance = 0.0384724 m
frame 1 RightFoot clearance = 0.0223798 m
AssertionError: clearance <= 0.0035 m
```

These values came from the new strict foot region rather than the earlier
leg/shin/foot aggregate. GREEN changes the source foot rotations and removes
neutral seated pelvis idle sway while retaining spine/chest motion and explicit
event body sway.

`wave`, `present`, and `emphasis` now each run in a separate one-event seated
plan and compare frame 15 against a neutral seated plan at frame 15:

| Plan | Independently measured channels vs neutral |
| --- | --- |
| `wave` | shoulder R `0.0400`, upper arm R `1.0400`, forearm R `1.1500` rad |
| `present` | upper arms L/R `0.4774`, forearms L/R `0.7161` rad |
| `emphasis` | body `0.0348`, upper arms L/R `0.1591`, forearms L/R `0.1989` rad |

`wave` and `present` assert that body remains equal to neutral; `present` and
`emphasis` assert that shoulders remain equal to neutral. Every plan separately
asserts all six leg/shin/foot local rotations equal neutral within `1e-6`, so
removing any one event makes its own positive channel assertions fail.

### Foot-Dominant Sampling Contract

For each side, the test resolves only `boneMap[foot_l/foot_r]`. A source vertex
is eligible only when that Foot group has weight at least `0.5` and is the
vertex's maximum influence. Leg and shin positive weights are not included.

The sole region is then selected as follows:

1. Transform rest normals with the inverse-transpose world matrix and retain
   downward vertices with world normal Z at most `-0.25`.
2. Find the rest-space world minimum Z and take the fixed lowest `5 mm` band.
3. Require the same indices to remain downward after deformation, with
   evaluated world normal Z at most `-0.10`.
4. Read those indices from the Armature-evaluated mesh and compute the minimum
   deformed world Z against the actual `z=0` plane.

The sampled mesh is `part_00000001.001`. It has exactly one enabled modifier:
`Armature`, bound to the tested armature. The test fails if an ARMATURE modifier
is disabled or targets another object, or if any non-ARMATURE modifier is
enabled for viewport or render. It additionally checks evaluated/source vertex
counts. Therefore topology/index preservation is established by the active
modifier contract, not inferred from equal counts alone.

Each side yields `16` sole-band vertices. Rest minimum Z is `0.000233568 m` for
both sides.

### Final Calibration and Clearance

Final source-rig values are:

```text
root   (0.0, 0.12, -0.301)
foot L (0.0, 0.0, -0.229)
foot R (-0.07175, -0.2125, 0.146)
```

| Frame | Left minimum/clearance | Right minimum/clearance |
| ---: | ---: | ---: |
| 1 | `0.001323823 m` | `0.001204500 m` |
| 15 | `0.001323825 m` | `0.001204513 m` |
| 29 | `0.001323819 m` | `0.001204511 m` |

All raw values are nonnegative and below `3 mm`; no tolerance is needed for
these observed results. Maximum three-frame variation is below `1.4e-8 m`.

Blender reports `scene.unit_settings.scale_length == 1.0`, so one Blender unit
is treated as one meter in this calibrated scene. Blender mesh coordinates and
evaluated vectors use single-precision components; IEEE-754 float32 epsilon is
approximately `1.19e-7`, before composed FBX, armature, and matrix operations.
The fixed `0.5 mm` comparison allowance is a conservative cross-platform
numeric margin chosen independently of the measured clearances. The target
contract remains `0-3 mm`, and current raw values sit near its center.

### Final Verification

```text
python3 mcp/ip_avatar_3d/test_server.py
Ran 90 tests in 0.149s
OK (skipped=2)

/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --python-exit-code 1 --python mcp/ip_avatar_3d/test_blender_character_rig.py
27 direct-runner tests passed; exit 0

/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --python-exit-code 1 --python mcp/ip_avatar_3d/test_blender_hand_refinement.py
11 focused tests passed; 54 QA samples rendered; exit 0

python3 -m py_compile <five Task 4 Python files>
exit 0

git diff --check
exit 0
```

### Self-Review and Concerns

No blocking concern remains. Standing idle/body behavior is unchanged because
the pelvis idle-sway subtraction applies only in seated mode; the standing
bounce/step/weight-shift regression remains green. Explicit seated `emphasis`
still moves the body channel, while `wave` and `present` preserve it. Head,
mouth, wrist, and independent three-segment finger regressions remain green.

Blender 5.1's pre-existing `Material.use_nodes` deprecation warnings remain
non-failing. Copied GLB/FBX/canonical assets remain untracked and were not
included in commit `153db9b7`.
