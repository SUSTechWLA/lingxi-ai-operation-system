# 审核页状态反馈优化 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 优化审核页面的按钮反馈、任务状态机可视化和当前执行状态展示。

**Architecture:** 纯前端改动，在 `DirectorStudioPage.tsx` 中修改 3 个组件（ActionButton、ReviewStageRelay→StateMachineBar、新增 NowGeneratingBanner），在 `directorStudioLogic.ts` 中添加一个辅助函数。沿用现有 Tailwind CSS + React Icons 技术栈。

**Tech Stack:** React + TypeScript + Tailwind CSS + react-icons

## Global Constraints

- 不涉及后端改动
- 不改动数据结构或 API 调用
- 保持现有组件整体布局不变
- 使用项目中已有的 Tailwind 类名和 react-icons 图标

---

### Task 1: ActionButton 增加 loading 状态反馈

**Files:**
- Modify: `frontend/src/pages/DirectorStudioPage.tsx` — `ActionButton` 组件 (line 955-958)
- Modify: `frontend/src/pages/DirectorStudioPage.tsx` — `ReviewPage` 组件 (line 502-639) 增加 `activeAction` state

**Interfaces:**
- Consumes: 现有 `ActionButton` props (`color`, `icon`, `label`, `disabled`, `onClick`)
- Produces: 新 `ActionButton` props 增加 `loading: boolean`, `loadingLabel: string`

- [ ] **Step 1: 修改 ActionButton 组件，增加 loading props 和渲染逻辑**

将 `ActionButton` 组件 (line 955-958) 替换为：

```tsx
function ActionButton({ color, icon, label, disabled, onClick, loading = false, loadingLabel = '' }: { color: 'green' | 'red' | 'amber' | 'violet'; icon: React.ReactNode; label: string; disabled: boolean; onClick: () => void; loading?: boolean; loadingLabel?: string }) {
  const map = { green: 'border-green-200 bg-green-50 text-green-700 hover:bg-green-100', red: 'border-red-200 bg-red-50 text-red-700 hover:bg-red-100', amber: 'border-amber-200 bg-amber-50 text-primary-dark hover:bg-amber-100', violet: 'border-violet-200 bg-violet-50 text-violet hover:bg-violet-100' }
  return (
    <button
      disabled={disabled}
      onClick={onClick}
      className={clsx(
        'flex min-w-0 items-center justify-center gap-2 rounded-lg border px-3 py-3 text-sm font-black leading-tight transition disabled:cursor-not-allowed',
        loading ? 'opacity-70' : '',
        disabled && !loading ? 'opacity-45' : '',
        map[color],
      )}
    >
      <span className="shrink-0">{loading ? <FiRefreshCw className="animate-spin" /> : icon}</span>
      <span className="[overflow-wrap:anywhere]">{loading && loadingLabel ? loadingLabel : label}</span>
    </button>
  )
}
```

- [ ] **Step 2: 在 ReviewPage 组件中增加 `activeAction` state**

在 `ReviewPage` 函数体开头 (line 503 附近)，在现有 `useState` 之后添加：

```tsx
const [activeAction, setActiveAction] = useState<string | null>(null)
```

- [ ] **Step 3: 修改 ReviewPage 的 actOnReview 调用，传递 activeAction**

找到 `ReviewPage` 里的 `onAction` 调用处 (line 604-607)，给 4 个 ActionButton 都加上 loading 相关 props。同时需要修改 `ReviewPage` 的 props 或内部处理来追踪当前 action。

实际上，`ReviewPage` 的 `onAction` 是个回调，`loading` 从父组件传入。需要在 `ReviewPage` 内部拦截 action，设置 `activeAction`，并传给按钮。

修改 `ReviewPage` 组件内部，在 action buttons 区域 (line 597-616)：

```tsx
{isPending && (
  <section className="rounded-lg border border-primary/30 bg-white p-4 shadow-card ring-1 ring-primary/10">
    <div className="flex items-center gap-2">
      <FiShield className="text-primary" />
      <h3 className="text-base font-black text-ink">决策操作</h3>
    </div>
    <div className="mt-4 grid grid-cols-2 gap-2">
      <ActionButton color="green" icon={<FiCheck />} label="通过" loadingLabel="通过中…" loading={loading && activeAction === 'approve'} disabled={loading} onClick={() => { setActiveAction('approve'); onAction('approve'); }} />
      <ActionButton color="red" icon={<FiX />} label="驳回" loadingLabel="驳回中…" loading={loading && activeAction === 'reject'} disabled={loading} onClick={() => { setActiveAction('reject'); onAction('reject'); }} />
      <ActionButton color="amber" icon={<FiEdit3 />} label="修改提交" loadingLabel="提交中…" loading={loading && activeAction === 'edit'} disabled={loading} onClick={() => { setActiveAction('edit'); onAction('edit'); }} />
      <ActionButton color="violet" icon={<FiRefreshCw />} label="重新生成" loadingLabel="重新生成中…" loading={loading && activeAction === 'regenerate'} disabled={loading} onClick={() => { setActiveAction('regenerate'); onAction('regenerate'); }} />
    </div>
    {/* ... textarea unchanged ... */}
  </section>
)}
```

同时需要在 `loading` 变为 `false` 时重置 `activeAction`。在 `ReviewPage` 组件中添加 `useEffect`：

```tsx
useEffect(() => {
  if (!loading) setActiveAction(null)
}, [loading])
```

- [ ] **Step 4: 修改父组件 DirectorStudioPage 中 actOnReview 确保 finally 中 setLoading(false) 正常工作**

现有代码在 `actOnReview` 的 `finally` 中已经执行 `setLoading(false)` (line 213)，这会触发上面 `useEffect` 重置 `activeAction`。无需额外改动。

- [ ] **Step 5: 验证**

```bash
cd frontend && npm run build
```

期望：编译通过，无类型错误。

---

### Task 2: StateMachineBar 任务状态机可视化

**Files:**
- Modify: `frontend/src/pages/DirectorStudioPage.tsx` — `ReviewStageRelay` 组件 (line 459-499) 重写为 `StateMachineBar`
- Modify: `frontend/src/pages/DirectorStudioPage.tsx` — `ReviewPage` 中对 `ReviewStageRelay` 的引用改为 `StateMachineBar`

**Interfaces:**
- Consumes: `DirectorStage[]` (现有类型)
- Produces: 新的 `StateMachineBar` 组件，相同 props 签名

- [ ] **Step 1: 在 directorStudioLogic.ts 中添加状态展示配置辅助函数**

在 `directorStudioLogic.ts` 末尾添加：

```typescript
export interface StageStateDisplay {
  icon: 'check' | 'shield' | 'refresh' | 'x' | 'cpu'
  label: string
  colorClass: string
  animate: boolean
  active: boolean
}

export function getStageStateDisplay(status: DirectorStageStatus): StageStateDisplay {
  switch (status) {
    case 'done':
      return { icon: 'check', label: '已通过', colorClass: 'border-green-300 bg-green-50 text-green-700', animate: false, active: false }
    case 'review':
      return { icon: 'shield', label: '待审核', colorClass: 'border-amber-300 bg-amber-50 text-amber-700', animate: false, active: true }
    case 'running':
    case 'active':
      return { icon: 'refresh', label: '生成中', colorClass: 'border-blue-300 bg-blue-50 text-blue-700', animate: true, active: true }
    case 'blocked':
    case 'failed':
      return { icon: 'x', label: '已阻断', colorClass: 'border-red-300 bg-red-50 text-red-700', animate: false, active: false }
    case 'pending':
    default:
      return { icon: 'cpu', label: '等待中', colorClass: 'border-stone-200 bg-stone-50/60 text-stone-400', animate: false, active: false }
  }
}
```

- [ ] **Step 2: 将 ReviewStageRelay 替换为 StateMachineBar**

在 `DirectorStudioPage.tsx` 中，将 `ReviewStageRelay` 函数 (line 459-499) 整体替换为：

```tsx
function StateMachineBar({ stages }: { stages: DirectorStage[] }) {
  const runningStage = stages.find((stage) => stage.status === 'running')
  const reviewStage = stages.find((stage) => stage.status === 'review')

  return (
    <section className="card col-span-12 p-5">
      <div className="flex items-center justify-between gap-3 mb-4">
        <div>
          <h3 className="text-base font-black text-ink">任务状态机</h3>
          <p className="mt-1 text-xs text-ink-soft">
            {runningStage ? `${runningStage.displayName} 正在生成，产物完成后进入审核。` : reviewStage ? `${reviewStage.displayName} 产物已输出，等待确认。` : '审核通过后，下个角色立即进入生成中。'}
          </p>
        </div>
        <div className="flex items-center gap-3 text-xs font-semibold text-ink-soft">
          <span className="inline-flex items-center gap-1"><span className="h-2 w-2 rounded-full bg-blue-500" /> 生成中</span>
          <span className="inline-flex items-center gap-1"><span className="h-2 w-2 rounded-full bg-amber-500" /> 待审核</span>
          <span className="inline-flex items-center gap-1"><span className="h-2 w-2 rounded-full bg-green-500" /> 已通过</span>
          <span className="inline-flex items-center gap-1"><span className="h-2 w-2 rounded-full bg-stone-300" /> 等待中</span>
        </div>
      </div>
      <div className="flex items-center gap-1 overflow-x-auto pb-1">
        {stages.map((item, index) => {
          const display = getStageStateDisplay(item.status)
          return (
            <React.Fragment key={item.id}>
              {index > 0 && (
                <span className="shrink-0 text-stone-300 text-sm font-bold px-1">→</span>
              )}
              <div
                className={clsx(
                  'relative shrink-0 rounded-xl border-2 px-4 py-3 text-center transition-all min-w-[110px]',
                  display.colorClass,
                  display.active && 'shadow-md scale-105 ring-2 ring-offset-1',
                  display.active && item.status === 'running' && 'animate-pulse',
                )}
                title={item.goal}
              >
                <div className="flex items-center justify-center gap-1.5">
                  <span className="text-lg">
                    {display.icon === 'check' && <FiCheck />}
                    {display.icon === 'shield' && <FiShield />}
                    {display.icon === 'refresh' && <FiRefreshCw className="animate-spin" />}
                    {display.icon === 'x' && <FiX />}
                    {display.icon === 'cpu' && <FiCpu />}
                  </span>
                </div>
                <div className="mt-1.5 text-xs font-black truncate" title={item.displayName}>
                  {stageActionLabel(item.stage)}
                </div>
                <div className="mt-0.5 text-[10px] font-semibold opacity-70">
                  {display.label}
                </div>
              </div>
            </React.Fragment>
          )
        })}
      </div>
    </section>
  )
}
```

需要在文件顶部 import `getStageStateDisplay`：

```tsx
import {
  // ... existing imports ...
  getStageStateDisplay,
} from './directorStudioLogic'
```

- [ ] **Step 3: 将 ReviewPage 中的 `<ReviewStageRelay stages={stages} />` 改为 `<StateMachineBar stages={stages} />`**

在 `ReviewPage` 组件 (line 534)：

```tsx
<StateMachineBar stages={stages} />
```

- [ ] **Step 4: 验证编译**

```bash
cd frontend && npm run build
```

期望：编译通过。

---

### Task 3: NowGeneratingBanner 状态横幅

**Files:**
- Modify: `frontend/src/pages/DirectorStudioPage.tsx` — 新增 `NowGeneratingBanner` 组件
- Modify: `frontend/src/pages/DirectorStudioPage.tsx` — `ReviewPage` 中引入

**Interfaces:**
- Consumes: `DirectorStage[]`
- Produces: `<NowGeneratingBanner stages={stages} />`

- [ ] **Step 1: 新增 NowGeneratingBanner 组件**

在 `DirectorStudioPage.tsx` 中，`StateMachineBar` 函数之后添加：

```tsx
function NowGeneratingBanner({ stages }: { stages: DirectorStage[] }) {
  const runningStage = stages.find((stage) => stage.status === 'running' || stage.status === 'active')
  const reviewStage = stages.find((stage) => stage.status === 'review')
  const allDone = stages.length > 0 && stages.every((stage) => stage.status === 'done')

  if (allDone) {
    return (
      <div className="rounded-xl border border-green-200 bg-green-50 px-5 py-4 transition-all">
        <div className="flex items-center gap-3">
          <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-green-500 text-white">
            <FiCheck />
          </span>
          <div>
            <p className="text-sm font-black text-green-800">全部阶段已完成</p>
            <p className="text-xs text-green-600 mt-0.5">所有审核已通过，可在产物页查看和导出最终视频。</p>
          </div>
        </div>
      </div>
    )
  }

  if (runningStage) {
    return (
      <div className="rounded-xl border border-blue-200 bg-blue-50 px-5 py-4 transition-all">
        <div className="flex items-center gap-3">
          <span className="relative grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-blue-500 text-white">
            <span className="absolute inset-0 rounded-lg bg-blue-400 animate-ping opacity-30" />
            <FiRefreshCw className="animate-spin relative z-10" />
          </span>
          <div>
            <p className="text-sm font-black text-blue-800">
              系统正在生成【{runningStage.displayName}】的{stageActionLabel(runningStage.stage)}
            </p>
            <p className="text-xs text-blue-600 mt-0.5">生成完成后将自动进入审核阶段，请稍候…</p>
          </div>
        </div>
      </div>
    )
  }

  if (reviewStage) {
    return (
      <div className="rounded-xl border border-amber-200 bg-amber-50 px-5 py-4 transition-all">
        <div className="flex items-center gap-3">
          <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-amber-500 text-white">
            <FiShield />
          </span>
          <div>
            <p className="text-sm font-black text-amber-800">
              【{reviewStage.displayName}】产物已输出，等待你的确认
            </p>
            <p className="text-xs text-amber-600 mt-0.5">请审核下方内容，确认后下游阶段将自动继续执行。</p>
          </div>
        </div>
      </div>
    )
  }

  // No stages active yet
  return (
    <div className="rounded-xl border border-stone-200 bg-stone-50 px-5 py-4 transition-all">
      <div className="flex items-center gap-3">
        <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-stone-400 text-white">
          <FiCpu />
        </span>
        <div>
          <p className="text-sm font-black text-stone-700">等待任务启动</p>
          <p className="text-xs text-stone-500 mt-0.5">在概览页输入主题并启动项目，审核内容将显示在这里。</p>
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Step 2: 在 ReviewPage 中引入 NowGeneratingBanner**

在 `ReviewPage` 的 `<section className="card col-span-12 overflow-hidden p-0">` 之前（line 535），插入横幅和 StateMachineBar：

```tsx
return (
  <div className="grid grid-cols-12 gap-5">
    <NowGeneratingBanner stages={stages} />
    <StateMachineBar stages={stages} />
    <section className="card col-span-12 overflow-hidden p-0">
      {/* ... 现有内容 ... */}
    </section>
  </div>
)
```

注意：`StateMachineBar` 不再在 `<section>` 内部渲染，而是作为独立行。

- [ ] **Step 3: 验证编译**

```bash
cd frontend && npm run build
```

期望：编译通过，无类型错误。

---

### Task 4: 集成验证与收尾

- [ ] **Step 1: 完整构建验证**

```bash
cd frontend && npm run build
```

- [ ] **Step 2: 检查 React.Fragment 导入**

确认 `DirectorStudioPage.tsx` 顶部已 import `React` 或至少 `Fragment`。当前代码中 `React` 未在 import 中出现（只从 'react' 导入了 `useCallback, useEffect, useMemo, useState`）。需要在 `StateMachineBar` 中使用 `<React.Fragment>` 或改为 `<>...</>` 简写。

更简单的做法：直接使用 `<>` fragment 简写，无需额外 import：

```tsx
{index > 0 && <span className="...">→</span>}
```
使用 `<React.Fragment key={item.id}>` 需要 key，所以要么导入 `Fragment`，要么用 `<React.Fragment>`。

实际上在文件中已经使用了 JSX，`React` 在 JSX 转换中不需要显式导入（React 17+ 的 automatic JSX runtime）。但 `React.Fragment` 需要 `React` 变量。最简单的做法是用 `<>...</>` 并在外层包裹一个带 key 的 div。或者直接 import { Fragment } from 'react'。

检查当前 import：
```tsx
import { useCallback, useEffect, useMemo, useState } from 'react'
```

添加 `Fragment`：
```tsx
import { Fragment, useCallback, useEffect, useMemo, useState } from 'react'
```

然后在 `StateMachineBar` 中使用 `<Fragment key={item.id}>`。

- [ ] **Step 3: 提交**

```bash
git add frontend/src/pages/DirectorStudioPage.tsx frontend/src/pages/directorStudioLogic.ts
git commit -m "feat: enhance review page with state feedback - loading buttons, state machine bar, generating banner"
```
