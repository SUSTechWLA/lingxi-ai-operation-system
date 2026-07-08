# LocalIpTalkingAvatarRenderTool

`LocalIpTalkingAvatarRenderTool` is a deterministic local renderer for cartoon IP talking-avatar videos. It does not call AIGC video generation, image-to-video, text-to-video, realistic face animation, Seedance, or Stable Diffusion.

## What It Does

The tool loads an existing local IP character by `characterId`, analyzes narration audio, generates a simple lip-sync timeline, generates motion events, renders a local puppet avatar layer, and composes the final MP4 with FFmpeg.

Supported first-version behavior:

- audio-driven mouth open/close and simple mouth shape switching
- automatic blink
- idle breathing
- sentence-end nods
- keyword gesture triggers
- segmented local prosody preview audio when `script` is provided without `audioPath`
- independent left/right arm gestures, both-hands presentation, foot bounce, and body weight shift in `svg2d`
- optional subtitle composition when the local FFmpeg build supports the `subtitles` filter
- background image/video composition
- final H.264 MP4 output
- render QA report
- `svg2d` puppet mode for HyperGen-controllable IP parts
- character voice profile output for narration matching

## What It Does Not Do

- no AIGC video generation
- no Seedance
- no Stable Diffusion
- no image-to-video
- no realistic human face lip-sync
- no complex 3D digital human
- no Live2D rendering unless a Cubism model package and renderer bridge are provided

## Character Asset Format

Characters live under:

```text
assets/characters/{characterId}/
```

Each character must include `character.json`:

```json
{
  "characterId": "demo_ip_001",
  "displayName": "Demo IP",
  "type": "cartoon_ip",
  "renderer": "sprite2d",
  "defaultExpression": "normal",
  "defaultPose": "front_talking",
  "canvas": { "width": 1920, "height": 1080, "fps": 30 },
  "anchor": { "x": 960, "y": 720, "scale": 1.0 },
  "assets": {
    "body": "base/body.png",
    "head": "base/head.png",
    "hair": "base/hair.png",
    "leftArm": "base/left_arm.png",
    "rightArm": "base/right_arm.png",
    "eyeOpen": "eyes/eye_open.png",
    "eyeHalf": "eyes/eye_half.png",
    "eyeClose": "eyes/eye_close.png",
    "mouthClosed": "mouths/mouth_closed.png",
    "mouthA": "mouths/mouth_a.png",
    "mouthO": "mouths/mouth_o.png",
    "mouthE": "mouths/mouth_e.png",
    "mouthI": "mouths/mouth_i.png",
    "mouthU": "mouths/mouth_u.png"
  },
  "capabilities": {
    "lipSync": true,
    "blink": true,
    "headNod": true,
    "headShake": true,
    "simpleGesture": true,
    "expressionSwitch": true
  }
}
```

All listed paths are relative to the character directory. Required assets are validated before rendering. Missing assets return a clear error and do not panic.

`renderer` can be:

- `sprite2d`: layered PNG sprites, useful for a simple first pass.
- `svg2d`: deterministic local puppet rendering with stable SVG part IDs and `rig.json` control channels. This is the current preferred mode for 波波 and 阿斯特.
- `live2d`: reserved for real Live2D Cubism assets. It requires `.model3.json`, texture atlas, physics, and motion files. Plain multi-view PNG references are not enough to become a high-quality Live2D model.

The repository includes:

```text
assets/characters/bobo/
assets/characters/aster/
```

Both define `referenceSvg`, `rig.json`, and `voiceProfile`. HyperGen should bind its layer animation to `avatar_scene.json.hypergenControl` instead of guessing part names.

## Input JSON

```json
{
  "characterId": "demo_ip_001",
  "script": "今天我们测试一个本地 IP 数字人口播工具。",
  "audioPath": "testdata/audio/demo.wav",
  "subtitlePath": "testdata/subtitle/demo.srt",
  "backgroundPath": "testdata/background/demo.png",
  "bgmPath": "",
  "outputDir": "tmp/ip_talking_avatar_demo",
  "renderMode": "svg2d",
  "interactionLevel": "expressive",
  "resolution": { "width": 1920, "height": 1080 },
  "fps": 30,
  "style": {
    "position": "center_bottom",
    "scale": 1.0,
    "subtitleEnabled": true,
    "backgroundEnabled": true,
    "transparentAvatarVideo": true
  },
  "motionPolicy": {
    "autoBlink": true,
    "autoBreath": true,
    "sentenceNod": true,
    "keywordGesture": true
  }
}
```

`audioPath` can be WAV or another FFmpeg-readable audio file. MP3 input is converted to temporary PCM before RMS analysis.

If `audioPath` is omitted and `script` is present, the tool can create a deterministic local preview narration with macOS `say`. It now splits the script into prosody segments with varied rate, volume, and pauses, and writes `narration_prosody_plan.json`. That preview is marked `previewOnly` in `voice_profile.json`; production output should still use uploaded or provider-generated narration audio that matches the IP personality.

Default voice personas:

- `bobo` / 波波: young, lively, playful, faster rhythm.
- `aster` / 阿斯特: calm, precise, scholarly, slower rhythm.

## Output Files

The output directory contains:

```text
audio_analysis.json
lip_sync_timeline.json
motion_timeline.json
avatar_timeline.json
avatar_scene.json
voice_profile.json
narration_prosody_plan.json
frames/frame_000001.png
avatar_layer.webm
final.mp4
render_report.json
ffmpeg_compose_command.json
```

If the local FFmpeg build does not support subtitle burn-in, the tool writes `subtitle_composition_warning.txt` and still generates `final.mp4`.

## Local Command

The local runner command is:

```text
LOCAL_IP_TALKING_AVATAR_RENDER
```

The cloud tool manifest name is:

```text
local_ip_talking_avatar_render
```

It is intended for prompts such as `IP口播`, `卡通数字人`, `虚拟人口播`, `品牌角色讲解`, and `本地数字人`.

It is not intended for realistic human face generation, AIGC video generation, Seedance video generation, image-to-video, or face restoration.

## Running The Demo In Tests

The integration test generates a temporary character, WAV, SRT, background, and final video:

```bash
go test -C local-backend ./internal/localtool -run TestLocalIpTalkingAvatarRenderGeneratesTimelinesAndScene -v
```

The repository also contains `assets/characters/demo_ip_001/` as a minimal sprite protocol example, plus `assets/characters/bobo/` and `assets/characters/aster/` as controllable `svg2d` IP puppet examples.

## FFmpeg Dependency

Required:

```bash
ffmpeg -version
ffprobe -version
```

The tool uses FFmpeg for:

- audio duration probing
- audio to PCM conversion
- avatar layer encoding
- final MP4 composition

## Adding A New IP Character

1. Create `assets/characters/{characterId}/character.json`.
2. Choose `sprite2d`, `svg2d`, or future `live2d`.
3. For `svg2d`, provide `renderer/{character}_puppet.svg` and `renderer/rig.json` with stable part IDs.
4. Add `voiceProfile` so narration selection, prosody defaults, and downstream TTS prompts match the IP persona.
5. Keep all paths relative to the character folder.
6. Run the localtool integration test with `characterId`.
7. Preview `final.mp4` before approving the asset.

## Live2D Extension Path

`renderMode: "live2d"` is reserved at the protocol level. To enable it for an IP:

1. Build a real Live2D Cubism model from layered art, not just a flat PNG.
2. Add `renderer/live2d/{character}.model3.json`, textures, physics, expressions, and `motion3.json` files.
3. Map Cubism parameters such as `ParamMouthOpenY`, `ParamEyeLOpen`, `ParamEyeROpen`, `ParamAngleX`, `ParamAngleY`, and body motion parameters to `lip_sync_timeline.json` and `motion_timeline.json`.
4. Add a renderer bridge that reads the same `avatar_scene.json` and outputs `avatar_layer.webm` or `avatar_layer.mp4`.

This keeps the orchestration, QA, FFmpeg composition, user review, and HyperGen integration unchanged while replacing only the avatar renderer.

## Troubleshooting

- `character asset not found`: check `assets/characters/{characterId}/character.json` or set `TANGYING_IP_CHARACTER_ROOT`.
- `audioPath is required`: pass a real narration audio path, or provide `script` so the local preview voice can be generated.
- `renderMode live2d`: provide a Cubism renderer bridge and model package, or use `svg2d`.
- `ffmpeg is required`: install FFmpeg and ensure it is on `PATH`.
- `No such filter: subtitles`: local FFmpeg lacks subtitle burn-in support; the tool will generate the video without burned subtitles and keep the SRT for UI preview.
