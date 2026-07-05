import type {
  ProjectContextQuestionnaire,
  ProjectContextQuestionWithAnswer,
} from './biaoshuProjectContextQuestionnaire'

interface BiaoshuProjectContextDialogProps {
  open: boolean
  questionnaire: ProjectContextQuestionnaire
  currentIndex: number
  saving: boolean
  savedRecently: boolean
  submitting: boolean
  error: string | null
  onIndexChange: (index: number) => void
  onAnswerChange: (questionId: string, answer: ProjectContextQuestionWithAnswer['answer']) => void
  onSaveDraft: () => void
  onSubmit: () => void
  onClose: () => void
}

export function BiaoshuProjectContextDialog(props: BiaoshuProjectContextDialogProps) {
  if (!props.open) return null

  const question = props.questionnaire.questions[props.currentIndex]
  if (!question) return null

  const total = props.questionnaire.questions.length
  const answeredCount = props.questionnaire.questions.filter(
    (q) => q.answer.selected.length > 0 || q.answer.text.trim() || q.answer.extraText.trim(),
  ).length

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={props.onClose}
    >
      <div
        className="w-full max-w-3xl max-h-[90vh] rounded-xl bg-white shadow-2xl flex flex-col"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="border-b border-line px-6 py-4 shrink-0">
          <div className="flex items-center justify-between">
            <p className="text-xs font-bold text-primary">
              问题 {props.currentIndex + 1} / {total}（已回答 {answeredCount} 题）
            </p>
            {question.required && (
              <span className="text-xs font-bold text-red-500 bg-red-50 px-2 py-0.5 rounded">必填</span>
            )}
          </div>
          <h3 className="mt-1 text-lg font-black text-ink">{question.title}</h3>
          <p className="mt-2 text-sm text-ink-muted">{question.prompt}</p>
          {question.helpText && (
            <p className="mt-1 text-xs text-ink-soft">{question.helpText}</p>
          )}
        </div>

        {/* Error banner */}
        {props.error && (
          <div className="mx-6 mt-3 rounded-lg border border-red-200 bg-red-50 px-4 py-2 text-sm text-red-700 flex items-start gap-2 shrink-0">
            <svg className="w-4 h-4 mt-0.5 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
            <span>{props.error}</span>
          </div>
        )}

        {/* Content */}
        <div className="flex-1 overflow-y-auto px-6 py-5">
          <QuestionInput
            question={question}
            onChange={(answer) => props.onAnswerChange(question.id, answer)}
          />
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between border-t border-line px-6 py-4 shrink-0">
          <div className="flex gap-2">
            <button
              onClick={props.onClose}
              className="rounded-lg bg-white px-4 py-2 text-sm font-bold text-ink-muted ring-1 ring-line hover:bg-background-mist transition-colors"
            >
              关闭
            </button>
            <button
              onClick={props.onSaveDraft}
              disabled={props.saving || props.savedRecently}
              className={`rounded-lg px-4 py-2 text-sm font-bold ring-1 transition-colors ${
                props.savedRecently
                  ? 'bg-green-50 text-green-700 ring-green-300'
                  : 'bg-white text-blue-700 ring-blue-200 hover:bg-blue-50 disabled:opacity-50 disabled:cursor-not-allowed'
              }`}
            >
              {props.saving ? '保存中...' : props.savedRecently ? '已保存 ✓' : '保存草稿'}
            </button>
          </div>
          <div className="flex gap-2">
            <button
              disabled={props.currentIndex === 0}
              onClick={() => props.onIndexChange(props.currentIndex - 1)}
              className="rounded-lg bg-background-mist px-4 py-2 text-sm font-bold text-ink-muted hover:bg-line disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
            >
              上一题
            </button>
            {props.currentIndex < total - 1 ? (
              <button
                onClick={() => props.onIndexChange(props.currentIndex + 1)}
                className="rounded-lg bg-primary px-4 py-2 text-sm font-bold text-white hover:bg-primary-dark transition-colors"
              >
                下一题
              </button>
            ) : (
              <button
                onClick={props.onSubmit}
                disabled={props.submitting}
                className={`rounded-lg px-4 py-2 text-sm font-bold text-white transition-colors ${
                  props.submitting
                    ? 'bg-green-400 cursor-not-allowed'
                    : 'bg-green-600 hover:bg-green-700'
                }`}
              >
                {props.submitting ? '生成中...' : '生成确认表'}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

// ── Question Input Dispatcher ──

function QuestionInput({
  question,
  onChange,
}: {
  question: ProjectContextQuestionWithAnswer
  onChange: (answer: ProjectContextQuestionWithAnswer['answer']) => void
}) {
  if (question.inputType === 'multi_choice') {
    return <MultiChoiceQuestion question={question} onChange={onChange} />
  }
  if (question.inputType === 'single_choice') {
    return <SingleChoiceQuestion question={question} onChange={onChange} />
  }
  if (question.inputType === 'boolean') {
    return <BooleanQuestion question={question} onChange={onChange} />
  }
  return <TextQuestion question={question} onChange={onChange} />
}

// ── Multi Choice ──

function MultiChoiceQuestion({
  question,
  onChange,
}: {
  question: ProjectContextQuestionWithAnswer
  onChange: (answer: ProjectContextQuestionWithAnswer['answer']) => void
}) {
  const selected = new Set(question.answer.selected)
  const toggle = (value: string) => {
    const next = new Set(selected)
    if (next.has(value)) {
      next.delete(value)
    } else {
      next.add(value)
    }
    onChange({
      ...question.answer,
      selected: Array.from(next),
      needsReview: next.has('not_specified'),
      answeredAt: new Date().toISOString(),
    })
  }

  return (
    <div className="space-y-3">
      {(question.options || []).map((option) => (
        <label
          key={option.value}
          className="flex items-start gap-3 rounded-lg border border-line bg-white p-3 hover:bg-blue-50 cursor-pointer transition-colors"
        >
          <input
            type="checkbox"
            checked={selected.has(option.value)}
            onChange={() => toggle(option.value)}
            className="mt-1 h-4 w-4 accent-primary"
          />
          <span>
            <span className="block text-sm font-bold text-ink">{option.label}</span>
            {option.description && (
              <span className="block text-xs text-ink-muted">{option.description}</span>
            )}
          </span>
        </label>
      ))}
      <textarea
        value={question.answer.extraText}
        onChange={(e) =>
          onChange({
            ...question.answer,
            extraText: e.target.value,
            answeredAt: new Date().toISOString(),
          })
        }
        placeholder="补充说明（可选）"
        className="min-h-[80px] w-full rounded-lg border border-line p-3 text-sm focus:ring-2 focus:ring-blue-200 focus:border-blue-400 outline-none"
      />
    </div>
  )
}

// ── Single Choice ──

function SingleChoiceQuestion({
  question,
  onChange,
}: {
  question: ProjectContextQuestionWithAnswer
  onChange: (answer: ProjectContextQuestionWithAnswer['answer']) => void
}) {
  const currentValue = question.answer.selected[0] || ''

  return (
    <div className="space-y-3">
      {(question.options || []).map((option) => (
        <label
          key={option.value}
          className="flex items-start gap-3 rounded-lg border border-line bg-white p-3 hover:bg-blue-50 cursor-pointer transition-colors"
        >
          <input
            type="radio"
            name={`single_${question.id}`}
            checked={currentValue === option.value}
            onChange={() =>
              onChange({
                ...question.answer,
                selected: [option.value],
                needsReview: option.value === 'not_specified',
                answeredAt: new Date().toISOString(),
              })
            }
            className="mt-1 h-4 w-4 accent-primary"
          />
          <span>
            <span className="block text-sm font-bold text-ink">{option.label}</span>
            {option.description && (
              <span className="block text-xs text-ink-muted">{option.description}</span>
            )}
          </span>
        </label>
      ))}
      <textarea
        value={question.answer.extraText}
        onChange={(e) =>
          onChange({
            ...question.answer,
            extraText: e.target.value,
            answeredAt: new Date().toISOString(),
          })
        }
        placeholder="补充说明（可选）"
        className="min-h-[80px] w-full rounded-lg border border-line p-3 text-sm focus:ring-2 focus:ring-blue-200 focus:border-blue-400 outline-none"
      />
    </div>
  )
}

// ── Boolean (Yes/No) ──

function BooleanQuestion({
  question,
  onChange,
}: {
  question: ProjectContextQuestionWithAnswer
  onChange: (answer: ProjectContextQuestionWithAnswer['answer']) => void
}) {
  const currentValue = question.answer.selected[0] || ''

  return (
    <div className="space-y-3">
      <div className="flex gap-4">
        {[
          { value: 'yes', label: '是', description: '确认' },
          { value: 'no', label: '否', description: '不涉及' },
          { value: 'not_specified', label: '未明确', description: '后续确认' },
        ].map((opt) => (
          <label
            key={opt.value}
            className={`flex items-center gap-2 rounded-lg border px-5 py-3 cursor-pointer transition-colors ${
              currentValue === opt.value
                ? 'border-primary bg-primary-soft text-primary-dark font-bold'
                : 'border-line bg-white hover:bg-blue-50'
            }`}
          >
            <input
              type="radio"
              name={`bool_${question.id}`}
              checked={currentValue === opt.value}
              onChange={() =>
                onChange({
                  ...question.answer,
                  selected: [opt.value],
                  needsReview: opt.value === 'not_specified',
                  answeredAt: new Date().toISOString(),
                })
              }
              className="h-4 w-4 accent-primary"
            />
            <div>
              <span className="text-sm font-bold">{opt.label}</span>
              <span className="ml-2 text-xs text-ink-muted">{opt.description}</span>
            </div>
          </label>
        ))}
      </div>
      <textarea
        value={question.answer.extraText}
        onChange={(e) =>
          onChange({
            ...question.answer,
            extraText: e.target.value,
            answeredAt: new Date().toISOString(),
          })
        }
        placeholder="补充说明（可选）"
        className="min-h-[80px] w-full rounded-lg border border-line p-3 text-sm focus:ring-2 focus:ring-blue-200 focus:border-blue-400 outline-none"
      />
    </div>
  )
}

// ── Text / Textarea ──

function TextQuestion({
  question,
  onChange,
}: {
  question: ProjectContextQuestionWithAnswer
  onChange: (answer: ProjectContextQuestionWithAnswer['answer']) => void
}) {
  if (question.inputType === 'text') {
    return (
      <input
        type="text"
        value={question.answer.text}
        onChange={(e) =>
          onChange({
            ...question.answer,
            text: e.target.value,
            answeredAt: new Date().toISOString(),
          })
        }
        placeholder="请输入回答"
        className="w-full rounded-lg border border-line p-3 text-sm focus:ring-2 focus:ring-blue-200 focus:border-blue-400 outline-none"
      />
    )
  }

  return (
    <textarea
      value={question.answer.text}
      onChange={(e) =>
        onChange({
          ...question.answer,
          text: e.target.value,
          answeredAt: new Date().toISOString(),
        })
      }
      placeholder="请输入回答"
      className="min-h-[180px] w-full rounded-lg border border-line p-3 text-sm focus:ring-2 focus:ring-blue-200 focus:border-blue-400 outline-none"
    />
  )
}
