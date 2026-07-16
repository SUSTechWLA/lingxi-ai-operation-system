# Sloth Warm Studio Integration Closure Report

Date: 2026-07-14
Branch: `codex/sloth-warm-studio-integration`
Status: COMPLETE

## Delivered

- Integrated the canonical sloth A-roll character into the packed warm studio.
- Added explicit `standing`, `seated`, and normalized `auto` presentation modes.
- Preserved the source humanoid rig, original materials, facial topology, jaw, eyes,
  tongue, wrists, and independently animated three-segment source fingers.
- Added deterministic character placement, floor contact, collision clearance, and
  camera safe-frame calibration for both modes.
- Locked production speech to local GPT-SoVITS voice
  `main_ip_warm_knowledge_host_v1`; production fallback remains disabled.
- Published two standalone demos, one dual-mode reel, two contact sheets, one
  lighting comparison, and one machine-readable provenance/QA report.

## Published Outputs

| Artifact | SHA-256 |
| --- | --- |
| `outputs/Sloth_WarmStudio_Standing_Demo_1080p.mp4` | `84f0035f31b65bc1d1e10564eba3ef3d0a03721f8915458443a1c39d8eb24e59` |
| `outputs/Sloth_WarmStudio_Seated_Demo_1080p.mp4` | `18afba176650173539ff252a5c5639063ec54469db3b2b50c4b2e6c63094c31c` |
| `outputs/Sloth_WarmStudio_DualMode_Reel_1080p.mp4` | `9c8946921ec102ab01d21d545dc1128d639828578073d207371b9be1d28e1678` |
| `outputs/Sloth_WarmStudio_Standing_ContactSheet.png` | `68aeca2438cc7df3b99ed2990b16946bbe3b96e32af5e43c9b1f99428d3f54c0` |
| `outputs/Sloth_WarmStudio_Seated_ContactSheet.png` | `da86facf6446dbca56f359f4df1691552d59b0e6d0b95b201d7b58a4da9790af` |
| `outputs/Sloth_WarmStudio_Lighting_Comparison.png` | `afe26f516adbfe2719ea7b64430a2539a0ceb5b45d6974be064aab4bab5156f6` |
| `outputs/Sloth_WarmStudio_Integration_Report.json` | `c40222194ae1fc3ccaaf857581e1e96abd24b39d07ffbd5fa7995ba24552f15f` |

Generated binaries and videos remain untracked by repository policy.

## Media QA

| Mode | Duration | Frames | Loudness | True peak |
| --- | ---: | ---: | ---: | ---: |
| Standing | 15.733 s | 472 | -16.4 LUFS | -2.3 dBFS |
| Seated | 12.833 s | 385 | -16.3 LUFS | -2.3 dBFS |
| Dual-mode reel | 28.567 s | 857 | -16.3 LUFS | -1.6 dBFS |

All three files independently probe as H.264, yuv420p, 1920x1080, 30 fps CFR,
with AAC 48 kHz mono audio. FFmpeg freeze detection found no adjacent frozen
segments of 0.1 seconds or longer.

## Visual And Geometry QA

- Standing and seated contact sheets were inspected at original resolution.
- Face and clothing remain brighter than the room while practical lights and
  wood/material detail remain visible.
- Measured room exposure is 1.0 to 1.5 stops below the face; highlight clipping
  gates pass for Eevee and Cycles reference renders in both modes.
- Character-to-desk, character-to-chair, and hand-to-set intersection reports
  pass for both modes.
- Standing feet and seated foot targets pass bounded floor-clearance checks.
- Medium cameras retain the complete head and both hands through sampled limb
  motion extrema; wide and three-quarter cuts provide usable editorial variety.
- Greeting, explanation, neutral, side-angle, mouth, eye, wrist, elbow, and
  independent source-finger poses were visually sampled without visible tearing.

## Regression Evidence

- `test_warm_studio_contract.py`: 12 passed.
- `test_warm_studio_demo.py`: 4 passed.
- `test_voice_policy.py`: 10 passed.
- `test_gpt_sovits_client.py`: 11 passed.
- `test_server.py`: 90 passed, 2 skipped.
- `go test ./...` in `local-backend`: all packages passed.
- `test_blender_character_rig.py`: 27 passed.
- `test_blender_hand_refinement.py`: completed all rendered hand, expression,
  viseme, and 15-action QA views with exit code 0.
- `test_blender_scene_contract.py`: 27 passed.

## Accepted Limitations

- The source character has three authored digits per hand rather than a human
  five-digit hand; every available digit has independent proximal, middle, and
  distal control.
- Standing and seated are separate shot modes. No automatic stand-to-sit
  transition is included.
- Production voice generation requires the pinned local GPT-SoVITS bundle and
  reference voice; failure is intentional when that bundle is unavailable.
- Eevee is the production renderer for 1080p throughput. Cycles remains a still
  lighting reference, not the animation renderer.
