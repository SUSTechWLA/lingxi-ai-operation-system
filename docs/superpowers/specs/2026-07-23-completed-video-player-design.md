# Completed Video Player Design

## Goal

Let a user open a completed project from “我的视频” and immediately watch its latest finished video inside the desktop client.

## Interaction

- Completed and archived project cards use the action “观看成片” and open the `preview` step.
- The preview and delivery steps prefer the latest current video when no artifact has been explicitly selected.
- Explicit artifact choices still win, so users can continue reviewing JSON, Markdown, images, and historical versions.
- Video artifacts render through one reusable player with play/pause, seek, elapsed and total time, mute, volume, playback rate, fullscreen, and download.
- The player supports Space, Left/Right, M, and F keyboard shortcuts while focused.

## Architecture

`SimpleVideoPlayer.tsx` owns only media playback state and browser media APIs. `ArtifactProofingCanvas.tsx` and `PreviewDeliveryPanel.tsx` supply resolved media URLs and reuse the component. Pure creator-studio logic selects the project entry step and the default artifact, keeping routing and selection independently testable.

## Error Handling and Accessibility

- A media error replaces the player with the existing artifact fallback path.
- Buttons, ranges, and the rate selector have Chinese accessible labels.
- Keyboard shortcuts are ignored while a select or range control has focus.
- Fullscreen failure does not block playback.
- Download remains a normal link so Electron and browser builds use their native download behavior.

## Acceptance Criteria

1. A completed or archived card opens `#/videos/<project-id>/steps/preview` and says “观看成片”.
2. Preview defaults to a current, non-stale video even if a QA JSON artifact is the step current artifact.
3. An explicit artifact selection is never replaced by video preference.
4. Preview and artifact proofing use the same accessible player.
5. Creator tests, frontend build, lint, packaging, and installed-app smoke checks pass.
