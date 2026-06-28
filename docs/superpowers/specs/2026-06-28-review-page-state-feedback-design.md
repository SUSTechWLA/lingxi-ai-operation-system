# 审核页状态反馈优化

## 目标

优化用户端审核页面，让用户对操作反馈、任务状态机、生成进度有直观清晰的感知。

## 改动范围

- `frontend/src/pages/DirectorStudioPage.tsx` — 组件层
- `frontend/src/pages/directorStudioLogic.ts` — 辅助函数（按需）
- 不涉及后端改动

## 设计

### 1. ActionButton 按钮反馈

点击任意按钮后，被点击的按钮显示加载状态：图标变旋转动画、文字变进行时，其他按钮保持 disabled。

Props 变更：
- 新增 `activeAction: string` — 当前正在执行的操作
- 新增 `loadingLabel: string` — loading 时显示的文字

行为：
- `loading && activeAction === action` → 图标 `<FiRefreshCw animate-spin>`，文字 `loadingLabel`
- `loading && activeAction !== action` → disabled + opacity-45
- 非 loading → 保持原样

### 2. StateMachineBar 状态机可视化

将 `ReviewStageRelay` 改造为 `StateMachineBar`，横向流水线展示：

| 状态 | 图标 | 颜色 | 动画 |
|------|------|------|------|
| running | FiRefreshCw spin | 蓝色 | 脉冲呼吸 |
| review | FiShield | 琥珀色 | 高亮 |
| done | FiCheck | 绿色 | 静态 |
| blocked/failed | FiX | 红色 | 静态 |
| pending | ○ 空心圆 | 灰色 | 静态半透明 |

- 阶段之间 `→` 箭头连接
- 当前活跃阶段卡片稍大 + 阴影
- hover tooltip 显示阶段目标
- 底部 label 显示中文状态名

### 3. NowGeneratingBanner 状态横幅

审核区顶部横幅，根据系统状态动态切换：

| 系统状态 | 横幅样式 | 文案 |
|---------|---------|------|
| 有 running 阶段 | 蓝色 + 脉冲光点 + spin | "系统正在生成【{角色名}】的{任务名}…" |
| 有 review 阶段 | 琥珀色 + FiShield | "【{角色名}】产物已输出，等待你的确认" |
| 全部 done | 绿色 + FiCheck | "当前阶段已确认，下游阶段正在执行" |

- `transition-all` 平滑切换
- 根据 `stages` 数据自动推导当前状态
