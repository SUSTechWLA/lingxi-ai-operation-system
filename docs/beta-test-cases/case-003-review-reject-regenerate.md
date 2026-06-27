# Case 003: Review Reject And Regenerate

## Input

Use an existing run with a pending review.

## Steps

1. Open Director Studio.
2. Go to Review.
3. Enter a rejection comment.
4. Click reject.
5. Refresh run state.
6. Trigger regenerate with a concrete hint.

## Expected Result

- Review action records a rejected decision.
- Downstream artifacts are marked stale when applicable.
- Regenerate returns a clear success or structured error.
- Trace and error panel expose node, stage, tool, artifact kind, or raw error when available.

## Pass Criteria

- `reject_available`: yes
- `regenerate_available`: yes
- `error_message_readable`: yes
- `stale_artifact_warning_visible`: yes when downstream artifacts exist
