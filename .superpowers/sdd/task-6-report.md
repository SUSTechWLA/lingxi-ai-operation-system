# Task 6 Report: Subject-First Lighting And Color Calibration

## Status

Incomplete and intentionally not committed. The current candidate does not yet
meet the Task 6 evidence contract. Work is preserved in the worktree; no further
lighting iteration was started after the execution audit request.

Baseline: `8294db380a6ffc414f70ae5bf2b92ba4b22b3a33`.

## Current Candidate

| Source | Energy | Temperature | Area | Target |
| --- | ---: | ---: | --- | --- |
| `IP_Subject_Key` | `520 W` | `4500 K` | `2.40 x 2.40 m` | standing head focus |
| `IP_Subject_Fill` | `115 W` | `5200 K` | `2.20 x 2.20 m` | standing head focus |
| `IP_Subject_Rim` | `260 W` | `3200 K` | `1.40 x 1.40 m` | standing head focus |
| `Window_Softbox` | `140 W` | `4800 K` | `2.25 x 2.65 m` | room/subject |
| `Practical_Wall` | `42 W` | `2700 K` | point | wall fixture |
| `Practical_Shelf` | `48 W` | `2700 K` | point | shelf fixture |
| `Downlight_01..03` | `36 W` each | `3000 K` | `0.34 x 0.34 m` | room |

- World strength: `0.12`.
- AgX look: `AgX - Medium High Contrast`.
- Authored Eevee exposure: `-0.6`.
- Cycles comparison exposure: `-0.6`.
- Subject/window/downlight colors use neutral RGB with Blender native color
  temperature. This avoids multiplying an orange RGB tint by a warm Kelvin tint.
- No character material, action, character profile, or voice file was changed.

## TDD State

RED was observed for:

- missing `IP_Subject_Key` in the saved scene;
- missing geometry-derived subject/face mask API;
- missing full evidence render plan;
- missing fail-closed lighting-evidence validator.

The following focused checks subsequently passed:

```text
Blender saved-scene subject-light contract plus mask/measurement unit tests: PASS
Task 6 pure mask, linear-luminance, clipping, render-matrix, and evidence-gate tests: PASS
python3 -m py_compile for the four owned Python files: exit 0
```

Pre-change baseline checks were:

```text
Blender scene contract: 17 PASS, exit 0
python3 mcp/ip_avatar_3d/test_server.py: 90 run, OK, 2 skipped
```

The complete Blender scene contract has not been rerun after the current
changes, so those baseline results must not be treated as current regression
evidence.

## Rendered Evidence

Completed low-resolution diagnostic images (`480x270`, canonical master,
standing frame 29):

- `/tmp/task6-debug-evidence/eevee-standing-medium.png`
- `/tmp/task6-debug-evidence/eevee-standing-three-quarter.png`
- `/tmp/task6-debug-evidence/eevee-standing-wide.png`
- `/tmp/task6-debug-evidence/eevee-standing-medium-empty.png`
- corresponding scene-linear EXRs under
  `/tmp/task6-debug-evidence/linear/`

The tracked studio outputs were rebuilt:

- `ip形象/main_ip/scenes/warm-sloth-studio-v1.blend`
- `ip形象/main_ip/scenes/warm-sloth-studio-v1-preview.png`

These diagnostics are not final Task 6 evidence. No seated image, Cycles image,
subject ID matte, face mask, or `lighting-evidence.json` completed.

## Measurements

No compliant per-mode measurement is available. The required mask is produced
from an actual rendered character geometry ID matte and intersected with live
semantic-head projection; the run was stopped before that matte was rendered.
Consequently these values are intentionally reported as unavailable rather than
estimated from fixed screen coordinates:

| Mode / engine | Linear face Y | Linear background Y | Stops below face | Non-catchlight clip |
| --- | ---: | ---: | ---: | ---: |
| standing / Eevee | unavailable | unavailable | unavailable | unavailable |
| seated / Eevee | unavailable | unavailable | unavailable | unavailable |
| standing / Cycles | unavailable | unavailable | unavailable | unavailable |
| seated / Cycles | unavailable | unavailable | unavailable | unavailable |

## Visual Audit

- Standing medium shows visible cardigan seams, zipper, buttons, and fur detail;
  no obvious large pure-white area was seen in the diagnostic PNG.
- The scene still reads orange overall. The pale center wall and face remain
  visually close in display brightness, so 1.0 to 1.5 stops of separation cannot
  be claimed without the required linear measurement.
- The three standing camera files are not valid multi-view evidence. Their PNG
  hashes differ, but medium versus three-quarter and medium versus wide both have
  mean absolute pixel difference `3.78237707820972e-08` and maximum difference
  `0.003921568393707275`. Frame-29 timeline camera markers override direct camera
  assignments, so all three renders are effectively the medium camera.

## Latest Failed Command

```bash
/Applications/Blender.app/Contents/MacOS/Blender --background \
  --factory-startup --python-exit-code 1 \
  --python mcp/ip_avatar_3d/render_warm_studio_qa.py -- \
  ip形象/main_ip/scenes/warm-sloth-studio-v1.blend \
  /tmp/task6-debug-evidence eevee \
  --subject-evidence \
  --master ip形象/main_ip/models/main-ip-aroll-master.blend \
  --width 480 --height 270 --frame 29
```

The audit request stopped this command during standing Cycles medium. It exited
`1` after the interrupt because neither
`cycles-standing-medium.png` nor its linear EXR completed. This is an interrupted
diagnostic, not evidence of a Blender render crash.

## Remaining Blockers

1. Clear or bypass timeline camera markers during each QA still, then prove that
   medium, three-quarter, and wide are materially different views.
2. Complete standing and seated Eevee coverage at final evidence resolution.
3. Complete standing and seated Cycles medium plus same-engine empty controls.
4. Generate actual-geometry subject mattes and dynamic semantic-head face masks.
5. Produce per-mode scene-linear luminance and AgX PNG clipping measurements;
   require `1.0..1.5` stops and clipping strictly below `0.5%`.
6. Visually inspect every final image for white-clothing texture, fur retention,
   color neutrality, composition, and cross-engine consistency.
7. Run the complete Blender scene contract, server tests, validator, py_compile,
   and diff checks after evidence passes.

## Commit And Concerns

- Commit: none. The brief is not satisfied, so committing would misrepresent the
  state.
- The tracked `.blend` and preview contain the current candidate values and are
  modified but unstaged.
- `render_warm_studio_qa.py` contains the unfinished evidence pipeline. It has
  focused unit coverage but not a completed end-to-end run.
- A builder-generated untracked
  `ip形象/main_ip/scenes/warm-sloth-studio-v1.blend1` backup is present and is
  not staged.
- Existing untracked canonical model/turnaround assets remain preserved and are
  not part of the intended Task 6 commit.

## Task 6A Measurement Foundation Update (2026-07-14)

Status: **DONE for 6A only**. No light energy, temperature, world strength,
exposure, character material, or generated Blend/preview asset was changed by
this subtask. The earlier camera and mask blockers above are superseded by this
section; Task 6 lighting calibration remains incomplete.

### Diagnosis And Fix

- Root cause confirmed: frame-29 timeline camera markers reapplied the authored
  medium camera during render after the QA caller assigned another camera.
- Every QA still now temporarily removes only camera-bearing timeline markers,
  captures the active camera in a Blender `render_pre` handler, and restores the
  original markers after the still. The focused test forces another
  `frame_set()` inside the render callback and verifies both isolation and
  restoration.
- Character and practical-highlight IDs are rendered in one Raw RGB geometry
  matte: character objects are red, emissive/practical fixture geometry is
  green, all other renderable geometry and the world are black. Original
  material slots, world values, compositor, camera, render settings, and view
  transform are restored in `finally`.
- Face masks are no longer screen rectangles. They rasterize `16,757` evaluated,
  posed mesh triangles selected from Head/Jaw/Eye/Hair-related vertex weights,
  then intersect that projected geometry with the visible character ID matte.
- Background masks are the non-character image region minus rendered practical
  IDs and clipped display highlights. Empty subject, face, or background masks
  fail closed in both measurement and payload validation.
- Scene-linear Rec.709 EXRs provide face/background luminance and stops. AgX
  Medium High Contrast PNGs provide non-catchlight clipping ratios.

### Camera Evidence

All records report frame `29`, `timelineCameraMarkerCountDuringRender: 0`, and
matching requested/active camera names. Distinct 4x4 camera matrices are stored
with each comparison.

| Mode | Comparison | RGB pixel MAE |
| --- | --- | ---: |
| standing | medium vs three-quarter | `0.2997740990` |
| standing | medium vs wide | `0.2084549220` |
| seated | medium vs three-quarter | `0.2835041262` |
| seated | medium vs wide | `0.2148200945` |

The validator requires MAE strictly greater than `0.001`; the previous invalid
same-view evidence was approximately `3.78e-08`.

### Geometry Mask Evidence

| Mode | Subject px | Face px | Practical px | Eevee background px | Cycles background px |
| --- | ---: | ---: | ---: | ---: | ---: |
| standing | `10,467` | `2,302` | `21` | `46,967` | `46,026` |
| seated | `10,149` | `2,230` | `28` | `47,255` | `46,525` |

Evidence root: `/tmp/task6a-evidence`

- Payload: `/tmp/task6a-evidence/lighting-evidence.json`
- Display stills: `/tmp/task6a-evidence/{eevee,cycles}-{standing,seated}-*.png`
- Linear EXRs: `/tmp/task6a-evidence/linear/`
- RGB ID mattes and binary masks: `/tmp/task6a-evidence/masks/`

### Trustworthy Measurements At 320x180

These values are diagnostic inputs for the next lighting pass, not passing Task
6 values. The low separation and Cycles clipping are now measured failures rather
than estimates.

| Mode / engine | Linear face Y | Linear background Y | Stops below face | Non-catchlight clip |
| --- | ---: | ---: | ---: | ---: |
| standing / Eevee | `0.8715305664` | `0.6048914673` | `0.5268749537` | `0.0000000000` |
| standing / Cycles | `1.4912704742` | `0.9937092781` | `0.5856462048` | `0.0455717971` |
| seated / Eevee | `0.8772832520` | `0.5964597534` | `0.5566179324` | `0.0000000000` |
| seated / Cycles | `1.4502087560` | `0.9692917999` | `0.5812576382` | `0.0351758794` |

Current payload errors are limited to the expected lighting gates: all four
measurements are below `1.0` stop, and both Cycles measurements exceed `0.5%`
clipping. Camera, metadata, source, path, and non-empty mask validation pass.

### Verification

```text
Focused 6A Blender tests: 7 PASS
Complete Blender scene contract: 25 PASS, exit 0
python3 -m py_compile for the four Task 6 Python files: exit 0
git diff --check: exit 0
```

No commit was created. The next agent can tune lighting against
`/tmp/task6a-evidence/lighting-evidence.json` and rerun the same evidence command
at the required final resolution.

## Task 6B Lighting Calibration Stop (2026-07-14)

Status: **STOPPED after two measured tuning rounds**. Neither round modified the
tracked builder, generated Blend, preview, QA code, or tests. The candidates and
evidence are temporary diagnostics only. Task 6 remains incomplete and no commit
was created.

### Tuning Method And Parameters

Round 1 changed one independent variable, `room_light_scale = 0.45`, while
holding the subject rig and exposure fixed:

| Parameter | Round 1 |
| --- | ---: |
| World strength | `0.054` |
| `Window_Softbox` | `63 W`, `4800 K` |
| `Practical_Wall` | `18.9 W`, `2700 K` |
| `Practical_Shelf` | `21.6 W`, `2700 K` |
| `Downlight_01..03` | `16.2 W` each, `3000 K` |
| `Lamp_Emissive_Warm` emission strength | `1.08` |
| `Lamp_Shade_Warm` emission strength | `0.072` |
| `IP_Subject_Key` | `520 W`, `4500 K` |
| `IP_Subject_Fill` | `115 W`, `5200 K` |
| `IP_Subject_Rim` | `260 W`, `3200 K` |
| Eevee / Cycles exposure | `-0.6 / -0.6` |

Round 1 brought Cycles clipping below the hard limit, but separation improved by
only `0.020..0.062` stops. The paired baseline/round-1 measurements show that
removing all remaining room-source contribution would asymptote near `0.64`
stops; continuing to reduce those sources would not reach the contract.

Round 2 changed the single independent subject-key/room ratio to `8.0`. Key
energy and exposure compensation were linked so that display brightness and the
already-passing clipping gate stayed controlled:

| Parameter | Round 2 |
| --- | ---: |
| Room parameters | same as Round 1 |
| `IP_Subject_Key` | `4160 W`, `4500 K` |
| `IP_Subject_Fill` | `115 W`, `5200 K` |
| `IP_Subject_Rim` | `260 W`, `3200 K` |
| Eevee / Cycles exposure | `-2.769925 / -2.769925` |

### Measured Results At 320x180

Round 1 payload: `/tmp/task6b-attempt1/lighting-evidence.json`

| Mode / engine | Linear face Y | Linear background Y | Stops below face | Non-catchlight clip |
| --- | ---: | ---: | ---: | ---: |
| standing / Eevee | `0.7049197876` | `0.4726982971` | `0.5765394360` | `0.0000000000` |
| standing / Cycles | `1.2185477788` | `0.8009145041` | `0.6054426684` | `0.0022929206` |
| seated / Eevee | `0.7034852722` | `0.4679147644` | `0.5882744686` | `0.0000000000` |
| seated / Cycles | `1.1652051842` | `0.7586864550` | `0.6190083391` | `0.0027588925` |

Round 2 payload: `/tmp/task6b-attempt2/lighting-evidence.json`

| Mode / engine | Linear face Y | Linear background Y | Stops below face | Non-catchlight clip |
| --- | ---: | ---: | ---: | ---: |
| standing / Eevee | `1.6554444336` | `0.7437819702` | `1.1542669047` | `0.0000000000` |
| standing / Cycles | `3.3347034050` | `2.0374259766` | `0.7108108064` | `0.0000000000` |
| seated / Eevee | `1.6907611328` | `0.7600437988` | `1.1535183894` | `0.0000000000` |
| seated / Cycles | `3.0952739196` | `1.9343357730` | `0.6782288402` | `0.0000000000` |

Round 2 passes both Eevee separation gates and all clipping gates. It fails both
Cycles separation gates by `0.2891891936` stops standing and `0.3217711598`
stops seated. The fail-closed command exited `2` with only those two errors.

Evidence roots:

- `/tmp/task6b-attempt1/`
- `/tmp/task6b-attempt2/`
- candidate scenes: `/tmp/task6b-attempt1-scene.blend` and
  `/tmp/task6b-attempt2-scene.blend`

Every requested render reports `timelineCameraMarkerCountDuringRender: 0` and a
matching requested/active camera. Round-2 Eevee camera MAEs are `0.3352406701`
and `0.2150380022` standing, and `0.3451955778` and `0.2373921478` seated, all
well above the `0.001` distinct-view threshold.

### Visual Self-Review

- All six requested Eevee views and both Cycles medium views exist under the
  round-2 evidence root, and none is overridden by a timeline marker.
- Round 1 retains visible cardigan seams, buttons, zipper, and fur, but both
  engines still read as an orange-dominant room with weak subject separation.
- Round 2 makes Eevee subject/background separation visibly stronger and keeps
  garment/fur structure, but introduces pronounced full-frame high-frequency
  speckling at the diagnostic resolution.
- Round 2 Cycles remains pale orange and low-contrast despite zero measured
  clipping. White clothing edges and seams remain visible, but face, cardigan,
  wall, and shelving still occupy an overly compressed warm tonal range.
- The Eevee/Cycles mismatch is too large to approve. No 960x540 final evidence
  was attempted after the two-round stop condition fired.

### Gate And Concerns

- Lighting evidence gate: **FAIL**, only Cycles standing/seated separation.
- Current tracked scene validator:
  `/tmp/task6b-current-validation.json`, exit `1`, `16` errors. The validator
  still expects orange RGB practical/downlight tints and `105 W` downlights,
  while the 6A builder and saved scene use neutral RGB and `36 W`; this contract
  drift predates 6B and was not changed after the stop condition.
- Full scene contract, server regression, final-resolution evidence, and rebuild
  were not rerun because 6B made no tracked implementation and the mandatory
  lighting gate had already failed.
- The measured Cycles response indicates indirect bounce from the broad key is
  lifting the back wall along with the face. A next pass needs one explicit
  transport-control change such as a closer key with inverse-square energy
  compensation, physical flag/barn-door geometry, or verified cross-engine
  light linking. Further energy-only tuning is not justified by these results.
- Commit: none. Existing Task 6/6A tracked modifications and untracked model,
  turnaround, and `.blend1` files remain preserved and unstaged.

## Final Resolution (2026-07-14)

Status: **PASS**. The earlier diagnostic stop and remaining-blocker sections are
superseded by the production rebuild and final-resolution evidence below.

### Root-Cause Fix

- The distant 180-degree area key was lighting the character and rear wall
  together. The production key is now physically closer to the subject at
  `(-1.10, -0.55, 2.55)`, `825 W`, `4500 K`, `1.6 m` square, with a `145` degree
  spread. This preserves a soft character key while limiting wall spill in both
  Eevee and Cycles.
- Room sources use a `0.45` scale: world `0.054`, window `63 W`, practicals
  `18.9/21.6 W`, downlights `16.2 W`, and reduced visible lamp emission.
- Eevee uses `128` production/QA samples. Authored Eevee exposure is
  `-2.769925`; Cycles uses an independent display exposure of `-4.0` while the
  linear evidence remains scene-referred.
- Timeline camera markers are isolated and restored for each QA render. Subject,
  face, practical, and background masks come from rendered/evaluated geometry.

### Final 960x540 Evidence

Evidence root: `/tmp/task6-final-evidence-v3`

| Mode / engine | Face Y | Background Y | Stops below face | Non-catchlight clip |
| --- | ---: | ---: | ---: | ---: |
| standing / Eevee | `1.861773` | `0.746176` | `1.319089` | `0.000000%` |
| standing / Cycles | `4.197344` | `1.589049` | `1.401313` | `0.000000%` |
| seated / Eevee | `1.772527` | `0.827033` | `1.099791` | `0.000000%` |
| seated / Cycles | `3.536725` | `1.632413` | `1.115408` | `0.000000%` |

All four records pass the `1.0..1.5` stop and `<0.5%` clipping gates. Requested
and active camera names match with zero timeline camera markers during render.
Standing medium/three-quarter and medium/wide MAE are `0.325546` and `0.217506`;
seated values are `0.315771` and `0.238362`, all above the `0.001` threshold.

### Visual Review

- Standing/seated medium, three-quarter, and wide frames retain the full hair
  tuft and safe hand framing appropriate to their shot sizes.
- White cardigan panels, knit seams, buttons, face fur, eyes, and hair remain
  legible. The character reads brighter and more neutral than the restrained
  warm wood/plaster background without flattening the room.
- Eevee and Cycles differ in surface smoothness as expected, but preserve the
  same lighting direction, color hierarchy, and composition.

### Review Fixes

- Enabled a `4500 K` camera white balance instead of cooling every authored
  source. The brightest 30 percent of subject pixels now pass explicit warm-
  neutral chroma gates in every mode/engine: red/blue `1.1068..1.1932` and
  red/green `1.0755..1.1291`. White cardigan and facial fur no longer inherit
  the previous room-wide orange cast, while `2700 K` practicals stay warm.
- Every beauty PNG, linear EXR, empty-room control, subject/face/background/
  practical mask, and ID matte now carries a verified SHA-256 in the evidence
  payload. Validation rejects missing or changed artifacts, non-positive masks,
  contradictory median RGB ratios, and stop values inconsistent with the
  reported scene-linear luminance.
- The Blender test entrypoint opens the canonical production scene when invoked
  from `--factory-startup` and explicitly exits nonzero on any test exception.

### Verification

```text
Final lighting evidence: PASS, 960x540, 12 beauty/control renders
Warm studio validator: 0 errors, 0 warnings
Blender scene contract: 25 PASS
Real standing/seated character validation: success=true
Python MCP server: 90 PASS, 2 skipped
Python warm studio contract: 11 PASS
python3 -m py_compile: PASS
git diff --check: PASS
```
