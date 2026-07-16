# Task 10 Report: Deterministic A-roll Master Visual QA

## Outcome

Task 10 adds a fail-closed Blender QA renderer and focused Blender coverage for
the complete A-roll master contract. The renderer produces 52 deterministic
samples and writes `qa-report.json` only after every required file, silhouette,
pixel-difference, and adjacent-frame check passes.

The facial contract reports `blinkCapability=squint_only`, renders
`face/squint.png`, and explicitly sets `fullBlinkClaimed=false`. It does not
render or claim a full blink.

## Sample Contract

The frozen `QA_SAMPLES` manifest contains:

- six right-hand close frames: open, fist, pinch, and count one/two/three;
- three isolated finger-roll phases for each of the right and left hands;
- neutral, happy, serious, squint, A, E, O, and MBP face crops;
- one `Camera_Medium` and one `Camera_Wide` frame for each of all 16 canonical
  `Aroll_*` Actions.

The hand renderer creates fixed `QA_Hand_Close.R` and `QA_Hand_Close.L`
cameras from staged open-pose Action bounds before sampling. Their location,
rotation, and lens stay unchanged across each side's comparison set. A separate
fixed `QA_Face_Close` camera frames the face samples.

## Validation And Metrics

Before writing any output, the script requires exactly one Armature, valid
`ip_avatar_bone_map` metadata, complete three-segment L/R finger roles, all
required reusable Actions, `Camera_Medium`, `Camera_Wide`, all required mouth
and squint Shape Keys, and a Shape Key owner declaring
`blink_capability=squint_only`. Missing assets are listed together in the
raised `A-roll QA contract failed` error.

Every JSON sample includes `action`, `camera`, `path`, sampled bone rotations,
fingertip displacement ratios relative to the open pose, camera framing,
silhouette bounds/coverage, and an FFmpeg frame MD5. Paths are output-relative
so the report is stable across machines.

Alpha-mask checks isolate meshes owned by the character Armature or
`IP_Character_Master`; studio geometry cannot turn the silhouette into a full
frame. Existing authored cameras and lights remain active for the samples.

The acceptance checks include:

- open/fist alpha-mask pixel difference of at least `0.012`;
- each adjacent L/R finger-roll phase difference of at least `0.0015`;
- nonblank, non-full-frame alpha silhouettes for all 52 samples;
- all required output files;
- no adjacent duplicate hand, L/R finger-roll, face, or Action frames using
  FFmpeg's deterministic `framemd5` output.

## Test Fixture

The Task 10 Blender tests build a minimal scene entirely with `bpy`: one
Armature, six three-segment digit chains, bone-weighted palm/finger/body meshes,
a deformable face with the required Shape Keys, medium/wide cameras, lighting,
and the canonical reusable Action library. The fixture does not read a profile,
production Blend, or untracked final master.

The fail-closed test removes and restores an Action, camera, and squint Shape
Key to verify specific validation errors. The render test writes all samples to
a temporary directory, exercises the real image and FFmpeg helpers, verifies
stable hand framing and uniform JSON metrics, checks a deliberate duplicate,
checks a deliberate missing file, and then deletes the temporary directory.

## TDD And Verification

The initial RED run failed at the intended boundary with:

```text
ModuleNotFoundError: No module named 'render_aroll_master_qa'
```

The Task 10-only Blender run then passed the manifest, fail-closed fixture, and
52-frame render checks. The required full focused command also passed all nine
checks on Blender 5.1.2:

```bash
/Applications/Blender.app/Contents/MacOS/Blender -b --python mcp/ip_avatar_3d/test_blender_hand_refinement.py
```

This includes the three new Task 10 checks and all six existing hand rig,
weight, independent digit, finger-roll reset, and A-roll Action checks.

`python3 -m py_compile` passed for the renderer and focused test, and
`git diff --check` passed for the owned code/test files.

## Final Production Execution

The approved source-surface master was rendered through the real QA script to
`tmp/ip_avatar_3d/final-audit-master-qa`. The report is
`ready` with all 52 samples. Open/fist pixel difference is `0.019739`; adjacent
right finger-roll differences are `0.007117` and `0.016582`; adjacent left
differences are `0.006504` and `0.014158`. All exceed their acceptance floors.

The user changed this iteration's delivery target from 2K to 1080p. Final
Task 10 deliverables are:

- `outputs/MainIP_Sloth_Aroll_Action_Reel_1080p.mp4`;
- `outputs/MainIP_Sloth_Aroll_Hand_QA_1080p.mp4`;
- `outputs/MainIP_Sloth_Aroll_Face_QA.png`.

The action reel is the real 28-second MCP release render with the pinned local
GPT-SoVITS voice. The hand QA is a 1920x1080, 30 fps sequence of all 12 close
hand samples. The face sheet crops the eight neutral/expression/viseme samples
for original-size visual review. The face contract remains
`blinkCapability=squint_only`, `fullBlinkClaimed=false`.
