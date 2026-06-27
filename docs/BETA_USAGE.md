# Tangying Video Agent Beta-0.2 Usage

## Beta Positioning

Beta-0.2 is a small-scope video creation director workspace for personal use and 1-3 familiar creator testers.

The supported loop is:

1. Enter a video idea.
2. Start a Dynamic Agent Run.
3. Review stage artifacts.
4. Approve, reject, edit, or regenerate.
5. Inspect trace and artifacts.
6. Copy or export publish materials.

## Supported

- Voice/graphic short-video planning and production artifacts.
- AIGC shot pre-production up to story, script, shot list, character/scene/prop notes, keyframe prompts, and video prompts.
- Dynamic Agent Runtime with PlanGuard, PlanCompiler, transient DAG execution, Quality Gate, and Artifact Review.
- Director Studio stage view, review view, trace view, artifact view, and export view.
- Manual publish-material export for Xiaohongshu and Bilibili.
- Local Runner and HyperFrames path when local services are available.
- Video-project-scoped assistant endpoints for project questions, artifact revision guidance, and stage explanation.

## Unsupported

- `/api/chat/*` global chat.
- `/api/bid/*` bid/proposal workflows.
- Automatic publishing.
- Automatic operation review.
- Video question answering.
- Long-video understanding.
- Full asset-library automation.
- Multi-user approval workflows.
- Heavy VideoAgent dependencies such as CosyVoice, DiffSinger, ImageBind, fish-speech, seed-vc, or VideoRAG.

## Startup

Start cloud backend:

```bash
cd cloud-backend
cp .env.example .env
docker compose up -d
go build -o build/tangying-ai-os cmd/tangying-ai-os/main.go
./build/tangying-ai-os
```

Start local backend:

```bash
bash scripts/start-local-backend.sh
```

Start frontend:

```bash
cd frontend
npm install
npm run dev
```

## First Use

1. Log in.
2. Open Director Studio.
3. Check the preflight status.
4. Enter a video idea and duration.
5. Click start.
6. Review pending stage artifacts.
7. Use approve, reject, edit submit, or regenerate.
8. Open Trace to inspect node status and errors.
9. Open Artifacts to copy artifact metadata.
10. Open Export to copy Xiaohongshu/Bilibili publish materials or download Markdown/JSON.

## Beta Test Cases

Run the fixed cases under `docs/beta-test-cases/`:

- `case-001-voice-workflow.md`
- `case-002-aigc-shot-workflow.md`
- `case-003-review-reject-regenerate.md`
- `case-004-local-runner-preflight.md`
- `case-005-publish-copy-export.md`

Use `beta-test-report-template.md` to record results.

## Common Errors

`RENDER_DEPENDENCY_MISSING`: preview, composition, or Local Runner readiness is missing before final render.

`ARTIFACT_MANIFEST_INVALID`: a local tool returned incomplete artifact metadata.

`CRITICAL_ARTIFACT_SYNC_FAILED`: a required artifact could not be written into the artifact index.

`PACKAGE_DEPENDENCY_MISSING`: final video or quality report is missing before packaging.

Suggested actions:

- Retry the current stage.
- Edit the input and regenerate.
- Check cloud API base configuration.
- Check local-backend health.
- Check Local Runner and HyperFrames availability.
- Use fake mode only for test runs when model provider configuration is incomplete.

## Feedback

Collect feedback with these questions:

1. What video idea did you enter?
2. Did the system generate usable content?
3. Which step got stuck?
4. Was the generated result usable?
5. Which page was hardest to understand?
6. Was the error message understandable?
7. Did you get publish-ready material?
8. What should the next beta version improve first?

## Limitations

Beta-0.2 is not a commercial launch. It is a controlled technical beta for proving the end-to-end video creation loop. Generated videos may still need manual editing, manual checking, and manual publishing.
