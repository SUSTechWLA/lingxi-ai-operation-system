# Stage: render_review

对照已审核的口播稿和 HyperFrames 参考文档，全面审核渲染后的 MP4 视频。

## 审核维度

1. **时长和节奏**：总时长是否在目标范围，BEAT 时长是否与参考文档一致（±10%），节奏转换是否合理
2. **字幕可读性**：字号是否足够，对比度是否充分，出现/消失时机是否同步，关键词高亮是否正确
3. **画面完整性**：每个 BEAT 主画面是否与参考文档一致，图片是否正确加载，动画是否匹配，转场是否正确
4. **视觉质量**：图片是否清晰无失真，文字渲染是否清晰无锯齿，叠加层是否有遮挡/重叠，色调风格是否一致
5. **内容核对**：画面是否支持论点（非仅装饰关键词），是否有未指定内容

## 输出格式

```json
{
  "decision": "accepted | accepted_with_minor_issues | rejected",
  "summary": "一句话总结",
  "checks": {
    "timing": {"passed": true, "issues": []},
    "captions": {"passed": true, "issues": []},
    "visuals": {"passed": false, "issues": ["BEAT 02 图片显示为默认占位图"]},
    "transitions": {"passed": true, "issues": []},
    "audio": {"passed": true, "issues": []}
  },
  "recommendations": ["具体可操作的修复建议"]
}
```

## 质量规则

- 逐 BEAT 对照审核不遗漏
- 问题描述包含时间码和可操作修复建议
- 判定：accepted（无不通过项）、accepted_with_minor_issues（少量不影响理解的小问题）、rejected（影响内容理解的重大问题）

## 禁止事项

- 不凭感觉判断，必须有具体时间码或 BEAT 编号依据
- 不跳过审核维度
- 不提出参考文档范围外的修改建议
