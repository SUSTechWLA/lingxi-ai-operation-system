# Stage: hyperframes_render

将已构建的 HyperFrames 项目渲染为 MP4 视频文件。

## 处理流程

1. **加载项目**：从上游获取项目引用和构建状态
2. **检测 CLI 渲染能力**：检查 CLI 是否支持 render 子命令，确认输出格式
3. **渲染**（CLI 可用）：执行渲染命令，指定输出路径/分辨率/编码，监控进度，收集日志
4. **返回指引**（CLI 不可用）：输出精确渲染命令、分辨率/编码器/比特率建议、预期渲染时间

## 音频处理

- 有真人录音用真人录音
- 无录音需预览用 TTS 生成可替换预览音轨，在报告中标注
- 音频画面同步由 HyperFrames 时间轴控制

## 输出格式

```json
{
  "renderArtifacts": [{
    "path": "output/video.mp4",
    "duration": "62.5s",
    "resolution": "1080x1920",
    "codec": "h264",
    "size": "45.2MB",
    "status": "rendered | guidance_only"
  }],
  "cliAvailable": true,
  "renderLogs": "渲染日志",
  "diagnostics": {"totalFrames": 1875, "warnings": []},
  "guidance": "CLI不可用时提供精确渲染命令"
}
```

## 禁止事项

- 渲染过程中不修改项目内容
- 不跳过渲染验证
- CLI 不可用时不返回空 renderArtifacts——返回完整渲染指引
