# 04b-Frontend-Requirements: 标书生成 — 前端需求

> 不写前端代码。此文档定义前端团队独立开发所需的全部规格。

## 页面清单

### view-01: 标书项目列表页
- **入口**: 侧边栏"标书管理"导航
- **角色**: 投标专员
- **数据**: GET /api/bid/projects (分页)
- **状态**:
  - 列表项: 项目名称、状态标签 (DRAFT/PARSING/PLANNING/GENERATING/REVIEWING/EXPORTING/COMPLETED)、进度百分比、更新时间
  - 空状态: "暂无标书项目，点击创建第一个"
  - 加载态: Skeleton 卡片
- **动作**:
  - 创建新项目 → view-02
  - 点击项目 → view-03
  - 删除项目 (DELETE /api/bid/projects/:id)
- **验收**: 列表正确展示各状态项目，分页正常

### view-02: 创建标书项目
- **入口**: view-01 的"创建项目"按钮
- **数据**: POST /api/bid/projects
  - 项目名称 (必填)
  - 上传招标文件 (POST /api/bid/projects/:id/upload-tender)
  - 选择标书模板 (GET /api/bid/templates)
  - 行业分类选择
- **状态**: 上传中、解析中 (进度指示)
- **动作**: 提交创建 → 跳转 view-03
- **验收**: 上传成功，解析结果展示

### view-03: 标书项目详情/工作台
- **入口**: view-01 点击项目
- **数据**: GET /api/bid/projects/:id (含 chapters、进度)
- **子面板**:
  1. 招标文件解析结果 (评分项列表、技术要求)
  2. 标书章节结构 (树形目录，可拖拽排序)
  3. 章节内容编辑区 (Markdown 编辑器)
  4. 合规检查结果 (覆盖率仪表盘)
- **动作**:
  - 启动生成 (POST /api/bid/projects/:id/start)
  - 审核通过 (POST .../chapters/:chId/approve)
  - 驳回 (POST .../chapters/:chId/reject)
  - 重新生成 (POST .../chapters/:chId/regenerate)
  - 导出 (POST /api/bid/projects/:id/export)
  - 暂停/恢复
- **状态**:
  - 各阶段进度条 + 忙碌状态指示
  - 章节状态标签: PENDING/GENERATING/REVIEWING/APPROVED/REJECTED
  - 审核面板: 绿色通过 / 红色驳回
- **验收**: 完整流程可操作，进度实时可见

### view-04: 标书预览/导出
- **入口**: view-03 "导出"按钮
- **数据**: GET /api/bid/projects/:id/export/status (轮询)
- **状态**: 导出中 → 下载链接
- **验收**: .docx 可下载打开

## 视频创作交互要求
- 长操作显示进度条 (解析、生成、导出)
- 操作失败显示错误信息和建议
- 审核驳回必须填写驳回理由
- 支持暗色模式 (遵循现有 UI 系统)
