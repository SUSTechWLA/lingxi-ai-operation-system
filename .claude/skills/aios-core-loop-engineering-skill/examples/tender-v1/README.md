# 自动化标书试点：Core 视角

本示例只判断标书需求对 AIOS Core 的要求。评分项提取、RAG、章节生成、合规检查和 Word 导出属于外部能力。本 Skill 不实现这些工具。只有当前 Core 缺少通用长任务状态、恢复、Tool 版本锁定、错误路由或跨 Tool Trace 时，才创建 Core 更新任务。
