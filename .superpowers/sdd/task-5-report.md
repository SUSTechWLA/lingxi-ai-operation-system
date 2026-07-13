# Task 5 Report: A-roll Action Pack and Script Motion-Plan Mapping

## Summary

Implemented the reusable IP A-roll action pack and semantic planner mapping.

Owned files changed:
- `mcp/ip_avatar_3d/aroll_actions.py`
- `mcp/ip_avatar_3d/blender_renderer.py`
- `mcp/ip_avatar_3d/server.py`
- `mcp/ip_avatar_3d/test_server.py`
- `mcp/ip_avatar_3d/test_blender_hand_refinement.py`
- `.superpowers/sdd/task-5-report.md`

## TDD Evidence

RED 1:
`python3 mcp/ip_avatar_3d/test_server.py`

Result:
- Ran 29 tests.
- Failed 1 test: `test_motion_plan_maps_aroll_semantics_without_same_group_conflicts`.
- Expected failure: planner produced no `Aroll_*` action events (`set()`).

RED 2:
`/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_hand_refinement.py`

Result:
- Failed 1 focused test: `test_aroll_action_pack_names_reset_interpolation_and_safe_hand_stage`.
- Expected failure: all 16 `Aroll_*` actions were missing from the action report.
- Existing focused hand tests still passed.

GREEN:
`python3 mcp/ip_avatar_3d/test_server.py`

Result:
- Ran 29 tests.
- OK.

GREEN:
`/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_hand_refinement.py`

Result:
- PASS `test_aroll_action_pack_names_reset_interpolation_and_safe_hand_stage`.
- PASS all 5 existing focused hand-refinement tests.

GREEN:
`/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_character_rig.py`

Result:
- PASS all full character-rig tests in the direct runner, including generated rig, enhanced FBX rig, action library, timeline interpolation, and exported GLB reimport coverage.

## Action Pack Metrics

Exact A-roll names implemented:
- `Aroll_Idle_Listening`
- `Aroll_Greeting_Wave`
- `Aroll_OpenPalm_Explain`
- `Aroll_Explain_Left`
- `Aroll_Explain_Right`
- `Aroll_Count_One`
- `Aroll_Count_Two`
- `Aroll_Count_Three`
- `Aroll_Point_Left`
- `Aroll_Point_Right`
- `Aroll_Pinch_Detail`
- `Aroll_Emphasis_SoftFist`
- `Aroll_Think`
- `Aroll_Agree_Nod`
- `Aroll_Disagree_Shake`
- `Aroll_Transition_Reset`

Duration and phases:
- 16 actions generated.
- Each action uses 5 phases: reset, anticipation, hold-in, hold-out or release cue, reset.
- At 30 fps all actions span frames 1-60, nominal 2.0 seconds.

Endpoint and curve integrity:
- Maximum start/end rotation delta across sampled roles: `0.0`.
- Interpolation defects: `0`.
- All sampled keyframes use `BEZIER` with `AUTO_CLAMPED` handles.

Shared pose source:
- A-roll specs use `__digit_pose_l` and `__digit_pose_r` markers.
- The renderer resolves those markers through the existing shared `HAND_POSES`, `DigitPose`, `hand_pose_eulers`, and legacy fallback path.
- No duplicate finger pose tables were added.

Face-safe hand offsets from the enhanced FBX source rig, sampled at hold:
- `Aroll_Count_One` right hand: `[-0.6533, -0.1490, -0.3529]`, outside face box.
- `Aroll_Count_Two` right hand: `[-0.6533, -0.1490, -0.3529]`, outside face box.
- `Aroll_Count_Three` right hand: `[-0.6533, -0.1490, -0.3529]`, outside face box.
- `Aroll_OpenPalm_Explain` right hand: `[-0.6533, -0.1490, -0.3529]`, outside face box.
- `Aroll_OpenPalm_Explain` left hand: `[0.6565, -0.1551, -0.3563]`, outside face box.
- `Aroll_Pinch_Detail` right hand: `[-0.6533, -0.1490, -0.3529]`, outside face box.
- `Aroll_Point_Left` left hand: `[0.7167, -0.1021, -0.3481]`, outside face box.
- `Aroll_Point_Right` right hand: `[-0.7116, -0.0987, -0.3462]`, outside face box.

Leg exaggeration:
- Focused action test samples leg, shin, and foot roles during all A-roll holds.
- Maximum sampled leg-role hold rotation is below `1e-6`.

## Planner Metrics

Final mixed Mandarin/English planner sample:
- Total motion events: 49.
- A-roll semantic events: 19.
- Group participation: `right_hand=15`, `left_hand=2`, `head=4`.
- Same-group conflicts across primary and multi-group events: `0`.
- Repeated planner call returned identical `motionEvents`.

Representative semantic mappings:
- Greeting: `Aroll_Greeting_Wave`, `right_hand`, `motion=wave`, starts at `0.2s`.
- Enumeration: `Aroll_Count_One`, `Aroll_Count_Two`, `Aroll_Count_Three`, `right_hand`, old `point_left`/`point_right` motions preserved.
- Explanation: `Aroll_OpenPalm_Explain`, `gestureGroup=right_hand`, `gestureGroups=[right_hand, left_hand]`, `motion=present`.
- Detail: `Aroll_Pinch_Detail`, `right_hand`, `motion=finger_wave`.
- Agreement: `Aroll_Agree_Nod`, `head`, `motion=nod`.
- Disagreement: `Aroll_Disagree_Shake`, `head`, `motion=head_shake`.
- Emphasis/conclusion: `Aroll_Emphasis_SoftFist`, `right_hand`, `motion=emphasis`.

Compatibility:
- Existing `motion` names remain present for procedural timeline playback.
- New fields are additive: `action`, `semantic`, `eventName`, `gestureGroup`, and optional `gestureGroups`.
- Existing `Gesture_*` actions are unchanged and continue to be generated.

## Self-Review

Deterministic ordering:
- Planner events are sorted by time, group, action, and motion.
- The new server test verifies repeatability by comparing two complete `motionEvents` outputs for the same script.

No conflicting hand gestures:
- Action-bearing events are shifted by occupied `gestureGroup`.
- Multi-hand explanation events additionally reserve both `right_hand` and `left_hand` via `gestureGroups`.
- The server test checks there are no overlapping A-roll same-group events.

Legacy rig compatibility:
- A-roll specs reuse the existing action builder and shared digit-pose marker path.
- Existing legacy two-segment and source-humanoid action tests remain green.

Repeatability:
- No random timing or action selection was introduced.
- Keyword and semantic mapping is table-driven, not hardcoded to one script.

Concerns:
- None currently known.
