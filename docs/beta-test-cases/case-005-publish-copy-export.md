# Case 005: Publish Copy Export

## Input

Use a run with script/storyboard/prompt artifacts, with or without final video.

## Steps

1. Open Export.
2. Review Xiaohongshu publish material.
3. Review Bilibili publish material.
4. Copy each field.
5. Download Markdown.
6. Download JSON.

## Expected Result

- Xiaohongshu and Bilibili sections are visible.
- Each platform has title, description, tags, cover text, and publish tips.
- Each field has a copy button.
- Markdown and JSON export buttons work.
- No automatic publishing is triggered.

## Pass Criteria

- `xiaohongshu_copy_visible`: yes
- `bilibili_copy_visible`: yes
- `field_copy_available`: yes
- `markdown_export_available`: yes
- `json_export_available`: yes
- `auto_publish_absent`: yes
