# 04b Frontend Requirements

No frontend code is implemented in this loop.

## Required Views For Later Client Work

- `video_project_list`: list projects by mode/status, show current run and last updated time.
- `workflow_run_detail`: show stages, CONTROL nodes waiting for review, progress, trace URL, and artifact versions.
- `stage_review_panel`: display Markdown/image/audio/video artifacts, approve/reject/revise actions, and downstream rerun impact.
- `external_tool_health`: show missing tool registrations for a workflow before run starts.

## API Dependencies

- Existing `/api/skills`, `/api/workflows`, `/api/video-projects`, `/api/video-projects/:pid/workflow-runs`, `/api/task/:id`, `/api/trace/:taskId`.
- Existing manual approval path can use `/api/node/:nodeId/success` after displaying the waiting CONTROL node.
