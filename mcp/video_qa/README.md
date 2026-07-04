# Video QA MCP

Standard stdio MCP server for Tangying rendered-video QA.

## Tools

| Logical tool | MCP tool | Purpose |
|---|---|---|
| `video_qa.analyze_video` | `analyze_video` | Extract frames, build a contact sheet, produce shot-level QA reports, and generate a repair plan. |

## Run

```bash
python3 -m pip install -r mcp/video_qa/requirements.txt
python3 mcp/video_qa/server.py
```

`VIDEO_FRAME_QA` in the local runner starts this MCP server over stdio by default. The Go executor validates paths and calls `video_qa.analyze_video`; the MCP server owns the media analysis implementation.
