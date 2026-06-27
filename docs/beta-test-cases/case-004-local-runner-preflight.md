# Case 004: Local Runner Preflight

## Input

Open Director Studio before starting a run.

## Steps

1. Start cloud backend.
2. Start frontend.
3. Test once with local-backend stopped.
4. Start local-backend and test again.
5. If HyperFrames is unavailable, start a script/prompt-only flow.

## Expected Result

- Preflight shows cloud, local runner, HyperFrames, local tool, and warning status.
- Local-backend offline does not block pure text generation.
- HyperFrames offline does not block script or prompt generation.
- Missing model provider blocks real generation unless fake mode is intentionally used.

## Pass Criteria

- `preflight_visible`: yes
- `local_offline_readable`: yes
- `script_prompt_path_available`: yes
- `render_dependency_warning_visible`: yes when render prerequisites are missing
