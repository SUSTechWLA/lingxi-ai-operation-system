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
| `ip_avatar_3d.check_gpt_sovits_voice` | `check_gpt_sovits_voice` | Validate a pinned local GPT-SoVITS voice bundle and hashes without synthesis or network I/O. |
| `ip_avatar_3d.generate_voice_auditions` | `generate_voice_auditions` | Generate atomic, content-addressed HeyGen A/B/C auditions for blind selection. |
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
  "width": 1920,
  "height": 1080,
  "qualityPreset": "production_1080p",
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

## Local GPT-SoVITS Production Voice

`gpt_sovits_local` uses the official GPT-SoVITS `api_v2.py` contract. It calls
`GET /set_gpt_weights`, `GET /set_sovits_weights`, then `POST /tts` while
holding one process lock per endpoint so concurrent model switches cannot
interleave. The default endpoint is `http://127.0.0.1:9880`; non-loopback
endpoints are rejected unless `allowRemoteEndpoint` is explicitly enabled.

A publishable profile requires a stable nonempty `voiceId`,
`fallbackPolicy="error"`, an explicitly verified reference transcript, readable
reference/GPT/SoVITS files, a model version, and deterministic inference
settings. Expected SHA-256 values are optional, but any supplied value must
match before HTTP. The adapter validates a nonempty WAV response and reports
the actual reference, checkpoint, and generated-file hashes in production
metadata.

`$VARNAME` and `~` are expanded in local reference and checkpoint paths before
resolution. The default loopback restriction still prevents those private
resolved paths from being sent remotely unless `allowRemoteEndpoint` is
deliberately enabled.

```json
{
  "renderMode": "production",
  "provider": "gpt_sovits_local",
  "voiceId": "main_ip_warm_knowledge_host_v1",
  "fallbackPolicy": "error",
  "gptSovitsLocal": {
    "endpoint": "http://127.0.0.1:9880",
    "allowRemoteEndpoint": false,
    "referenceAudioPath": "voice/reference/main_ip_voice_ref_v1.wav",
    "expectedReferenceAudioSha256": "<sha256>",
    "promptText": "<human-verified exact transcript>",
    "promptTextVerified": true,
    "promptLanguage": "zh",
    "textLanguage": "zh",
    "gptWeightsPath": "$GPT_SOVITS_HOME/pretrained_models/model.ckpt",
    "expectedGptWeightsSha256": "<sha256>",
    "sovitsWeightsPath": "~/.local/share/tangying-aios/GPT-SoVITS/model.pth",
    "expectedSovitsWeightsSha256": "<sha256>",
    "modelVersion": "<trained-bundle-version>",
    "seed": 20260714,
    "timeoutSec": 120,
    "settings": {}
  }
}
```

The canonical main-IP profile pins `main_ip_warm_knowledge_host_v1`, its
human-verified prompt, the stable reference-clip hash, both checkpoint hashes,
`v2ProPlus`, and seed `20260714`. The source and stable reference WAVs remain
untracked local assets. `check_gpt_sovits_voice` reports `bundleReady=true`
when those local files match; preflight metadata keeps `productionReady=false`
because no generated output WAV exists yet.

For A-roll, explicit local synthesis is mastered to a separate 48 kHz mono
PCM16 WAV with restrained 55 Hz high-pass and 18 kHz low-pass filters, gentle
1.5:1 compression, and `loudnorm=I=-16:TP=-1.5:LRA=7`. The raw generated hash
is retained. The mastered hash and measured integrated loudness, true peak,
and loudness range are added to provenance. The mastered path is returned only
when it parses as the required WAV format, measures within `-16 +/-0.5 LUFS`,
and has true peak at or below `-1.5 dBTP`; otherwise production fails without
fallback.

## Notes

- First version uses Blender background rendering, not AIGC video generation.
- The main-IP production test profile renders Full HD (`1920x1080`) at 30 fps with `qualityPreset=production_1080p`. `production_2k` remains available for later final masters, while `preview` is reserved for layout checks.
- If `audioPath` is omitted, the provider uses the voice pinned by the character profile. Production providers are local GPT-SoVITS, HeyGen, and ElevenLabs; Kokoro and macOS Apple voices remain preview-only. The main sloth profile pins its verified local GPT-SoVITS bundle and keeps Apple Eddy only as an explicit preview voice.
- `facialTopologyMode=source_retopology` traces and splits the original mouth groove, keeps the visible source face and lips, and adds only hidden oral-cavity, teeth, and tongue geometry. It augments preserved humanoid rigs with jaw, eye, and three-segment tongue bones. The current sloth source safely supports independent eye squint only; it does not claim true eyelid topology or a full blink.
- A profile that explicitly pins a provider fails when that provider cannot synthesize; it does not silently downgrade to macOS `say`. The `auto` provider retains `say` only as a last-resort preview fallback.
- The default `rigMode=auto` preserves an input Armature, skin weights, materials, and existing actions. Common Generic/Mixamo-style bone names are mapped to presenter controls. For the approved main-IP source, `enhanceExistingRig=true` adds three-segment articulation to each of the three source digits while preserving the original hand surface and verifies nonzero weighted-vertex evidence at both joints of every digit. A generated cartoon rig is only used when the model has no Armature.
- `mouthMode=auto` reuses existing `Mouth_*` shape keys. For organic characters with a modeled mouth groove, combine `source_mesh_visemes` with `source_retopology`: visemes deform the split source lip boundaries and expose the real oral interior without a visible replacement mouth. Independent mouth geometry is only a fallback for compatible screen-face characters.
- The presenter assets include shoulders, elbows, wrists, independently controlled three-segment digits, multi-axis limbs, source-mesh visemes, restrained eye/brow/cheek shapes, and reusable gesture/expression Actions. The exported GLB keeps deform bones, morphs, and actions.
- When `sceneBlendPath` is supplied, Blender opens the authored `.blend`, imports the character at `IP_Character_Spawn`, focuses cameras on `IP_Focus_Head`, and renders the character and set together. This is the preferred A-roll path because the character receives the set lighting and casts real floor/wall shadows.
- Authored studios should expose `Camera_Wide`, `Camera_Medium`, and `Camera_Close`. `cameraPreset=auto` creates restrained timeline cuts between those cameras; a fixed `wide`, `medium`, or `close` preset is also supported.
- `lightingPreset` scales lights that store `ip_base_energy` and `ip_light_role`. Supported values are `editorial_soft`, `editorial_crisp`, `night_analysis`, and `scene_default`.
- `renderEngine` supports `BLENDER_EEVEE_NEXT` (mapped to the installed Eevee enum) and `CYCLES`. Eevee is recommended for full talking videos; Cycles is intended for short high-quality shots.
- When `backgroundPath` is supplied, Blender renders an RGBA avatar pass and FFmpeg composites that pass over one static A-roll plate.
- A `.blend` scene takes precedence when both scene and plate are configured. `backgroundBrightness` only affects the static-plate fallback.
- The bundled `ip形象/main_ip/scenes/editorial-news-studio.blend` is the dark editorial/news-analysis A-roll default for knowledge sharing, opinion commentary, and current affairs.
