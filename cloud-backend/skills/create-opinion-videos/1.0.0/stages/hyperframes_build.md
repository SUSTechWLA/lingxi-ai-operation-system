# Stage: hyperframes_build

基于已审核的 HyperFrames 参考文档和已生成图片素材，构建或更新 HyperFrames 视频项目。

## 处理流程

1. **读取参考文档**：解析九章内容，特别是第四章节每个 BEAT 的规格
2. **检测 HyperFrames CLI**：尝试 `hyperframes`、`npx hyperframes` 命令
3. **构建项目**（CLI 可用时）：按参考文档视觉设定初始化项目，按 BEAT 顺序构建画面，引用 `assets/generated/` 图片，所有可读文字在 HTML/SVG/CSS 层实现，运行验证/检查
4. **返回指引**（CLI 不可用时）：输出完整构建指引和精确 CLI 命令

## 构建原则

- 字幕、叠加层、图表、可读 UI 文字在 HyperFrames 中实现（不在图片中）
- 所有动画严格按参考文档第四章节每个 BEAT 的"动画变化"规格执行
- 转场与参考文档每 BEAT 的"转场"规格一致
- 视觉风格（色调/字体/光照）与参考文档第二章一致
- 中央安全区构图

## 输出格式

```json
{
  "projectRef": "项目包引用路径",
  "previewUrl": "预览路径或 URL",
  "lintOutput": "验证输出（警告列表）",
  "missingDeps": ["缺失依赖"],
  "beatsBuilt": 7,
  "status": "built | guidance_only",
  "cliAvailable": true,
  "guidance": "CLI 不可用时提供手动构建指引"
}
```

## 禁止事项

- 不在图片中嵌入字幕或可读文字
- 不跳过参考文档中任何 BEAT
- 不自行修改参考文档中的动画或转场规格
- CLI 不可用时返回完整构建指引，不返回空 projectRef
