---
name: create-opinion-videos
description: Use when creating opinion-led talking-head or faceless videos from vague notes, theses, scripts, articles, topic ideas, or Chinese 口播/观点输出 content that must become a recording script, HyperFrames editing reference, imagegen asset plan, or fully rendered HyperFrames video. Trigger for requests like 口播视频, 观点输出视频, 内容转视频, 帮我把想法做成视频, 生成口播稿, 生成 HyperFrames 参考文档, or one-click opinion video production.
---

# Create Opinion Videos

Turn a rough idea into an opinion-led video production package, then optionally build and render it with HyperFrames. The canonical intermediate deliverable is exactly two files in the project `source/` directory:

- `source/口播稿.md` — the clean recording script the user can read aloud.
- `source/hyperframes-video-reference.md` — the editing/reference document HyperFrames uses to build the video.

If continuing a legacy project that already has a single `source/script-*.md` reference file, preserve that filename only when the user clearly wants continuity. New projects use `source/hyperframes-video-reference.md`.

## Core Contract

Do not jump directly from a vague topic to visuals. First clarify or infer the point of view, then write the recording script, then create the HyperFrames reference. When the user asks for one-click production, continue through image generation, HyperFrames build, render, and verification instead of stopping at documents.

## Workflow

1. **Intake and resume.** Locate or create the project directory. Check `source/口播稿.md`, `source/hyperframes-video-reference.md`, existing `source/script-*.md`, and any user-provided viewpoint file.
2. **Viewpoint clarification.** If the user gives only a vague idea, ask 1-3 high-leverage questions per round. If the user says "默认", "直接做", "一键", or "自动", infer missing details and record assumptions in the reference document.
3. **Viewpoint dossier.** Build a concise `观点档案` containing thesis, audience, tension, concrete example, evidence, counterargument, emotional arc, and final takeaway. Keep it in conversation and embed it in `source/hyperframes-video-reference.md`; do not create a third required file unless the user asks.
4. **Recording script.** Write `source/口播稿.md` from the dossier. This file is for recording only: retention-first conversational phrasing, beat timings, pauses, emphasis, and voice notes. No visual production instructions.
5. **HyperFrames reference.** Write `source/hyperframes-video-reference.md` from the recording script. This file contains global visual direction, beat-by-beat editing specs, animation notes, caption rules, imagegen prompts, asset paths, and implementation constraints.
6. **Assets and video.** If building the video now, use `imagegen` for bitmap assets marked as needed, save final project-bound images under `assets/generated/` or `assets/characters/`, then implement/verify/render with HyperFrames.

Use the helper prompts only when the task is large enough to benefit from isolation:

- `agents/viewpoint-expansion.md` for vague ideas or incomplete viewpoint files.
- `agents/recording-script.md` for `source/口播稿.md`.
- `agents/hyperframes-reference.md` for `source/hyperframes-video-reference.md`.

Templates are available in `templates/recording-script.md` and `templates/hyperframes-video-reference.md`.

## Viewpoint Clarification

Minimum viable viewpoint:

| Field | Purpose |
| --- | --- |
| Core thesis | The one sentence the video must prove or make felt |
| Audience | Who should feel "this is about me" |
| Trigger scene | A concrete daily moment that opens the video |
| Tension | What common belief or behavior the video challenges |
| Evidence | Personal experience, observation, data, analogy, or case |
| Counterargument | The strongest reasonable objection |
| Boundary | What the video is not claiming |
| Ending | The final line or action the viewer should remember |
| Format | Platform, aspect ratio, target duration, language |

Questioning rules:

- Ask only the missing questions that change the output materially.
- Prefer concrete questions over abstract ones: "你想用哪个工作场景开头？" beats "你想表达什么？"
- Do not dump a long questionnaire. Ask 1-3 questions, summarize the inferred viewpoint, then continue.
- In one-click mode, make conservative assumptions and label them under `## 观点档案 / Assumptions`.

## Recording Script Output

Write `source/口播稿.md` with this structure:

```markdown
# 《<标题>》

## BEAT 01｜<功能>｜0—8秒

【<录音语气，不超过一行>】

<口播句子，使用 / 表示短停顿，// 表示长停顿>

【<可选强调说明>】

<继续口播，关键重音可用 **粗体**>

---
```

Default speaking style:

- Use **短视频强留存 + 真人聊天感** as the default style. The script should feel like the creator is talking to one specific viewer, not reading a formal reference document.
- Treat `source/口播稿.md` as the recording稿. Treat `source/hyperframes-video-reference.md` as the制作参考文档稿. Do not let the recording稿 inherit the reference document's rigid summary language.
- Open with a concrete scene, contradiction, direct question, or counterintuitive claim before explaining the concept.
- Every BEAT needs a **留存钩子**: an unanswered question, a small reversal, a concrete detail, or a forward pull that gives the viewer a reason to keep listening.
- Use natural conversational connectors sparingly: "你有没有发现", "说白了", "问题是", "但这里有个关键点", "我后来意识到", "你会发现".
- Prefer short spoken lines, mild self-correction, and direct address over complete essay paragraphs.

Script rules:

- Make it sound spoken, not like an essay or report.
- Keep visual instructions out of this file.
- Use beat ranges that add up to the target duration.
- Use `【语气】`, `/`, `//`, and selective `**强调**` so the user can record naturally.
- Start with a concrete hook within the first 3 seconds and make the central thesis clear within the first 15 seconds.
- Prefer one vivid story plus one sharp abstraction over many generic claims.
- For Mandarin recording, estimate roughly 4.5-5.5 Chinese characters per second, then adjust beat timings by rhythm and pauses.
- Avoid fake casualness: no marketing tone, no forced catchphrases, no over-polished aphorisms, no broadcast-host voice, no continuous rhetorical questions.

## HyperFrames Reference Output

Write `source/hyperframes-video-reference.md` with this structure:

```markdown
# 《<标题>》HyperFrames 视频参考文档

## 一、观点档案
<thesis, audience, tension, examples, counterargument, boundary, ending, assumptions>

## 二、视频总体设定
<aspect ratio, duration, form, visual style, palette, rhythm, audio source>

## 三、重要制作原则
<imagegen rules, text rendering rules, caption rules, central crop rules>

# 四、逐段画面脚本

## BEAT 01｜<名称>
**参考时长**：0—8秒

### 口播
<match the recording script text, without recording marks unless useful for timing>

### 主画面
<what the viewer sees>

### 是否需要 imagegen 生成图片
<不需要 | 需要：图片 ID | 复用：图片 ID>

### 动画变化
<ordered animation and motion notes>

### 字幕重点
<keywords or caption treatment>

### 转场
<transition into next beat>

# 五、imagegen 图片清单
## 图片 00｜<名称>
### 使用位置
<beats>
### 输出路径
`assets/generated/<file>.png`
### 提示词
<complete imagegen prompt>

# 六、图片在视频中的处理方式
<crop, parallax, reuse, masking, overlays>

# 七、字幕和文字规则
<HTML/SVG text rules, font hierarchy, highlight behavior>

# 八、整体节奏控制
<timing, pacing, breath points>

# 九、推荐项目素材目录
<assets and source tree>
```

Reference rules:

- The `口播` section must match `source/口播稿.md` beat-for-beat.
- Every beat needs a concrete visual plan, animation plan, caption emphasis, and transition.
- Prefer HyperFrames HTML/SVG/CSS for readable text, diagrams, numbers, tables, charts, UI, subtitles, and labels.
- Use generated images for people, office/work scenes, mood, objects, environments, and non-readable background visuals.
- Keep the visual style concise, clean, premium, and non-realistic by default unless the user specifies otherwise.
- Include enough implementation detail that another Codex instance can build `index.html` without reinterpreting the thesis.

## Imagegen Policy

Use the `imagegen` skill when the video needs raster bitmap assets. Default to built-in `image_gen` mode.

Required policy for project-bound image assets:

- Plan image assets in `source/hyperframes-video-reference.md` first.
- Generate one distinct asset per prompt; do not ask one image to contain several unrelated scenes.
- Do not ask generated images to contain readable Chinese/English text, Excel data, chat messages, UI labels, reports, or complex charts. Draw those in HyperFrames.
- For recurring people, create a character reference image in `assets/characters/` and reuse its style/person description in later prompts.
- Move or copy final generated images into the project before referencing them: `assets/generated/` for scene assets and `assets/characters/` for character references.
- Record the final saved path and final prompt in the reference document.
- Do not overwrite existing generated assets unless the user explicitly asks; create versioned filenames such as `office-files-v2.png`.

Default prompt style:

```text
Use case: productivity-visual
Asset type: HyperFrames scene background
Primary request: <scene>
Style/medium: clean premium non-realistic editorial 3D illustration, minimal shapes, subtle texture, modern business technology tone
Composition/framing: 16:9, safe central subject area, usable negative space for HTML captions and overlays
Lighting/mood: soft controlled studio lighting, calm cinematic contrast
Color palette: white, light gray, deep blue, restrained teal accents, occasional orange-red only for pressure/risk
Text: no readable text, no letters, no numbers
Constraints: no watermark, no logo, no photorealism, no clutter, no distorted hands or faces
```

## HyperFrames Execution

When the user asks for a rendered video:

1. Read `source/口播稿.md` and `source/hyperframes-video-reference.md`.
2. Generate missing image assets marked in the reference using `imagegen`.
3. Build or update the HyperFrames project with the reference doc as the production source of truth.
4. Use human-recorded audio if provided. If no recording exists and the user asks for an automatic preview, generate TTS as a replaceable preview track and note it in the final report.
5. Implement captions, overlays, diagrams, and readable UI text inside HyperFrames, not inside generated images.
6. Run HyperFrames validation/inspection. For visual projects, create screenshots or contact sheets and fix obvious layout issues before rendering.
7. Render MP4 and report paths for source docs, assets, preview URL if running, and final render.

## Resume Table

| Current state | Continue with |
| --- | --- |
| No project directory | Create project directory, then `source/` |
| Vague idea only | Viewpoint clarification |
| Viewpoint file exists, no `source/口播稿.md` | Expand viewpoint and write recording script |
| `source/口播稿.md` exists, no reference doc | Write `source/hyperframes-video-reference.md` |
| Reference doc exists, assets missing | Generate imagegen assets and update paths |
| Assets exist, no HyperFrames project | Build HyperFrames project |
| HyperFrames project exists, no render | Validate, preview, render |
| Render exists | Report final package and residual risks |

## Quality Bar

- The user can record directly from `source/口播稿.md` without seeing production notes.
- The HyperFrames builder can implement the video from `source/hyperframes-video-reference.md` without asking what the visuals mean.
- The thesis is sharp, not a neutral explainer.
- The opening is concrete before it becomes abstract.
- The recording script sounds like a real person trying to persuade or share a discovery, with enough tension to improve completion rate.
- Visuals support the argument rather than decorating generic keywords.
- Generated images are clean, premium, non-realistic, and free of readable text.
- All project-referenced assets live inside the project directory.

## Common Failures

- Asking a long questionnaire instead of progressing the idea.
- Creating extra required documents and making the workflow harder to resume.
- Mixing camera/animation notes into the recording script.
- Writing a polished essay, reference-document draft, or formal explainer that is hard to speak.
- Adding fake casualness: marketing voice, excessive punchlines, exaggerated emotion, or filler-heavy rambling.
- Letting generated images render text, tables, charts, or UI labels.
- Producing a HyperFrames reference that says "show animation" without timing, layers, or transition details.
- Finishing after documents when the user asked for one-click rendered output.
