export const CREATOR_STEP_IDS = ['requirements', 'direction', 'script', 'shots', 'preview', 'delivery'] as const

export type DeveloperDiagnosticsView =
  | 'summary'
  | 'timeline'
  | 'tools'
  | 'artifacts'
  | 'gates'
  | 'recovery'

type CreatorPageRoute =
  | { kind: 'creator'; page: 'create'; shouldReplace?: true }
  | { kind: 'creator'; page: 'videos'; shouldReplace?: true }
  | { kind: 'creator'; page: 'settings'; shouldReplace?: true }
  | { kind: 'creator'; page: 'step'; projectId: string; stepId: typeof CREATOR_STEP_IDS[number]; shouldReplace?: true }

export type AppRoute = CreatorPageRoute | { kind: 'developer'; view: DeveloperDiagnosticsView }

const developerViews: readonly DeveloperDiagnosticsView[] = ['summary', 'timeline', 'tools', 'artifacts', 'gates', 'recovery']

export function parseAppRoute(hash: string, developerConsoleEnabled: boolean): AppRoute {
  if (!hash) return { kind: 'creator', page: 'create' }
  if (hash === '#') return creatorFallback()
  if (!hash.startsWith('#/')) return creatorFallback()

  const segments = hash.slice(2).split('/')
  const decoded = segments.map(safelyDecode)
  if (decoded.some((segment) => segment === undefined)) return creatorFallback()

  if (decoded.length === 1 && decoded[0] === 'create') return { kind: 'creator', page: 'create' }
  if (decoded.length === 1 && decoded[0] === 'videos') return { kind: 'creator', page: 'videos' }
  if (decoded.length === 1 && decoded[0] === 'settings') return { kind: 'creator', page: 'settings' }
  if (decoded.length === 4 && decoded[0] === 'videos' && decoded[2] === 'steps') {
    const projectId = decoded[1]
    const stepId = decoded[3]
    if (projectId && isCreatorStepId(stepId)) return { kind: 'creator', page: 'step', projectId, stepId }
  }
  if (decoded[0] === 'developer') {
    if (!developerConsoleEnabled) return creatorFallback()
    if (decoded.length === 1) return { kind: 'developer', view: 'summary' }
    if (decoded.length === 2 && isDeveloperView(decoded[1])) return { kind: 'developer', view: decoded[1] }
  }
  return creatorFallback()
}

export function replaceHashRoute(route: string): string {
  return route.startsWith('#') ? route : `#${route}`
}

function safelyDecode(value: string): string | undefined {
  try {
    const decoded = decodeURIComponent(value)
    return decoded.length > 0 ? decoded : undefined
  } catch {
    return undefined
  }
}

function isCreatorStepId(value: string | undefined): value is typeof CREATOR_STEP_IDS[number] {
  return Boolean(value && CREATOR_STEP_IDS.includes(value as typeof CREATOR_STEP_IDS[number]))
}

function isDeveloperView(value: string | undefined): value is DeveloperDiagnosticsView {
  return Boolean(value && developerViews.includes(value as DeveloperDiagnosticsView))
}

function creatorFallback(): AppRoute {
  return { kind: 'creator', page: 'create', shouldReplace: true }
}
