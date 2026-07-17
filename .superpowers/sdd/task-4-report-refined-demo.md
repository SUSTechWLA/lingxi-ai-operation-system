# Task 4 Third-Review Fix Final Report

Status: **DONE**

The full rerender passed visual sign-off and automated Blender, media, oral-mask,
gesture, and encoded-audio checks. The publisher is bound to the exact signed v3 artifact,
which was atomically published over the final path. Generated binaries and evidence remain
untracked.

## Signed Artifact

- Signed staging: `outputs/final/refined-aroll-evidence/production-third-review-v3/ip_layer.mp4`
- Published final: `outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p.mp4`
- SHA-256: `73faca148d127e0e0f1e990846b9d6933c3f1dcebcdcfea52948dc2b8c7e4448`
- Size: 9,388,433 bytes
- Permissions: `0644`
- Action extrema: `outputs/final/refined-aroll-evidence/third-review-extrema-v3/gesture-extrema-contact-sheet.png`
- Hand close-ups: `outputs/final/refined-aroll-evidence/third-review-extrema-v3/hand-closeups.png`
- Oral extrema: `outputs/final/refined-aroll-evidence/third-review-extrema-v3/oral-extrema-contact-sheet.png`
- Final media evidence: `outputs/final/refined-aroll-evidence/production-third-review-v3/evidence/`

The action contact-sheet cell order is:

1. wave phase 1
2. wave phase 2
3. wave phase 3
4. open palm
5. count three
6. pinch

## Reproducible Recipe

The tracked recipe in `mcp/ip_avatar_3d/front_talking_demo.py` is now exact:

- Duration: 15.06 seconds
- Resolution: 1920x1080
- Frame rate: 30 fps
- Presentation: standing
- Camera: `front_talking`
- B-roll windows: none
- Required visemes: A, E, O, MBP
- Permanent voice: `gpt_sovits_local/main_ip_warm_knowledge_host_v1`

Required action sequence:

1. `Aroll_Greeting_Wave`
2. `Aroll_OpenPalm_Explain`
3. `Aroll_Count_Three`
4. `Aroll_Pinch_Detail`
5. `Aroll_Transition_Reset`

## P1 Oral Assembly Fix

The second independent review found evaluated intersections that static rest-pose checks
missed: the tongue intersected the lower dental arch and cavity in A/E/O/U. Runtime
containment version 4 keeps every approved oral role at scale 1.0, moves the cavity deeper,
lowers the lower dental arch, and adds driven A/E/O/U lifts to the tongue and cavity.
Those lifts copy the source face viseme values through Blender drivers, so Rest and MBP are
unchanged while open visemes stay readable and collision-free.

Real Blender regression gates now require:

- At least 95% of native oral-role width and vertical thickness.
- No evaluated BVH overlap for tongue/upper teeth, tongue/lower teeth, tongue/upper gum,
  tongue/oral cavity, or upper/lower teeth in every A/E/O/U extreme.
- A/E/O/U each expose distinct cavity, dental, and tongue regions.
- Rest and MBP expose no oral-role pixels.
- Rendered 768x432 masks contain no detached oral specks or displaced cavity fragments.
- The tongue and cavity expose driven Mouth_A/E/O/U Shape Keys with source-viseme drivers.

The six-view visual gate is:
`outputs/final/refined-aroll-evidence/third-review-extrema-v3/oral-extrema-contact-sheet.png`.

## P2 Gesture Readability Fix

Source-rig action calibration now gives the wave a larger alternating wrist rotation while
holding the elbow and forearm stable. The pinch opposes the two outer three-digit tips,
instead of reading as an open claw. Open-palm and count-three remain distinct.

The real Blender rendered regression requires:

- Every wave phase keeps the same camera-facing palm orientation with score at least 0.75.
- Consecutive wave phases differ by at least 0.20 radians and 0.008 normalized screen width,
  while upper-arm and forearm drift remains below 0.03 radians.
- Open-palm score at least 0.90 and minimum fingertip separation at least 0.035.
- Count-three minimum fingertip separation at least 0.05.
- Pinch outer-tip distance is at most 58% of open/count-three distance, remains above 0.08
  normalized chain length, and no digit pair collapses below 0.05.
- Every required hand mask above 1,000 pixels and all fingertips inside frame.
- Count-three/pinch rendered-mask difference at least 0.0025.

The test passed for `Aroll_Greeting_Wave`, `Aroll_OpenPalm_Explain`,
`Aroll_Count_Three`, and `Aroll_Pinch_Detail`. The six-image extrema gate was reviewed
before the final 451-frame render was composed.

Final visual sign-off covered opening, three wave phases, open-palm explanation, count
three, pinch, semantic repeats, and final reset. It confirmed clean Rest/MBP, readable
cavity/dental/tongue layers, camera-facing palms, real wrist oscillation, converged pinch,
and no clipping, inversion, black limbs, or end-frame pixel corruption.

## P3 Encoded Audio Fix

The previous `+0.2 dB` mastered-audio compose compensation produced `-16.51 LUFS` on the
production narration and failed the final production gate. A new behavioral regression now
generates deterministic mastered WAV audio, composes an actual H.264/AAC MP4 through the
production path, and validates measured encoded LUFS and true peak.

That test failed at `-16.56 LUFS` with `+0.2 dB` and passes with `+0.4 dB`. The limiter and
true-peak gate are unchanged. Final published audio measures:

- Integrated loudness: -16.38 LUFS
- True peak: -2.24 dBTP
- Loudness range: 1.1 LU
- Sample rate: 48 kHz
- Channels: mono

## Media Verification

- H.264 High, yuv420p, 1920x1080
- 30 fps constant frame rate
- 451 source PNGs, no gaps
- 451 encoded frames and 451 decoded frame hashes
- 0 adjacent exact duplicate decoded frames
- Video duration: 15.033333 seconds
- Audio duration: 15.018 seconds
- A/V duration delta: 0.015333 seconds, below one 30 fps frame
- AAC LC, 48 kHz mono
- Full `ffmpeg -xerror` decode: passed
- Delivery contract validation: passed

Evidence:

- `outputs/final/refined-aroll-evidence/production-third-review-v3/evidence/final-v3-ffprobe.json`
- `outputs/final/refined-aroll-evidence/production-third-review-v3/evidence/final-v3.framemd5`
- `outputs/final/refined-aroll-evidence/production-third-review-v3/evidence/final-v3.sha256`
- `outputs/final/refined-aroll-evidence/production-third-review-v3/evidence/final-v3-media-evidence.json`

## Verification Matrix

CPython:

```bash
python3 -m unittest -v mcp.ip_avatar_3d.test_front_talking_demo mcp.ip_avatar_3d.test_server
PYTHONPATH=mcp/ip_avatar_3d python3 -m unittest -v mcp/ip_avatar_3d/test_aroll_actions.py
```

Results:

- Front/server: 117 tests, 6 expected Blender-only skips, 0 failures/errors.
- A-roll action contracts: 15 tests, 0 failures/errors.

Blender 5.1.2:

```bash
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup --python mcp/ip_avatar_3d/test_front_talking_demo.py
```

Result: 13 tests, 0 failures/errors. This includes real warm-studio runtime append,
A/E/O/U evaluated oral BVH collision checks, rendered oral masks, multi-phase wave and
pinch semantics, rendered hand silhouettes, and rollback.

Go integration packages:

```bash
(cd local-backend && go test ./internal/localmcp ./internal/localagent ./internal/localrunner)
(cd cloud-backend && go test ./internal/core/agentruntime ./internal/core/localrunner ./internal/core/worker/tool/builtin)
```

Result: all six package targets passed.

## Atomic Publication

The tracked publisher binds:

- `SIGNED_OFF_STAGING_RELATIVE_PATH` to the v3 staging video.
- `SIGNED_OFF_VIDEO_SHA256` to `73faca148d127e0e0f1e990846b9d6933c3f1dcebcdcfea52948dc2b8c7e4448`.
- `FINAL_VIDEO_RELATIVE_PATH` to `outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p.mp4`.

Publication validated the staging hash before copying, wrote and `fsync`ed a same-directory
temporary file, preserved mode `0644`, replaced the final path with `os.replace`, and
revalidated the final hash. Signed staging and final bytes, decoded frame hashes, and media
metrics match exactly.
