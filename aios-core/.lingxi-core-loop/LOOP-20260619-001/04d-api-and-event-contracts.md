# 04d API And Event Contracts

## Existing APIs Used

- `GET /api/skills`
- `POST /api/skills/:name/:version/compile`
- `GET /api/workflows`
- `POST /api/workflows/:id/instantiate`
- `POST /api/video-projects/:pid/workflow-runs`
- `POST /api/node/:nodeId/success`
- `GET /api/task/:taskId`
- `GET /api/trace/:taskId`

## Event Behavior

- Non-CONTROL READY nodes still publish `ai.node.ready`.
- CONTROL READY nodes no longer publish `ai.node.ready`; task enters `PAUSED` with a review reason.
- Node success invokes dependency checking so manual approvals can continue the DAG.
- Review-required context type: `NODE_REVIEW_REQUIRED`.
- Condition skip context type: `NODE_SKIPPED`.
