import type { ProjectContextQuestion } from '../services/api'

export type ProjectContextQuestionnaireStatus = 'draft' | 'submitted' | 'confirmed'

export interface ProjectContextAnswer {
  selected: string[]
  text: string
  extraText: string
  source: 'user' | 'system' | 'default' | 'imported'
  needsReview: boolean
  answeredAt?: string
}

export interface ProjectContextQuestionWithAnswer extends ProjectContextQuestion {
  answer: ProjectContextAnswer
}

export interface ProjectContextQuestionnaire {
  schemaVersion: 'biaoshu.project_context_answers.v1'
  runId?: string
  projectId?: string
  projectName?: string
  sourceFile?: string
  analysisReportPath: string
  questionnairePath: string
  contextReportPath: string
  status: ProjectContextQuestionnaireStatus
  createdAt: string
  updatedAt: string
  submittedAt?: string | null
  questions: ProjectContextQuestionWithAnswer[]
  audit: Array<{
    type: string
    questionId?: string
    at: string
  }>
}

export function createQuestionnaire(params: {
  questions: ProjectContextQuestion[]
  analysisReportPath: string
  questionnairePath: string
  contextReportPath: string
  runId?: string
  projectName?: string
  sourceFile?: string
}): ProjectContextQuestionnaire {
  const now = new Date().toISOString()
  return {
    schemaVersion: 'biaoshu.project_context_answers.v1',
    runId: params.runId,
    projectName: params.projectName,
    sourceFile: params.sourceFile,
    analysisReportPath: params.analysisReportPath,
    questionnairePath: params.questionnairePath,
    contextReportPath: params.contextReportPath,
    status: 'draft',
    createdAt: now,
    updatedAt: now,
    submittedAt: null,
    questions: params.questions.map((question) => ({
      ...question,
      answer: {
        selected: [],
        text: '',
        extraText: '',
        source: 'user',
        needsReview: false,
      },
    })),
    audit: [{ type: 'created', at: now }],
  }
}

export function isQuestionAnswered(question: ProjectContextQuestionWithAnswer): boolean {
  const isChoiceType =
    question.inputType === 'multi_choice' ||
    question.inputType === 'single_choice' ||
    question.inputType === 'boolean'

  if (isChoiceType) {
    return question.answer.selected.length > 0 || question.answer.extraText.trim().length > 0
  }
  return question.answer.text.trim().length > 0
}

export function validateQuestionnaire(questionnaire: ProjectContextQuestionnaire): {
  valid: boolean
  firstInvalidIndex: number
  message: string
} {
  const index = questionnaire.questions.findIndex(
    (question) => question.required && !isQuestionAnswered(question),
  )
  if (index >= 0) {
    return {
      valid: false,
      firstInvalidIndex: index,
      message: `请先回答必填问题：${questionnaire.questions[index].title}`,
    }
  }

  const otherIndex = questionnaire.questions.findIndex(
    (question) =>
      question.answer.selected.includes('other') && !question.answer.extraText.trim(),
  )
  if (otherIndex >= 0) {
    return {
      valid: false,
      firstInvalidIndex: otherIndex,
      message: `问题"${questionnaire.questions[otherIndex].title}"选择了"其他"，请填写补充说明。`,
    }
  }

  return { valid: true, firstInvalidIndex: -1, message: '' }
}
