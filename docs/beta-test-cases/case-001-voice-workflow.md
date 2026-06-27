# Case 001: Voice/Graphic Workflow

## Input

```text
请帮我做一个 45 秒视频，讲智能体改变的是工作流。
```

## Expected Result

- A Dynamic Agent Run starts and shows a Run ID.
- VideoIntent selects `voice_visual`.
- Trace shows node status updates.
- At least three artifacts are visible.
- A pending review appears.
- Reject or regenerate returns a clear result or error.
- Export page shows publish copy.

## Pass Criteria

- `run_id_visible`: yes
- `trace_visible`: yes
- `pending_review_visible`: yes
- `artifact_count >= 3`
- `publish_copy_complete`: yes
- `/api/chat/*` and `/api/bid/*` are not used.
