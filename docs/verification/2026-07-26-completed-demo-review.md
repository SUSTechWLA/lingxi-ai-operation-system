# Completed demo review verification

Date: 2026-07-27 (Asia/Shanghai)

## Outcome

The creator-review changes compile, lint, and pass every frontend contract suite. The newest matching completed demo that is already persisted on this machine is a valid 30-second MP4 with both video and audio streams. The rebuilt macOS application is version 0.2.1, contains the current local-agent binary, passes strict code-signature verification, and is byte-identical to the application copied into the normal delivery directory.

End-to-end regeneration and interactive playback could not be executed in this run because this managed environment denies both loopback listeners and Computer Use access to the packaged application. Those are environment-level blockers: the Go packages compile successfully, while representative runtime tests fail before their first assertion when `httptest` attempts to bind `[::1]:0`.

## Automated verification

| Area | Command | Result |
| --- | --- | --- |
| Creator review contracts | `npm run test:creator` | PASS |
| Settings and theme | `npm run test:settings` | PASS |
| Diagnostics production modes | `npm run test:developer-build` | PASS |
| Frontend TypeScript and production bundle | `npm run build` | PASS |
| Frontend lint | `npm run lint` | PASS |
| Deployment/build contracts | `python3 scripts/test_one_click_deploy.py` | PASS, 9 tests |
| Cloud Go compile sweep | `go test -run '^$' ./...` | PASS |
| Local Go compile sweep | `go test -run '^$' ./...` | PASS |
| Cloud listener-dependent runtime test | representative focused `go test` | BLOCKED by sandbox `listen tcp6 [::1]:0: bind: operation not permitted` |
| Local listener-dependent runtime test | representative focused `go test` | BLOCKED by the same sandbox restriction |
| Patch hygiene | `git diff --check` | PASS |

The frontend checks cover readable creator projections, deterministic selected-text mapping, scoped revision behavior, completed-task Shot projection, local media URL handling, completed delivery recovery, and separation of creator views from Developer Diagnostics.

## Real completed demo evidence

The matching completed project was identified only in transient local inspection; its project, task, and artifact identifiers are intentionally omitted here.

| Evidence | Value |
| --- | --- |
| Duration | 30.000 seconds |
| Container | MP4 (`mov,mp4,m4a,3gp,3g2,mj2`) |
| Video | H.264, 1280x720, 15 fps |
| Audio | AAC, 48 kHz, mono |
| Streams | 2 |
| Shot plan | 5 Shots |
| Per-Shot packages | 5 |
| Per-Shot generation plans | 5 |
| Video prompts | 5 |
| Designed/required visual layers | 3 / 3 |
| A-roll packages | 1 |
| Persisted project outputs | 27 total: 14 images, 2 videos, 6 JSON data files, 3 presentation documents, 2 other render-support files |
| Local media artifacts | 2 |

The stored delivery metadata identifies this specific render as a fallback IP/storyboard composite and not production-eligible. It is suitable for validating the 30-second client review and media path, but it must not be represented as a final commercial-quality regeneration.

The cloud delivery version and a version increment could not be read offline. No replacement version was invented and no upstream script, Shot, prompt, or asset was changed.

## Packaged client

| Check | Result |
| --- | --- |
| Application version | 0.2.1 |
| Electron runtime | 43.1.0 |
| Architecture | arm64 |
| Embedded local agent | arm64 Mach-O, present |
| Strict deep code-signature check | PASS |
| Application directory package | PASS |
| Copy in normal delivery directory | PASS; Info.plist, app.asar, and local-agent binary match the verified build |
| DMG | BLOCKED; the managed sandbox denied DNS access needed by the packaging dependency |
| Computer Use | BLOCKED; host returned `Computer Use was not approved to use Tangying AI Video Creation Assistant` on the single allowed attempt |

The previous delivered application was retained in `.workspace-archive/task6-preupdate/`. An intermediate package built with the wrong Electron generation was retained separately in `.workspace-archive/task6-intermediate-electron33/` and was not delivered.

## Defects fixed during verification

- Replaced light-only diagnostic warning/error colors with semantic theme tokens so Developer Diagnostics remains legible in dark mode.
- Refreshed the generated review API type so review timestamps, reviewer identity, and comments match the backend contract.
- Corrected the escaped-text contract fixture and documented the colocated DOM-selection test helper for lint.
- Made the desktop build default to repository-writable Go and Electron Builder caches.
- Added a deployment contract test that prevents those cache defaults from regressing.

## Acceptance matrix

| Requirement | Status | Evidence or blocker |
| --- | --- | --- |
| Creator content is readable instead of raw JSON | PASS (automated) | Creator projection and client contract suite |
| Direct selected-text revision preserves surrounding text | PASS (automated) | UTF-16/escaped selection and scoped replacement contracts |
| Completed task has visible Shot structure | PASS (persisted + automated) | Five persisted Shots and completed Shot projection contracts |
| Images, voice, clips, and final media resolve through creator-safe URLs | PASS (automated) | Local media resolution, MIME, range, and state contracts |
| Final MP4 has valid metadata, video, and audio | PASS (file inspection) | `ffprobe` evidence above |
| In-client playback, seek, rate, volume, fullscreen, and download | BLOCKED | Computer Use host approval denied |
| Regenerate broken delivery and verify version increment | BLOCKED | Cloud/local services cannot bind loopback ports in this sandbox |
| Developer Diagnostics retains technical payloads | PASS (automated) | Diagnostics build checks for disabled, unset, and enabled modes |
| Installable `.app` delivery | PASS | Signed arm64 app version 0.2.1 copied to the delivery directory |
| DMG delivery | BLOCKED | Sandbox DNS restriction during packaging dependency resolution |

## Remaining action outside this sandbox

On a normal macOS host that permits local listeners and application automation:

1. Start the cloud and local services.
2. Open the newest matching item under completed/archived projects.
3. If the final delivery reports missing or unsupported media, choose `重新生成成片`.
4. Confirm the durable delivery version increases, then exercise play, seek, volume, rate, fullscreen, and download in the packaged app.

No code defect observed in this run remains hidden behind a passing result; environment-blocked checks are explicitly marked instead.
