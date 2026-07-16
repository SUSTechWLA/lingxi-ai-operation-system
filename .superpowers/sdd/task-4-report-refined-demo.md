# Task 4 Independent-Review Fix Final Report

Status: **DONE**

The full rerender passed user visual sign-off and automated Blender, media, oral-mask,
gesture, and encoded-audio checks. The publisher is bound to the exact signed v2 artifact,
which was atomically published over the final path. Generated binaries and evidence remain
untracked.

## Signed Artifact

- Signed staging: `outputs/final/refined-aroll-evidence/production-review-fix-candidate-v2/ip_layer.mp4`
- Published final: `outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p.mp4`
- SHA-256: `f32056457a73f3467e580b2ea21cad40da67cf667ce560aa969c3a238d8770eb`
- Size: 9,382,211 bytes
- Permissions: `0644`
- Action extrema: `outputs/final/refined-aroll-evidence/production-review-fix-candidate-v2/evidence/candidate-action-extrema.png`
- Oral mask sheet: `outputs/final/refined-aroll-evidence/production-review-fix-candidate-v2/evidence/oral-mask-contact-sheet.png`
- Complete final evidence: `outputs/final/refined-aroll-evidence/production-review-fix-candidate-v2/evidence/final-published-v2-media-evidence.json`

The action contact-sheet cell order is:

1. frame 1, opening / A
2. frame 37, `Aroll_Greeting_Wave`
3. frame 95, `Aroll_OpenPalm_Explain`
4. frame 150, `Aroll_Count_Three`
5. frame 200, `Aroll_Pinch_Detail`
6. frame 241, `Aroll_Transition_Reset`
7. frame 361, E viseme / semantic count
8. frame 451, final reset

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

Root cause: runtime containment version 1 uniformly compressed the entire oral assembly to
approximately 21% of its native vertical thickness. That kept geometry behind the lips but
made the cavity, dental arches, gums, and tongue visually unreadable.

Runtime containment version 2 removes whole-assembly X/Z scaling. It preserves scale 1.0 and
uses role-specific local placement for the cavity, upper/lower gums, upper/lower teeth, and
tongue. Source lips and approved master geometry remain unchanged.

Real Blender regression gates now require:

- At least 95% of native oral-role width and vertical thickness.
- No evaluated BVH overlap for tongue/upper teeth, tongue/upper gum, or upper/lower teeth.
- A/E/O/U each expose distinct cavity, dental, and tongue regions.
- Rest and MBP expose no oral-role pixels.

Rendered mask results at 384x216:

| Viseme | Cavity pixels | Dental pixels | Tongue pixels | Result |
| --- | ---: | ---: | ---: | --- |
| A | 301 | 18 | 7 | pass |
| E | 198 | 20 | 14 | pass |
| O | 234 | 23 | 10 | pass |
| U | 170 | 14 | 12 | pass |
| Rest | 0 | 0 | 0 | pass, fully occluded |
| MBP | 0 | 0 | 0 | pass, fully occluded |

Thresholds are 150 cavity pixels, 10 dental pixels, and 6 tongue pixels for every open
viseme, with exactly 0 total oral pixels for Rest/MBP. The machine-readable report is:

`outputs/final/refined-aroll-evidence/production-review-fix-candidate-v2/evidence/oral-mask-metrics.json`

## P2 Gesture Readability Fix

Source-rig action calibration now gives the wave and open-palm explanation camera-facing
palms, keeps three digits separated, and makes count-three and pinch use clearly different
silhouettes. Wrist rotation and restrained curl limits remain compatible with close A-roll
framing.

The real Blender rendered regression requires:

- Wave palm-facing score at least 0.75 and minimum fingertip separation at least 0.02.
- Open-palm score at least 0.90 and minimum fingertip separation at least 0.035.
- Count-three minimum fingertip separation at least 0.05.
- Every required hand mask above 1,000 pixels and all fingertips inside frame.
- Count-three/pinch rendered-mask difference at least 0.0025.

The test passed for `Aroll_Greeting_Wave`, `Aroll_OpenPalm_Explain`,
`Aroll_Count_Three`, and `Aroll_Pinch_Detail`. The user also passed the ten-image
pre-rerender visual gate before the final 451-frame render was composed.

Final visual sign-off covered frames 1, 15, 20, 39, 40, 51, 88, 95, 125, 150, 190,
200, 225, 260, 270, 300, 317, 339, 361, 362, 385, 409, 434, and 450. The reviewer
confirmed clean Rest/MBP, readable cavity/dental/tongue layers in open visemes, no escaped
oral geometry, camera-facing wave and explanation palms, distinct count-three/pinch, and
no clipping, inversion, or black limbs.

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

- `outputs/final/refined-aroll-evidence/production-review-fix-candidate-v2/evidence/final-published-v2-ffprobe.json`
- `outputs/final/refined-aroll-evidence/production-review-fix-candidate-v2/evidence/final-published-v2.framemd5`
- `outputs/final/refined-aroll-evidence/production-review-fix-candidate-v2/evidence/final-published-v2.sha256`
- `outputs/final/refined-aroll-evidence/production-review-fix-candidate-v2/evidence/final-published-v2-media-evidence.json`

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
evaluated oral geometry, rendered oral masks, rendered hand silhouettes, and rollback.

Go integration packages:

```bash
(cd local-backend && go test ./internal/localmcp ./internal/localagent ./internal/localrunner)
(cd cloud-backend && go test ./internal/core/agentruntime ./internal/core/localrunner ./internal/core/worker/tool/builtin)
```

Result: all six package targets passed.

## Atomic Publication

The tracked publisher now binds:

- `SIGNED_OFF_STAGING_RELATIVE_PATH` to the v2 staging video.
- `SIGNED_OFF_VIDEO_SHA256` to `f32056457a73f3467e580b2ea21cad40da67cf667ce560aa969c3a238d8770eb`.
- `FINAL_VIDEO_RELATIVE_PATH` to `outputs/final/MainIP_Sloth_Refined_Aroll_Demo_1080p.mp4`.

Publication validated the staging hash before copying, wrote and `fsync`ed a same-directory
temporary file, preserved mode `0644`, replaced the final path with `os.replace`, and
revalidated the final hash. Signed staging and final bytes, decoded frame hashes, and media
metrics match exactly.
