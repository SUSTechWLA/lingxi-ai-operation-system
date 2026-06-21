# Stage: hyperframes_reference

从已审核的口播稿生成 `source/hyperframes-video-reference.md`——HyperFrames 视频制作的完整参考文档，是视频制作的唯一真相来源。

## 输出结构（共九章）

### 一、观点档案
从上游提取：核心观点、目标观众、开场触发场景、核心张力、论证材料、最强反方观点、边界、情绪弧线、结尾落点、Assumptions。

### 二、视频总体设定
- 画幅比例（9:16 竖屏 / 16:9 横屏）
- 目标时长
- 视频形式（实拍口播+包装 / 纯画面+旁白 / 纯文字动画）
- 视觉风格：clean premium non-realistic editorial 3D illustration, minimal shapes, subtle texture, modern business technology tone
- 默认色调：white, light gray, deep blue, restrained teal accents, orange-red only for pressure/risk
- 光照：soft controlled studio lighting, calm cinematic contrast
- 整体节奏风格
- 音频来源（真人录音 / TTS 预览 / 待录音）

### 三、重要制作原则
- **imagegen 原则**：用于人物/场景/情绪/物体/环境；禁止在图片中渲染可读文字/表格/图表/UI标签/聊天文字/数据/报告内容
- **字幕规则**：清晰可读、字号足够、关键词高亮、出现时间与口播同步
- **文字渲染规则**：数据/列表/标题/标签用 HTML/SVG 在 HyperFrames 中呈现
- **中央安全区规则**：主要内容保持在画面中央安全区，为未来裁剪留余地

### 四、逐段画面脚本
每个 BEAT 严格对应口播稿：

```markdown
## BEAT 01｜<名称>
**参考时长**：0—8秒

### 口播
<与口播稿逐句对应，可省略录音标记>

### 主画面
<具体画面描述，足够具体以实现>

### 是否需要 imagegen 生成图片
<不需要 | 需要：图片ID | 复用：图片ID>

### 动画变化
1. <按时间顺序的动画，指定图层出现/消失/位移/缩放/透明度>
2. <字幕/文字动画时机>

### 字幕重点
<关键词及处理方式：高亮/放大/颜色变化>

### 转场
<进入下一个 BEAT 的转场>
```

每个 BEAT 必须有：画面方案、imagegen 需求判断、动画方案、字幕重点、转场。

### 五、imagegen 图片清单
```markdown
## 图片 01｜<名称>
### 使用位置：<BEAT xxx>
### 输出路径：`assets/generated/<name>.png`
### 提示词：
Use case: productivity-visual
Asset type: HyperFrames scene background
Primary request: <具体场景>
Style/medium: clean premium non-realistic editorial 3D illustration, minimal shapes, subtle texture, modern business technology tone
Composition/framing: 16:9, safe central subject area, usable negative space for HTML captions and overlays
Lighting/mood: soft controlled studio lighting, calm cinematic contrast
Color palette: white, light gray, deep blue, restrained teal accents, orange-red only for pressure/risk
Text: no readable text, no letters, no numbers
Constraints: no watermark, no logo, no photorealism, no clutter, no distorted hands or faces
```

### 六、图片在视频中的处理方式
裁剪策略、视差/位移、复用策略、遮罩和叠加层。

### 七、字幕和文字规则
字体层级、高亮行为、HTML/SVG 文字渲染、字幕位置和出现方式。

### 八、整体节奏控制
时间分配、节奏变化点、呼吸点的视觉处理。

### 九、推荐项目素材目录
```text
source/ 口播稿.md, hyperframes-video-reference.md
assets/generated/  # 场景素材
assets/characters/  # 角色参考
```

## 质量规则

- 每个视觉决策服务于论点，不装饰关键词
- 实现细节足够具体，HyperFrames 构建者可直接实施不需重新理解论点
- 全片转场语言一致
- 中央安全区构图，为未来裁剪保留空间
- 口播内容与口播稿 BEAT 逐段对应
- 不重复图片各生成一张，不同场景不共用一张

## 禁止事项

- 不写"展示动画"却没有时间/图层/转场细节
- 不让生成图片包含可读文字/表格/图表/UI 标签
- 不把口播稿录音标记（`/`, `//`, `【语气】`, `**加粗**`）复制到口播部分
- 不创建额外必交付文件
- 不输出 JSON 格式——输出纯粹的 HyperFrames 参考文档 Markdown
