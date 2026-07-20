import type { CreatorStep, CreatorStepId } from '../types'
import {
  CREATOR_WORKSPACE_STEP_IDS,
  creatorStepLabel,
  isCreatorStepReadable,
} from '../logic'

interface CreationStripProps {
  steps: readonly CreatorStep[]
  currentStepId: CreatorStepId
  onSelect: (stepId: CreatorStepId) => void
}

const statusCopy: Record<CreatorStep['state'], { icon: string; text: string }> = {
  not_started: { icon: '○', text: '未开始' },
  generating: { icon: '◌', text: '生成中' },
  needs_review: { icon: '●', text: '待确认' },
  confirmed: { icon: '✓', text: '已确认' },
  needs_attention: { icon: '!', text: '需要处理' },
  failed: { icon: '×', text: '生成失败' },
}

export default function CreationStrip({ steps, currentStepId, onSelect }: CreationStripProps) {
  const stepById = new Map(steps.map(step => [step.id, step]))
  return (
    <nav className="creation-strip" aria-label="创作步骤">
      {CREATOR_WORKSPACE_STEP_IDS.map((stepId, index) => {
        const step = stepById.get(stepId) ?? {
          id: stepId,
          label: creatorStepLabel(stepId),
          state: 'not_started' as const,
          allowedActions: [],
        }
        const status = statusCopy[step.state]
        const isCurrent = currentStepId === stepId
        const canSelect = isCreatorStepReadable(step) || isCurrent
        return (
          <button
            key={stepId}
            type="button"
            className={`creation-strip-step is-${step.state}${isCurrent ? ' is-current' : ''}`}
            aria-current={isCurrent ? 'step' : undefined}
            aria-label={`第 ${index + 1} 步，${creatorStepLabel(stepId)}，${status.text}`}
            disabled={!canSelect}
            onClick={() => onSelect(stepId)}
          >
            <span className="creation-strip-number">{index + 1}</span>
            <span className="creation-strip-name">{creatorStepLabel(stepId)}</span>
            <span className="creation-strip-status" aria-hidden="true">{status.icon} {status.text}</span>
          </button>
        )
      })}
    </nav>
  )
}
