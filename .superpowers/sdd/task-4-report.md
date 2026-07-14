# Task 4: Independent Three-Segment Digit Poses And Correct Closure

## Status

Complete. `aroll_actions.py` is the shared source for semantic digit poses,
per-chain Euler mapping, and timeline blending. Action construction and
procedural animation both apply those shared poses.

## TDD Evidence

RED was captured before production changes:

```text
FAIL test_each_digit_moves_independently_and_fist_closes
  right digit 1 curl displacement 0.0370 is below 2.5828
  right digit 2 curl displacement 0.0385 is below 2.6935
  right digit 3 curl displacement 0.0355 is below 2.4791
FAIL test_three_segment_digit_poses_key_independent_semantic_curls
  (1, (0.019999999552965164, 0.0, 0.23999999463558197))
```

The focused command is now green:

```bash
/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_hand_refinement.py
```

All five focused checks pass, including independent three-segment rotations,
fist closure, isolated-roll drift, open curls/splay, and pinch opposition.

## Motion Sampling

Palm-relative, world-scale-normalized ratios from `Gesture_Fist` and
`Gesture_FingerWave`:

| Side | Digit 1 fist | Digit 2 fist | Digit 3 fist |
| --- | ---: | ---: | ---: |
| Left | 0.697 | 0.684 | 0.678 |
| Right | 0.697 | 0.684 | 0.699 |

| Roll path | Selected ratio | Unselected maximum |
| --- | ---: | ---: |
| Right-hand action digit 1 | 0.649 | 0.000 |
| Right-hand action digit 2 | 0.650 | 0.000 |
| Right-hand action digit 3 | 0.649 | 0.000 |
| Procedural `animate()` digit 2 | 0.497 | 0.000 |

The contract ratios remain 25% for fist closure, 18% for the selected roll,
and 6% maximum for unselected drift. Each sampled distal tail is transformed
through its evaluated Hand pose-bone inverse before applying armature world
scale. The denominator is the sum of all three segment vectors at that same
world scale, not the chain chord. This removes wrist, palm, and upstream arm
motion without changing any threshold.

## Review Follow-Up TDD Evidence

The enhanced procedural test was added before the timeline fix and initially
failed as expected:

```text
AssertionError: (1, 0.03929193305929791, 0.011831490942318093)
```

That was an unselected finger-roll digit moving 20.0% of its hand-relative
chain length against the unchanged 6% maximum. `finger_roll_pose()` now lets
the procedural timeline retain the shared `relaxed_hand` pose for unselected
digits while action-library rolls keep their explicit open-hand baseline.

## Final-Frame Follow-Up

The focused endpoint contract was added before the final marker fix and
captured the unmarked three-segment fallback at frame 60:

```text
digit 1 start: (0.14000000059604645, 0.0, 0.014999999664723873)
digit 1 end:   (0.10000000149011612, 0.0, 0.05000000074505806)
```

`Gesture_FingerWave` now marks both frame 1 and frame 60 with the shared
right-hand `open_hand` pose. The focused regression verifies every digit's
proximal, middle, and distal rotation, plus hand-relative tips, match the
start endpoint within `1e-6`; resulting final-frame drift is zero. Roll frames
15, 30, and 45 retain their independent shared roll poses, and the legacy
two-segment fallback remains restricted to unmarked legacy chains.

## Verification

```text
PASS focused test_blender_hand_refinement.py (5 checks), including the
     final-frame `Gesture_FingerWave` endpoint regression
PASS test_blender_character_rig.py, including enhanced-rig `animate()` fist,
     finger-roll isolation, and `point_right` coverage
PASS test_server.py (28 tests)
PASS action integrity: 396 curves, 0 duplicate-key curves, 0 unclamped points,
     repeated create_action_library call reused Gesture_Fist
```

## Compatibility Review

- Three-segment chains explicitly key proximal, middle, and distal rotations.
- Automatic distal behavior is retained only for exact legacy two-segment
  chains; generated single-segment fingers still receive their proximal pose.
- `create_action_library` resets every keyed frame and procedural `animate`
  clears the active action before rebuilding its timeline.
- The enhanced procedural regression checks all three segments on fist and
  the selected roll digit, hand-relative unselected drift, and `point_right`.
- All shared poses use source-rig semantic Z curl axes. The imported FBX axis
  sample confirmed Z is a bending axis; no palm or wrist compensation was
  added for closure, and no topology or skin weights changed.

## Concern

Blender emits pre-existing background-addon and Material API deprecation
messages during tests. They do not affect any test result.
