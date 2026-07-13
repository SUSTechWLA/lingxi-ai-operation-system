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

All four focused checks pass, including independent three-segment rotations,
fist closure, isolated-roll drift, open curls/splay, and pinch opposition.

## Motion Sampling

World-space chain-length ratios from `Gesture_Fist` and `Gesture_FingerWave`:

| Digit | Fist tip displacement | Roll selected | Roll unselected maximum |
| --- | ---: | ---: | ---: |
| 1 | 0.567 | 0.649 | 0.000 |
| 2 | 0.592 | 0.650 | 0.000 |
| 3 | 0.577 | 0.649 | 0.000 |

The contract ratios remain 25% for fist closure, 18% for the selected roll,
and 6% maximum for unselected drift. Their length normalization now uses the
same world space as the sampled tips; the FBX armature has a non-unit object
scale, so comparing those tips against raw local bone lengths was invalid.

## Verification

```text
PASS focused test_blender_hand_refinement.py (4 checks)
PASS test_blender_character_rig.py (15 checks)
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
- All shared poses use source-rig semantic Z curl axes. The imported FBX axis
  sample confirmed Z is a bending axis; no palm or wrist compensation was
  added for closure, and no topology or skin weights changed.

## Concern

Blender emits pre-existing background-addon and Material API deprecation
messages during tests. They do not affect any test result.
