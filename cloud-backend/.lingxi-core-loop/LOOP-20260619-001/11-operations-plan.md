# 11 Operations Plan

## Metrics To Watch

- Skill load failures and unhealthy skill count.
- Workflow template upsert count and errors.
- CONTROL nodes waiting for review.
- External tool missing/HTTP error rate.
- Long-running node heartbeat timeout rate.
- Workflow run success/failure/cancelled rate.
- Artifact version growth for video projects.

## Runbook

- Missing external tool: register manifest/endpoint, then retry affected node.
- CONTROL stuck: inspect `/api/task/:taskId/pause-reason`, review artifact, call node success or failure.
- Heartbeat timeout: inspect external tool logs by `task_id` and `node_id`, retry node after root cause.
