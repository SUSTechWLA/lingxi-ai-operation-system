# Stage: publish_package

基于已审核的口播稿、HyperFrames 参考文档、渲染/外部生成指引和成片审核结果，生成可交付给用户的发布素材包。输出必须严格围绕用户原始主题，不得换题。

## 输出格式

只输出 JSON，不要 Markdown，不要代码块，不要额外解释：

```json
{
  "title": "适合发布页的一条中文标题，必须包含用户主题中的关键实体",
  "description": "80-160字简介，说明这条视频讲什么、为什么值得看，并自然覆盖用户主题",
  "keywords": ["关键词1", "关键词2", "关键词3", "关键词4", "关键词5"],
  "videoImportPackage": {
    "status": "ready_for_external_generation | rendered | guidance_only",
    "sourceSkill": "create-opinion-videos",
    "scriptRef": "口播稿引用或摘要",
    "hyperframesRef": "HyperFrames 参考文档引用或摘要",
    "renderRef": "渲染结果、项目引用或外部生成指引",
    "importSteps": [
      "把口播稿用于录音或 TTS",
      "按 HyperFrames 参考文档构建画面",
      "导入图片素材和字幕规则",
      "渲染 MP4 后回传到发布包"
    ]
  },
  "notes": ["必要提醒，如素材仍需外部网页视频工具生成"]
}
```

## 质量规则

- 标题、简介、关键词必须和用户原始主题一致；例如用户说端午节和粽子，就不得输出 AI、职场或其它无关主题。
- 标题不超过 28 个中文字符，避免标题党和虚假承诺。
- 简介要可直接复制到短视频平台，不写制作过程说明。
- 关键词 4-8 个，必须包含主题实体、知识点和平台搜索词。
- `videoImportPackage` 面向制作交付，保留口播稿、画面参考、外部生成/渲染指引的引用或摘要。

## 禁止事项

- 不输出 `[object Object]`、空字符串或嵌套对象作为 title/description/keywords 的元素。
- 不生成两个标题、两个简介或多份关键词列表。
- 不把观点档案、口播稿全文塞进 description。
- 不改变用户主题，不补写和主题无关的热点或商业观点。
