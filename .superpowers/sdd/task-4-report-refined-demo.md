# Task 4 Refined A-roll Demo: Final Production Report

Status: **DONE**

Visual sign-off passed for the corrected render. The signed-off staging artifact was
published atomically, the default tracked profile now selects the refined master, and the
final focused test/media matrix passed. Output binaries and generated evidence remain
untracked.

## Scope

- Refined master: `ip形象/main_ip/models/main-ip-aroll-master-refined.blend`
- Warm studio: `ip形象/main_ip/scenes/warm-sloth-studio-v1.blend`
- Runtime profile: `ip形象/main_ip/character-profile.json`
- Permanent voice: `gpt_sovits_local/main_ip_warm_knowledge_host_v1`
- Staging video: `outputs/final/refined-aroll-evidence/production-oral-fixed/ip_layer.mp4`
- Published final: `outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p.mp4`

## Failed Sign-off Reproduction

The failed production render visibly showed one or two pink curved strips below the
muzzle/chin at frames 16, 88, 150, 260, 350, 380, and 415.

Role-isolation renders proved the strips were generated oral geometry:

- Hiding `upper_gum` and `lower_gum` removed the two long arcs.
- Hiding `tongue` removed the remaining central strip.
- The oral roles have no Shape Keys or object actions. Their motion comes from the
  expected Head, Jaw, and Tongue armature weights.
- Source-mouth Shape Keys and jaw animation exposed the defect but did not create the
  displaced geometry.
- Runtime append matrices were valid after the existing dependency-graph update. The
  source lip boundary and oral assembly shared the expected world transform.

Root cause: the approved master oral assembly used character-scale vertical construction
offsets. Its lower roles extended outside the preserved source lip/muzzle boundary after
jaw deformation. This was a geometry-containment defect, not an append-order, parenting,
driver, or action-transform defect.

Evidence:

- `outputs/final/refined-aroll-evidence/oral-failure-before-contact-sheet.png`
- `outputs/final/refined-aroll-evidence/oral-isolates/frame_0016_role-comparison.png`
- `outputs/final/refined-aroll-evidence/runtime-oral-diagnostic.json`

## TDD Regression and Fix

Added Blender integration coverage in `test_front_talking_demo.py`. The test opens the
warm studio, runtime-appends the refined master through the production path, prepares the
existing rig, and evaluates all six oral roles at `A/E/O/U/MBP/closed` extrema.

RED result before the fix:

- 36 oral-role/viseme containment failures.
- Every required extremum had at least one role outside the evaluated source-mouth
  envelope.

GREEN result after the fix:

- 36/36 role/extremum combinations contained.
- Rest and MBP oral vertices remain behind the closed source-muzzle occlusion depth.
- Blender front integration suite: 8/8 passed.

The runtime-only fix fits the complete appended oral assembly around the preserved Basis
lip-boundary center, keeps all roles together, and preserves their depth ordering. It then
places the nearest oral surface behind the backmost source boundary plus a depth margin.
No approved master binary or source facial mesh is modified.

Applied runtime metrics in the rerendered `.blend`:

- containment version: 1
- X scale: 0.6984975726
- Z scale: 0.2125639422
- rearward depth shift: 0.0240112958 m
- roles: oral cavity, upper/lower teeth, upper/lower gums, tongue

## Rerender

The production render was regenerated from the same script and deterministic permanent
voice configuration. It retained the same 15.06-second motion plan and 451 source frames.

Actions:

1. `Aroll_Greeting_Wave`
2. `Aroll_OpenPalm_Explain`
3. `Aroll_Count_Three`
4. `Aroll_Pinch_Detail`
5. `Aroll_Transition_Reset`

Voice provenance:

- provider/voice: `gpt_sovits_local/main_ip_warm_knowledge_host_v1`
- seed: 20260714
- reference SHA-256: `0119b8a407ed1be5731db699d6166b7e4c86de540435c69bf4fb5229415382d8`
- GPT weights SHA-256: `87133414860ea14ff6620c483a3db5ed07b44be42e2c3fcdad65523a729a745a`
- SoVITS weights SHA-256: `d42a22bbbf65fb2bbdd45ad6a66841156977db45c7aabe0a6992ff378d9c7d3b`
- Raw and mastered audio are byte-identical to the previous render.

Rerendered video SHA-256:
`0ce20701c6df6498ca471d64f6d3dd5d41c385f8c1387e821798a8e82cadd8c1`

## Exact Before/After Review

Paired sheet, top row before and bottom row after, ordered left-to-right as
16/88/150/260/350/380/415:

- `outputs/final/refined-aroll-evidence/oral-before-after-user-frames.png`

Results:

| Frame | Before | After |
| ---: | --- | --- |
| 16 | Pink gum/tongue strip below chin | No escaped pink geometry; oral interior centered |
| 88 | Two pink arcs below muzzle | No escaped pink geometry |
| 150 | Wide pink lower arc | No escaped pink geometry |
| 260 | Pink arc and central strip | No escaped pink geometry |
| 350 | Pink lower strip | No escaped pink geometry |
| 380 | Two pink strips | No escaped pink geometry |
| 415 | Pink lower arcs | No escaped pink geometry |

User visual confirmation received after rerender: the external pink oral strips are absent
in frames 1, 15, 16, 20, 39, 40, 51, 88, 125, 150, 190, 225, 260, 300, 350,
361, 380, 415, and 450. Visual sign-off passed and finalization was authorized.

Viseme extrema inspected:

- A: frame 1, jaw 0.25 rad
- O: frame 16, jaw 0.163900003 rad
- closed: frame 21, jaw 0
- U: frame 40, jaw 0.080100000 rad
- MBP: frame 52, jaw 0
- E: frame 361, jaw 0.096479997 rad
- evidence: `outputs/final/refined-aroll-evidence/oral-fixed-viseme-extrema.png`

No fragmented teeth, teeth at Rest/MBP, escaped tongue/gums, oral clipping, or weak mouth
silhouette was observed at these extrema. Rest and MBP remain visually closed.

Gesture and framing extrema inspected:

- first/middle/last: frames 1/226/451
- wave: frames 37/50
- open-palm explanation: frame 95
- count-three: frame 150
- pinch: frame 200
- relaxed reset: frame 241
- evidence: `outputs/final/refined-aroll-evidence/oral-fixed-gesture-extrema.png`
- detailed count/pinch: `outputs/final/refined-aroll-evidence/oral-fixed-count-pinch-close.png`

Both hands remain visible where required. No finger collapse, palm inversion, wrist seam,
five-finger silhouette, black limb, clipped hand, hidden gesture, or bad front lighting was
observed.

## Media Verification

- H.264 High, yuv420p, 1920x1080
- `r_frame_rate=30/1`, `avg_frame_rate=30/1`
- 451 encoded and 451 decoded video frames
- 15.033333-second video/format duration
- AAC LC, 48 kHz, mono, 15.018-second audio duration
- A/V duration delta: 0.015333 seconds, below one 30 fps frame
- Full decode with `ffmpeg -xerror`: passed
- Source PNG sequence: 451, frame 1 through frame 451, 0 gaps/order errors
- Decoded adjacent exact duplicates: 0
- Independent loudness: -16.4 LUFS, 1.6 LU LRA, -2.2 dBFS true peak
- Pipeline loudness: -16.38 LUFS, 1.1 LU LRA, -2.24 dBTP

Evidence:

- `outputs/final/refined-aroll-evidence/oral-fixed-ffprobe.json`
- `outputs/final/refined-aroll-evidence/oral-fixed.framemd5`
- `outputs/final/refined-aroll-evidence/production-oral-fixed/render_report.json`
- `outputs/final/refined-aroll-evidence/production-oral-fixed/rig_report.json`

## Final Publication

Publication command:

```bash
python3 -c "import json, pathlib, sys; sys.path.insert(0, 'mcp/ip_avatar_3d'); import front_talking_demo as d; print(json.dumps(d.publish_signed_off_video(pathlib.Path.cwd()), indent=2))"
```

The publisher is bound in tracked code to the exact signed-off staging path, final path,
and SHA-256. It copies to a temporary file in the destination directory, preserves the
staging permissions, flushes and `fsync`s the file, then uses `os.replace` for atomic
publication. A mismatched staging hash fails before the final path is touched.

Result:

- final path: `outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p.mp4`
- staging/final SHA-256: `0ce20701c6df6498ca471d64f6d3dd5d41c385f8c1387e821798a8e82cadd8c1`
- size: 9,225,941 bytes
- final permissions: `0644`
- profile master: `models/main-ip-aroll-master-refined.blend`
- hash evidence: `outputs/final/refined-aroll-evidence/final-published.sha256`
- media evidence: `outputs/final/refined-aroll-evidence/final-published-media-evidence.json`
- ffprobe evidence: `outputs/final/refined-aroll-evidence/final-published-ffprobe.json`
- frame hashes: `outputs/final/refined-aroll-evidence/final-published.framemd5`

## TDD and Diff Review

Finalization regressions were exercised red before green:

1. Profile/media/publisher tests initially produced two assertion failures and one missing-
   API error: legacy master selected, 44.1 kHz accepted, and no signed-artifact publisher.
   All three pass after the focused changes.
2. Replacing `append_runtime_master_collection` with the shared append helper caused both
   warm-studio Blender tests to error with `persisted hand aesthetic geometry does not
   match current mesh`. The shared helper validates world-space hand geometry before the
   collection is linked. The runtime helper is therefore retained and covered by a real
   warm-studio regression.
3. Forced append validation failure initially left ten imported character objects plus
   dependencies in `bpy.data`. The corrected rollback snapshots Blender IDs and
   `batch_remove`s every newly appended datablock. The exact pre/post datablock set now
   matches.
4. Atomic publication initially inherited `mkstemp` mode `0600`. A focused regression
   caught this; publication now preserves staging mode `0644` without changing bytes.

Root-cause review found no evidence that actions, drivers, parenting, append/world
transform order, or Shape Key deltas created the pink strips. The containment fix remains
runtime-only, version-marked, and idempotent. It applies one coherent fit to all six oral
roles and is evaluated at every required viseme/jaw extremum. The approved refined master
binary was not edited.

## Exact Final Tests

Focused CPython:

```bash
python3 -m unittest -v mcp.ip_avatar_3d.test_front_talking_demo mcp.ip_avatar_3d.test_server
```

Result: 115 tests, 5 expected Blender-only skips, 0 failures/errors.

Blender 5.1.2:

```bash
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup --python mcp/ip_avatar_3d/test_front_talking_demo.py
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup --python mcp/ip_avatar_3d/test_blender_oral_refinement.py
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup --python mcp/ip_avatar_3d/test_blender_master_asset.py
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup --python mcp/ip_avatar_3d/test_blender_character_rig.py
```

Results: front-talking 12/12, oral refinement 6/6, refined-master publication 7/7,
character rig 32/32.

Go package equivalents for the current two-module repository:

```bash
(cd local-backend && go test ./internal/localmcp ./internal/localagent ./internal/localrunner)
(cd cloud-backend && go test ./internal/core/agentruntime ./internal/core/localrunner ./internal/core/worker/tool/builtin)
```

Result: all six package targets passed.

The brief's literal commands were also audited. They are stale environment commands, not
equivalent pass/fail gates:

```bash
python3 -m unittest discover -s mcp/ip_avatar_3d -p 'test_*.py' -v
go test ./service/localmcp ./gateway/api ./service/agentbridge
```

- CPython discovery: 219 tests, 8 skips, 2 loader errors, 0 assertion failures. Only
  `test_blender_master_asset.py` and `test_blender_oral_refinement.py` error because they
  intentionally require Blender's `bpy`; the same files pass 7/7 and 6/6 under Blender.
- Root Go command: exits before package selection because the repository root has no
  `go.mod`. The current local/cloud module package equivalents pass above.

## Final Media Commands

```bash
ffprobe -v error -count_frames -show_entries 'format=duration,size,bit_rate:stream=index,codec_type,codec_name,profile,width,height,pix_fmt,r_frame_rate,avg_frame_rate,time_base,duration,nb_frames,nb_read_frames,sample_rate,channels,channel_layout' -of json outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p.mp4
ffmpeg -v error -xerror -i outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p.mp4 -f null -
ffmpeg -v error -i outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p.mp4 -map 0:v:0 -an -f framemd5 outputs/final/refined-aroll-evidence/final-published.framemd5 -y
ffmpeg -nostats -i outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p.mp4 -filter_complex ebur128=peak=true -f null -
shasum -a 256 outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p.mp4 outputs/final/refined-aroll-evidence/production-oral-fixed/ip_layer.mp4
```

Results:

- H.264 High, yuv420p, 1920x1080
- `r_frame_rate=30/1`, `avg_frame_rate=30/1`: 30 fps CFR
- video duration 15.033333 seconds; format duration 15.033333 seconds
- 451 declared frames, 451 decoded frames
- AAC LC, 48 kHz, mono; audio duration 15.018 seconds
- A/V duration delta 0.015333 seconds, below one 30 fps frame
- full `ffmpeg -xerror` decode passed
- source sequence 1..451: 451 files, 0 gaps/order errors
- frame MD5: 451 frames, 0 adjacent exact duplicates
- independent audio: -16.4 LUFS integrated, 1.6 LU LRA, -2.2 dBFS true peak
- permanent actual/requested voice: `gpt_sovits_local/main_ip_warm_knowledge_host_v1`
- `git diff --check`: passed

## Commit Scope

Owned tracked integration scope:

- `.superpowers/sdd/task-4-report-refined-demo.md`
- `ip形象/main_ip/character-profile.json`
- `mcp/ip_avatar_3d/blender_renderer.py`
- `mcp/ip_avatar_3d/front_talking_demo.py`
- `mcp/ip_avatar_3d/oral_refinement.py`
- `mcp/ip_avatar_3d/server.py`
- `mcp/ip_avatar_3d/test_front_talking_demo.py`
- `mcp/ip_avatar_3d/test_server.py`

The user-owned `.superpowers/sdd/task-4-report.md` remains untouched and unstaged. Models,
voice files, videos, frames, media evidence, `outputs/`, and `tmp/` remain untracked and
unstaged.
