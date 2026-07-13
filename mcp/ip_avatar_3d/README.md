# IP Avatar 3D MCP

Standard stdio MCP provider for rendering a 3D cartoon IP talking-video layer from:

- narration script
- local GLB / GLTF model or rigged FBX source
- optional narration audio
- optional static background or authored Blender studio

Tangying core should not know the implementation details. It calls this provider through `LOCAL_MCP_TOOL_CALL`, receives a playable IP layer video, then previews and composes that artifact in the normal shot pipeline.

## Tools

| Logical tool | MCP tool | Purpose |
|---|---|---|
| `ip_avatar_3d.check_status` | `check_status` | Check Blender / FFmpeg / FFprobe availability. |
| `ip_avatar_3d.validate_character_asset` | `validate_character_asset` | Validate GLB skin, semantic bones, visemes, and profile assets before rendering. |
| `ip_avatar_3d.plan_motion` | `plan_motion` | Convert narration into lip-sync and motion timelines. |
| `ip_avatar_3d.render_talking_video` | `render_talking_video` | Render a GLB/GLTF/FBX avatar to `ip_layer.mp4`. |

## Register Provider

```bash
python3 -m pip install -r mcp/ip_avatar_3d/requirements.txt

curl -X PUT http://127.0.0.1:18080/api/local/mcp-providers \
  -H 'Content-Type: application/json' \
  -d '{
    "providers": [
      {
        "id": "ip_avatar_3d",
        "label": "IP Avatar 3D MCP",
        "transport": "stdio",
        "command": "python3",
        "args": ["/absolute/path/to/mcp/ip_avatar_3d/server.py"],
        "toolPrefix": "ip_avatar_3d.",
        "enabled": true
      }
    ]
  }'
```

If Blender is not on `PATH`, set:

```bash
export TANGYING_BLENDER_BIN=/Applications/Blender.app/Contents/MacOS/Blender
```

## Direct Call Arguments

```json
{
  "script": "今天分享一个值得关注的观点。",
  "characterProfilePath": "/absolute/path/to/ip形象/main_ip/character-profile.json",
  "modelPath": "/absolute/path/to/main-ip-rigged.glb",
  "audioPath": "",
  "sceneBlendPath": "/absolute/path/to/editorial-news-studio.blend",
  "outputDir": "tmp/ip_avatar_3d_main_ip_demo",
  "durationSec": 8,
  "fps": 30,
  "width": 2560,
  "height": 1440,
  "qualityPreset": "production_2k",
  "faceScreenMode": "source",
  "rigMode": "auto",
  "preserveExistingRig": true,
  "enhanceExistingRig": true,
  "mouthMode": "auto",
  "facialDetailMode": "rich",
  "facialTopologyMode": "source_retopology",
  "cameraPreset": "medium",
  "lightingPreset": "editorial_soft",
  "renderEngine": "BLENDER_EEVEE_NEXT",
  "motionStyle": "expressive"
}
```

`modelPath` may be omitted after `model.path` is filled in the character profile.

Returned fields include `videoPath`, `localPath`, `previewImagePath`, `motionPlanPath`, `subtitlePath`, `riggedBlendPath`, `riggedGlbPath`, `rigReportPath`, and `renderReportPath`.

## Notes

- First version uses Blender background rendering, not AIGC video generation.
- The default production render is QHD 2K (`2560x1440`) with 128-sample Eevee rendering and H.264 CRF 16 encoding. Use `qualityPreset=preview` only for fast layout checks.
- If `audioPath` is omitted, the provider uses the voice pinned by the character profile. Supported providers are HeyGen, ElevenLabs, Kokoro, and macOS Apple voices. The main sloth profile pins `Eddy (中文（中国大陆）)` and applies warm knowledge-host mastering so repeated videos keep the same voice, pace, EQ, compression, and loudness.
- `facialTopologyMode=source_retopology` traces and splits the original mouth groove, keeps lips and eyelids on the textured source mesh, and adds only hidden oral-cavity, teeth, and tongue geometry. It also augments preserved humanoid rigs with jaw, eye, and three-segment tongue bones.
- A profile that explicitly pins a provider fails when that provider cannot synthesize; it does not silently downgrade to macOS `say`. The `auto` provider retains `say` only as a last-resort preview fallback.
- The default `rigMode=auto` preserves an input Armature, skin weights, materials, and existing actions. Common Generic/Mixamo-style bone names are mapped to presenter controls. `enhanceExistingRig=true` can add two-segment three-digit hand articulation when a rigged FBX/GLB only contains wrist bones. A generated cartoon rig is only used when the model has no Armature.
- `mouthMode=auto` reuses existing `Mouth_*` shape keys. For organic characters with a modeled mouth groove, combine `source_mesh_visemes` with `source_retopology`: visemes deform the split source lip boundaries and expose the real oral interior without a visible replacement mouth. Independent mouth geometry is only a fallback for compatible screen-face characters.
- The presenter assets include shoulders, elbows, wrists, two-segment three-digit hands, multi-axis limbs, source-mesh visemes, restrained eye/brow/cheek shapes, and reusable gesture/expression Actions. The exported GLB keeps deform bones, morphs, and actions.
- When `sceneBlendPath` is supplied, Blender opens the authored `.blend`, imports the character at `IP_Character_Spawn`, focuses cameras on `IP_Focus_Head`, and renders the character and set together. This is the preferred A-roll path because the character receives the set lighting and casts real floor/wall shadows.
- Authored studios should expose `Camera_Wide`, `Camera_Medium`, and `Camera_Close`. `cameraPreset=auto` creates restrained timeline cuts between those cameras; a fixed `wide`, `medium`, or `close` preset is also supported.
- `lightingPreset` scales lights that store `ip_base_energy` and `ip_light_role`. Supported values are `editorial_soft`, `editorial_crisp`, `night_analysis`, and `scene_default`.
- `renderEngine` supports `BLENDER_EEVEE_NEXT` (mapped to the installed Eevee enum) and `CYCLES`. Eevee is recommended for full talking videos; Cycles is intended for short high-quality shots.
- When `backgroundPath` is supplied, Blender renders an RGBA avatar pass and FFmpeg composites that pass over one static A-roll plate.
- A `.blend` scene takes precedence when both scene and plate are configured. `backgroundBrightness` only affects the static-plate fallback.
- The bundled `ip形象/main_ip/scenes/editorial-news-studio.blend` is the dark editorial/news-analysis A-roll default for knowledge sharing, opinion commentary, and current affairs.
