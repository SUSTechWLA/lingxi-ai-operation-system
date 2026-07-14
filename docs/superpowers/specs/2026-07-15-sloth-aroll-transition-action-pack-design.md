# Sloth A-roll Transition And Action Pack Design

## Context

The current warm studio integration supports separate standing and seated
shots, but it treats them as independent presentation modes. The combined
review reel cuts from standing to seated at approximately 15.7 seconds. The
first seated frames expose asymmetric thigh deformation and a hard center seam
while the chair is fully hidden by the desk, which reads as a pixel or broken
mesh artifact. The current visemes also keep the jaw and lips too close to the
rest pose for expressive A-roll delivery.

This iteration turns standing and seated presentation into a continuous,
stateful performance system. It preserves the production character, original
facial geometry, permanent IP voice, warm studio, and 1080p render profile.

## Goals

- Author physically plausible standing-to-seated and seated-to-standing
  transitions with stable feet, knees, pelvis, torso, hands, and gaze.
- Remove the visible seated leg seam and make the authored stool/chair legible
  in appropriate camera angles.
- Expand the reusable standing and seated A-roll action vocabulary.
- Increase mouth readability without adding overlay geometry or replacing the
  source character's face.
- Expose action state and transition metadata through the existing MCP so a
  script planner can assemble continuous performances safely.
- Render a corrected dual-mode reel and a dedicated seated demo with automated
  and visual QA evidence.

## Non-Goals

- Do not redesign or replace the source character.
- Do not add locomotion, running, jumping, or cinematic stunt animation.
- Do not generate a dynamic background or composite the character over a flat
  background plate.
- Do not add an artificial mouth card, face texture overlay, or unrelated face
  mesh.
- Do not change the pinned GPT-SoVITS IP voice or restore 2K test rendering.

## Root Cause And Correction

The 16-second artifact is not a codec error. The seated source-rig pose rotates
the left and right leg chains asymmetrically toward the camera. The wide seated
camera then reveals two thigh/knee lobes and their central boundary above the
desk while the chair is completely occluded. The geometry is stable across
adjacent encoded frames, confirming a pose and framing defect.

The correction uses a new seated reference pose calibrated against explicit
seat, pelvis, knee, and foot targets. Both leg chains use mirrored semantic
angles rather than unrelated source-bone Euler values. Camera framing exposes
enough of the stool to explain the pose while keeping the desk useful as
foreground context. A silhouette gate samples the seated wide and transition
frames for central spikes, left/right discontinuity, chair visibility, floor
support, and furniture intersection.

## Pose State Model

Every reusable action declares its start and end state:

- `standing`
- `seated`
- `either`

The production action manifest also declares duration, loopability, semantic
intent, affected body channels, and whether the action may be layered over
speech. The planner may concatenate actions directly when their states match.
When they do not match, it inserts the required transition action.

The default stable poses are:

- `Aroll_Standing_Idle`: feet planted, knees relaxed, pelvis centered, upright
  torso, camera-facing head, relaxed hands.
- `Aroll_Seated_Idle`: pelvis supported by the seat, thighs naturally forward,
  knees below the desk edge, feet on floor targets, slight forward torso lean,
  hands visible and clear of the desktop.

Existing callers that specify only `presentationMode` remain compatible. The
mode selects the initial state, while a motion plan may explicitly request
state changes later in the shot.

## Standing-To-Seated Transition

`Aroll_Transition_StandToSit` is an authored 1.6 to 2.2 second action with five
phases:

1. Preparation: gaze remains near camera, feet settle, arms move to a balanced
   neutral position.
2. Weight shift: chest inclines slightly forward and pelvis travels backward
   toward `IP_Seat_Target`.
3. Descent: hips and knees bend together while foot controls remain locked to
   `IP_Foot_Target.L` and `IP_Foot_Target.R`.
4. Contact: pelvis reaches the seat target and vertical velocity eases before
   the body appears to compress into the chair.
5. Settle: spine returns toward upright, shoulders relax, and hands enter the
   seated speaking range.

The transition uses eased interpolation with a short contact hold. Feet may
roll subtly through the ankles but may not translate visibly. Knees remain
separated and behind the desk silhouette; thighs may not intersect the body,
desk, or each other.

## Seated-To-Standing Transition

`Aroll_Transition_SitToStand` is authored independently rather than reversing
the sit animation. Its phases are:

1. Preparation: feet draw into a supported stance and the torso inclines
   forward.
2. Load: pelvis shifts toward the feet while the head remains controlled.
3. Lift: hips and knees extend together; arms provide restrained counterbalance.
4. Stack: pelvis moves over the planted feet and the spine becomes upright.
5. Settle: knees soften and hands return to the standing speaking range.

The character must look weight-bearing rather than vertically translated.
Neither transition uses a camera cut to hide invalid intermediate poses.

## Rig And Deformation Refinement

The source humanoid rig remains canonical. Refinement is limited to controls,
constraints, action data, and corrective deformation needed for production:

- calibrate mirrored hip, knee, ankle, pelvis, spine, shoulder, elbow, wrist,
  and digit semantics;
- preserve three-segment independent finger chains and wrist twist;
- add foot-lock and seat-contact targets for transitions;
- add pelvis and torso counter-rotation so sit and stand retain balance;
- add targeted corrective shape keys only when pose-space deformation cannot
  remove a visible thigh, knee, elbow, or wrist collapse;
- normalize affected weights and keep no more than four deform influences per
  vertex when weights are changed.

The original mouth, brows, eyelids, and eyes remain the facial source. No
replacement face layer is introduced.

## Mouth And Expression Refinement

The existing source-mouth visemes are retained and strengthened:

- increase jaw travel for open vowels to a production target near 0.22 to 0.24
  radians, subject to collision review;
- increase lower-lip displacement and mouth-cavity exposure for `Mouth_A`,
  `Mouth_O`, `Mouth_U`, and `Mouth_Surprise` by approximately 35 to 55 percent;
- preserve a true closure for `Mouth_MBP`, with `Mouth_Rest` relaxed rather than
  clamped shut;
- use viseme-specific intensity so wide and round vowels remain distinct;
- add two-to-three-frame attack/release smoothing and minimum readable holds
  without delaying synchronization;
- keep eye, brow, head, and antenna cues subtle enough that speech remains the
  primary performance.

QA compares rest, MBP, A, E, O, U, smile, and surprise from the production
camera and rejects mouth-card artifacts, lip inversion, face penetration, or
indistinguishable open vowels.

## Core A-roll Action Pack

The existing action library remains available. This iteration adds or
formalizes the following reusable actions:

- `Aroll_Standing_Idle`
- `Aroll_Transition_StandToSit`
- `Aroll_Transition_SitToStand`
- `Aroll_Welcome_OpenArms`
- `Aroll_Question_PalmUp`
- `Aroll_Compare_TwoSides`
- `Aroll_KeyPoint_OneFinger`
- `Aroll_List_Three`
- `Aroll_Caution_Stop`
- `Aroll_Quote_Frame`
- `Aroll_Conclusion_HandsTogether`
- `Aroll_Seated_Explain`
- `Aroll_Seated_OpenPalm`
- `Aroll_Seated_LeanIn`

Actions use anticipation, readable hold, and release phases. Wrist orientation
and digit poses are authored with each gesture. Standing gestures keep feet
planted; seated gestures stay within a smaller range above or beside the desk.
No action may drive an arm through the torso, desk, chair, or face.

## MCP Contract

The MCP action catalog returns stateful metadata for every action. A motion
event may request an explicit action:

```json
{
  "type": "avatar_action",
  "action": "Aroll_Transition_StandToSit",
  "startFrame": 240,
  "intensity": 0.8
}
```

The renderer validates start-state compatibility before Blender execution. In
`auto` mode, the planner may insert a transition based on semantic script cues
such as opening standing, moving into a deeper seated explanation, and standing
for a conclusion. Invalid state sequences fail with an actionable error rather
than silently snapping the root transform.

The render report records the resolved state timeline, inserted transitions,
action versions, character hash, studio hash, camera, and QA results so the
same performance is reproducible.

## Camera And Editing

Transitions use a continuous medium-wide or three-quarter camera that shows the
seat relationship, feet, and hands. The system may cut to a tighter camera only
after the target pose has settled. The corrected review reel does not use the
old hard cut at 15.7 seconds to conceal state change.

The seated hero camera reveals part of the stool silhouette without turning the
shot into a furniture showcase. Lighting, color management, depth of field,
1920x1080 resolution, and 30 fps CFR remain aligned with the approved warm
studio production profile.

## Validation

Automated tests cover:

- action catalog uniqueness and required state metadata;
- planner insertion of stand-to-sit and sit-to-stand transitions;
- rejection of impossible state sequences;
- mirrored seated leg semantics and stable seated reference pose;
- frame-sampled foot translation, seat contact, joint limits, and furniture
  clearance through both transitions;
- chair visibility and absence of the central seated thigh spike in hero
  cameras;
- mouth-open range, MBP closure, viseme separation, smoothing, and keyframe
  timing;
- 1920x1080, 30 fps CFR output, expected duration, motion continuity, audio
  synchronization, loudness, and true peak.

Visual evidence includes transition contact sheets, seated camera frames,
viseme comparisons, and an action-pack review reel. Validation samples all
motion extrema, not only the first and last poses.

## Deliverables

- `outputs/Sloth_WarmStudio_Seated_Demo_1080p.mp4`
- `outputs/Sloth_WarmStudio_StandSit_Demo_1080p.mp4`
- `outputs/Sloth_WarmStudio_DualMode_Reel_1080p.mp4`
- `outputs/Sloth_WarmStudio_ActionPack_1080p.mp4`
- `outputs/Sloth_WarmStudio_Transition_ContactSheet.png`
- `outputs/Sloth_WarmStudio_Viseme_Comparison.png`
- `outputs/Sloth_WarmStudio_ActionPack_Report.json`

The stand/sit demo visibly performs standing speech, sits without a cut,
continues seated speech, then stands without a cut. The action-pack reel
demonstrates a representative subset of both states while preserving the
permanent IP voice.

## Acceptance

The work is complete when both transitions read as supported human-like motion,
the 16-second artifact is absent, the stool is visually understandable, mouth
shapes are clearly distinguishable at normal playback size, all declared
actions are callable through the MCP, rendered motion remains smooth at 30 fps,
all automated tests pass, and the new demos pass contact-sheet and full-motion
visual review.
