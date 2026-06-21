# Stage: image_assets

根据 HyperFrames 参考文档第五章节（imagegen 图片清单）的位图素材需求，为每个需要的图片生成完整的 imagegen 提示词，并尽可能调用图片生成服务产出实际图片素材。

## 处理流程

1. 解析 HyperFrames 参考文档，提取所有"需要：图片 ID"的条目
2. 对每个需要的图片使用 LLM 基于默认模板扩展完整 imagegen 提示词
3. 如配置了图片生成服务，调用生成实际图片；否则返回完整提示词作为可操作中间态

## 默认提示词模板

每个图片提示词必须包含所有维度：

```text
Use case: <用途，如 productivity-visual, office-scene, mood-atmosphere>
Asset type: HyperFrames scene background / character reference
Primary request: <具体场景描述>
Style/medium: clean premium non-realistic editorial 3D illustration, minimal shapes, subtle texture, modern business technology tone
Composition/framing: 16:9, safe central subject area, usable negative space for HTML captions and overlays
Lighting/mood: soft controlled studio lighting, calm cinematic contrast
Color palette: white, light gray, deep blue, restrained teal accents, orange-red only for pressure/risk
Text: no readable text, no letters, no numbers
Constraints: no watermark, no logo, no photorealism, no clutter, no distorted hands or faces
```

## 图片生成规则

- 每个提示词一张独立图片，不合并多个不相关场景
- 重复角色先创建角色参考图（`assets/characters/`），后续引用其风格描述
- 场景素材 → `assets/generated/`，角色参考 → `assets/characters/`
- 不覆盖已有素材，版本化文件名如 `office-scene-v2.png`

## 输出格式

```json
{
  "imageRequests": [
    {
      "id": "图片ID",
      "prompt": "完整imagegen提示词",
      "dimensions": {"width": 1920, "height": 1080},
      "status": "generated | prompt_only",
      "storageRef": "存储引用路径",
      "thumbnailUrl": "缩略图URL",
      "outputPath": "assets/generated/<name>.png"
    }
  ],
  "summary": {
    "total": 3,
    "generated": 0,
    "promptOnly": 3,
    "note": "图片生成服务未配置，已输出完整提示词"
  }
}
```

## 禁止事项

- 不在图片中渲染可读文字/表格/图表/UI 标签
- 不共用一张图片表达多个不相关场景
- 不生成带水印/Logo/逼真照片风格的图片
