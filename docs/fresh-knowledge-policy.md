# Fresh Knowledge Policy

Fresh knowledge / news search 由三层共同控制：

- `KnowledgePolicy` 判断是否允许、可选或必须检索。
- `HybridToolRetriever` 根据 policy 过滤候选。
- `PlanGuard` 对 Planner 输出做二次拦截。

## 正向触发

以下请求会进入 `RetrievalPolicy = required`：

- 明确说“最新、今天、最近、刚刚、实时、现在、发生了什么”。
- 新闻视频、时事视频、政策解读、比赛结果、公司动态、人物近况、突发事件。
- 事实核验、引用来源、查证、数据更新。
- 明确当前年份、当前版本、当前价格、当前法规等时效表达，例如“2026 年最新 AI 视频模型对比”。

Required policy 会设置：

- `MustUseFacts = true`
- `MustCiteFacts = true`
- `BlockOnEmptyFacts = true`
- `RequiredCapabilities` 包含 `fresh_knowledge` / `news_search` / `web_search` / `current_event_retrieval`
- `Reason` 写入 tool trace

## 负向触发

以下请求默认 `RetrievalPolicy = none`，并禁止 fresh knowledge capabilities：

- 用户明确说不要联网、不要搜索、不查资料。
- 纯虚构故事。
- 架空世界。
- 脑洞短片。
- 创意口播。
- 历史常识。
- 通用科普，且不要求最新数据。
- 情绪类、文案类、风格化脚本类请求。
- 改写、润色、扩写已有内容。
- 视频主题不依赖现实世界最新事实。

用户明确禁止联网时，还会禁止 `external_api`。Guard 必须阻止 external API、web search、news search 和 current event retrieval。

## Retriever 行为

`HybridToolRetriever` 接收 `KnowledgePolicy`：

- policy forbidden capability 命中时硬过滤。
- fresh/news 工具只有 policy 为 `required` 或 `optional` 才能进入候选。
- `whenNotToUse` 命中时过滤或强降权。
- candidate reason 必须包含 knowledge policy reason、匹配 capability/tag/whenToUse、成本风险 penalty。

## Guard 行为

`PlanGuard` 不信任 Planner。即使 Planner 选择了新闻搜索工具，只要 policy 禁止 fresh knowledge，Guard 会拒绝计划并返回可读 reason。

典型验收：

| 用户输入 | 结果 |
|---|---|
| `帮我写一个赛博朋克虚构短片` | 不允许 fresh knowledge。 |
| `不要联网，帮我写一个产品宣传视频脚本` | 不允许 fresh knowledge 和 external API。 |
| `帮我做今天 AI 新闻短视频` | 必须检索并引用事实。 |
| `最近某公司发生了什么，做成短视频` | 必须检索并引用事实。 |
| `写一个关于牛顿三定律的科普视频` | 不需要 fresh knowledge。 |
| `写一个 2026 年最新 AI 视频模型对比` | 必须检索并引用事实。 |
