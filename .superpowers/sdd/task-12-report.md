# Task 12 Report: Real-Script 1080p MCP Regression

## Outcome

Task 12 completed through the public `render_talking_video` MCP function using
the approved master, authored editorial studio, and pinned local GPT-SoVITS
voice `main_ip_warm_knowledge_host_v1`. The user explicitly selected 1080p for
this iteration instead of the plan's earlier 2K target.

The 28-second Mandarin knowledge-sharing script contains greeting,
enumeration, explanation, contrast, and conclusion. Its motion plan includes
greeting wave, open-palm explanation, count one/two/three, soft-fist emphasis,
head disagreement, think, nod/gaze beats, source-mouth visemes, and independent
eye squint.

## Delivery

- `outputs/MainIP_Sloth_Aroll_Release_Test_1080p.mp4`
- `outputs/MainIP_Sloth_Aroll_Action_Reel_1080p.mp4`
- `outputs/MainIP_Sloth_Aroll_Hand_QA_1080p.mp4`
- `outputs/MainIP_Sloth_Aroll_Face_QA.png`
- `outputs/MainIP_Sloth_Aroll_Release_Report.json`

The release video SHA-256 is
`246c2fc1fd2c220a30e7b98fef06a5181ac0ec2a6a34d6f64d3568a0bdfef8b8`.

## Release QA

- H.264, 1920x1080, CFR 30 fps, 840 frames, 28.000 seconds;
- AAC, 48 kHz mono;
- zero adjacent exact duplicate video frames;
- final decoded audio: `-16.39 LUFS`, `-2.31 dBTP`, `1.90 LU` LRA;
- production voice provenance includes matching reference, GPT, SoVITS, raw,
  and mastered hashes, `productionReady=true`, and no preview fallback;
- contact-sheet review found no hand spikes, torn mouth corners, body
  intersections, clipping, or character/background lighting mismatch.

Final AAC verification initially exposed a `-1.43 dBTP` encode overshoot. A
RED test was added for mastered-audio headroom. Final composition now bypasses
secondary loudnorm for mastered GPT-SoVITS audio and applies a restrained
limiter with encode headroom; the corrected output measures `-2.31 dBTP`.
Production renders now remeasure the final encoded file inside the MCP, delete
it, and fail closed before returning `ready` when LUFS or dBTP is out of range.
The persisted release report includes full voice hashes and final AAC metrics.

The final rig report also proves all 12 real finger-joint transition zones
(two joints across six digits) contain nonzero blended-vertex evidence; it no
longer reports a synthetic support-offset count.

## Regression

- MCP server: 84 passed, 2 skipped;
- voice policy: 10 passed;
- GPT-SoVITS client: 11 passed;
- focused Blender hand/QA runner: all 11 checks passed;
- full Blender character runner: all checks passed, including GLB round-trip;
- Blender scene contract: 5 passed;
- `go test ./...` under `local-backend`: passed;
- `python3 -m py_compile`: passed;
- `git diff --check`: passed.

The approved previous master was retained until source-surface QA passed, then
published atomically. Canonical and profile-local master hashes match.
