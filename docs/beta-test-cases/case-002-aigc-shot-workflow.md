# Case 002: AIGC Shot Pre-Production

## Input

```text
我想做一个 60 秒动画短片，讲猫咪通过 0 和 1 传输“你好”的故事。
```

## Expected Result

- A Dynamic Agent Run starts and shows a Run ID.
- VideoIntent selects `aigc_shot`.
- The run produces story, script, shot list, character/scene/prop notes, keyframe prompts, and video prompts.
- Manual asset information can be attached through the manual asset manifest path when assets are imported.
- Export page shows title, description, tags, and cover copy.

## Pass Criteria

- `intent.videoType == aigc_shot`
- `story_or_script_present`: yes
- `shot_list_present`: yes
- `prompt_artifacts_present`: yes
- `publish_copy_complete`: yes
- No automatic video provider or auto-publish step is required.
