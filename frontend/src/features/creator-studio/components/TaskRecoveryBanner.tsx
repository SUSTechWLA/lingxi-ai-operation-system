import type { CreatorTask } from '../types'

export default function TaskRecoveryBanner({ tasks }: { tasks: readonly CreatorTask[] }) {
  if (tasks.length === 0) return null
  return (
    <aside className="creator-recovery-banner" aria-live="polite">
      <span aria-hidden="true">◌</span>
      <div>
        <strong>生成仍在后台继续</strong>
        <p>{tasks.map(task => task.label).join('、')}</p>
      </div>
    </aside>
  )
}
